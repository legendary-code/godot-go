package gdextension

/*
#cgo CFLAGS: -I${SRCDIR} -I${SRCDIR}/../../godot

#include "shim.h"
*/
import "C"

import (
	"runtime"
	"unsafe"
)

// VariantNewCopy clones src into dst. dst must point to an uninitialized
// Variant slot of size variant_size (16 bytes in 32-bit float builds, 24 in
// 64-bit float builds — sized by extension_api.json#builtin_class_sizes).
func VariantNewCopy(dst, src VariantPtr) {
	C.godot_go_call_variant_new_copy(iface.variantNewCopy,
		C.GDExtensionUninitializedVariantPtr(dst),
		C.GDExtensionConstVariantPtr(src))
}

// VariantDestroy releases an initialized Variant slot. The backing memory is
// the caller's responsibility; this only runs the value's destructor.
func VariantDestroy(self VariantPtr) {
	C.godot_go_call_variant_destroy(iface.variantDestroy, C.GDExtensionVariantPtr(self))
}

// VariantGetType returns the runtime type tag carried by a Variant slot.
func VariantGetType(self VariantPtr) VariantType {
	return VariantType(C.godot_go_call_variant_get_type(iface.variantGetType, C.GDExtensionConstVariantPtr(self)))
}

// GetPtrConstructor resolves the constructor with index `ctor` for the given
// builtin type. Constructor indices are stable per Godot version and are
// consumed from extension_api.json#builtin_classes[*].constructors.
func GetPtrConstructor(t VariantType, ctor int32) PtrConstructor {
	fn := C.godot_go_call_variant_get_ptr_constructor(iface.variantGetPtrConstructor,
		C.GDExtensionVariantType(t), C.int32_t(ctor))
	return PtrConstructor(unsafe.Pointer(fn))
}

// GetPtrDestructor resolves the destructor for the given builtin type. Some
// builtin types (e.g. plain numeric/struct values) have no destructor — the
// host returns nil for those.
func GetPtrDestructor(t VariantType) PtrDestructor {
	fn := C.godot_go_call_variant_get_ptr_destructor(iface.variantGetPtrDestructor, C.GDExtensionVariantType(t))
	return PtrDestructor(unsafe.Pointer(fn))
}

// GetPtrBuiltinMethod resolves a builtin-class method by name and consistency
// hash. The hash is generated from extension_api.json#builtin_classes[*].methods[*].hash
// and lets the host detect codegen drift across Godot versions.
func GetPtrBuiltinMethod(t VariantType, method StringNamePtr, hash int64) PtrBuiltInMethod {
	fn := C.godot_go_call_variant_get_ptr_builtin_method(iface.variantGetPtrBuiltinMethod,
		C.GDExtensionVariantType(t), C.GDExtensionConstStringNamePtr(method), C.GDExtensionInt(hash))
	return PtrBuiltInMethod(unsafe.Pointer(fn))
}

// GetPtrOperatorEvaluator resolves an operator implementation specialized for
// the (left, right) type pair.
func GetPtrOperatorEvaluator(op VariantOperator, a, b VariantType) PtrOperatorEvaluator {
	fn := C.godot_go_call_variant_get_ptr_operator_evaluator(iface.variantGetPtrOperatorEvaluator,
		C.GDExtensionVariantOperator(op), C.GDExtensionVariantType(a), C.GDExtensionVariantType(b))
	return PtrOperatorEvaluator(unsafe.Pointer(fn))
}

// GetPtrIndexedSetter resolves the indexed-write hook for an indexable builtin
// type (e.g. Array, Packed*Array, Vector{2,3,4}).
func GetPtrIndexedSetter(t VariantType) PtrIndexedSetter {
	fn := C.godot_go_call_variant_get_ptr_indexed_setter(iface.variantGetPtrIndexedSetter, C.GDExtensionVariantType(t))
	return PtrIndexedSetter(unsafe.Pointer(fn))
}

// GetPtrIndexedGetter resolves the indexed-read hook.
func GetPtrIndexedGetter(t VariantType) PtrIndexedGetter {
	fn := C.godot_go_call_variant_get_ptr_indexed_getter(iface.variantGetPtrIndexedGetter, C.GDExtensionVariantType(t))
	return PtrIndexedGetter(unsafe.Pointer(fn))
}

// GetPtrKeyedSetter resolves the keyed-write hook (Dictionary, Object).
func GetPtrKeyedSetter(t VariantType) PtrKeyedSetter {
	fn := C.godot_go_call_variant_get_ptr_keyed_setter(iface.variantGetPtrKeyedSetter, C.GDExtensionVariantType(t))
	return PtrKeyedSetter(unsafe.Pointer(fn))
}

// GetPtrKeyedGetter resolves the keyed-read hook.
func GetPtrKeyedGetter(t VariantType) PtrKeyedGetter {
	fn := C.godot_go_call_variant_get_ptr_keyed_getter(iface.variantGetPtrKeyedGetter, C.GDExtensionVariantType(t))
	return PtrKeyedGetter(unsafe.Pointer(fn))
}

