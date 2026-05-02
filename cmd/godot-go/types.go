package main

import (
	"fmt"
	"go/ast"
)

// typeInfo describes how a Go primitive maps onto Godot's GDExtension ABI
// for callee-side method args and returns. MVP scope: bool, int / int32 /
// int64, float32 / float64, string. Pointers to engine classes, packed
// arrays, and arbitrary variant types are deferred.
//
// Each func field is a code-gen helper, not a runtime value. The emitter
// invokes them while building per-method Call / PtrCall bodies so the
// generated bindings can read host args, dispatch to the user method, and
// write the return.
//
// `bindings` is the Go alias for the user's bindings package — typically
// the package name in their //go:generate file (e.g. "godot"). The
// emitters prepend `<bindings>.` to the variant-marshalling helpers so
// the generated source compiles against the user's chosen bindings dir
// rather than a framework-internal path.
type typeInfo struct {
	GoType      string // user-facing Go type as it appears in the source signature (e.g. "int32")
	VariantType string // bare gdextension.VariantType const name
	ArgMeta     string // bare gdextension.MethodArgumentMetadata const name

	// EnumName is the bare Go type name (e.g. "Mode") for a user-defined
	// `type X int` declaration tagged with @enum or @bitfield. Empty for
	// primitives and untagged user int aliases. Caller qualifies it as
	// "<MainClass>.<EnumName>" when populating the registration's
	// class_name field — the editor uses that to render typed-enum
	// autocomplete instead of plain int.
	EnumName string

	// ClassName is the unqualified class identity for `*<UserClass>` /
	// `*<EngineClass>` boundaries (Phase 6+ of the array/object work).
	// Unlike EnumName this value goes into ArgClassNames / ReturnClassName
	// directly, with no `<MainClass>.` prefix — Godot's class_name
	// registration is bare for OBJECT-typed slots. Empty for non-class
	// types.
	ClassName string

	PtrCallReadArg     func(bindings string, idx int) string
	PtrCallWriteReturn func(bindings, expr string) string
	CallReadArg        func(bindings string, idx int) string
	CallWriteReturn    func(bindings, expr string) string
	BuildVariant       func(bindings string, idx int, srcExpr string) string
}

