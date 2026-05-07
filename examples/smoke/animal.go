package main

import (
	"github.com/legendary-code/godot-go/godot"
)

// AnimalAbstract declares the contract subclasses of Animal must
// implement. The codegen synthesizes a Go-side dispatcher for each
// method on `*Animal` that routes through Godot's variant call
// (`Object::call`) — so a parent-typed reference holding a Dog
// instance dispatches to Dog's concrete implementation, both from
// Go and from GDScript.
//
// @abstract_methods
type AnimalAbstract interface {
	// Speak returns the animal's signature noise. Subclasses must
	// implement.
	Speak() string

	// Move advances the animal by `distance` units in some
	// implementation-defined direction.
	Move(distance int64)
}

// Animal is an abstract base class with no instantiable form of its
// own — `@abstract` blocks `Animal.new()` from GDScript. The class
// exists so subclasses can extend it (via `@extends Animal`) and so
// the codegen has somewhere to attach the dispatchers for the methods
// declared on AnimalAbstract.
//
// @class
// @abstract
type Animal struct {
	// @extends
	godot.RefCounted
}
