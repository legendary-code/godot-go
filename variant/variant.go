package variant

import (
	"unsafe"

	"github.com/legendary-code/godot-go/internal/gdextension"
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
