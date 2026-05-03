package main

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

// emitArrayRuntime writes <cfg.Dir>/arrayruntime.gen.go, the package-internal
// helpers the codegen tool reaches for when a user method takes or returns
// a slice (Phase 4+ of the array support plan in .claude/ARRAYS.md). Phases
// 1 and 2 in scope:
//
//   - Variant<->typed-array adapters for `Array` and the ten `Packed<X>Array`
//     types (Phase 1).
//   - Go-slice constructors (Make<Name>) and ToSlice methods on each
//     Packed type (Phase 2). `Array` itself isn't in this set — its
//     slice form is a TypedArray, which needs the typed-init constructor
//     (Phase 3). The Make<Name> name is deliberately distinct from the
//     existing zero-arg New<Name> constructor the per-builtin generator
//     emits — variadic and zero-arg can't share a Go function name.
//
// The emitted file lives in the same package as the per-type *.gen.go
// files, so it references their existing `<x>FromType` / `<x>ToType` lazy
// lookups directly without redeclaring them.
func emitArrayRuntime(cfg *genConfig) error {
	data := struct {
		Pkg   string
		Types []arrayRuntimeType
	}{
		Pkg:   cfg.Package,
		Types: buildArrayRuntimeTypes(floatGoType),
	}

	var buf bytes.Buffer
	if err := arrayRuntimeTmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("template: %w", err)
	}
	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		broken := filepath.Join(cfg.Dir, "arrayruntime.gen.go.broken")
		_ = os.WriteFile(broken, buf.Bytes(), 0o644)
		return fmt.Errorf("gofmt: %w (raw at %s)", err, broken)
	}
	out := filepath.Join(cfg.Dir, "arrayruntime.gen.go")
	return os.WriteFile(out, formatted, 0o644)
}

// arrayRuntimeType describes one Godot variant type whose marshaled form is
// an opaque byte array (Array + Packed<X>Array family). Name is the Go-side
// type identifier (matches the per-type *.gen.go file's `type <Name>
// [N]byte` decl); LowerName is the first-letter-lowercased form used to
// build the existing `<lowerName>FromType` / `<lowerName>ToType` lookup
// identifiers we reference from the emitted helpers.
//
// SliceElem / WireElem / ToWire / FromWire describe the slice form for
// Packed types — left empty for `Array` since untyped Array's slice form
// is a TypedArray (Phase 3, separate code path). When SliceElem is set,
// the template emits a `New<Name>(values ...SliceElem) <Name>` constructor
// and a `(p *<Name>) ToSlice() []SliceElem` reader.
//
//   - SliceElem: the user-facing element type ("byte", "int32", "Vector2", …).
//   - WireElem:  what the type's Get/Set actually take ("int64", "Vector2", …).
//     Differs from SliceElem only for the narrow-int / narrow-float Packed
//     types where Godot's wire form is wider than the Go type.
//   - ToWire:    printf-style wrapper applied to a SliceElem expression to
//     produce a WireElem expression. "%s" is identity ("v" stays "v"); for
//     PackedByteArray it's "int64(%s)" so "v" becomes "int64(v)".
//   - FromWire:  printf-style wrapper applied to a WireElem expression to
//     produce a SliceElem expression. Inverse of ToWire.
type arrayRuntimeType struct {
	Name      string
	LowerName string

	SliceElem string
	WireElem  string
	ToWire    string
	FromWire  string
}

// makeVariantOnly returns an arrayRuntimeType that only emits the Phase 1
// variant adapters — used for `Array`, where the slice helpers belong to
// the TypedArray track (Phase 3) instead.
func makeVariantOnly(name string) arrayRuntimeType {
	return arrayRuntimeType{
		Name:      name,
		LowerName: strings.ToLower(name[:1]) + name[1:],
	}
}

// makePackedSlice returns an arrayRuntimeType for a Packed<X>Array type with
// the slice metadata populated. Use the identity form ("%s") for ToWire /
// FromWire when SliceElem and WireElem are the same type (no conversion).
func makePackedSlice(name, sliceElem, wireElem, toWire, fromWire string) arrayRuntimeType {
	return arrayRuntimeType{
		Name:      name,
		LowerName: strings.ToLower(name[:1]) + name[1:],
		SliceElem: sliceElem,
		WireElem:  wireElem,
		ToWire:    toWire,
		FromWire:  fromWire,
	}
}

