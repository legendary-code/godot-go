// Package godot holds the Godot bindings generated from
// extension_api.json by cmd/godot-go-bindgen. This file is the
// trigger: `go generate ./godot/...` (or, more commonly, the project
// Taskfile's bindings task) runs the bindgen against the JSON
// alongside this file and writes the *.gen.go output here, picking
// up the package name and target directory from the GOFILE /
// GOPACKAGE env vars `go generate` populates.
//
// extension_api.json is the version pin: replace it with a newer
// dump (e.g. `godot --headless --dump-extension-api`) and re-run
// generate to retarget. The framework's runtime warns on
// major/minor drift between bindings and the live host.
//
// Everything else in this directory is generated and gitignored
// (godot/**/*.gen.go); only this file, the JSON, the C header, and
// the README are tracked.
package godot

//go:generate go run github.com/legendary-code/godot-go/cmd/godot-go-bindgen -api ./extension_api.json
