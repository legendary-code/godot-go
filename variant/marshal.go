package variant

import (
	"sync"
	"unsafe"

	"github.com/legendary-code/godot-go/internal/gdextension"
)

// Phase 5d marshalling — helpers used by code generated for user-method
// args / returns. The helpers read from / write to a host-supplied
// VariantPtr (Call ABI) or a host-supplied PtrCall slot directly,
// without spinning up an intermediate `Variant` value on the Go heap.
//
// All helpers go through Godot's variant_to_type / variant_from_type
// constructors. Constructors are resolved lazily on first call so this
// file doesn't have to register against the init-level system.

var (
	variantFromBool = sync.OnceValue(func() gdextension.VariantFromTypeFunc {
		return gdextension.GetVariantFromTypeConstructor(gdextension.VariantTypeBool)
	})
	variantToBool = sync.OnceValue(func() gdextension.VariantToTypeFunc {
		return gdextension.GetVariantToTypeConstructor(gdextension.VariantTypeBool)
	})
	variantFromFloat = sync.OnceValue(func() gdextension.VariantFromTypeFunc {
		return gdextension.GetVariantFromTypeConstructor(gdextension.VariantTypeFloat)
	})
	variantToFloat = sync.OnceValue(func() gdextension.VariantToTypeFunc {
		return gdextension.GetVariantToTypeConstructor(gdextension.VariantTypeFloat)
	})
	variantToString = sync.OnceValue(func() gdextension.VariantToTypeFunc {
		return gdextension.GetVariantToTypeConstructor(gdextension.VariantTypeString)
	})
)

// VariantAsBool reads a BOOL-typed variant slot. Behavior is undefined if
// src holds any other type — generated callee bodies validate types
// upstream of this helper.
func VariantAsBool(src gdextension.VariantPtr) bool {
	var out bool
	gdextension.CallTypeFromVariant(variantToBool(),
		gdextension.TypePtr(unsafe.Pointer(&out)), src)
	return out
}

// VariantSetBool writes b into an uninitialized variant slot dst (type =
// BOOL). The host typically passes ret as an uninitialized slot the
// callee must populate; double-writing or writing into a still-live
// slot is the caller's bug.
func VariantSetBool(dst gdextension.VariantPtr, b bool) {
	src := b
	gdextension.CallVariantFromType(variantFromBool(), dst,
		gdextension.TypePtr(unsafe.Pointer(&src)))
}

// VariantAsInt64 reads an INT-typed variant slot. Mirrors NewVariantInt /
// (*Variant).AsInt but takes the host-style VariantPtr directly so
// generated bindings don't need to round-trip through *Variant.
func VariantAsInt64(src gdextension.VariantPtr) int64 {
	var out int64
	gdextension.CallTypeFromVariant(variantToInt(),
		gdextension.TypePtr(unsafe.Pointer(&out)), src)
	return out
}

// VariantSetInt64 writes i into an uninitialized variant slot dst (type =
// INT).
func VariantSetInt64(dst gdextension.VariantPtr, i int64) {
	src := i
	gdextension.CallVariantFromType(variantFromInt(), dst,
		gdextension.TypePtr(unsafe.Pointer(&src)))
}

// VariantAsFloat64 reads a FLOAT-typed variant slot. Godot's FLOAT is
// always double on the wire regardless of the float_64 / double_64
// build config — this matches engine bindings, which always use float64.
func VariantAsFloat64(src gdextension.VariantPtr) float64 {
	var out float64
	gdextension.CallTypeFromVariant(variantToFloat(),
		gdextension.TypePtr(unsafe.Pointer(&out)), src)
	return out
}

// VariantSetFloat64 writes f into an uninitialized variant slot dst (type
// = FLOAT).
func VariantSetFloat64(dst gdextension.VariantPtr, f float64) {
	src := f
	gdextension.CallVariantFromType(variantFromFloat(), dst,
		gdextension.TypePtr(unsafe.Pointer(&src)))
}

// VariantAsString reads a STRING-typed variant slot and returns its
// contents as a Go string. The intermediate `String` is destroyed
// before return; the host's variant slot is untouched.
func VariantAsString(src gdextension.VariantPtr) string {
	var tmp String
	gdextension.CallTypeFromVariant(variantToString(),
		gdextension.TypePtr(unsafe.Pointer(&tmp)), src)
	defer stringDestroy(&tmp)
	return stringToGo(&tmp)
}

// VariantSetString writes s into an uninitialized variant slot dst (type
// = STRING). The intermediate String is destroyed after the variant
// constructor copies it; the host now owns the variant's storage.
func VariantSetString(dst gdextension.VariantPtr, s string) {
	var tmp String
	stringFromGo(&tmp, s)
	defer stringDestroy(&tmp)
	gdextension.CallVariantFromType(variantFromString(), dst,
		gdextension.TypePtr(unsafe.Pointer(&tmp)))
}

// PtrCallArgString reads the idx-th PtrCall argument as a Go string. The
// host hands the slot as a `*String`; we copy it out to Go memory and
// don't destroy the host's copy (host owns the arg slot).
func PtrCallArgString(args unsafe.Pointer, idx int) string {
	p := (*String)(gdextension.PtrCallArg(args, idx))
	return stringToGo(p)
}

// PtrCallStoreString writes s into a PtrCall return slot. The slot is
// uninitialized on entry; we construct a fresh String there in place by
// having the host's string constructor write directly into ret. Caller
// (Godot) takes ownership of the resulting String.
func PtrCallStoreString(ret unsafe.Pointer, s string) {
	stringFromGo((*String)(ret), s)
}