// buildArrayRuntimeTypes returns the closed list of array-shaped variant
// types we emit helpers for. Mirrors gdextension's VARIANT_TYPE_*ARRAY
// constants. PackedBoolArray doesn't exist in Godot — `[]bool` lands as a
// TypedArray<bool> (Phase 3), not in this list.
//
// Float Packed types (PackedFloat32Array / PackedFloat64Array) both surface
// elements as Godot's `"float"` (real_t) at the ABI boundary, so their
// Go-side element type follows the build config: float32 under single-
// precision builds, float64 under double-precision builds. PackedFloat64Array's
// storage is doubles regardless, but single-precision builds narrow at the
// boundary — the Go signature has to match what the per-type Get/Set
// methods actually take after their own typemap-driven generation.
func buildArrayRuntimeTypes(realT string) []arrayRuntimeType {
	return []arrayRuntimeType{
		makeVariantOnly("Array"),
		// Dictionary lives here despite the file's "arrayruntime" name —
		// the variant-adapter shape is identical (opaque byte struct
		// converted via the per-type FromType / ToType lookup).
		// Slice helpers are skipped (Dictionary has no slice form).
		makeVariantOnly("Dictionary"),
		makePackedSlice("PackedByteArray", "byte", "int64", "int64(%s)", "byte(%s)"),
		makePackedSlice("PackedInt32Array", "int32", "int64", "int64(%s)", "int32(%s)"),
		makePackedSlice("PackedInt64Array", "int64", "int64", "%s", "%s"),
		makePackedSlice("PackedFloat32Array", realT, realT, "%s", "%s"),
		makePackedSlice("PackedFloat64Array", realT, realT, "%s", "%s"),
		makePackedSlice("PackedStringArray", "string", "string", "%s", "%s"),
		makePackedSlice("PackedVector2Array", "Vector2", "Vector2", "%s", "%s"),
		makePackedSlice("PackedVector3Array", "Vector3", "Vector3", "%s", "%s"),
		makePackedSlice("PackedColorArray", "Color", "Color", "%s", "%s"),
		makePackedSlice("PackedVector4Array", "Vector4", "Vector4", "%s", "%s"),
	}
}