// GetPtrUtilityFunction resolves a utility function (extension_api.json#utility_functions).
func GetPtrUtilityFunction(name StringNamePtr, hash int64) PtrUtilityFunction {
	fn := C.godot_go_call_variant_get_ptr_utility_function(iface.variantGetPtrUtilityFunction,
		C.GDExtensionConstStringNamePtr(name), C.GDExtensionInt(hash))
	return PtrUtilityFunction(unsafe.Pointer(fn))
}

// GetVariantFromTypeConstructor resolves the function that wraps a typed
// value in a Variant slot.
func GetVariantFromTypeConstructor(t VariantType) VariantFromTypeFunc {
	fn := C.godot_go_call_get_variant_from_type_constructor(iface.getVariantFromTypeConstructor, C.GDExtensionVariantType(t))
	return VariantFromTypeFunc(unsafe.Pointer(fn))
}

// GetVariantToTypeConstructor resolves the function that unwraps a Variant
// slot into a typed value.
func GetVariantToTypeConstructor(t VariantType) VariantToTypeFunc {
	fn := C.godot_go_call_get_variant_to_type_constructor(iface.getVariantToTypeConstructor, C.GDExtensionVariantType(t))
	return VariantToTypeFunc(unsafe.Pointer(fn))
}

// CallPtrConstructor invokes a previously resolved PtrConstructor. base must
// point to an uninitialized buffer of the constructed type's size; args may
// be nil for nullary constructors.
func CallPtrConstructor(fn PtrConstructor, base TypePtr, args []TypePtr) {
	pinner := pinArgs(args)
	defer pinner.Unpin()
	C.godot_go_call_ptr_constructor(C.godot_go_anyfn(unsafe.Pointer(fn)),
		C.GDExtensionUninitializedTypePtr(base), typeArgArray(args))
}

// CallPtrDestructor invokes a previously resolved PtrDestructor. Pass nil-fn
// safely; the call is a no-op for trivially-destructible types where the
// host returned no destructor.
func CallPtrDestructor(fn PtrDestructor, base TypePtr) {
	if fn == nil {
		return
	}
	C.godot_go_call_ptr_destructor(C.godot_go_anyfn(unsafe.Pointer(fn)), C.GDExtensionTypePtr(base))
}

// CallPtrBuiltinMethod invokes a previously resolved builtin-class method.
// `base` is the receiver, `args` are the typed argument pointers, `ret`
// receives the return value (pass nil for `void` returns).
func CallPtrBuiltinMethod(fn PtrBuiltInMethod, base TypePtr, args []TypePtr, ret TypePtr) {
	pinner := pinArgs(args)
	defer pinner.Unpin()
	C.godot_go_call_ptr_builtin_method(C.godot_go_anyfn(unsafe.Pointer(fn)),
		C.GDExtensionTypePtr(base), typeArgArray(args), C.GDExtensionTypePtr(ret), C.int32_t(len(args)))
}

// CallPtrOperatorEvaluator invokes a previously resolved operator.
func CallPtrOperatorEvaluator(fn PtrOperatorEvaluator, left, right, result TypePtr) {
	C.godot_go_call_ptr_operator_evaluator(C.godot_go_anyfn(unsafe.Pointer(fn)),
		C.GDExtensionConstTypePtr(left), C.GDExtensionConstTypePtr(right), C.GDExtensionTypePtr(result))
}

// CallPtrIndexedSetter writes value into base[index].
func CallPtrIndexedSetter(fn PtrIndexedSetter, base TypePtr, index int64, value TypePtr) {
	C.godot_go_call_ptr_indexed_setter(C.godot_go_anyfn(unsafe.Pointer(fn)),
		C.GDExtensionTypePtr(base), C.GDExtensionInt(index), C.GDExtensionConstTypePtr(value))
}

// CallPtrIndexedGetter reads base[index] into value.
func CallPtrIndexedGetter(fn PtrIndexedGetter, base TypePtr, index int64, value TypePtr) {
	C.godot_go_call_ptr_indexed_getter(C.godot_go_anyfn(unsafe.Pointer(fn)),
		C.GDExtensionConstTypePtr(base), C.GDExtensionInt(index), C.GDExtensionTypePtr(value))
}

// CallPtrKeyedSetter writes value into base[key].
func CallPtrKeyedSetter(fn PtrKeyedSetter, base TypePtr, key, value TypePtr) {
	C.godot_go_call_ptr_keyed_setter(C.godot_go_anyfn(unsafe.Pointer(fn)),
		C.GDExtensionTypePtr(base), C.GDExtensionConstTypePtr(key), C.GDExtensionConstTypePtr(value))
}

// CallPtrKeyedGetter reads base[key] into value.
func CallPtrKeyedGetter(fn PtrKeyedGetter, base TypePtr, key, value TypePtr) {
	C.godot_go_call_ptr_keyed_getter(C.godot_go_anyfn(unsafe.Pointer(fn)),
		C.GDExtensionConstTypePtr(base), C.GDExtensionConstTypePtr(key), C.GDExtensionTypePtr(value))
}

