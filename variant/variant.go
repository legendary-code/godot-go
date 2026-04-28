package variant

import (
	"sync"
	"unsafe"

	"github.com/legendary-code/godot-go/gdextension"
)

// variantSize is the byte size of a Variant slot under the float_64 build
// config (64-bit pointers, single-precision floats). Sourced from
// extension_api.json#builtin_class_sizes.
const variantSize = 24

// Variant is the opaque Godot Variant slot. Build one via NewVariant or via
// any builtin's ToVariant() method, and release the contents with Destroy.
type Variant [variantSize]byte

// NewVariant returns a zero-initialized Variant (type = NIL). Safe to copy
// while still NIL; once populated, treat as non-copyable and call Destroy.
func NewVariant() *Variant {
	return new(Variant)
}

// Type reports the runtime type tag carried by the Variant.
func (v *Variant) Type() gdextension.VariantType {
	return gdextension.VariantGetType(gdextension.VariantPtr(unsafe.Pointer(v)))
}

// Destroy releases the slot's contents. Safe to call on a zero Variant.
func (v *Variant) Destroy() {
	gdextension.VariantDestroy(gdextension.VariantPtr(unsafe.Pointer(v)))
}

// Lazily-resolved Variant<->Type converters used by the hand-written helpers
// below. Resolved on first call rather than at init() so this file doesn't
// have to register against the init-level system the *.gen.go files use.
var (
	variantFromInt = sync.OnceValue(func() gdextension.VariantFromTypeFunc {
		return gdextension.GetVariantFromTypeConstructor(gdextension.VariantTypeInt)
	})
	variantToInt = sync.OnceValue(func() gdextension.VariantToTypeFunc {
		return gdextension.GetVariantToTypeConstructor(gdextension.VariantTypeInt)
	})
	variantFromString = sync.OnceValue(func() gdextension.VariantFromTypeFunc {
		return gdextension.GetVariantFromTypeConstructor(gdextension.VariantTypeString)
	})
)

// NewVariantInt wraps an int64 in a fresh Variant. The result is owned by
// the caller and must be released with Destroy.
func NewVariantInt(i int64) Variant {
	var out Variant
	src := i
	gdextension.CallVariantFromType(variantFromInt(),
		gdextension.VariantPtr(unsafe.Pointer(&out)),
		gdextension.TypePtr(unsafe.Pointer(&src)))
	return out
}

// NewVariantString wraps a Go string in a fresh Variant (type = STRING). The
// result is owned by the caller and must be released with Destroy.
func NewVariantString(s string) Variant {
	var tmp String
	stringFromGo(&tmp, s)
	defer stringDestroy(&tmp)
	var out Variant
	gdextension.CallVariantFromType(variantFromString(),
		gdextension.VariantPtr(unsafe.Pointer(&out)),
		gdextension.TypePtr(unsafe.Pointer(&tmp)))
	return out
}

// NewVariantBool wraps a Go bool in a fresh Variant (type = BOOL). Result
// must be released with Destroy.
func NewVariantBool(b bool) Variant {
	var out Variant
	src := b
	gdextension.CallVariantFromType(variantFromBool(),
		gdextension.VariantPtr(unsafe.Pointer(&out)),
		gdextension.TypePtr(unsafe.Pointer(&src)))
	return out
}

// NewVariantFloat wraps a Go float64 in a fresh Variant (type = FLOAT).
// Godot's FLOAT is always double-width on the wire regardless of build
// config. Result must be released with Destroy.
func NewVariantFloat(f float64) Variant {
	var out Variant
	src := f
	gdextension.CallVariantFromType(variantFromFloat(),
		gdextension.VariantPtr(unsafe.Pointer(&out)),
		gdextension.TypePtr(unsafe.Pointer(&src)))
	return out
}

// AsInt unwraps an INT-typed Variant. Behavior is undefined if the Variant
// holds any other type — callers that don't know the runtime type should
// inspect Type() first.
func (v *Variant) AsInt() int64 {
	var out int64
	gdextension.CallTypeFromVariant(variantToInt(),
		gdextension.TypePtr(unsafe.Pointer(&out)),
		gdextension.VariantPtr(unsafe.Pointer(v)))
	return out
}
