package main

// This go:generate call will run our tool and generate binding/registration code for the current file in a separate
// file, `<sourcefile>_bindings.go`.  One file = one Godot class — exactly one struct in this file must carry the
// `@class` doctag, and that's the struct codegen registers with Godot's ClassDB. Other structs in the file are plain
// Go types and aren't registered.
//go:generate godot-go

import "github.com/legendary-code/godot-go/godot"

// LocaleLanguage implements language for a locale
//
// aside: note the doc tags below, these specify things that can't be expressed in golang, additional documentation
// to embed in the document xml, or additional information for how to bind this type.
//
// @description here overrides the
// already generated description by taking the comment above: "LocaleLanguage implements language for a locale", removing
// the type name "LocaleLanguage" and uppercasing the first word to form: "Implements language for a locale"
//
// @abstract marks this class as abstract
//
// @class
// @abstract
// @description This is my overridden description
// @deprecated
type LocaleLanguage struct {
	// @extends specifies the parent class — this class extends Node in
	// Godot. Alternatively, `core.Node` could be used; `godot.Node` is
	// just an alias for `core.Node`. Only one embedded field per class
	// can carry @extends since Godot only allows single inheritance.
	//
	// @extends
	godot.Node
}

// Language defines an enum type, which will automatically be part of LocaleLanguage, since that's our main type being
// exported here.  In order to be detected as an enum type, you must use a typedef to an int.  In Godot, the name will
// be transformed to SCREAMING_SNAKE_CASE without the "Language" prefix, so, "UNKNOWN", "ENGLISH" and "GERMAN" in this
// case.
type Language int

const (
	LanguageUnknown Language = iota
	LanguageEnglish
	LanguageGerman
)

// Parse is registered as a class-level static via the `@static`
// doctag — Godot exposes it as `LocaleLanguage.parse(value)` with
// no instance lookup. The receiver stays unnamed since the body
// doesn't need one.
//
// @static
func (LocaleLanguage) Parse(value string) Language {
	switch value {
	case "en":
		return LanguageEnglish
	case "de":
		return LanguageGerman
	}
	return LanguageUnknown
}

// DoSomething demonstrates a regular member function of LocaleLanguage.  Because the receiver has an assigned name,
// this is an instance function rather than a static function.  Exported methods will be bound to their snake case
// equivalent, unless a @name doc comment is provided as shown below.
//
// @name do_something_alt_name
func (l *LocaleLanguage) DoSomething() {

}

// Process here demonstrates implementing virtual methods that exist on the parent class — in this case,
// overriding Node._process. The `@override` doctag opts into virtual binding; codegen routes through
// RegisterClassVirtual and prepends the leading underscore Godot expects on engine virtuals.
//
// @override
func (l *LocaleLanguage) Process(_ float32) {

}

// Sum demonstrates a slice argument at the @class boundary. Godot
// passes a PackedInt64Array; the codegen marshals it through to a Go
// `[]int64` for the user method and the result rides back through the
// scalar int64 return path.
//
// @static
func (LocaleLanguage) Sum(values []int64) int64 {
	var total int64
	for _, v := range values {
		total += v
	}
	return total
}

// Names demonstrates a slice return at the @class boundary. The
// codegen builds a PackedStringArray from the returned []string before
// handing the value back to Godot.
//
// @static
func (LocaleLanguage) Names() []string {
	return []string{"unknown", "english", "german"}
}
