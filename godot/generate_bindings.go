// Package godot holds the Godot bindings generated from one of the
// versioned extension_api_*.json files by cmd/godot-go-bindgen. The
// //go:generate directive below stamps the bindings into this same
// directory using the package name and dir from GOFILE / GOPACKAGE.
//
// Three JSON files are committed (extension_api_4_4.json,
// extension_api_4_5.json, extension_api_4_6.json) so the framework's
// own CI can run the matrix against multiple Godot versions. The
// default trigger picks 4.6; switch by re-running the bindgen with
// `-api ./godot/extension_api_4_5.json` (or _4_4) for older targets.
//
// The framework's runtime warns on major/minor drift between bindings
// and the live host, so a 4.5-built bindings package loaded into a
// 4.6 Godot will surface the mismatch in the engine's Output dock
// rather than misbehave silently.
//
// Everything else in this directory is generated and gitignored
// (godot/**/*.gen.go); only this file, the JSONs, the C header, and
// the README are tracked.
package godot

//go:generate go run github.com/legendary-code/godot-go/cmd/godot-go-bindgen -api ./extension_api_4_6.json
