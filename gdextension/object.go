package gdextension

/*
#cgo CFLAGS: -I${SRCDIR} -I${SRCDIR}/../godot

#include "shim.h"
*/
import "C"

import (
	"runtime"
	"unsafe"
)

// ConstructObject creates a new instance of the named engine class via the
// host's classdb. Mirrors `classdb_construct_object2` (Godot 4.4+); the older
// `classdb_construct_object` is intentionally not exposed. The returned
// ObjectPtr must eventually be passed to DestroyObject (or to a refcount-
// aware unref path, once one is added).
func ConstructObject(class StringNamePtr) ObjectPtr {
	o := C.godot_go_call_classdb_construct_object2(iface.classdbConstructObject2,
		C.GDExtensionConstStringNamePtr(class))
	return ObjectPtr(o)
}

// GetMethodBind resolves the MethodBind for class.method against the host
// classdb. The hash is generated from extension_api.json#classes[*].methods[*].hash
// and lets the host detect codegen drift between the engine and the bindings.
func GetMethodBind(class, method StringNamePtr, hash int64) MethodBindPtr {
	mb := C.godot_go_call_classdb_get_method_bind(iface.classdbGetMethodBind,
		C.GDExtensionConstStringNamePtr(class),
		C.GDExtensionConstStringNamePtr(method),
		C.GDExtensionInt(hash))
	return MethodBindPtr(mb)
}

// CallMethodBindPtrCall invokes a previously resolved engine-class method on
// instance using the typed (ptrcall) ABI. args may be nil for nullary methods;
// ret may be nil for `void` returns. Mirrors the variant CallPtrBuiltinMethod
// shape so generated code can reuse the same argPrep/returnPrep machinery.
func CallMethodBindPtrCall(mb MethodBindPtr, instance ObjectPtr, args []TypePtr, ret TypePtr) {
	pinner := pinArgs(args)
	defer pinner.Unpin()
	C.godot_go_call_object_method_bind_ptrcall(iface.objectMethodBindPtrcall,
		C.GDExtensionMethodBindPtr(mb),
		C.GDExtensionObjectPtr(instance),
		typeArgArray(args),
		C.GDExtensionTypePtr(ret))
}

// CallError mirrors the relevant fields of GDExtensionCallError. Populated by
// CallMethodBindCall (the vararg dispatch path); always OK on the typed path.
type CallError struct {
	Type     CallErrorType
	Argument int32
	Expected int32
}

// CallMethodBindCall is the vararg dispatch path. Args are Variant pointers,
// the return slot is an uninitialized Variant. Used for methods marked
// `is_vararg: true` in extension_api.json.
func CallMethodBindCall(mb MethodBindPtr, instance ObjectPtr, args []VariantPtr, ret VariantPtr) CallError {
	var pinner runtime.Pinner
	defer pinner.Unpin()
	for _, a := range args {
		if a != nil {
			pinner.Pin(a)
		}
	}
	var argv *C.GDExtensionConstVariantPtr
	if len(args) > 0 {
		argv = (*C.GDExtensionConstVariantPtr)(unsafe.Pointer(&args[0]))
	}
	var cerr C.GDExtensionCallError
	C.godot_go_call_object_method_bind_call(iface.objectMethodBindCall,
		C.GDExtensionMethodBindPtr(mb),
		C.GDExtensionObjectPtr(instance),
		argv,
		C.GDExtensionInt(len(args)),
		C.GDExtensionUninitializedVariantPtr(ret),
		&cerr)
	return CallError{
		Type:     CallErrorType(cerr.error),
		Argument: int32(cerr.argument),
		Expected: int32(cerr.expected),
	}
}

// DestroyObject calls the host's object_destroy on o. Note: this is the raw
// destructor; for refcounted classes you'll want to go through an unref path
// instead. For non-refcounted objects (Engine, Node, …) the host owns the
// lifetime; only call this on instances you constructed yourself.
func DestroyObject(o ObjectPtr) {
	if o == nil {
		return
	}
	C.godot_go_call_object_destroy(iface.objectDestroy, C.GDExtensionObjectPtr(o))
}

// GetSingleton looks up a global singleton by name. Returns nil if no
// singleton with that name is registered (e.g. Editor singletons outside the
// editor). The returned pointer is owned by the host — do not destroy.
func GetSingleton(name StringNamePtr) ObjectPtr {
	o := C.godot_go_call_global_get_singleton(iface.globalGetSingleton,
		C.GDExtensionConstStringNamePtr(name))
	return ObjectPtr(o)
}
