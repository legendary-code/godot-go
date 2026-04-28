// Package gdextension is the cgo bridge between Godot and the Go runtime.
//
// It exposes a single C entry point, gdextension_library_init, matching
// GDExtensionInitializationFunction in gdextension_interface.h. Godot calls it
// when loading the extension; the package wires the proc-address resolver and
// initialization-level callbacks through small C trampolines in shim.c.
//
// At Phase 0 this package only logs to confirm the load path is alive. Later
// phases will hang the runtime registry, classdb integration, and method-bind
// dispatch off of this same wiring.
package gdextension
