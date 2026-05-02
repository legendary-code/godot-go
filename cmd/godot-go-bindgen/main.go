// godot-go-bindgen reads extension_api.json and emits the Go bindings
// for builtin classes, engine classes, global enums, utility functions,
// and native structures into a single user-chosen package directory.
// Plus a sibling `runtime/` sub-package with binding-aware convenience
// helpers (IsEditorHint, RunOnMain re-exports, ...).
//
// Typical invocation is via go generate from the user's project:
//
//	//go:generate go run github.com/legendary-code/godot-go/cmd/godot-go-bindgen -api ./extension_api.json
//	package godot
//
// `go generate` sets GOFILE / GOPACKAGE env vars; the bindgen reads them
// to learn the output directory and target package name. Direct
// invocation (without go generate) needs `-out` and `-package` flags.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// goNativePrimitives are the four Godot "builtin classes" that map directly
// to Go primitives — we don't generate wrappers for these.
var goNativePrimitives = map[string]bool{
	"Nil":   true,
	"bool":  true,
	"int":   true,
	"float": true,
}

// genConfig captures everything the per-target emit functions need: where
// to write, what to call the package, what the framework's import path is,
// and what the user's bindings package full import path is (so the runtime
// sub-package can import its parent).
type genConfig struct {
	Dir           string // absolute path to the bindings directory
	Package       string // package name used in the generated files
	ModulePath    string // import path of the framework (e.g., github.com/legendary-code/godot-go)
	PkgImportPath string // full import path of the bindings dir (e.g., example.com/foo/godot); empty if undetectable
}

func main() {
	apiPath := flag.String("api", "extension_api.json", "Path to extension_api.json")
	outFlag := flag.String("out", "", "Output directory (default: directory of $GOFILE when run via go generate)")
	pkgFlag := flag.String("package", "", "Target package name (default: $GOPACKAGE when run via go generate)")
	importPathFlag := flag.String("import-path", "", "Full import path of the bindings dir (default: auto-detected from go.mod). Required for the runtime sub-package import.")
	buildConfig := flag.String("build-config", "float_64", "Target build configuration (float_32, float_64, double_32, double_64)")
	flag.Parse()

	cfg, err := resolveConfig(*outFlag, *pkgFlag, *importPathFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "godot-go-bindgen: %v\n", err)
		os.Exit(1)
	}

	api, err := loadAPI(*apiPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "godot-go-bindgen: %v\n", err)
		os.Exit(1)
	}

	if api.Header.Precision != precisionForBuildConfig(*buildConfig) {
		fmt.Fprintf(os.Stderr,
			"godot-go-bindgen: build-config %q expects %q precision but extension_api.json reports %q\n",
			*buildConfig, precisionForBuildConfig(*buildConfig), api.Header.Precision)
		os.Exit(1)
	}

	// Tell the type table which Go float type maps to Godot's "float"
	// (real_t) under the chosen build config — float32 for single-precision
	// builds, float64 for double-precision. Must run before any emit* call
	// since the type resolver reads it eagerly.
	setBuildPrecision(*buildConfig)

	if err := os.MkdirAll(cfg.Dir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "godot-go-bindgen: %v\n", err)
		os.Exit(1)
	}

	emitted, err := emitBuiltinClasses(api, *buildConfig, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "godot-go-bindgen: emit builtins: %v\n", err)
		os.Exit(1)
	}

	// Engine-class sweep. Register class names inside emitEngineClasses
	// so cross-class references resolve (a method on Node returning
	// Node3D needs Node3D to be a known class).
	if err := emitEngineClasses(api, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "godot-go-bindgen: emit engine classes: %v\n", err)
		os.Exit(1)
	}
	classCount := len(api.Classes)

	if err := emitUtilityFunctions(api, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "godot-go-bindgen: emit utility: %v\n", err)
		os.Exit(1)
	}

	if err := emitGlobalEnums(api, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "godot-go-bindgen: emit enums: %v\n", err)
		os.Exit(1)
	}

	if err := emitNativeStructures(api, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "godot-go-bindgen: emit native: %v\n", err)
		os.Exit(1)
	}

	if err := emitVersion(api, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "godot-go-bindgen: emit version: %v\n", err)
		os.Exit(1)
	}

	if err := emitRuntime(api, *buildConfig, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "godot-go-bindgen: emit runtime: %v\n", err)
		os.Exit(1)
	}

	if err := emitArrayRuntime(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "godot-go-bindgen: emit array runtime: %v\n", err)
		os.Exit(1)
	}

	if cfg.PkgImportPath != "" {
		if err := emitRuntimeSubpkg(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "godot-go-bindgen: emit runtime subpkg: %v\n", err)
			os.Exit(1)
		}
	} else {
		fmt.Fprintln(os.Stderr, "godot-go-bindgen: skipping runtime/ sub-package: bindings import path unknown (pass -import-path or run from inside a go.mod tree)")
	}

	fmt.Fprintf(os.Stderr, "godot-go-bindgen: generated %d builtin classes + %d engine classes into %s (package %q) against %s (precision=%s, build_config=%s)\n",
		len(emitted), classCount, cfg.Dir, cfg.Package, api.Header.VersionFullName, api.Header.Precision, *buildConfig)
}

