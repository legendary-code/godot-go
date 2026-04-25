// godot-go-bindgen reads godot/extension_api.json and emits the framework's
// generated Go bindings into variant/, core/, editor/, etc. It is run by
// godot-go maintainers when the json bumps; it is NOT the //go:generate target
// that user extensions invoke (that one lives at cmd/godot-go).
//
// Phase 2b scope: scaffold + Vector2 only. The walker, type mapper, and
// builtin-class emitter all exist but are deliberately exercised on a single
// type to validate the codegen pipeline. Phase 2c expands to all 38 builtin
// classes.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	apiPath := flag.String("api", "godot/extension_api.json", "Path to extension_api.json")
	outDir := flag.String("out", ".", "Module root; generated files are written under <out>/variant, <out>/core, etc.")
	buildConfig := flag.String("build-config", "float_32", "Target build configuration (float_32, float_64, double_32, double_64)")
	flag.Parse()

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

	variantDir := filepath.Join(*outDir, "variant")
	if err := os.MkdirAll(variantDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "godot-go-bindgen: %v\n", err)
		os.Exit(1)
	}

	// Phase 2b: only generate Vector2 to validate the pipeline.
	for _, name := range []string{"Vector2"} {
		bc := api.FindBuiltin(name)
		if bc == nil {
			fmt.Fprintf(os.Stderr, "godot-go-bindgen: builtin class %q not found in extension_api.json\n", name)
			os.Exit(1)
		}
		if err := emitBuiltinClass(api, *buildConfig, bc, variantDir); err != nil {
			fmt.Fprintf(os.Stderr, "godot-go-bindgen: emit %s: %v\n", name, err)
			os.Exit(1)
		}
	}

	fmt.Fprintf(os.Stderr, "godot-go-bindgen: generated against %s (precision=%s, build_config=%s)\n",
		api.Header.VersionFullName, api.Header.Precision, *buildConfig)
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
