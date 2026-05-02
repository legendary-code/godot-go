package main

import (
	"github.com/legendary-code/godot-go/godot"
	"github.com/legendary-code/godot-go/godot/runtime"
)

// Greeter exercises cross-file enum references. It lives in a separate
// file from LocaleLanguage but takes a Language enum value (declared
// alongside LocaleLanguage) as its arg. Without cross-file enum
// resolution, the codegen would fail to find Language in this file's
// scope; with it, the registration's ArgClassNames carries
// "LocaleLanguage.Language" — the owning class's qualifier — so the
// editor renders the typed-enum identity correctly.
//
// @class
type Greeter struct {
	// @extends
	godot.Object
}

// Init is registered as the engine _init virtual via @override —
// Godot calls this hook as part of instance construction (whether
// triggered by Greeter.new() from GDScript or NewGreeter() from Go).
// Same explicit-opt-in shape as @override on _process / _ready.
//
// @override
func (g *Greeter) Init() {
	runtime.Print("Greeter.Init: constructor virtual fired")
}

// GreetIn returns a localized greeting string for the given Language.
// Demonstrates cross-file enum resolution at the @class boundary —
// Language is declared alongside LocaleLanguage but used here.
//
// @static
func (Greeter) GreetIn(lang Language) string {
	switch lang {
	case LanguageEnglish:
		return "hello"
	case LanguageGerman:
		return "hallo"
	}
	return ""
}
