package main

import (
	"fmt"

	"github.com/legendary-code/godot-go/core"
	"github.com/legendary-code/godot-go/internal/runtime"
)

//go:generate godot-go

// MyNode is a minimal Go-defined extension class — embeds core.Node so the
// host treats instances as nodes, and exposes Hello/Add/Greet to GDScript.
type MyNode struct {
	core.Node
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
