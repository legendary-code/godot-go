package gdextension

/*
#cgo CFLAGS: -I${SRCDIR} -I${SRCDIR}/../godot

#include "shim.h"
*/
import "C"

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"unsafe"
)

// maxMethodArgs mirrors GODOT_GO_MAX_METHOD_ARGS in shim.c — the C side
// stack-allocates PropertyInfo arrays to this size, so the Go side rejects
// anything larger before the cgo call.
const maxMethodArgs = 32

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
// Phase 5d wires up the typed argument / return info that Godot needs in
// order to type-check GDScript callers and route typed PtrCall dispatch.
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

	// HasReturn declares whether the method returns a value. ReturnType /
	// ReturnMetadata are ignored when false.
	HasReturn      bool
	ReturnType     VariantType
	ReturnMetadata MethodArgumentMetadata

	// ArgTypes / ArgMetadata are parallel slices describing each positional
	// argument. They MUST have the same length; that length becomes the
	// method's `argument_count` in the host's classdb. nil/empty registers
	// a no-arg method.
	ArgTypes    []VariantType
	ArgMetadata []MethodArgumentMetadata

	// ArgNames optionally carries the source-level identifier for each
	// positional arg (parallel to ArgTypes). When supplied, the editor
	// shows the name in autocomplete/hover (e.g. `take_damage(amount: int)`
	// rather than `take_damage(arg0: int)`). Empty entries — or a nil/short
	// slice — fall back to anonymous slots, keeping callers that don't
	// care about names working unchanged. If non-nil, must be the same
	// length as ArgTypes; the registration call panics otherwise.
	ArgNames []string

	// ArgClassNames optionally qualifies each argument with a typed-enum
	// or class identity (parallel to ArgTypes). The slot's wire type
	// stays whatever ArgTypes says (typically VariantTypeInt for enums);
	// ArgClassNames carries strings like "MyClass.Mode" so the editor
	// renders typed-enum dropdowns/autocomplete instead of plain int.
	// Empty entries register as untyped. If non-nil, must be the same
	// length as ArgTypes.
	ArgClassNames []string

	// ReturnClassName qualifies the return type the same way ArgClassNames
	// does for arguments. Empty (default) registers as untyped — fine for
	// primitives. Set to "<Class>.<EnumName>" for typed-enum returns.
	ReturnClassName string
}

type registeredClass struct {
	def ClassDef
}

type registeredMethod struct {
	def ClassMethodDef
}

// ClassVirtualDef registers a Go implementation of a virtual method on a
// previously-registered class. Two engine-side hooks are wired in one
// step:
//
//   - The override is added to ClassDB via the same path
//     RegisterClassMethod uses, with MethodFlagVirtual set. This makes
//     the method visible to GDScript explicit calls (`n._process(0.5)`)
//     and to Object::call() — without it, ClassDB lookups fail.
//   - The override is recorded in the per-(class, name) virtual table
//     consulted by the framework's `get_virtual_call_data_func` shim, so
//     engine-internal dispatch (e.g. SceneTree's per-frame _process) hits
//     our Go callback through `call_virtual_with_data_func`.
//
// godot-cpp does the equivalent under the hood when binding a virtual
// override; we surface it as one call here so users (and codegen) don't
// have to know about both halves.
//
// Phase 5e: virtual dispatch is ptr-typed only (no Variant Call path on
// the virtual table). The ClassDB shadow does carry a Call so explicit
// GDScript invocations type-check, but engine-driven dispatch always
// goes through PtrCall.
type ClassVirtualDef struct {
	// Class names a previously-registered extension class.
	Class string
	// Name is the engine-visible virtual name, including any leading
	// underscore (e.g. "_process", "_ready", "_input").
	Name string
	// PtrCall receives the typed argument array and return slot, exactly
	// as ClassMethodDef.PtrCall does. instanceUserdata is the value
	// ClassDef.Construct returned for this object.
	PtrCall func(instanceUserdata unsafe.Pointer, args unsafe.Pointer, ret unsafe.Pointer)
	// Call is the variant-dispatch path used when GDScript explicitly
	// invokes the virtual by name. Optional — when nil, the framework
	// auto-fills a thin wrapper that converts arguments via the supplied
	// ArgTypes/ReturnType metadata, but for codegen-emitted overrides
	// the generator hands a fully-typed Call shim here for consistency
	// with regular methods.
	Call func(instanceUserdata unsafe.Pointer, args []VariantPtr, ret VariantPtr) CallErrorType
	// HasReturn / ReturnType / ReturnMetadata mirror ClassMethodDef.
	HasReturn      bool
	ReturnType     VariantType
	ReturnMetadata MethodArgumentMetadata
	// ArgTypes / ArgMetadata are parallel — same shape as ClassMethodDef.
	ArgTypes    []VariantType
	ArgMetadata []MethodArgumentMetadata
	// ArgNames optionally carries the source-level identifier for each
	// arg. See ClassMethodDef.ArgNames for semantics.
	ArgNames []string
	// ArgClassNames / ReturnClassName carry typed-enum identities the
	// same way ClassMethodDef does. Engine virtuals rarely take typed
	// enums, but the codegen passes whatever the source signature uses
	// so the editor's docs page reflects it.
	ArgClassNames   []string
	ReturnClassName string
}

