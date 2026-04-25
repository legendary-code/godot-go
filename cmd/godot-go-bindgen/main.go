// godot-go-bindgen reads godot/extension_api.json and emits the framework's
// generated Go bindings into variant/, core/, editor/, etc. It is run by
// godot-go maintainers when the json bumps; it is NOT the //go:generate target
// that user extensions invoke (that one lives at cmd/godot-go).
//
// Phase 2c scope: every builtin class in extension_api.json#builtin_classes
// (minus the Go-native primitives Nil/bool/int/float) plus a sibling
// godot/aliases.gen.go re-exporting the high-traffic types.
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

func main() {
	apiPath := flag.String("api", "godot/extension_api.json", "Path to extension_api.json")
	outDir := flag.String("out", ".", "Module root; generated files are written under <out>/variant, <out>/godot, etc.")
	buildConfig := flag.String("build-config", "float_64", "Target build configuration (float_32, float_64, double_32, double_64)")
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
	godotDir := filepath.Join(*outDir, "godot")
	if err := os.MkdirAll(godotDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "godot-go-bindgen: %v\n", err)
		os.Exit(1)
	}

	emitted := make([]string, 0, len(api.BuiltinClasses))
	for i := range api.BuiltinClasses {
		bc := &api.BuiltinClasses[i]
		if goNativePrimitives[bc.Name] {
			continue
		}
		if err := emitBuiltinClass(api, *buildConfig, bc, variantDir); err != nil {
			fmt.Fprintf(os.Stderr, "godot-go-bindgen: emit %s: %v\n", bc.Name, err)
			os.Exit(1)
		}
		emitted = append(emitted, bc.Name)
	}

	if err := emitAliases(emitted, godotDir); err != nil {
		fmt.Fprintf(os.Stderr, "godot-go-bindgen: emit aliases: %v\n", err)
		os.Exit(1)
	}

	// Engine-class sweep. Register all class names first so cross-class
	// type references resolve (a method on Node returning Node3D won't
	// otherwise know that Node3D is a known class).
	registerEngineClasses(api)
	classCount := 0
	for i := range api.Classes {
		c := &api.Classes[i]
		if err := emitEngineClass(api, c, *outDir); err != nil {
			fmt.Fprintf(os.Stderr, "godot-go-bindgen: emit class %s: %v\n", c.Name, err)
			os.Exit(1)
		}
		classCount++
	}

	fmt.Fprintf(os.Stderr, "godot-go-bindgen: generated %d builtin classes + %d engine classes against %s (precision=%s, build_config=%s)\n",
		len(emitted), classCount, api.Header.VersionFullName, api.Header.Precision, *buildConfig)
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