var arrayRuntimeTmpl = template.Must(template.New("arrayruntime").Parse(`// Code generated by godot-go-bindgen. DO NOT EDIT.

package {{.Pkg}}

import (
	"unsafe"

	"github.com/legendary-code/godot-go/gdextension"
)

// Variant <-> typed-array adapters and Go-slice converters. One block per
// Array / Packed<X>Array type. Each function references the lazy lookup
// defined in the matching <type>.gen.go file (e.g. arrayFromType in
// array.gen.go), so no new sync.OnceValue declarations live here.
//
// Refcount note: Array and Packed<X>Array are refcounted. Variant<-Type
// goes through the type's copy constructor, which bumps the refcount —
// after VariantSet<X> / NewVariant<X> the source value still owns its
// reference, so callers should call (*<X>).Destroy() on it once they're
// done if it was a temporary they constructed for the conversion.
//
// Make<Packed<X>Array> values are freshly constructed (refcount 1); the
// caller owns them and must call (*<X>).Destroy() once no longer needed
// (typically via defer right after construction).

{{range .Types}}// VariantAs{{.Name}} reads a {{.Name}}-typed variant slot from a host-supplied pointer.
func VariantAs{{.Name}}(src gdextension.VariantPtr) {{.Name}} {
	var out {{.Name}}
	gdextension.CallTypeFromVariant({{.LowerName}}ToType(),
		gdextension.TypePtr(unsafe.Pointer(&out)), src)
	return out
}

// VariantSet{{.Name}} writes a into an uninitialized variant slot dst (type = {{.Name}}).
// Caller still owns a; call (*{{.Name}}).Destroy() on it after if no longer needed.
func VariantSet{{.Name}}(dst gdextension.VariantPtr, a {{.Name}}) {
	gdextension.CallVariantFromType({{.LowerName}}FromType(), dst,
		gdextension.TypePtr(unsafe.Pointer(&a)))
}

// NewVariant{{.Name}} wraps a in a fresh Variant. Release the Variant with Destroy.
// Caller still owns a; call (*{{.Name}}).Destroy() on it after if no longer needed.
func NewVariant{{.Name}}(a {{.Name}}) Variant {
	var out Variant
	gdextension.CallVariantFromType({{.LowerName}}FromType(),
		gdextension.VariantPtr(unsafe.Pointer(&out)),
		gdextension.TypePtr(unsafe.Pointer(&a)))
	return out
}
{{if .SliceElem}}
// Make{{.Name}} constructs a fresh {{.Name}} from a Go slice. Caller owns
// the result; release with (*{{.Name}}).Destroy() when done. Distinct
// from the zero-arg New{{.Name}} constructor since variadic and zero-arg
// can't share a Go function name.
func Make{{.Name}}(values ...{{.SliceElem}}) {{.Name}} {
	var p {{.Name}}
	gdextension.CallPtrConstructor({{.LowerName}}Ctor0(),
		gdextension.TypePtr(unsafe.Pointer(&p)), nil)
	for _, v := range values {
		p.PushBack({{printf .ToWire "v"}})
	}
	return p
}

// ToSlice copies the contents into a fresh Go slice. Returns nil for empty.
func (p *{{.Name}}) ToSlice() []{{.SliceElem}} {
	n := p.Size()
	if n == 0 {
		return nil
	}
	out := make([]{{.SliceElem}}, n)
	for i := int64(0); i < n; i++ {
		out[i] = {{printf .FromWire "p.Get(i)"}}
	}
	return out
}
{{end}}
{{end}}

// makeTypedArray builds an empty Array with the given Variant-type filter
// applied. Class name is used for OBJECT-typed arrays (the engine class to
// filter on) and for INT-typed arrays (the typed-enum class identity).
// The returned Array's refcount is 1; the caller owns it.
func makeTypedArray(typ gdextension.VariantType, className string) Array {
	var base Array
	gdextension.CallPtrConstructor(arrayCtor0(),
		gdextension.TypePtr(unsafe.Pointer(&base)), nil)
	defer (&base).Destroy()
	var script Variant
	return NewArrayBaseTypeClassNameScript(base, int64(typ), className, script)
}

// MakeArrayOfBools constructs Array[bool] from a Go slice. There is no
// PackedBoolArray in Godot, so []bool's wire form is a TypedArray. Caller
// owns the result; release with (*Array).Destroy() when done.
func MakeArrayOfBools(values ...bool) Array {
	a := makeTypedArray(gdextension.VariantTypeBool, "")
	for _, v := range values {
		variant := NewVariantBool(v)
		a.PushBack(variant)
		variant.Destroy()
	}
	return a
}

// ToBoolSlice extracts the bool elements of a typed-bool Array. Returns nil
// for empty. Behavior is undefined if a holds elements of any other type.
func (a *Array) ToBoolSlice() []bool {
	n := a.Size()
	if n == 0 {
		return nil
	}
	out := make([]bool, n)
	for i := int64(0); i < n; i++ {
		v := a.Get(i)
		out[i] = v.AsBool()
		v.Destroy()
	}
	return out
}

// MakeArrayOfInts constructs Array[int] (TypedArray of int64). Caller
// owns the result; release with (*Array).Destroy(). Most users want
// PackedInt64Array (Phase 2's MakePackedInt64Array) instead — it's
// contiguous and faster — but Array[int] is here for callers who need
// the variant-typed form.
//
// Note: Godot rejects setting a class_name on non-OBJECT typed arrays,
// so this helper deliberately does not accept a class_name parameter.
// User enum slices route through PackedInt64Array as well — the
// runtime has no enum-typed Array concept; only Array[Object] with
// a class_name carries an identity (handled by MakeArrayOfObjects).
func MakeArrayOfInts(values ...int64) Array {
	a := makeTypedArray(gdextension.VariantTypeInt, "")
	for _, v := range values {
		variant := NewVariantInt(v)
		a.PushBack(variant)
		variant.Destroy()
	}
	return a
}

// ToInt64Slice extracts int64 elements of a typed-int Array. Returns nil
// for empty. Used by codegen for both plain typed-int arrays and
// []<UserEnum> arrays (codegen casts each int64 to the user's enum type).
func (a *Array) ToInt64Slice() []int64 {
	n := a.Size()
	if n == 0 {
		return nil
	}
	out := make([]int64, n)
	for i := int64(0); i < n; i++ {
		v := a.Get(i)
		out[i] = v.AsInt()
		v.Destroy()
	}
	return out
}

// MakeArrayOfObjects constructs Array[<className>] from a slice of raw
// ObjectPtrs. Codegen calls this when the user method takes/returns
// []*<EngineClass> or []*<UserClass>; the per-class wrapper's Ptr() method
// supplies each element. Caller owns the result.
func MakeArrayOfObjects(className string, ptrs ...gdextension.ObjectPtr) Array {
	a := makeTypedArray(gdextension.VariantTypeObject, className)
	for _, p := range ptrs {
		variant := NewVariantObject(p)
		a.PushBack(variant)
		variant.Destroy()
	}
	return a
}

// ToObjectSlice extracts ObjectPtrs from a typed-Object Array. Returns nil
// for empty. Codegen reconstructs *<EngineClass> / *<UserClass> from each
// pointer via the per-class wrapper or side-table lookup.
func (a *Array) ToObjectSlice() []gdextension.ObjectPtr {
	n := a.Size()
	if n == 0 {
		return nil
	}
	out := make([]gdextension.ObjectPtr, n)
	for i := int64(0); i < n; i++ {
		v := a.Get(i)
		out[i] = v.AsObject()
		v.Destroy()
	}
	return out
}
`))
