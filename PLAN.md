Agent
=====
Do not modify this file when planning, maintain a separate `PLAN_EXECUTION.md` file for planning.

Project
=======
The goal of this project is to implement an intuitive, easy-to-use framework for building Godot extensions
(GDExtension) in golang.  One of the main goals of the project is to make it easier for the user to build
extensions without needing to deal with the hassles of registration.  The user simply implements their
classes, optionally with doc strings, adds a `//go:generate` in order to generate all required bindings
and registrations for the specified classes.  Below is a full example of how to handle various features
of go classes, to demonstrate what it should look like in practice.

`main.go`
```go
package main

// This go:generate call will run our tool and generate binding/registration code for the current file in a separate
// file, `main_bindings.go`.  By convention, only one struct declaration without an `@innerclass` doc string is allowed
// and will be the main class we are generating bindings for.  One file = one godot class to register.  Any additional
// struct declarations must use `@innerclass` doc string will be treated as inner classes of the main class.
//go:generate godot-go


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
// @abstract
// @description This is my overridden description
// @deprecated
type LocaleLanguage struct {
	godot.Node  // here we are specifying that this class extends Node in Godot.  Alternatively, `core.Node` could be
	            // used as well here, `godot.Node` is just an alias for `core.Node`.  Only one struct that represents
				// a godot built-in type or another user-defined type can be embedded, since Godot only allows for single
				// inheritance
}

// InnerExample demonstrates how to declare inner classes 
// @innerclass
type InnerExample struct {
	godot.Object
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

// Parse is, by convention, a static function in Godot.  In general, if the receiver of a method has no
// assigned name, we treat this like a static function in Godot, attached to our `LocaleLanguage` class in this instance
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

// Process here demonstrates overriding a virtual method on the parent class — in this case, `Node._process`.
// Use the `@override` doc tag to declare an override; codegen routes it through the engine's virtual dispatch
// path and prepends the leading underscore Godot uses for engine virtuals (`Process` → `_process`). Without
// `@override`, lowercase methods are treated as Go-private helpers and not registered with Godot at all,
// matching Go's own export model.
//
// @override
func (l *LocaleLanguage) Process(_ float32) {
	
}
```

File structure
==============
In this project, the required header file and json file for built-in types has already been provided under
the `godot` directory.  When generating built-in types, they should be placed in a package named after the
`api_type` field in `extension_api.json`.  In order to provide a flexible user experience, a type alias
should also be provided for the same built-in type in the `godot` (top-level) package.

