package main

import (
	"github.com/legendary-code/godot-go/core"
	"github.com/legendary-code/godot-go/internal/runtime"
)

//go:generate godot-go

// MyNode is a minimal Go-defined extension class — embeds core.Node so the
// host treats instances as nodes, and exposes a single Hello() method to
// GDScript.
type MyNode struct {
	core.Node
}

// Hello is the method GDScript reaches via `n.hello()`.
func (n *MyNode) Hello() {
	runtime.Print("godot-go: MyNode.Hello() reached from GDScript")
}
