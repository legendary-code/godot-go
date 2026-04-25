// Package main is the godot-go smoke extension.
//
// It builds as a c-shared library and exists to confirm the load path and
// the Phase 1 interface table. At SCENE init it greets Godot and reports
// the host version it's running against; at SCENE deinit it says goodbye.
package main

import (
	"github.com/legendary-code/godot-go/internal/gdextension"
	"github.com/legendary-code/godot-go/internal/runtime"
)

func init() {
	gdextension.RegisterInitCallback(gdextension.InitLevelScene, func() {
		v := gdextension.GetGodotVersion()
		runtime.Printf("godot-go: hello godot-go (Godot %s — %s)", v.Short(), v.String)
	})
	gdextension.RegisterDeinitCallback(gdextension.InitLevelScene, func() {
		runtime.Print("godot-go: SCENE deinit, goodbye")
	})
}

func main() {}
