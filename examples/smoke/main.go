// Package main is the Phase 0 smoke extension.
//
// It exists only to be linked as a c-shared library so we can confirm that
// Godot can load the DLL and reach our entry point. The blank import pulls
// in the gdextension_library_init export from internal/gdextension.
package main

import _ "github.com/legendary-code/godot-go/internal/gdextension"

func main() {}
