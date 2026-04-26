package main

// This file is hand-written for Phase 5a/5b — it's the registration glue
// that cmd/godot-go will *generate* in Phase 5c. Once the codegen tool
// emits the equivalent output for mynode.go, delete this file (or let the
// tool overwrite it).
//
// Keeping the hand-bound version alongside mynode.go in the meantime
// gives Phase 5b's discovery pass an authoritative reference for what
// the eventual generated output should look like.

import (
	"sync"
	"unsafe"

	"github.com/legendary-code/godot-go/internal/gdextension"
)

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