// typeTable indexes typeInfo by Go type ident. Keys must match exactly
// what appears in the source AST — `int` is distinct from `int64` even
// though they're often the same width on a 64-bit platform.
var typeTable = map[string]*typeInfo{
	"bool": {
		GoType:      "bool",
		VariantType: "VariantTypeBool",
		ArgMeta:     "ArgMetaNone",
		PtrCallReadArg: func(b string, idx int) string {
			return fmt.Sprintf("arg%d := *(*bool)(gdextension.PtrCallArg(args, %d))", idx, idx)
		},
		PtrCallWriteReturn: func(b, expr string) string {
			return fmt.Sprintf("*(*bool)(ret) = %s", expr)
		},
		CallReadArg: func(b string, idx int) string {
			return fmt.Sprintf("arg%d := %s.VariantAsBool(args[%d])", idx, b, idx)
		},
		CallWriteReturn: func(b, expr string) string {
			return fmt.Sprintf("%s.VariantSetBool(ret, %s)", b, expr)
		},
		BuildVariant: func(b string, idx int, srcExpr string) string {
			return fmt.Sprintf("arg%d := %s.NewVariantBool(%s)", idx, b, srcExpr)
		},
	},
	"int": {
		GoType:      "int",
		VariantType: "VariantTypeInt",
		ArgMeta:     "ArgMetaIntIsInt64",
		PtrCallReadArg: func(b string, idx int) string {
			return fmt.Sprintf("arg%d := int(*(*int64)(gdextension.PtrCallArg(args, %d)))", idx, idx)
		},
		PtrCallWriteReturn: func(b, expr string) string {
			return fmt.Sprintf("*(*int64)(ret) = int64(%s)", expr)
		},
		CallReadArg: func(b string, idx int) string {
			return fmt.Sprintf("arg%d := int(%s.VariantAsInt64(args[%d]))", idx, b, idx)
		},
		CallWriteReturn: func(b, expr string) string {
			return fmt.Sprintf("%s.VariantSetInt64(ret, int64(%s))", b, expr)
		},
		BuildVariant: func(b string, idx int, srcExpr string) string {
			return fmt.Sprintf("arg%d := %s.NewVariantInt(int64(%s))", idx, b, srcExpr)
		},
	},
	"int32": {
		GoType:      "int32",
		VariantType: "VariantTypeInt",
		ArgMeta:     "ArgMetaIntIsInt32",
		PtrCallReadArg: func(b string, idx int) string {
			return fmt.Sprintf("arg%d := int32(*(*int64)(gdextension.PtrCallArg(args, %d)))", idx, idx)
		},
		PtrCallWriteReturn: func(b, expr string) string {
			return fmt.Sprintf("*(*int64)(ret) = int64(%s)", expr)
		},
		CallReadArg: func(b string, idx int) string {
			return fmt.Sprintf("arg%d := int32(%s.VariantAsInt64(args[%d]))", idx, b, idx)
		},
		CallWriteReturn: func(b, expr string) string {
			return fmt.Sprintf("%s.VariantSetInt64(ret, int64(%s))", b, expr)
		},
		BuildVariant: func(b string, idx int, srcExpr string) string {
			return fmt.Sprintf("arg%d := %s.NewVariantInt(int64(%s))", idx, b, srcExpr)
		},
	},
	"int64": {
		GoType:      "int64",
		VariantType: "VariantTypeInt",
		ArgMeta:     "ArgMetaIntIsInt64",
		PtrCallReadArg: func(b string, idx int) string {
			return fmt.Sprintf("arg%d := *(*int64)(gdextension.PtrCallArg(args, %d))", idx, idx)
		},
		PtrCallWriteReturn: func(b, expr string) string {
			return fmt.Sprintf("*(*int64)(ret) = %s", expr)
		},
		CallReadArg: func(b string, idx int) string {
			return fmt.Sprintf("arg%d := %s.VariantAsInt64(args[%d])", idx, b, idx)
		},
		CallWriteReturn: func(b, expr string) string {
			return fmt.Sprintf("%s.VariantSetInt64(ret, %s)", b, expr)
		},
		BuildVariant: func(b string, idx int, srcExpr string) string {
			return fmt.Sprintf("arg%d := %s.NewVariantInt(%s)", idx, b, srcExpr)
		},
	},
	"float32": {
		GoType:      "float32",
		VariantType: "VariantTypeFloat",
		ArgMeta:     "ArgMetaRealIsFloat",
		PtrCallReadArg: func(b string, idx int) string {
			return fmt.Sprintf("arg%d := float32(*(*float64)(gdextension.PtrCallArg(args, %d)))", idx, idx)
		},
		PtrCallWriteReturn: func(b, expr string) string {
			return fmt.Sprintf("*(*float64)(ret) = float64(%s)", expr)
		},
		CallReadArg: func(b string, idx int) string {
			return fmt.Sprintf("arg%d := float32(%s.VariantAsFloat64(args[%d]))", idx, b, idx)
		},
		CallWriteReturn: func(b, expr string) string {
			return fmt.Sprintf("%s.VariantSetFloat64(ret, float64(%s))", b, expr)
		},
		BuildVariant: func(b string, idx int, srcExpr string) string {
			return fmt.Sprintf("arg%d := %s.NewVariantFloat(float64(%s))", idx, b, srcExpr)
		},
	},
	"float64": {
		GoType:      "float64",
		VariantType: "VariantTypeFloat",
		ArgMeta:     "ArgMetaRealIsDouble",
		PtrCallReadArg: func(b string, idx int) string {
			return fmt.Sprintf("arg%d := *(*float64)(gdextension.PtrCallArg(args, %d))", idx, idx)
		},
		PtrCallWriteReturn: func(b, expr string) string {
			return fmt.Sprintf("*(*float64)(ret) = %s", expr)
		},
		CallReadArg: func(b string, idx int) string {
			return fmt.Sprintf("arg%d := %s.VariantAsFloat64(args[%d])", idx, b, idx)
		},
		CallWriteReturn: func(b, expr string) string {
			return fmt.Sprintf("%s.VariantSetFloat64(ret, %s)", b, expr)
		},
		BuildVariant: func(b string, idx int, srcExpr string) string {
			return fmt.Sprintf("arg%d := %s.NewVariantFloat(%s)", idx, b, srcExpr)
		},
	},
	"string": {
		GoType:      "string",
		VariantType: "VariantTypeString",
		ArgMeta:     "ArgMetaNone",
		PtrCallReadArg: func(b string, idx int) string {
			return fmt.Sprintf("arg%d := %s.PtrCallArgString(args, %d)", idx, b, idx)
		},
		PtrCallWriteReturn: func(b, expr string) string {
			return fmt.Sprintf("%s.PtrCallStoreString(ret, %s)", b, expr)
		},
		CallReadArg: func(b string, idx int) string {
			return fmt.Sprintf("arg%d := %s.VariantAsString(args[%d])", idx, b, idx)
		},
		CallWriteReturn: func(b, expr string) string {
			return fmt.Sprintf("%s.VariantSetString(ret, %s)", b, expr)
		},
		BuildVariant: func(b string, idx int, srcExpr string) string {
			return fmt.Sprintf("arg%d := %s.NewVariantString(%s)", idx, b, srcExpr)
		},
	},
}

