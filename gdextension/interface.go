package gdextension

/*
#cgo CFLAGS: -I${SRCDIR} -I${SRCDIR}/../godot

#include "shim.h"
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// GodotVersion is the Go-side mirror of GDExtensionGodotVersion2.
type GodotVersion struct {
	Major     uint32
	Minor     uint32
	Patch     uint32
	Hex       uint32
	Status    string
	Build     string
	Hash      string
	Timestamp uint64
	String    string
}

// Short returns a human-readable "vMAJOR.MINOR.PATCH" string.
func (v GodotVersion) Short() string {
	return fmt.Sprintf("v%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// iface caches the host's function-pointer table. Populated once by
// loadInterface during gdextension_library_init; read-only thereafter.
var iface struct {
	getProcAddress C.GDExtensionInterfaceGetProcAddress
	library        C.GDExtensionClassLibraryPtr

	getGodotVersion2 C.GDExtensionInterfaceGetGodotVersion2
	printError       C.GDExtensionInterfacePrintError
	printWarning     C.GDExtensionInterfacePrintWarning
	printScriptError C.GDExtensionInterfacePrintScriptError
	memAlloc2        C.GDExtensionInterfaceMemAlloc2
	memRealloc2      C.GDExtensionInterfaceMemRealloc2
	memFree2         C.GDExtensionInterfaceMemFree2

	variantNewCopy  C.GDExtensionInterfaceVariantNewCopy
	variantDestroy  C.GDExtensionInterfaceVariantDestroy
	variantGetType  C.GDExtensionInterfaceVariantGetType

	getVariantFromTypeConstructor    C.GDExtensionInterfaceGetVariantFromTypeConstructor
	getVariantToTypeConstructor      C.GDExtensionInterfaceGetVariantToTypeConstructor
	variantGetPtrConstructor         C.GDExtensionInterfaceVariantGetPtrConstructor
	variantGetPtrDestructor          C.GDExtensionInterfaceVariantGetPtrDestructor
	variantGetPtrBuiltinMethod       C.GDExtensionInterfaceVariantGetPtrBuiltinMethod
	variantGetPtrOperatorEvaluator   C.GDExtensionInterfaceVariantGetPtrOperatorEvaluator
	variantGetPtrIndexedSetter       C.GDExtensionInterfaceVariantGetPtrIndexedSetter
	variantGetPtrIndexedGetter       C.GDExtensionInterfaceVariantGetPtrIndexedGetter
	variantGetPtrKeyedSetter         C.GDExtensionInterfaceVariantGetPtrKeyedSetter
	variantGetPtrKeyedGetter         C.GDExtensionInterfaceVariantGetPtrKeyedGetter
	variantGetPtrUtilityFunction     C.GDExtensionInterfaceVariantGetPtrUtilityFunction

	stringNewWithUtf8CharsAndLen2     C.GDExtensionInterfaceStringNewWithUtf8CharsAndLen2
	stringToUtf8Chars                 C.GDExtensionInterfaceStringToUtf8Chars
	stringNameNewWithUtf8CharsAndLen  C.GDExtensionInterfaceStringNameNewWithUtf8CharsAndLen

	classdbConstructObject2  C.GDExtensionInterfaceClassdbConstructObject2
	classdbGetMethodBind     C.GDExtensionInterfaceClassdbGetMethodBind
	objectMethodBindPtrcall  C.GDExtensionInterfaceObjectMethodBindPtrcall
	objectMethodBindCall     C.GDExtensionInterfaceObjectMethodBindCall
	objectDestroy            C.GDExtensionInterfaceObjectDestroy
	globalGetSingleton       C.GDExtensionInterfaceGlobalGetSingleton

	classdbRegisterExtensionClass5              C.GDExtensionInterfaceClassdbRegisterExtensionClass5
	classdbRegisterExtensionClassMethod         C.GDExtensionInterfaceClassdbRegisterExtensionClassMethod
	classdbRegisterExtensionClassProperty       C.GDExtensionInterfaceClassdbRegisterExtensionClassProperty
	classdbRegisterExtensionClassPropertyGroup  C.GDExtensionInterfaceClassdbRegisterExtensionClassPropertyGroup
	classdbRegisterExtensionClassPropertySubgroup C.GDExtensionInterfaceClassdbRegisterExtensionClassPropertySubgroup
	classdbRegisterExtensionClassSignal         C.GDExtensionInterfaceClassdbRegisterExtensionClassSignal
	classdbUnregisterExtensionClass             C.GDExtensionInterfaceClassdbUnregisterExtensionClass
	objectSetInstance                           C.GDExtensionInterfaceObjectSetInstance
}

// Library returns the GDExtensionClassLibraryPtr the host handed us. Required
// by every classdb_register_extension_class* call, so generated code reads it.
func Library() LibraryPtr {
	return LibraryPtr(unsafe.Pointer(iface.library))
}

func loadInterface(getProc C.GDExtensionInterfaceGetProcAddress, lib C.GDExtensionClassLibraryPtr) {
	iface.getProcAddress = getProc
	iface.library = lib

	iface.getGodotVersion2 = (C.GDExtensionInterfaceGetGodotVersion2)(unsafe.Pointer(resolveProc("get_godot_version2")))
	iface.printError = (C.GDExtensionInterfacePrintError)(unsafe.Pointer(resolveProc("print_error")))
	iface.printWarning = (C.GDExtensionInterfacePrintWarning)(unsafe.Pointer(resolveProc("print_warning")))
	iface.printScriptError = (C.GDExtensionInterfacePrintScriptError)(unsafe.Pointer(resolveProc("print_script_error")))
	iface.memAlloc2 = (C.GDExtensionInterfaceMemAlloc2)(unsafe.Pointer(resolveProc("mem_alloc2")))
	iface.memRealloc2 = (C.GDExtensionInterfaceMemRealloc2)(unsafe.Pointer(resolveProc("mem_realloc2")))
	iface.memFree2 = (C.GDExtensionInterfaceMemFree2)(unsafe.Pointer(resolveProc("mem_free2")))

	iface.variantNewCopy = (C.GDExtensionInterfaceVariantNewCopy)(unsafe.Pointer(resolveProc("variant_new_copy")))
	iface.variantDestroy = (C.GDExtensionInterfaceVariantDestroy)(unsafe.Pointer(resolveProc("variant_destroy")))
	iface.variantGetType = (C.GDExtensionInterfaceVariantGetType)(unsafe.Pointer(resolveProc("variant_get_type")))

	iface.getVariantFromTypeConstructor = (C.GDExtensionInterfaceGetVariantFromTypeConstructor)(unsafe.Pointer(resolveProc("get_variant_from_type_constructor")))
	iface.getVariantToTypeConstructor = (C.GDExtensionInterfaceGetVariantToTypeConstructor)(unsafe.Pointer(resolveProc("get_variant_to_type_constructor")))
	iface.variantGetPtrConstructor = (C.GDExtensionInterfaceVariantGetPtrConstructor)(unsafe.Pointer(resolveProc("variant_get_ptr_constructor")))
	iface.variantGetPtrDestructor = (C.GDExtensionInterfaceVariantGetPtrDestructor)(unsafe.Pointer(resolveProc("variant_get_ptr_destructor")))
	iface.variantGetPtrBuiltinMethod = (C.GDExtensionInterfaceVariantGetPtrBuiltinMethod)(unsafe.Pointer(resolveProc("variant_get_ptr_builtin_method")))
	iface.variantGetPtrOperatorEvaluator = (C.GDExtensionInterfaceVariantGetPtrOperatorEvaluator)(unsafe.Pointer(resolveProc("variant_get_ptr_operator_evaluator")))
	iface.variantGetPtrIndexedSetter = (C.GDExtensionInterfaceVariantGetPtrIndexedSetter)(unsafe.Pointer(resolveProc("variant_get_ptr_indexed_setter")))
	iface.variantGetPtrIndexedGetter = (C.GDExtensionInterfaceVariantGetPtrIndexedGetter)(unsafe.Pointer(resolveProc("variant_get_ptr_indexed_getter")))
	iface.variantGetPtrKeyedSetter = (C.GDExtensionInterfaceVariantGetPtrKeyedSetter)(unsafe.Pointer(resolveProc("variant_get_ptr_keyed_setter")))
	iface.variantGetPtrKeyedGetter = (C.GDExtensionInterfaceVariantGetPtrKeyedGetter)(unsafe.Pointer(resolveProc("variant_get_ptr_keyed_getter")))
	iface.variantGetPtrUtilityFunction = (C.GDExtensionInterfaceVariantGetPtrUtilityFunction)(unsafe.Pointer(resolveProc("variant_get_ptr_utility_function")))

	iface.stringNewWithUtf8CharsAndLen2 = (C.GDExtensionInterfaceStringNewWithUtf8CharsAndLen2)(unsafe.Pointer(resolveProc("string_new_with_utf8_chars_and_len2")))
	iface.stringToUtf8Chars = (C.GDExtensionInterfaceStringToUtf8Chars)(unsafe.Pointer(resolveProc("string_to_utf8_chars")))
	iface.stringNameNewWithUtf8CharsAndLen = (C.GDExtensionInterfaceStringNameNewWithUtf8CharsAndLen)(unsafe.Pointer(resolveProc("string_name_new_with_utf8_chars_and_len")))

	iface.classdbConstructObject2 = (C.GDExtensionInterfaceClassdbConstructObject2)(unsafe.Pointer(resolveProc("classdb_construct_object2")))
	iface.classdbGetMethodBind = (C.GDExtensionInterfaceClassdbGetMethodBind)(unsafe.Pointer(resolveProc("classdb_get_method_bind")))
	iface.objectMethodBindPtrcall = (C.GDExtensionInterfaceObjectMethodBindPtrcall)(unsafe.Pointer(resolveProc("object_method_bind_ptrcall")))
	iface.objectMethodBindCall = (C.GDExtensionInterfaceObjectMethodBindCall)(unsafe.Pointer(resolveProc("object_method_bind_call")))
	iface.objectDestroy = (C.GDExtensionInterfaceObjectDestroy)(unsafe.Pointer(resolveProc("object_destroy")))
	iface.globalGetSingleton = (C.GDExtensionInterfaceGlobalGetSingleton)(unsafe.Pointer(resolveProc("global_get_singleton")))

	iface.classdbRegisterExtensionClass5 = (C.GDExtensionInterfaceClassdbRegisterExtensionClass5)(unsafe.Pointer(resolveProc("classdb_register_extension_class5")))
	iface.classdbRegisterExtensionClassMethod = (C.GDExtensionInterfaceClassdbRegisterExtensionClassMethod)(unsafe.Pointer(resolveProc("classdb_register_extension_class_method")))
	iface.classdbRegisterExtensionClassProperty = (C.GDExtensionInterfaceClassdbRegisterExtensionClassProperty)(unsafe.Pointer(resolveProc("classdb_register_extension_class_property")))
	iface.classdbRegisterExtensionClassPropertyGroup = (C.GDExtensionInterfaceClassdbRegisterExtensionClassPropertyGroup)(unsafe.Pointer(resolveProc("classdb_register_extension_class_property_group")))
	iface.classdbRegisterExtensionClassPropertySubgroup = (C.GDExtensionInterfaceClassdbRegisterExtensionClassPropertySubgroup)(unsafe.Pointer(resolveProc("classdb_register_extension_class_property_subgroup")))
	iface.classdbRegisterExtensionClassSignal = (C.GDExtensionInterfaceClassdbRegisterExtensionClassSignal)(unsafe.Pointer(resolveProc("classdb_register_extension_class_signal")))
	iface.classdbUnregisterExtensionClass = (C.GDExtensionInterfaceClassdbUnregisterExtensionClass)(unsafe.Pointer(resolveProc("classdb_unregister_extension_class")))
	iface.objectSetInstance = (C.GDExtensionInterfaceObjectSetInstance)(unsafe.Pointer(resolveProc("object_set_instance")))
}

func resolveProc(name string) C.GDExtensionInterfaceFunctionPtr {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))
	return C.godot_go_resolve(iface.getProcAddress, cname)
}

// GetGodotVersion returns the version of the Godot host that loaded this extension.
func GetGodotVersion() GodotVersion {
	var v C.GDExtensionGodotVersion2
	C.godot_go_call_get_godot_version2(iface.getGodotVersion2, &v)
	return GodotVersion{
		Major:     uint32(v.major),
		Minor:     uint32(v.minor),
		Patch:     uint32(v.patch),
		Hex:       uint32(v.hex),
		Status:    C.GoString(C.godot_go_version2_status(&v)),
		Build:     C.GoString(C.godot_go_version2_build(&v)),
		Hash:      C.GoString(C.godot_go_version2_hash(&v)),
		Timestamp: uint64(v.timestamp),
		String:    C.GoString(C.godot_go_version2_string(&v)),
	}
}

// PrintWarning logs a warning through Godot's print_warning interface.
func PrintWarning(desc, fn, file string, line int32, editorNotify bool) {
	if iface.printWarning == nil {
		return
	}
	cdesc := C.CString(desc)
	cfn := C.CString(fn)
	cfile := C.CString(file)
	defer C.free(unsafe.Pointer(cdesc))
	defer C.free(unsafe.Pointer(cfn))
	defer C.free(unsafe.Pointer(cfile))
	C.godot_go_call_print_warning(iface.printWarning, cdesc, cfn, cfile, C.int32_t(line), boolToGD(editorNotify))
}

// PrintError logs an error through Godot's print_error interface.
func PrintError(desc, fn, file string, line int32, editorNotify bool) {
	if iface.printError == nil {
		return
	}
	cdesc := C.CString(desc)
	cfn := C.CString(fn)
	cfile := C.CString(file)
	defer C.free(unsafe.Pointer(cdesc))
	defer C.free(unsafe.Pointer(cfn))
	defer C.free(unsafe.Pointer(cfile))
	C.godot_go_call_print_error(iface.printError, cdesc, cfn, cfile, C.int32_t(line), boolToGD(editorNotify))
}

// PrintScriptError logs a script-language error through print_script_error.
func PrintScriptError(desc, fn, file string, line int32, editorNotify bool) {
	if iface.printScriptError == nil {
		return
	}
	cdesc := C.CString(desc)
	cfn := C.CString(fn)
	cfile := C.CString(file)
	defer C.free(unsafe.Pointer(cdesc))
	defer C.free(unsafe.Pointer(cfn))
	defer C.free(unsafe.Pointer(cfile))
	C.godot_go_call_print_script_error(iface.printScriptError, cdesc, cfn, cfile, C.int32_t(line), boolToGD(editorNotify))
}

// MemAlloc requests a heap allocation from Godot. Pass padAlign=true if you
// need at least 8 bytes of pre-padding (used by ref-counted layouts).
func MemAlloc(size uintptr, padAlign bool) unsafe.Pointer {
	return C.godot_go_call_mem_alloc2(iface.memAlloc2, C.size_t(size), boolToGD(padAlign))
}

// MemRealloc resizes a previously allocated block.
func MemRealloc(ptr unsafe.Pointer, size uintptr, padAlign bool) unsafe.Pointer {
	return C.godot_go_call_mem_realloc2(iface.memRealloc2, ptr, C.size_t(size), boolToGD(padAlign))
}

// MemFree releases a block previously returned by MemAlloc.
func MemFree(ptr unsafe.Pointer, padAlign bool) {
	C.godot_go_call_mem_free2(iface.memFree2, ptr, boolToGD(padAlign))
}

func boolToGD(b bool) C.GDExtensionBool {
	if b {
		return 1
	}
	return 0
}
