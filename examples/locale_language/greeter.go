package main

import (
	"github.com/legendary-code/godot-go/godot"
	"github.com/legendary-code/godot-go/godot/runtime"
)

// Greeter exercises cross-file enum references plus the default-init
// constructor hook. It lives in a separate file from LocaleLanguage
// but takes a Language enum value (declared alongside LocaleLanguage)
// as its arg. Without cross-file enum resolution, the codegen would
// fail to find Language in this file's scope; with it, the
// registration's ArgClassNames carries "LocaleLanguage.Language" —
// the owning class's qualifier — so the editor renders the typed-enum
// identity correctly.
//
// @class
type Greeter struct {
	// @extends
	godot.Object

	// defaultLang seeds the instance's preferred language. The
	// codegen-driven Construct hook routes through newGreeter()
	// when present, so this field starts out at LanguageEnglish for
	// every instance — whether built via NewGreeter() from Go or
	// Greeter.new() from GDScript.
	defaultLang Language
}

// newGreeter is the unexported package-level default-init hook.
// Codegen recognizes the `new<ClassName>() *<ClassName>` shape and
// calls it from the Construct callback in place of the zero-value
// `&Greeter{}` literal. Strict zero-arg signature: arg-bearing
// construction belongs on an `_init` virtual instead, since that's
// where Godot's `Class.new(args...)` forwarding actually delivers
// the args.
func newGreeter() *Greeter {
	return &Greeter{defaultLang: LanguageEnglish}
}

// Init is registered as the engine _init virtual via @override —
// Godot calls this hook as part of instance construction (whether
// triggered by Greeter.new() from GDScript or NewGreeter() from Go).
// Independent of newGreeter above; both run during construction
// (newGreeter populates the wrapper's defaults; Init fires after,
// once Godot has bound the engine pointer).
//
// @override
func (g *Greeter) Init() {
	runtime.PrintInfo("Greeter.Init: constructor virtual fired")
}

// Hello returns the greeting in the instance's defaultLang —
// exercises the newGreeter() factory's seeded state reaching a
// regular ClassDB-bound method.
func (g *Greeter) Hello() string {
	switch g.defaultLang {
	case LanguageEnglish:
		return "hello"
	case LanguageGerman:
		return "hallo"
	}
	return ""
}

// CountLetters demonstrates a map[K]V at the @class boundary. Wire
// form is Godot's untyped Dictionary in both directions; the
// codegen iterates the dictionary's keys, unwraps each k/v Variant
// pair, and rebuilds a fresh Dictionary on the way out.
//
// @static
func CountLetters(words []string) map[string]int64 {
	out := map[string]int64{}
	for _, w := range words {
		for _, r := range w {
			out[string(r)]++
		}
	}
	return out
}

// Echo round-trips a Dictionary unchanged — exercises the Variant →
// Go map → Variant path with the same shape on both sides.
//
// @static
func Echo(in map[string]int64) map[string]int64 {
	return in
}

// LangCodes demonstrates a map with a user-enum value type
// (Phase 2a) — Godot sees Dictionary; Go sees map[string]Language
// with the typed enum identity round-tripping through the int64 wire
// form. The codegen wraps each Language value as Variant::INT and
// unwraps it via a typed cast.
//
// @static
func LangCodes() map[string]Language {
	return map[string]Language{
		"en": LanguageEnglish,
		"de": LanguageGerman,
	}
}

// CharsByLang demonstrates a map with a slice value (Phase 2a) — wire
// form is Dictionary[string, PackedStringArray], each entry built
// inline by the codegen.
//
// @static
func CharsByLang() map[string][]string {
	return map[string][]string{
		"en": {"a", "b", "c"},
		"de": {"ä", "ö", "ü"},
	}
}

// GreetIn returns a localized greeting string for the given Language.
// Demonstrates cross-file enum resolution at the @class boundary —
// Language is declared alongside LocaleLanguage but used here.
//
// @static
func GreetIn(lang Language) string {
	switch lang {
	case LanguageEnglish:
		return "hello"
	case LanguageGerman:
		return "hallo"
	}
	return ""
}