// resolveType maps an AST type expression to a typeInfo entry. Returns
// (nil, error) for unsupported types — the caller (emit) reports the
// error with file:line context.
//
// User-defined int-backed enums are accepted as if they were int64 over
// the wire: read with a typed cast on the way in, stored as int64 on the
// way out. The Godot ABI carries the typed-enum identity through the
// registration's `class_name` field; resolveType sets EnumName on the
// returned typeInfo when the enum was declared with @enum or @bitfield
// so the caller knows to qualify it. Untagged user int aliases come
// back with EnumName = "" and register as plain int.
//
// Slice types `[]T` are supported when T is a primitive in the slice
// table (sliceCategories below). The codegen path for slices uses
// inline loops with explicit casts at the boundary, calling into the
// bindings' Variant<->Packed<X>Array adapters and per-type
// PushBack / Get methods.
//
// Pointer types `*T` are supported only when T names the file's @class
// — i.e., a same-class self-reference like `func (n *MyNode) Echo(other
// *MyNode) *MyNode`. The marshaling routes through a per-class engine-
// pointer side table (lookup<MainClass>ByEngine) which the emitter
// synthesizes alongside the trampolines. Cross-file user classes and
// engine class pointers are deferred to later phases.
func resolveType(expr ast.Expr, enums map[string]*enumInfo, mainClass string) (*typeInfo, error) {
	switch t := expr.(type) {
	case *ast.Ident:
		if info, ok := typeTable[t.Name]; ok {
			return info, nil
		}
		if e, ok := enums[t.Name]; ok {
			exposedName := ""
			if e.IsExposed {
				exposedName = e.Name
			}
			return enumTypeInfo(t.Name, exposedName), nil
		}
		return nil, fmt.Errorf("unsupported type %q (supported: bool, int, int32, int64, float32, float64, string, or a user @enum-int type declared in this file)", t.Name)
	case *ast.StarExpr:
		// `*T` — only the file's @class is supported as a self-reference.
		// Cross-file user classes and engine class pointers (`*godot.Node`)
		// are deferred to later phases of the array/object work.
		id, ok := t.X.(*ast.Ident)
		if !ok {
			return nil, fmt.Errorf("unsupported pointer type %T (only `*<MainClass>` self-references are supported in this phase)", t.X)
		}
		if mainClass == "" || id.Name != mainClass {
			return nil, fmt.Errorf("unsupported pointer type %q (only `*%s` self-references are supported in this phase; cross-file user classes and engine class pointers are deferred)", id.Name, mainClass)
		}
		return userClassTypeInfo(id.Name), nil
	case *ast.ArrayType:
		if t.Len != nil {
			return nil, fmt.Errorf("fixed-size arrays unsupported (only Go slices `[]T` cross the @class boundary)")
		}
		elem, err := resolveType(t.Elt, enums, mainClass)
		if err != nil {
			return nil, fmt.Errorf("slice element: %w", err)
		}
		return sliceTypeInfo(elem)
	case *ast.Ellipsis:
		// Variadic `...T` parameter — Go-side ergonomic sugar over a
		// slice. At the wire boundary it's identical to []T (single
		// Packed<X>Array / Array[T] arg), so route to the same slice
		// typeInfo. The caller flips IsVariadic on the emitMethod so
		// the dispatch site emits `f(argN...)` rather than `f(argN)`.
		elem, err := resolveType(t.Elt, enums, mainClass)
		if err != nil {
			return nil, fmt.Errorf("variadic element: %w", err)
		}
		return sliceTypeInfo(elem)
	default:
		return nil, fmt.Errorf("unsupported type %T (only primitive, user-enum, slice, variadic, and `*<MainClass>` types are supported)", expr)
	}
}

