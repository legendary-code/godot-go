package gdextension

/*
#cgo CFLAGS: -I${SRCDIR} -I${SRCDIR}/../../godot

#include "shim.h"
*/
import "C"

import (
	"sync"
	"sync/atomic"
	"unsafe"
)

// Class / method registry
//
// The host knows our extension classes by callback function pointers stored
// in a GDExtensionClassCreationInfo4. To dispatch back into Go we hand the
// host a small integer id (encoded as a void*) and look the corresponding
// Go entry up in side tables here. Storing Go pointers directly in C
// userdata fields would violate cgo pointer rules; the id approach sidesteps
// that and keeps the //export'd trampolines simple.

// ClassDef is everything user / generated code supplies to register a class
// with Godot's classdb. The framework owns the host-side function-pointer
// trampolines; only the Go-side decisions (parent class, how to construct,
// how to free) come from here.
type ClassDef struct {
	// Name is what GDScript and the editor see. Must be a unique non-empty
	// identifier; collides with built-in classes are rejected by the host.
	Name string

	// Parent names the class to inherit from — "Node", "Node3D", "Object",
	// or another extension class previously registered.
	Parent string

	// IsVirtual / IsAbstract / IsExposed mirror the GDExtensionClassCreationInfo4
	// flags. IsExposed=true is the common case (script can see the class).
	IsVirtual  bool
	IsAbstract bool
	IsExposed  bool

	// Construct is invoked when GDScript does `var n = MyNode.new()`.
	// Implementations should:
	//   1. Allocate the user's Go struct (e.g. a *MyNode).
	//   2. Construct the host-side parent object via ConstructObject(parent).
	//   3. Bind the parent ObjectPtr into the Go struct (so subsequent
	//      method calls have a backing host object).
	//   4. Return (objectPtr, instanceUserdata) where objectPtr is what to
	//      hand back to the host and instanceUserdata is a pointer-sized
	//      handle the framework will pin and pass back as p_instance to
	//      every subsequent callback (method calls, free).
	//
	// The instanceUserdata must NOT be a Go pointer the GC can move —
	// callers should box state in a stable allocation (e.g. via the
	// instance-handle helpers below) and pass that handle here.
	Construct func() (object ObjectPtr, instanceUserdata unsafe.Pointer)

	// Free is invoked when the host releases an instance previously
	// returned by Construct. instanceUserdata is the same value Construct
	// returned; implementations should free or unpin any side-table entry
	// keyed on it. The host already destroyed the object_ptr.
	Free func(instanceUserdata unsafe.Pointer)
}

// ClassMethodDef registers a single method on a previously-registered class.
// For Phase 5a we only support nullary void methods; arg/return marshalling
// arrives in Phase 5d.
type ClassMethodDef struct {
	Class string
	Name  string

	// Call is the variant-dispatch path (`call_func` in GDExtensionClassMethodInfo).
	// Used when GDScript invokes the method without static type info, or
	// when a Variant Object reference dispatches through Object::call.
	Call func(instanceUserdata unsafe.Pointer, args []VariantPtr, ret VariantPtr) CallErrorType

	// PtrCall is the typed dispatch path (`ptrcall_func`). Used when the
	// caller has full type info — typically from generated code or when the
	// host short-circuits a known-typed call.
	PtrCall func(instanceUserdata unsafe.Pointer, args unsafe.Pointer, ret unsafe.Pointer)

	// Flags mirrors GDExtensionClassMethodFlags. Defaults to NORMAL when zero.
	Flags MethodFlags
}

type registeredClass struct {
	def ClassDef
}

type registeredMethod struct {
	def ClassMethodDef
}

var (
	classRegMu     sync.RWMutex
	classByID      = map[uintptr]*registeredClass{}
	classByName    = map[string]uintptr{}
	nextClassID    atomic.Uintptr

	methodRegMu  sync.RWMutex
	methodByID   = map[uintptr]*registeredMethod{}
	nextMethodID atomic.Uintptr
)

// RegisterClass adds def.Name to the host's classdb. After this returns,
// `var x = <Name>.new()` from GDScript routes to def.Construct, and freeing
// the instance routes to def.Free.
//
// Must be called during InitLevelScene (or earlier) — registering after
// the engine has finished bootstrapping triggers host warnings.
func RegisterClass(def ClassDef) {
	if def.Construct == nil || def.Free == nil {
		panic("gdextension.RegisterClass: Construct and Free are required")
	}
	if def.Name == "" || def.Parent == "" {
		panic("gdextension.RegisterClass: Name and Parent are required")
	}

	id := nextClassID.Add(1)
	rc := &registeredClass{def: def}

	classRegMu.Lock()
	classByID[id] = rc
	classByName[def.Name] = id
	classRegMu.Unlock()

	className := InternStringName(def.Name)
	parentName := InternStringName(def.Parent)

	C.godot_go_register_extension_class(iface.classdbRegisterExtensionClass5,
		C.GDExtensionClassLibraryPtr(iface.library),
		C.GDExtensionConstStringNamePtr(className),
		C.GDExtensionConstStringNamePtr(parentName),
		unsafe.Pointer(id),
		boolToGD(def.IsVirtual),
		boolToGD(def.IsAbstract),
		boolToGD(def.IsExposed))
}