type registeredVirtual struct {
	def ClassVirtualDef
}

// virtualKey identifies a virtual override at the (className, methodName)
// granularity — the host queries by name within a class on its first
// dispatch, and we cache the (id encoded as void*) result it then keys on.
type virtualKey struct {
	Class, Method string
}

var (
	classRegMu     sync.RWMutex
	classByID      = map[uintptr]*registeredClass{}
	classByName    = map[string]uintptr{}
	classNameByID  = map[uintptr]string{}
	nextClassID    atomic.Uintptr

	methodRegMu  sync.RWMutex
	methodByID   = map[uintptr]*registeredMethod{}
	nextMethodID atomic.Uintptr

	virtualRegMu  sync.RWMutex
	virtualByID   = map[uintptr]*registeredVirtual{}
	virtualByKey  = map[virtualKey]uintptr{}
	nextVirtualID atomic.Uintptr
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
	classNameByID[id] = def.Name
	classRegMu.Unlock()

	className := InternStringName(def.Name)
	parentName := InternStringName(def.Parent)

	C.godot_go_register_extension_class(iface.classdbRegisterExtensionClass5,
		iface.classdbRegisterExtensionClass4,
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
	if len(def.ArgTypes) != len(def.ArgMetadata) {
		panic(fmt.Sprintf("gdextension.RegisterClassMethod: ArgTypes (len %d) and ArgMetadata (len %d) must be parallel",
			len(def.ArgTypes), len(def.ArgMetadata)))
	}
	if def.ArgNames != nil && len(def.ArgNames) != len(def.ArgTypes) {
		panic(fmt.Sprintf("gdextension.RegisterClassMethod: ArgNames (len %d) must match ArgTypes (len %d) when supplied",
			len(def.ArgNames), len(def.ArgTypes)))
	}
	if def.ArgClassNames != nil && len(def.ArgClassNames) != len(def.ArgTypes) {
		panic(fmt.Sprintf("gdextension.RegisterClassMethod: ArgClassNames (len %d) must match ArgTypes (len %d) when supplied",
			len(def.ArgClassNames), len(def.ArgTypes)))
	}
	if len(def.ArgTypes) > maxMethodArgs {
		panic(fmt.Sprintf("gdextension.RegisterClassMethod: too many args (%d > %d)",
			len(def.ArgTypes), maxMethodArgs))
	}

	id := nextMethodID.Add(1)
	rm := &registeredMethod{def: def}

	methodRegMu.Lock()
	methodByID[id] = rm
	methodRegMu.Unlock()

	className := InternStringName(def.Class)
	methodName := InternStringName(def.Name)
	emptyName := InternStringName("")
	emptyStr := internedEmptyString()

	flags := def.Flags
	if flags == 0 {
		flags = MethodFlagsDefault
	}

	// Translate the parallel arg-info slices into C-side scalar arrays. These
	// arrays contain only integer tags (no Go pointers), so cgo's pointer
	// rule allows us to take their address as a direct call argument.
	argCount := len(def.ArgTypes)
	var argTypesPtr, argMetaPtr *C.uint32_t
	var argNamesPtr, argClassNamesPtr *C.GDExtensionConstStringNamePtr
	var pinner runtime.Pinner
	defer pinner.Unpin()
	if argCount > 0 {
		argTypes := make([]C.uint32_t, argCount)
		argMeta := make([]C.uint32_t, argCount)
		argNames := make([]C.GDExtensionConstStringNamePtr, argCount)
		argClassNames := make([]C.GDExtensionConstStringNamePtr, argCount)
		for i := range def.ArgTypes {
			argTypes[i] = C.uint32_t(def.ArgTypes[i])
			argMeta[i] = C.uint32_t(def.ArgMetadata[i])
			// Empty / unspecified entries leave the slot as a NULL
			// StringNamePtr; the C trampoline falls back to the empty
			// StringName for those. Names are interned through the
			// shared cache so repeated args ("amount", "delta", etc.)
			// across many methods share storage. The cache backs each
			// slot with Go-heap memory (see intern.go), so cgo's
			// "Go pointer to unpinned Go pointer" rule fires unless we
			// pin the slot through the call.
			if i < len(def.ArgNames) && def.ArgNames[i] != "" {
				name := InternStringName(def.ArgNames[i])
				pinner.Pin(name)
				argNames[i] = C.GDExtensionConstStringNamePtr(name)
			}
			if i < len(def.ArgClassNames) && def.ArgClassNames[i] != "" {
				cls := InternStringName(def.ArgClassNames[i])
				pinner.Pin(cls)
				argClassNames[i] = C.GDExtensionConstStringNamePtr(cls)
			}
		}
		argTypesPtr = &argTypes[0]
		argMetaPtr = &argMeta[0]
		argNamesPtr = &argNames[0]
		argClassNamesPtr = &argClassNames[0]
	}

	var returnClassName C.GDExtensionConstStringNamePtr
	if def.HasReturn && def.ReturnClassName != "" {
		rc := InternStringName(def.ReturnClassName)
		pinner.Pin(rc)
		returnClassName = C.GDExtensionConstStringNamePtr(rc)
	}

	C.godot_go_register_extension_class_method(iface.classdbRegisterExtensionClassMethod,
		C.GDExtensionClassLibraryPtr(iface.library),
		C.GDExtensionConstStringNamePtr(className),
		C.GDExtensionStringNamePtr(methodName),
		unsafe.Pointer(id),
		C.uint32_t(flags),
		C.GDExtensionConstStringNamePtr(emptyName),
		C.GDExtensionConstStringPtr(emptyStr),
		boolToGD(def.HasReturn),
		C.uint32_t(def.ReturnType),
		C.uint32_t(def.ReturnMetadata),
		C.uint32_t(argCount),
		argTypesPtr,
		argMetaPtr,
		argNamesPtr,
		argClassNamesPtr,
		returnClassName)
}

// ClassPropertyDef registers a property on a previously-registered class.
// Phase 6: properties are wired by name to existing methods — the engine's
// classdb_register_extension_class_property API takes (class, info, setter,
// getter) where setter/getter are method names looked up at register time.
//
// Both Setter and Getter must name methods previously registered via
// RegisterClassMethod on the same class. An empty Setter registers the
// property as read-only.
//
// Hint + HintString control the inspector's editor — RANGE with "0,100"
// renders as a slider, ENUM with "A,B,C" as a dropdown, FILE with
// "*.png,*.jpg" as a file picker, etc. Default is HintNone with empty
// HintString.
type ClassPropertyDef struct {
	// Class names a previously-registered extension class.
	Class string
	// Name is the engine-visible property name (snake_case by convention).
	Name string
	// Type is the Godot variant type the property carries.
	Type VariantType
	// Setter / Getter name methods registered on Class. Setter may be
	// empty for read-only properties; Getter is required.
	Setter string
	Getter string
	// Hint + HintString are the property's editor metadata. See
	// PropertyHint constants in types.go for supported values; payload
	// format is hint-specific and matches what GDScript's `@export_*`
	// annotations produce.
	Hint       PropertyHint
	HintString string
	// ClassName qualifies the property's value-type identity for the
	// editor. "<MainClass>.<EnumName>" for typed-enum int properties so
	// the inspector renders the typed enum. Empty for plain primitive
	// properties (Godot ignores it then).
	ClassName string
}

// RegisterClassProperty wires a property on an extension class to its
// getter/setter methods. The methods must already be registered through
// RegisterClassMethod on the same class — the host resolves the names at
// register time and warns if either is missing.
func RegisterClassProperty(def ClassPropertyDef) {
	if def.Class == "" || def.Name == "" {
		panic("gdextension.RegisterClassProperty: Class and Name are required")
	}
	if def.Getter == "" {
		panic("gdextension.RegisterClassProperty: Getter is required")
	}

	className := InternStringName(def.Class)
	propertyName := InternStringName(def.Name)
	getterName := InternStringName(def.Getter)
	emptyName := InternStringName("")

	setterName := emptyName
	if def.Setter != "" {
		setterName = InternStringName(def.Setter)
	}

	hintStr := InternString(def.HintString)

	var valueClassName C.GDExtensionConstStringNamePtr
	var pinner runtime.Pinner
	defer pinner.Unpin()
	if def.ClassName != "" {
		cn := InternStringName(def.ClassName)
		pinner.Pin(cn)
		valueClassName = C.GDExtensionConstStringNamePtr(cn)
	}

	C.godot_go_register_extension_class_property(iface.classdbRegisterExtensionClassProperty,
		C.GDExtensionClassLibraryPtr(iface.library),
		C.GDExtensionConstStringNamePtr(className),
		C.GDExtensionStringNamePtr(propertyName),
		valueClassName,
		C.GDExtensionConstStringNamePtr(emptyName),
		C.GDExtensionConstStringNamePtr(setterName),
		C.GDExtensionConstStringNamePtr(getterName),
		C.GDExtensionConstStringPtr(hintStr),
		C.uint32_t(def.Type),
		C.uint32_t(def.Hint))
}

// ClassPropertyGroupDef registers a property group / subgroup header on
// an extension class. Group registration is positional: properties
// registered AFTER this call belong to the group until the next group
// or subgroup is registered. Phase 6 always passes empty Prefix; a
// future iteration may surface it.
type ClassPropertyGroupDef struct {
	Class  string
	Name   string
	Prefix string
}

// RegisterClassPropertyGroup adds a property group header to an
// extension class. Subsequent RegisterClassProperty calls fall under
// this group in the inspector until another group/subgroup is
// registered.
func RegisterClassPropertyGroup(def ClassPropertyGroupDef) {
	if def.Class == "" || def.Name == "" {
		panic("gdextension.RegisterClassPropertyGroup: Class and Name are required")
	}
	className := InternStringName(def.Class)
	groupName := InternString(def.Name)
	prefix := InternString(def.Prefix)
	C.godot_go_register_extension_class_property_group(iface.classdbRegisterExtensionClassPropertyGroup,
		C.GDExtensionClassLibraryPtr(iface.library),
		C.GDExtensionConstStringNamePtr(className),
		C.GDExtensionConstStringPtr(groupName),
		C.GDExtensionConstStringPtr(prefix))
}

// RegisterClassPropertySubgroup adds a property subgroup header — nested
// under the current group in the inspector. Same positional rules as
// RegisterClassPropertyGroup.
func RegisterClassPropertySubgroup(def ClassPropertyGroupDef) {
	if def.Class == "" || def.Name == "" {
		panic("gdextension.RegisterClassPropertySubgroup: Class and Name are required")
	}
	className := InternStringName(def.Class)
	subName := InternString(def.Name)
	prefix := InternString(def.Prefix)
	C.godot_go_register_extension_class_property_subgroup(iface.classdbRegisterExtensionClassPropertySubgroup,
		C.GDExtensionClassLibraryPtr(iface.library),
		C.GDExtensionConstStringNamePtr(className),
		C.GDExtensionConstStringPtr(subName),
		C.GDExtensionConstStringPtr(prefix))
}

// ClassSignalDef registers a signal on a previously-registered class.
// Phase 6: signal arg types travel as parallel scalar arrays (mirroring
// ClassMethodDef.ArgTypes / ArgMetadata), so the Go caller never builds a
// Go-allocated struct with pointers in it that's then passed to C.
//
// ArgMetadata is currently unused on the engine side for signals (the
// PropertyInfo "metadata" field is about hint encoding, not width); we
// carry the parallel slice for symmetry with method registration so a
// future engine version that grows it has a place to plug in.
type ClassSignalDef struct {
	Class       string
	Name        string
	ArgTypes    []VariantType
	ArgMetadata []MethodArgumentMetadata
	// ArgNames optionally carries the source-level identifier for each
	// signal arg. See ClassMethodDef.ArgNames for the same semantics:
	// names show up in the editor's signal-receiver autocomplete and
	// docs; nil/short slices fall back to anonymous slots.
	ArgNames []string
	// ArgClassNames qualifies each arg with a typed-enum identity
	// ("MyClass.Mode"). Same shape and rules as ClassMethodDef.ArgClassNames.
	ArgClassNames []string
}

// RegisterClassSignal adds a signal on an extension class. After this
// returns, GDScript can `obj.connect("name", callable)` against the
// signal and emissions through Object::emit_signal route to connected
// callables.
func RegisterClassSignal(def ClassSignalDef) {
	if def.Class == "" || def.Name == "" {
		panic("gdextension.RegisterClassSignal: Class and Name are required")
	}
	if len(def.ArgTypes) != len(def.ArgMetadata) {
		panic(fmt.Sprintf("gdextension.RegisterClassSignal: ArgTypes (len %d) and ArgMetadata (len %d) must be parallel",
			len(def.ArgTypes), len(def.ArgMetadata)))
	}
	if def.ArgNames != nil && len(def.ArgNames) != len(def.ArgTypes) {
		panic(fmt.Sprintf("gdextension.RegisterClassSignal: ArgNames (len %d) must match ArgTypes (len %d) when supplied",
			len(def.ArgNames), len(def.ArgTypes)))
	}
	if def.ArgClassNames != nil && len(def.ArgClassNames) != len(def.ArgTypes) {
		panic(fmt.Sprintf("gdextension.RegisterClassSignal: ArgClassNames (len %d) must match ArgTypes (len %d) when supplied",
			len(def.ArgClassNames), len(def.ArgTypes)))
	}
	if len(def.ArgTypes) > maxMethodArgs {
		panic(fmt.Sprintf("gdextension.RegisterClassSignal: too many args (%d > %d)",
			len(def.ArgTypes), maxMethodArgs))
	}

	className := InternStringName(def.Class)
	signalName := InternStringName(def.Name)
	emptyName := InternStringName("")
	emptyStr := internedEmptyString()

	argCount := len(def.ArgTypes)
	var argTypesPtr, argMetaPtr *C.uint32_t
	var argNamesPtr, argClassNamesPtr *C.GDExtensionConstStringNamePtr
	var pinner runtime.Pinner
	defer pinner.Unpin()
	if argCount > 0 {
		argTypes := make([]C.uint32_t, argCount)
		argMeta := make([]C.uint32_t, argCount)
		argNames := make([]C.GDExtensionConstStringNamePtr, argCount)
		argClassNames := make([]C.GDExtensionConstStringNamePtr, argCount)
		for i := range def.ArgTypes {
			argTypes[i] = C.uint32_t(def.ArgTypes[i])
			argMeta[i] = C.uint32_t(def.ArgMetadata[i])
			// See RegisterClassMethod for the same pin: the intern
			// cache hands back Go-heap-allocated slots, so cgo's
			// pointer check rejects the array unless we pin each name.
			if i < len(def.ArgNames) && def.ArgNames[i] != "" {
				name := InternStringName(def.ArgNames[i])
				pinner.Pin(name)
				argNames[i] = C.GDExtensionConstStringNamePtr(name)
			}
			if i < len(def.ArgClassNames) && def.ArgClassNames[i] != "" {
				cls := InternStringName(def.ArgClassNames[i])
				pinner.Pin(cls)
				argClassNames[i] = C.GDExtensionConstStringNamePtr(cls)
			}
		}
		argTypesPtr = &argTypes[0]
		argMetaPtr = &argMeta[0]
		argNamesPtr = &argNames[0]
		argClassNamesPtr = &argClassNames[0]
	}

	C.godot_go_register_extension_class_signal(iface.classdbRegisterExtensionClassSignal,
		C.GDExtensionClassLibraryPtr(iface.library),
		C.GDExtensionConstStringNamePtr(className),
		C.GDExtensionConstStringNamePtr(signalName),
		C.GDExtensionConstStringNamePtr(emptyName),
		C.GDExtensionConstStringPtr(emptyStr),
		C.uint32_t(argCount),
		argTypesPtr,
		argMetaPtr,
		argNamesPtr,
		argClassNamesPtr)
}

// ClassIntegerConstantDef registers a single class-scoped integer
// constant — typically one value of a `@enum` or `@bitfield` Go enum
// type. The combination of (Class, Enum, Name) is what the editor
// shows in autocomplete; values registered with the same (Class, Enum)
// share an enum dropdown, and IsBitfield switches between the
// dropdown-of-mutually-exclusive form and the bitwise-OR form.
type ClassIntegerConstantDef struct {
	Class      string
	Enum       string
	Name       string
	Value      int64
	IsBitfield bool
}

// RegisterClassIntegerConstant registers one integer constant on a
// previously-registered class. Call once per value of a typed enum.
// Empty Enum is allowed and registers the constant as a bare class
// constant (no enum grouping); Godot still namespaces it under the
// class.
func RegisterClassIntegerConstant(def ClassIntegerConstantDef) {
	if def.Class == "" || def.Name == "" {
		panic("gdextension.RegisterClassIntegerConstant: Class and Name are required")
	}
	if iface.classdbRegisterExtensionClassIntegerConstant == nil {
		// Symbol existed since 4.1 — no realistic version where it's
		// missing, but guard anyway since loadInterface treats every
		// resolution as best-effort.
		return
	}
	className := InternStringName(def.Class)
	enumName := InternStringName(def.Enum)
	constName := InternStringName(def.Name)

	C.godot_go_register_extension_class_integer_constant(
		iface.classdbRegisterExtensionClassIntegerConstant,
		C.GDExtensionClassLibraryPtr(iface.library),
		C.GDExtensionConstStringNamePtr(className),
		C.GDExtensionConstStringNamePtr(enumName),
		C.GDExtensionConstStringNamePtr(constName),
		C.int64_t(def.Value),
		boolToGD(def.IsBitfield))
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
		delete(classNameByID, id)
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

// RegisterClassVirtual records a Go implementation of an engine virtual
// method and exposes it via ClassDB so GDScript explicit calls
// (`n._process(0.5)`) resolve to the same override engine-internal
// dispatch hits.
//
// Must be called after RegisterClass for the same class — the host
// caches override discoveries per-class on first dispatch, but the
// ClassDB shadow registration still needs to happen before any caller
// looks the method up.
//
// Multiple calls with the same (Class, Name) replace the prior virtual
// entry; the ClassDB shadow registration is not deduplicated, so don't
// double-register.
func RegisterClassVirtual(def ClassVirtualDef) {
	if def.Class == "" || def.Name == "" {
		panic("gdextension.RegisterClassVirtual: Class and Name are required")
	}
	if def.PtrCall == nil {
		panic("gdextension.RegisterClassVirtual: PtrCall is required")
	}

	id := nextVirtualID.Add(1)
	rv := &registeredVirtual{def: def}
	key := virtualKey{Class: def.Class, Method: def.Name}

	virtualRegMu.Lock()
	virtualByID[id] = rv
	virtualByKey[key] = id
	virtualRegMu.Unlock()

	// ClassDB shadow registration. Without this, ClassDB lookups for the
	// virtual name fail and explicit GDScript calls error with
	// "Nonexistent function". The MethodFlagVirtual bit advertises the
	// method as a virtual override (engine treats it as one for caching
	// and editor display).
	RegisterClassMethod(ClassMethodDef{
		Class:           def.Class,
		Name:            def.Name,
		Call:            def.Call,
		PtrCall:         def.PtrCall,
		Flags:           MethodFlagsDefault | MethodFlagVirtual,
		HasReturn:       def.HasReturn,
		ReturnType:      def.ReturnType,
		ReturnMetadata:  def.ReturnMetadata,
		ArgTypes:        def.ArgTypes,
		ArgMetadata:     def.ArgMetadata,
		ArgNames:        def.ArgNames,
		ArgClassNames:   def.ArgClassNames,
		ReturnClassName: def.ReturnClassName,
	})
}

func lookupClass(id uintptr) *registeredClass {
	classRegMu.RLock()
	defer classRegMu.RUnlock()
	return classByID[id]
}

func lookupClassName(id uintptr) string {
	classRegMu.RLock()
	defer classRegMu.RUnlock()
	return classNameByID[id]
}

func lookupMethod(id uintptr) *registeredMethod {
	methodRegMu.RLock()
	defer methodRegMu.RUnlock()
	return methodByID[id]
}

func lookupVirtual(id uintptr) *registeredVirtual {
	virtualRegMu.RLock()
	defer virtualRegMu.RUnlock()
	return virtualByID[id]
}

func lookupVirtualID(class, method string) uintptr {
	virtualRegMu.RLock()
	defer virtualRegMu.RUnlock()
	return virtualByKey[virtualKey{Class: class, Method: method}]
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

// godotGoGetVirtualCallData is the framework-side handler the host invokes
// once per (class, virtual_name) pair to discover overrides. We return the
// virtual id encoded as void* — the host caches the result and threads it
// back through godotGoCallVirtualWithData on every dispatch, sidestepping
// a name lookup on the hot path.
//
// Returning NULL (id 0) means "no override" — the host falls back to the
// parent class's implementation. The hash arg (Godot 4.6 ABI) is the
// virtual signature hash, useful for ABI versioning checks; we ignore it
// for now and rely on (class, name).
//
//export godotGoGetVirtualCallData
func godotGoGetVirtualCallData(classUserdata unsafe.Pointer, name C.GDExtensionConstStringNamePtr, _ C.uint32_t) unsafe.Pointer {
	className := lookupClassName(uintptr(classUserdata))
	if className == "" {
		return nil
	}
	methodName := StringNameToGo(StringNamePtr(unsafe.Pointer(name)))
	id := lookupVirtualID(className, methodName)
	if id == 0 {
		return nil
	}
	return unsafe.Pointer(id)
}

// godotGoCallVirtualWithData dispatches a host virtual invocation to the
// registered Go PtrCall. The userdata pointer is the virtual id returned
// from godotGoGetVirtualCallData; we look up the registered virtual and
// forward args/ret unchanged.
//
//export godotGoCallVirtualWithData
func godotGoCallVirtualWithData(instance C.GDExtensionClassInstancePtr, _ C.GDExtensionConstStringNamePtr,
	virtualUserdata unsafe.Pointer, args *C.GDExtensionConstTypePtr, ret C.GDExtensionTypePtr) {
	rv := lookupVirtual(uintptr(virtualUserdata))
	if rv == nil || rv.def.PtrCall == nil {
		return
	}
	rv.def.PtrCall(unsafe.Pointer(instance), unsafe.Pointer(args), unsafe.Pointer(ret))
}