// userClassTypeInfo builds the marshal fragments for `*<MainClass>` —
// the file's own @class struct used as a method arg or return type.
// The Variant wire form is OBJECT (Phase 3's Object adapters); the
// per-class engine-pointer side table the emitter synthesizes lets us
// recover the *<MainClass> Go pointer from the engine ObjectPtr Godot
// passes us.
//
// Foreign-instance behavior: if the engine ObjectPtr doesn't appear in
// our side table (i.e., it was created by another extension or by
// GDScript without going through our Construct hook), the lookup
// returns nil and the codegen short-circuits — Call returns
// CallErrorInvalidArgument; PtrCall bails without invoking the user
// method (and writes a zero return slot if the method returns
// `*<MainClass>`).
func userClassTypeInfo(name string) *typeInfo {
	lookupName := "lookup" + name + "ByEngine"
	return &typeInfo{
		GoType:      "*" + name,
		VariantType: "VariantTypeObject",
		ArgMeta:     "ArgMetaNone",
		ClassName:   name, // bare class identity in ArgClassNames / ReturnClassName
		CallReadArg: func(b string, idx int) string {
			return fmt.Sprintf(`arg%d := %s(%s.VariantAsObject(args[%d]))
if arg%d == nil {
	return gdextension.CallErrorInvalidArgument
}`, idx, lookupName, b, idx, idx)
		},
		CallWriteReturn: func(b, expr string) string {
			return fmt.Sprintf(`var result_ptr gdextension.ObjectPtr
if %s != nil {
	result_ptr = %s.Ptr()
}
%s.VariantSetObject(ret, result_ptr)`, expr, expr, b)
		},
		PtrCallReadArg: func(b string, idx int) string {
			return fmt.Sprintf(`arg%d := %s(*(*gdextension.ObjectPtr)(gdextension.PtrCallArg(args, %d)))
if arg%d == nil {
	return
}`, idx, lookupName, idx, idx)
		},
		PtrCallWriteReturn: func(b, expr string) string {
			return fmt.Sprintf(`var result_ptr gdextension.ObjectPtr
if %s != nil {
	result_ptr = %s.Ptr()
}
*(*gdextension.ObjectPtr)(ret) = result_ptr`, expr, expr)
		},
		BuildVariant: func(b string, idx int, srcExpr string) string {
			return fmt.Sprintf(`var arg%d_ptr gdextension.ObjectPtr
if %s != nil {
	arg%d_ptr = %s.Ptr()
}
arg%d := %s.NewVariantObject(arg%d_ptr)`, idx, srcExpr, idx, srcExpr, idx, b, idx)
		},
	}
}

