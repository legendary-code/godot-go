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
// to write, what to call the package, what the framework's import path is.
type genConfig struct {
	Dir        string // absolute path to the bindings directory
	Package    string // package name used in the generated files
	ModulePath string // import path of the framework (e.g., github.com/legendary-code/godot-go)
}

func main() {
	apiPath := flag.String("api", "extension_api.json", "Path to extension_api.json")
	outFlag := flag.String("out", "", "Output directory (default: directory of $GOFILE when run via go generate)")
	pkgFlag := flag.String("package", "", "Target package name (default: $GOPACKAGE when run via go generate)")
	buildConfig := flag.String("build-config", "float_64", "Target build configuration (float_32, float_64, double_32, double_64)")
	flag.Parse()

	cfg, err := resolveConfig(*outFlag, *pkgFlag)
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

	if err := os.MkdirAll(cfg.Dir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "godot-go-bindgen: %v\n", err)
		os.Exit(1)
	}

	emitted := make([]string, 0, len(api.BuiltinClasses))
	for i := range api.BuiltinClasses {
		bc := &api.BuiltinClasses[i]
		if goNativePrimitives[bc.Name] {
			continue
		}
		if err := emitBuiltinClass(api, *buildConfig, bc, cfg); err != nil {
			fmt.Fprintf(os.Stderr, "godot-go-bindgen: emit %s: %v\n", bc.Name, err)
			os.Exit(1)
		}
		emitted = append(emitted, bc.Name)
	}

	// Engine-class sweep. Register class names first so cross-class
	// references resolve (a method on Node returning Node3D needs
	// Node3D to be a known class).
	registerEngineClasses(api)
	classCount := 0
	for i := range api.Classes {
		c := &api.Classes[i]
		if err := emitEngineClass(api, c, cfg); err != nil {
			fmt.Fprintf(os.Stderr, "godot-go-bindgen: emit class %s: %v\n", c.Name, err)
			os.Exit(1)
		}
		classCount++
	}

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

	fmt.Fprintf(os.Stderr, "godot-go-bindgen: generated %d builtin classes + %d engine classes into %s (package %q) against %s (precision=%s, build_config=%s)\n",
		len(emitted), classCount, cfg.Dir, cfg.Package, api.Header.VersionFullName, api.Header.Precision, *buildConfig)
}

// resolveConfig figures out where to put generated files and what to call
// the package, preferring explicit flags but falling back to the
// GOFILE / GOPACKAGE env vars `go generate` populates.
func resolveConfig(outFlag, pkgFlag string) (*genConfig, error) {
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
	return &genConfig{
		Dir:        dir,
		Package:    pkg,
		ModulePath: "github.com/legendary-code/godot-go",
	}, nil
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