// CallPtrUtilityFunction invokes a free utility function. ret may be nil.
func CallPtrUtilityFunction(fn PtrUtilityFunction, ret TypePtr, args []TypePtr) {
	pinner := pinArgs(args)
	defer pinner.Unpin()
	C.godot_go_call_ptr_utility_function(C.godot_go_anyfn(unsafe.Pointer(fn)),
		C.GDExtensionTypePtr(ret), typeArgArray(args), C.int32_t(len(args)))
}

// CallVariantFromType wraps src (a typed value) into the uninitialized
// Variant slot dst.
func CallVariantFromType(fn VariantFromTypeFunc, dst VariantPtr, src TypePtr) {
	C.godot_go_call_variant_from_type(C.godot_go_anyfn(unsafe.Pointer(fn)),
		C.GDExtensionUninitializedVariantPtr(dst), C.GDExtensionTypePtr(src))
}

// CallTypeFromVariant unwraps src (a Variant slot) into the uninitialized
// typed value dst.
func CallTypeFromVariant(fn VariantToTypeFunc, dst TypePtr, src VariantPtr) {
	C.godot_go_call_type_from_variant(C.godot_go_anyfn(unsafe.Pointer(fn)),
		C.GDExtensionUninitializedTypePtr(dst), C.GDExtensionVariantPtr(src))
}

// StringNewWithUtf8 fills the uninitialized String slot dst with the contents
// of s, encoded as UTF-8. The slot must be sized per builtin_class_sizes for
// String. Returns the parsed length (Godot reports a non-zero error code on
// invalid UTF-8 via the underlying interface — callers can usually ignore
// the return for trusted Go strings).
func StringNewWithUtf8(dst StringPtr, s string) {
	if len(s) == 0 {
		C.godot_go_call_string_new_with_utf8_chars_and_len2(iface.stringNewWithUtf8CharsAndLen2,
			C.GDExtensionUninitializedStringPtr(dst), nil, 0)
		return
	}
	C.godot_go_call_string_new_with_utf8_chars_and_len2(iface.stringNewWithUtf8CharsAndLen2,
		C.GDExtensionUninitializedStringPtr(dst),
		(*C.char)(unsafe.Pointer(unsafe.StringData(s))),
		C.GDExtensionInt(len(s)))
}

// StringToGo extracts the contents of a String slot as a Go string.
// Two-pass: first call returns the UTF-8 byte length; second fills the
// allocated buffer.
func StringToGo(self StringPtr) string {
	n := int64(C.godot_go_call_string_to_utf8_chars(iface.stringToUtf8Chars,
		C.GDExtensionConstStringPtr(self), nil, 0))
	if n <= 0 {
		return ""
	}
	buf := make([]byte, n)
	C.godot_go_call_string_to_utf8_chars(iface.stringToUtf8Chars,
		C.GDExtensionConstStringPtr(self),
		(*C.char)(unsafe.Pointer(&buf[0])),
		C.GDExtensionInt(n))
	return unsafe.String(&buf[0], n)
}

// StringNameNewWithUtf8 fills an uninitialized StringName slot from a Go
// string. StringName de-duplicates via the host's intern table.
func StringNameNewWithUtf8(dst StringNamePtr, s string) {
	if len(s) == 0 {
		C.godot_go_call_string_name_new_with_utf8_chars_and_len(iface.stringNameNewWithUtf8CharsAndLen,
			C.GDExtensionUninitializedStringNamePtr(dst), nil, 0)
		return
	}
	C.godot_go_call_string_name_new_with_utf8_chars_and_len(iface.stringNameNewWithUtf8CharsAndLen,
		C.GDExtensionUninitializedStringNamePtr(dst),
		(*C.char)(unsafe.Pointer(unsafe.StringData(s))),
		C.GDExtensionInt(len(s)))
}

// typeArgArray returns the C-side argv pointer for a slice of TypePtr, or
// nil for an empty slice. The slice's backing array must remain reachable
// for the duration of the call (cgo pins it). Note: cgo strips the const
// qualifier from `const GDExtensionConstTypePtr *`, so the parameter type
// is the single-asterisk `*GDExtensionConstTypePtr` here.
func typeArgArray(args []TypePtr) *C.GDExtensionConstTypePtr {
	if len(args) == 0 {
		return nil
	}
	return (*C.GDExtensionConstTypePtr)(unsafe.Pointer(&args[0]))
}

// pinArgs pins each non-nil argument target so the cgo runtime check accepts
// the args slice — passing a Go-allocated array of Go pointers would otherwise
// trip the "Go pointer to unpinned Go pointer" guard. Caller must Unpin().
func pinArgs(args []TypePtr) *runtime.Pinner {
	var pinner runtime.Pinner
	for _, a := range args {
		if a != nil {
			pinner.Pin(a)
		}
	}
	return &pinner
}
