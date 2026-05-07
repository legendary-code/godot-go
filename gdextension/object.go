package gdextension

/*
#cgo CFLAGS: -I${SRCDIR} -I${SRCDIR}/../godot

#include "shim.h"
*/
import "C"

import (
	"runtime"
	"sync"
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

// objectMethodCall caches the MethodBind for Object::call so the
// per-call lookup is amortized. Hash 3400424181 from
// extension_api.json#classes[Object].methods[call].hash; stable
// across Godot 4.4–4.6. (Earlier work used 3643564216, which is
// the hash for `Callable.call` — same name, wrong class. Symptom
// was a successful method-bind lookup that dispatched into Callable
// instead of Object, so the receiver pointer landed in the wrong
// signature and the call returned a NIL Variant.)
var objectMethodCall = sync.OnceValue(func() MethodBindPtr {
	return GetMethodBind(
		InternStringName("Object"),
		InternStringName("call"),
		3400424181)
})

// ObjectCall dispatches `instance.method_name(args...)` through Godot's
// variant call protocol — the engine looks up `method_name` against the
// instance's actual class hierarchy and invokes the matching
// registration. Lets parent-class Go-side methods route to whatever a
// subclass registered without any compile-time class knowledge — the
// foundation of `@abstract_methods` polymorphism.
//
// `instance` is the engine ObjectPtr (typically `wrapper.Ptr()`).
// `methodName` is an interned StringName (use InternStringName).
// `args` carries the per-arg Variants the caller already constructed
// — caller still owns each one and is responsible for Destroy.
// `ret` receives the return Variant; pass an uninitialized
// `[VariantSize]byte` slot for it. CallError is returned for the
// caller to surface (typical errors: method not found, arg-count
// mismatch, bad arg type).
func ObjectCall(instance ObjectPtr, methodName StringNamePtr, args []VariantPtr, ret VariantPtr) CallError {
	bind := objectMethodCall()
	if bind == nil {
		return CallError{Type: CallErrorInvalidMethod}
	}

	var nameSlot [VariantSize]byte
	nameVar := VariantPtr(unsafe.Pointer(&nameSlot))
	CallVariantFromType(variantFromStringName(), nameVar, TypePtr(unsafe.Pointer(methodName)))
	defer VariantDestroy(nameVar)

	allArgs := make([]VariantPtr, 0, len(args)+1)
	allArgs = append(allArgs, nameVar)
	allArgs = append(allArgs, args...)

	return CallMethodBindCall(bind, instance, allArgs, ret)
}

// GetSingleton looks up a global singleton by name. Returns nil if no
// singleton with that name is registered (e.g. Editor singletons outside the
// editor). The returned pointer is owned by the host — do not destroy.
func GetSingleton(name StringNamePtr) ObjectPtr {
	o := C.godot_go_call_global_get_singleton(iface.globalGetSingleton,
		C.GDExtensionConstStringNamePtr(name))
	return ObjectPtr(o)
}
