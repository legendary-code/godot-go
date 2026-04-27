package main

import (
	"fmt"

	"github.com/legendary-code/godot-go/core"
	"github.com/legendary-code/godot-go/internal/runtime"
)

//go:generate godot-go

// MyNode is a minimal Go-defined extension class — embeds core.Node so the
// host treats instances as nodes, and exposes Hello/Add/Greet to GDScript.
//
// Properties exercise the four cells of the @property matrix (Phase 6):
//   - Health     — field form, read-write. Codegen synthesizes Get/Set.
//   - MaxHealth  — field form, read-only. @readonly drops the setter.
//   - Score      — method form, read-only. User wrote GetScore only;
//                  no SetScore means the property is read-only.
//   - Tag        — method form, read-write. User wrote both GetTag
//                  and SetTag; codegen wires both as a normal property.
type MyNode struct {
	core.Node

	// @property
	Health int64

	// @readonly
	// @property
	MaxHealth int64

	// score / tag are private backings that the user-written getters
	// dispatch on. Lowercase = invisible to Godot; visible only to the
	// package itself.
	score int64
	tag   string
}

// Hello is the method GDScript reaches via `n.hello()`.
func (n *MyNode) Hello() {
	runtime.Print("godot-go: MyNode.Hello() reached from GDScript")
}

// Add exercises Phase 5d primitive arg / return marshalling.
func (n *MyNode) Add(a, b int64) int64 {
	return a + b
}

// Greet exercises Phase 5d string marshalling in both directions.
func (n *MyNode) Greet(name string) string {
	return fmt.Sprintf("hello, %s!", name)
}

// Origin exercises Phase 5e static dispatch — unnamed receiver makes this
// `MyNode.origin()` from GDScript, no instance lookup, no MethodFlagStatic
// double-counting.
func (MyNode) Origin() int64 {
	return 42
}

// GetScore demonstrates the method form of @property, read-only branch:
// the user owns the getter, no SetScore exists, so codegen registers
// `score` with no setter. Read-only is inferred — there's no @readonly
// tag here because there's nothing to disambiguate.
//
// @property
func (n *MyNode) GetScore() int64 {
	// score is initialized lazily so test_mynode.gd's first read sees a
	// deterministic value without us needing a constructor hook.
	if n.score == 0 {
		n.score = 99
	}
	return n.score
}

// GetTag / SetTag demonstrate the method form of @property, read-write
// branch: both methods exist in source AND both carry @property. The
// rule is symmetric — codegen requires the tag on each side so the
// user's intent is explicit on both halves of the property.
//
// @property
func (n *MyNode) GetTag() string { return n.tag }

// @property
func (n *MyNode) SetTag(v string) { n.tag = v }

// Process is a Phase 5e/6 virtual override — `@override` opts into
// virtual binding so codegen routes through RegisterClassVirtual and
// adds the leading underscore Godot expects on engine virtuals
// (`Process` → `_process`). The engine calls this on every frame for
// live nodes; the smoke test just invokes it directly from GDScript to
// keep the run deterministic without spinning up a scene tree.
//
// @override
func (n *MyNode) Process(delta float64) {
	runtime.Print(fmt.Sprintf("godot-go: MyNode._process(%.2f) reached from GDScript", delta))
}