// sliceCategory describes how to marshal a `[]T` boundary. The codegen
// inlines the per-element loop using these fragments rather than calling
// the user-facing Make<X>Array / ToSlice helpers from arrayruntime.gen.go,
// because those helpers are strict on the wire-element type — and
// integer/byte slices need explicit narrowing/widening at the boundary.
type sliceCategory struct {
	// PackedTypeName is the bindings type the slice marshals through.
	// "Array" for []bool (TypedArray, no PackedBoolArray exists);
	// "Packed<X>Array" for the rest.
	PackedTypeName string
	// VariantType is the bare gdextension.VariantType const name —
	// "VariantTypeArray" for []bool, "VariantTypePacked<X>Array" otherwise.
	VariantType string
	// UseTypedArray is true for []bool only — codegen routes through
	// MakeArrayOfBools / ToBoolSlice on the bindings' Array type rather
	// than a Packed<X>Array per-element pump.
	UseTypedArray bool
	// CastTo wraps an expression of the user-facing element Go type and
	// returns the wire element expression that PushBack expects. Empty
	// string means no cast needed (identity).
	CastTo string
	// CastFrom wraps an expression of the wire element type and returns
	// the user-facing element expression. Empty string means identity.
	CastFrom string
}

// sliceCategories indexes the slice marshaling metadata by Go element
// type identifier. Float64 is omitted in this phase — under single-
// precision bindings (the framework default), `Packed<X>Array.PushBack`
// takes float32 for both float Packed types, so `[]float64` would
// require a lossy narrowing cast. Tracked as a follow-up to add a
// precision-aware path (see ARRAYS.md).
var sliceCategories = map[string]sliceCategory{
	"bool":    {PackedTypeName: "Array", VariantType: "VariantTypeArray", UseTypedArray: true},
	"byte":    {PackedTypeName: "PackedByteArray", VariantType: "VariantTypePackedByteArray", CastTo: "int64", CastFrom: "byte"},
	"int32":   {PackedTypeName: "PackedInt32Array", VariantType: "VariantTypePackedInt32Array", CastTo: "int64", CastFrom: "int32"},
	"int":     {PackedTypeName: "PackedInt64Array", VariantType: "VariantTypePackedInt64Array", CastTo: "int64", CastFrom: "int"},
	"int64":   {PackedTypeName: "PackedInt64Array", VariantType: "VariantTypePackedInt64Array"},
	"float32": {PackedTypeName: "PackedFloat32Array", VariantType: "VariantTypePackedFloat32Array"},
	"string":  {PackedTypeName: "PackedStringArray", VariantType: "VariantTypePackedStringArray"},
}

// sliceTypeInfo synthesizes the typeInfo for `[]<elem>` from the element
// scalar's typeInfo. The fragments are multi-line: codegen emits a
// per-arg/per-return inline loop into the trampoline body. format.Source
// reformats the rendered source after template execution, so the
// hand-rolled multi-line strings come out gofmt-clean.
//
// Element-type dispatch:
//   - Standard primitive (bool/byte/int*/float32/string): sliceCategories
//     table → Packed<X>Array or Array[bool] (Phase 4).
//   - Tagged user enum (@enum / @bitfield): TypedArray with class_name
//     set, via per-enum helpers the emitter synthesizes alongside the
//     trampolines (Phase 5).
//   - Untagged user int alias: PackedInt64Array with explicit cast at
//     the boundary (Phase 5).
func sliceTypeInfo(elem *typeInfo) (*typeInfo, error) {
	if cat, ok := sliceCategories[elem.GoType]; ok {
		goType := "[]" + elem.GoType
		if cat.UseTypedArray {
			return sliceBoolTypeInfo(goType), nil
		}
		return slicePackedTypeInfo(goType, elem.GoType, cat), nil
	}
	if elem.VariantType == "VariantTypeInt" {
		// User int alias — `int` / `int32` / `int64` were caught above.
		// Both tagged enums (@enum / @bitfield) and untagged user int
		// aliases route through PackedInt64Array. Godot's runtime
		// has no concept of an enum-typed Array — `Array[<EnumName>]`
		// in GDScript is compile-time sugar over `Array[int]`, and
		// `set_typed(TYPE_INT, class_name, ...)` is rejected by Godot
		// ("Class names can only be set for type OBJECT"). Packed is
		// strictly better: typed at the variant level, contiguous
		// storage, and matches how Godot conventionally exposes int
		// slices.
		return userIntSliceTypeInfo(elem), nil
	}
	return nil, fmt.Errorf("unsupported slice element type %q (supported: bool, byte, int, int32, int64, float32, string, or a user int / @enum / @bitfield type declared in this file)", elem.GoType)
}

