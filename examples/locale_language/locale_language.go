package main

// One @class per file is registered with Godot's ClassDB. The package-
// wide bindings (a single `bindings.gen.go` at the package root) are
// produced from a `//go:generate godot-go` in `generate.go` — one
// trigger per package walks every @class file in the directory.

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

// Language defines an enum type that becomes part of LocaleLanguage's
// class-scoped registration. The `@enum` doctag is what surfaces the
// type to Godot — values register as integer constants under
// `LocaleLanguage.Language`, and method args / returns / slice element
// slots referencing `Language` carry the typed-enum class_name so the
// editor renders typed-enum autocomplete instead of plain int. Each
// const value's identifier gets transformed to SCREAMING_SNAKE_CASE
// with the "Language" prefix stripped — so "UNKNOWN", "ENGLISH",
// "GERMAN" appear under `LocaleLanguage.Language` in GDScript.
//
// @enum
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

// Languages demonstrates a slice-of-typed-enum return at the @class
// boundary. The wire form is Array[Language] (TypedArray); Godot sees
// each element as a typed Language value, not a bare int.
//
// @static
func (LocaleLanguage) Languages() []Language {
	return []Language{LanguageUnknown, LanguageEnglish, LanguageGerman}
}

// FilterLanguages demonstrates two boundary features at once: a
// variadic typed-enum parameter (Go's `...Language` is identical to
// `[]Language` at the wire boundary, just nicer at the call site) and
// a typed-enum slice return. Returns the subset matching the given
// known set.
//
// @static
func (LocaleLanguage) FilterLanguages(values ...Language) []Language {
	out := make([]Language, 0, len(values))
	for _, v := range values {
		if v == LanguageEnglish || v == LanguageGerman {
			out = append(out, v)
		}
	}
	return out
}

// ConcatNames demonstrates a primitive variadic at the @class boundary
// — `...string` becomes a single PackedStringArray arg on the wire,
// spread back into the user method via Go's call-site `parts...` form.
//
// @static
func (LocaleLanguage) ConcatNames(parts ...string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += ","
		}
		out += p
	}
	return out
}
