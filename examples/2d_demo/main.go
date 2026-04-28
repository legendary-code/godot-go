// Package main is the godot-go 2d_demo extension — a small Node2D
// subclass (Mover) that oscillates back and forth along the X axis,
// driven by Process(delta), and emits a signal at each bound. It's
// the canonical starting point for users building game-like
// extensions: shows property + signal + virtual override + actual
// engine state mutation in roughly fifty lines.
package main

import (
	"github.com/legendary-code/godot-go/internal/gdextension"
	"github.com/legendary-code/godot-go/internal/runtime"
)

func init() {
	gdextension.RegisterInitCallback(gdextension.InitLevelScene, func() {
		v := gdextension.GetGodotVersion()
		runtime.Printf("godot-go: 2d_demo loaded (Godot %s)", v.Short())
	})
	gdextension.RegisterDeinitCallback(gdextension.InitLevelScene, func() {
		runtime.Print("godot-go: 2d_demo SCENE deinit, goodbye")
	})
}

func main() {}