// resolveConfig figures out where to put generated files and what to call
// the package, preferring explicit flags but falling back to the
// GOFILE / GOPACKAGE env vars `go generate` populates. The bindings'
// full import path is auto-detected from the surrounding go.mod when
// possible (used by the runtime sub-package's import of its parent);
// the -import-path flag overrides detection.
func resolveConfig(outFlag, pkgFlag, importPathFlag string) (*genConfig, error) {
	dir := outFlag
	if dir == "" {
		if gofile := os.Getenv("GOFILE"); gofile != "" {
			abs, err := filepath.Abs(gofile)
			if err != nil {
				return nil, fmt.Errorf("resolve GOFILE %q: %w", gofile, err)
			}
			dir = filepath.Dir(abs)
		}
	} else {
		abs, err := filepath.Abs(dir)
		if err != nil {
			return nil, fmt.Errorf("resolve -out %q: %w", outFlag, err)
		}
		dir = abs
	}
	pkg := pkgFlag
	if pkg == "" {
		pkg = os.Getenv("GOPACKAGE")
	}
	if dir == "" || pkg == "" {
		return nil, fmt.Errorf("no output configured: pass -out and -package, or run via `go generate` (which sets GOFILE / GOPACKAGE)")
	}

	importPath := importPathFlag
	if importPath == "" {
		// Best-effort auto-detect; non-fatal if it fails. Detection only
		// matters for the runtime sub-package emit (which is skipped when
		// no import path is known).
		if p, err := detectImportPath(dir); err == nil {
			importPath = p
		}
	}

	return &genConfig{
		Dir:           dir,
		Package:       pkg,
		ModulePath:    "github.com/legendary-code/godot-go",
		PkgImportPath: importPath,
	}, nil
}

// detectImportPath walks up from `dir` looking for a go.mod, parses its
// module line, and returns module + the relative path from the go.mod's
// directory to `dir` (forward-slash joined). Returns ("", err) if no go.mod
// is found or the module line can't be parsed.
func detectImportPath(dir string) (string, error) {
	cur := dir
	for {
		gomod := filepath.Join(cur, "go.mod")
		if data, err := os.ReadFile(gomod); err == nil {
			module, err := parseGoModModuleLine(data)
			if err != nil {
				return "", err
			}
			rel, err := filepath.Rel(cur, dir)
			if err != nil {
				return "", err
			}
			rel = filepath.ToSlash(rel)
			if rel == "." || rel == "" {
				return module, nil
			}
			return module + "/" + rel, nil
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", fmt.Errorf("no go.mod found above %s", dir)
		}
		cur = parent
	}
}

// parseGoModModuleLine extracts the module path from go.mod content.
// Handles the simple `module path` form; both quoted and unquoted paths.
func parseGoModModuleLine(data []byte) (string, error) {
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "module") {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(line, "module"))
		rest = strings.TrimSpace(strings.SplitN(rest, "//", 2)[0])
		rest = strings.Trim(rest, "\"`")
		if rest == "" {
			continue
		}
		return rest, nil
	}
	return "", fmt.Errorf("no module line in go.mod")
}

func precisionForBuildConfig(bc string) string {
	switch bc {
	case "float_32", "float_64":
		return "single"
	case "double_32", "double_64":
		return "double"
	default:
		return ""
	}
}