// RegisterClassMethod adds a method to a previously-registered extension
// class. Class must match a Name passed to RegisterClass on this same
// library; calling against an unknown class is a no-op (the host logs the
// error).
func RegisterClassMethod(def ClassMethodDef) {
	if def.Class == "" || def.Name == "" {
		panic("gdextension.RegisterClassMethod: Class and Name are required")
	}
	if def.Call == nil && def.PtrCall == nil {
		panic("gdextension.RegisterClassMethod: at least one of Call or PtrCall must be set")
	}

	id := nextMethodID.Add(1)
	rm := &registeredMethod{def: def}

	methodRegMu.Lock()
	methodByID[id] = rm
	methodRegMu.Unlock()

	className := InternStringName(def.Class)
	methodName := InternStringName(def.Name)

	flags := def.Flags
	if flags == 0 {
		flags = MethodFlagsDefault
	}

	C.godot_go_register_extension_class_method(iface.classdbRegisterExtensionClassMethod,
		C.GDExtensionClassLibraryPtr(iface.library),
		C.GDExtensionConstStringNamePtr(className),
		C.GDExtensionStringNamePtr(methodName),
		unsafe.Pointer(id),
		C.uint32_t(flags))
}

// UnregisterClass tears down a previously-registered class. Call from the
// matching deinit callback so the host's classdb stays consistent across
// hot-reloads. Unregistering a parent before its inheritors is rejected
// by the host.
func UnregisterClass(name string) {
	classRegMu.Lock()
	id, ok := classByName[name]
	if ok {
		delete(classByName, name)
		delete(classByID, id)
	}
	classRegMu.Unlock()
	if !ok {
		return
	}
	className := InternStringName(name)
	C.godot_go_call_classdb_unregister_extension_class(iface.classdbUnregisterExtensionClass,
		C.GDExtensionClassLibraryPtr(iface.library),
		C.GDExtensionConstStringNamePtr(className))
}

func lookupClass(id uintptr) *registeredClass {
	classRegMu.RLock()
	defer classRegMu.RUnlock()
	return classByID[id]
}

func lookupMethod(id uintptr) *registeredMethod {
	methodRegMu.RLock()
	defer methodRegMu.RUnlock()
	return methodByID[id]
}

//export godotGoCreateInstance
func godotGoCreateInstance(classUserdata unsafe.Pointer, _ C.GDExtensionBool) C.GDExtensionObjectPtr {
	rc := lookupClass(uintptr(classUserdata))
	if rc == nil {
		return nil
	}
	obj, inst := rc.def.Construct()
	if obj == nil {
		return nil
	}
	className := InternStringName(rc.def.Name)
	C.godot_go_call_object_set_instance(iface.objectSetInstance,
		C.GDExtensionObjectPtr(obj),
		C.GDExtensionConstStringNamePtr(className),
		C.GDExtensionClassInstancePtr(inst))
	return C.GDExtensionObjectPtr(obj)
}

//export godotGoFreeInstance
func godotGoFreeInstance(classUserdata unsafe.Pointer, instance C.GDExtensionClassInstancePtr) {
	rc := lookupClass(uintptr(classUserdata))
	if rc == nil {
		return
	}
	rc.def.Free(unsafe.Pointer(instance))
}

//export godotGoMethodCall
func godotGoMethodCall(methodUserdata unsafe.Pointer, instance C.GDExtensionClassInstancePtr,
	args *C.GDExtensionConstVariantPtr, argCount C.GDExtensionInt,
	ret C.GDExtensionVariantPtr, errOut *C.GDExtensionCallError) {
	rm := lookupMethod(uintptr(methodUserdata))
	if rm == nil || rm.def.Call == nil {
		if errOut != nil {
			errOut.error = C.GDExtensionCallErrorType(CallErrorInvalidMethod)
		}
		return
	}
	var argSlice []VariantPtr
	if argCount > 0 && args != nil {
		// Treat the host array as a slice of opaque variant pointers.
		hdr := unsafe.Slice((*C.GDExtensionConstVariantPtr)(args), int(argCount))
		argSlice = make([]VariantPtr, argCount)
		for i, p := range hdr {
			argSlice[i] = VariantPtr(p)
		}
	}
	cerr := rm.def.Call(unsafe.Pointer(instance), argSlice, VariantPtr(ret))
	if errOut != nil {
		errOut.error = C.GDExtensionCallErrorType(cerr)
	}
}

//export godotGoMethodPtrcall
func godotGoMethodPtrcall(methodUserdata unsafe.Pointer, instance C.GDExtensionClassInstancePtr,
	args *C.GDExtensionConstTypePtr, ret C.GDExtensionTypePtr) {
	rm := lookupMethod(uintptr(methodUserdata))
	if rm == nil || rm.def.PtrCall == nil {
		return
	}
	rm.def.PtrCall(unsafe.Pointer(instance), unsafe.Pointer(args), unsafe.Pointer(ret))
}