// userIntSliceTypeInfo builds the marshal fragments for slices of any
// user `type X int` alias — tagged or untagged. Wire form is
// PackedInt64Array; the boundary cast bridges the user's named type
// and int64. Propagates the element's EnumName onto the returned
// typeInfo so the registration's ArgClassNames / ReturnClassName
// carries "<MainClass>.<EnumName>" — the editor's docs panel renders
// the enum identity on the Packed-array param/return even though the
// runtime wire form has no element-level typing of its own.
func userIntSliceTypeInfo(elem *typeInfo) *typeInfo {
	cat := sliceCategory{
		PackedTypeName: "PackedInt64Array",
		VariantType:    "VariantTypePackedInt64Array",
		CastTo:         "int64",      // user-type → wire
		CastFrom:       elem.GoType,  // wire → user-type
	}
	info := slicePackedTypeInfo("[]"+elem.GoType, elem.GoType, cat)
	info.EnumName = elem.EnumName
	return info
}

// sliceBoolTypeInfo builds the marshaling fragments for []bool, which
// rides on Array[bool] (a TypedArray) since Godot has no PackedBoolArray.
// Construction goes through Phase 3's MakeArrayOfBools / ToBoolSlice.
func sliceBoolTypeInfo(goType string) *typeInfo {
	return &typeInfo{
		GoType:      goType,
		VariantType: "VariantTypeArray",
		ArgMeta:     "ArgMetaNone",
		CallReadArg: func(b string, idx int) string {
			return fmt.Sprintf(`arg%d_arr := %s.VariantAsArray(args[%d])
defer arg%d_arr.Destroy()
arg%d := arg%d_arr.ToBoolSlice()`, idx, b, idx, idx, idx, idx)
		},
		CallWriteReturn: func(b, expr string) string {
			return fmt.Sprintf(`result_arr := %s.MakeArrayOfBools(%s...)
%s.VariantSetArray(ret, result_arr)
result_arr.Destroy()`, b, expr, b)
		},
		PtrCallReadArg: func(b string, idx int) string {
			return fmt.Sprintf(`arg%d_arr := *(*%s.Array)(gdextension.PtrCallArg(args, %d))
arg%d := arg%d_arr.ToBoolSlice()`, idx, b, idx, idx, idx)
		},
		PtrCallWriteReturn: func(b, expr string) string {
			return fmt.Sprintf(`result_arr := %s.MakeArrayOfBools(%s...)
*(*%s.Array)(ret) = result_arr`, b, expr, b)
		},
		BuildVariant: func(b string, idx int, srcExpr string) string {
			return fmt.Sprintf(`arg%d_arr := %s.MakeArrayOfBools(%s...)
arg%d := %s.NewVariantArray(arg%d_arr)
arg%d_arr.Destroy()`, idx, b, srcExpr, idx, b, idx, idx)
		},
	}
}

