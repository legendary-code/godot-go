package main

// MyNode is the Phase 5a hand-bound proof — a Go-defined extension class
// extending core.Node with a single Hello() method. The registration code
// below is what cmd/godot-go (Phase 5b–5c) will eventually generate from
// idiomatic Go declarations like:
//
//   type MyNode struct {
//       core.Node
//   }
//
//   //godot:method
//   func (n *MyNode) Hello() { ... }
//
// For now it lives entirely under the user's hand so we can validate the
// underlying ABI in isolation.

import (
	"sync"
	"unsafe"

	"github.com/legendary-code/godot-go/core"
	"github.com/legendary-code/godot-go/internal/gdextension"
	"github.com/legendary-code/godot-go/internal/runtime"
)

// MyNode is a minimal extension class — embeds core.Node so promoted method
// receivers reach the inherited methods, plus carries any Go-side state we
// want to remember across host callbacks (none for now).
type MyNode struct {
	core.Node
}

// Hello is the method GDScript will reach via `n.hello()`.
func (n *MyNode) Hello() {
	runtime.Print("godot-go: MyNode.Hello() reached from GDScript")
}

// Per-instance handles. The pointer the host hands back to us as
// p_instance is the same value Construct returned — it must be stable for
// the lifetime of the host-side object, which means storing the *MyNode
// in a side table keyed by a small id and handing the id (as a void*) to
// the host. Storing a Go pointer in C memory is forbidden by cgo rules.
var (
	myNodeInstancesMu sync.Mutex
	myNodeInstances   = map[uintptr]*MyNode{}
	myNodeNextID      uintptr
)

func registerMyNodeInstance(n *MyNode) unsafe.Pointer {
	myNodeInstancesMu.Lock()
	myNodeNextID++
	id := myNodeNextID
	myNodeInstances[id] = n
	myNodeInstancesMu.Unlock()
	return unsafe.Pointer(id)
}

func lookupMyNodeInstance(handle unsafe.Pointer) *MyNode {
	myNodeInstancesMu.Lock()
	defer myNodeInstancesMu.Unlock()
	return myNodeInstances[uintptr(handle)]
}

func releaseMyNodeInstance(handle unsafe.Pointer) {
	myNodeInstancesMu.Lock()
	defer myNodeInstancesMu.Unlock()
	delete(myNodeInstances, uintptr(handle))
}

func registerMyNode() {
	gdextension.RegisterClass(gdextension.ClassDef{
		Name:      "MyNode",
		Parent:    "Node",
		IsExposed: true,

		Construct: func() (gdextension.ObjectPtr, unsafe.Pointer) {
			n := &MyNode{}
			parent := gdextension.ConstructObject(gdextension.InternStringName("Node"))
			n.BindPtr(parent)
			return parent, registerMyNodeInstance(n)
		},

		Free: func(instance unsafe.Pointer) {
			releaseMyNodeInstance(instance)
		},
	})

	gdextension.RegisterClassMethod(gdextension.ClassMethodDef{
		Class: "MyNode",
		Name:  "hello",
		Call: func(instance unsafe.Pointer, _ []gdextension.VariantPtr, _ gdextension.VariantPtr) gdextension.CallErrorType {
			n := lookupMyNodeInstance(instance)
			if n == nil {
				return gdextension.CallErrorInstanceIsNull
			}
			n.Hello()
			return gdextension.CallErrorOK
		},
		PtrCall: func(instance unsafe.Pointer, _ unsafe.Pointer, _ unsafe.Pointer) {
			if n := lookupMyNodeInstance(instance); n != nil {
				n.Hello()
			}
		},
	})
}
