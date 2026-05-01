// Package main is the godot-go locale_language example — the verbatim
// PLAN.md showcase of doc-tagged class authoring (LocaleLanguage with
// @abstract, an enum, a @static method, an @name-renamed instance
// method, and a virtual override).
//
// At SCENE init it announces itself and reports whether ClassDB picked up
// the generated bindings; the rest of the surface is exercised by the
// accompanying GDScript driver (godot_project/test_locale_language.gd).
package main

import (
	"github.com/legendary-code/godot-go/gdextension"
	"github.com/legendary-code/godot-go/godot/runtime"
)

func init() {
	gdextension.RegisterInitCallback(gdextension.InitLevelScene, func() {
		v := gdextension.GetGodotVersion()
		runtime.Printf("godot-go: locale_language example loaded (Godot %s — %s)", v.Short(), v.String)
	})
	gdextension.RegisterDeinitCallback(gdextension.InitLevelScene, func() {
		runtime.Print("godot-go: locale_language SCENE deinit, goodbye")
	})
}

func main() {}