// slicePackedTypeInfo builds the marshaling fragments for []<primitive>
// types that ride on a Packed<X>Array. The codegen inlines a per-element
// loop with optional casts at the boundary so user-facing types like
// `[]int32` / `[]byte` round-trip cleanly through the int64-on-the-wire
// Packed Get/Set methods.
func slicePackedTypeInfo(goType, elemType string, cat sliceCategory) *typeInfo {
	return &typeInfo{
		GoType:      goType,
		VariantType: cat.VariantType,
		ArgMeta:     "ArgMetaNone",
		CallReadArg: func(b string, idx int) string {
			return fmt.Sprintf(`arg%d_arr := %s.VariantAs%s(args[%d])
defer arg%d_arr.Destroy()
arg%d_n := arg%d_arr.Size()
arg%d := make([]%s, arg%d_n)
for i := int64(0); i < arg%d_n; i++ {
	arg%d[i] = %s
}`, idx, b, cat.PackedTypeName, idx,
				idx,
				idx, idx,
				idx, elemType, idx,
				idx,
				idx, sliceCastExpr(cat.CastFrom, fmt.Sprintf("arg%d_arr.Get(i)", idx)))
		},
		CallWriteReturn: func(b, expr string) string {
			return fmt.Sprintf(`result_arr := %s.New%s()
defer result_arr.Destroy()
for _, v := range %s {
	result_arr.PushBack(%s)
}
%s.VariantSet%s(ret, result_arr)`,
				b, cat.PackedTypeName,
				expr,
				sliceCastExpr(cat.CastTo, "v"),
				b, cat.PackedTypeName)
		},
		PtrCallReadArg: func(b string, idx int) string {
			return fmt.Sprintf(`arg%d_arr := *(*%s.%s)(gdextension.PtrCallArg(args, %d))
arg%d_n := arg%d_arr.Size()
arg%d := make([]%s, arg%d_n)
for i := int64(0); i < arg%d_n; i++ {
	arg%d[i] = %s
}`, idx, b, cat.PackedTypeName, idx,
				idx, idx,
				idx, elemType, idx,
				idx,
				idx, sliceCastExpr(cat.CastFrom, fmt.Sprintf("arg%d_arr.Get(i)", idx)))
		},
		PtrCallWriteReturn: func(b, expr string) string {
			return fmt.Sprintf(`result_arr := %s.New%s()
for _, v := range %s {
	result_arr.PushBack(%s)
}
*(*%s.%s)(ret) = result_arr`,
				b, cat.PackedTypeName,
				expr,
				sliceCastExpr(cat.CastTo, "v"),
				b, cat.PackedTypeName)
		},
		BuildVariant: func(b string, idx int, srcExpr string) string {
			return fmt.Sprintf(`arg%d_arr := %s.New%s()
for _, v := range %s {
	arg%d_arr.PushBack(%s)
}
arg%d := %s.NewVariant%s(arg%d_arr)
arg%d_arr.Destroy()`, idx, b, cat.PackedTypeName,
				srcExpr,
				idx, sliceCastExpr(cat.CastTo, "v"),
				idx, b, cat.PackedTypeName, idx,
				idx)
		},
	}
}

// sliceCastExpr applies a narrowing/widening cast wrapper to expr if
// cast is non-empty; otherwise returns expr unchanged.
func sliceCastExpr(cast, expr string) string {
	if cast == "" {
		return expr
	}
	return cast + "(" + expr + ")"
}

// enumTypeInfo builds the marshalling helpers for a user int enum. The
// Godot wire type is int64 (just like `int`), but the user-facing arg /
// return uses the enum's named type so the generated binding compiles
// cleanly against the user's source. exposedName is the bare enum
// identifier when the enum carries @enum / @bitfield (so the caller
// surfaces it as the registration's class_name); empty for untagged
// user int aliases.
func enumTypeInfo(name, exposedName string) *typeInfo {
	return &typeInfo{
		GoType:      name,
		EnumName:    exposedName,
		VariantType: "VariantTypeInt",
		ArgMeta:     "ArgMetaIntIsInt64",
		PtrCallReadArg: func(b string, idx int) string {
			return fmt.Sprintf("arg%d := %s(*(*int64)(gdextension.PtrCallArg(args, %d)))", idx, name, idx)
		},
		PtrCallWriteReturn: func(b, expr string) string {
			return fmt.Sprintf("*(*int64)(ret) = int64(%s)", expr)
		},
		CallReadArg: func(b string, idx int) string {
			return fmt.Sprintf("arg%d := %s(%s.VariantAsInt64(args[%d]))", idx, name, b, idx)
		},
		CallWriteReturn: func(b, expr string) string {
			return fmt.Sprintf("%s.VariantSetInt64(ret, int64(%s))", b, expr)
		},
		BuildVariant: func(b string, idx int, srcExpr string) string {
			return fmt.Sprintf("arg%d := %s.NewVariantInt(int64(%s))", idx, b, srcExpr)
		},
	}
}
