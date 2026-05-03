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

	// IsEngineClass distinguishes Phase 6c engine-class boundaries
	// (`*<bindings>.<Class>` and slice variants) from Phase 6a/b
	// user-class boundaries. The two paths share the OBJECT wire form
	// but differ on wrapper construction: engine classes wrap
	// borrowed-view via &<bindings>.<Class>{} + BindPtr, while user
	// classes go through the per-class engine-pointer side table.
	IsEngineClass bool

	// HintEnum is set on slice-of-tagged-enum typeInfos so the codegen
	// can build a PROPERTY_HINT_TYPE_STRING hint string at registration
	// time. Setting class_name on a non-OBJECT typed return confuses
	// GDScript's autocomplete (it reads class_name as the return type),
	// so element-level enum identity for slices flows through the hint
	// channel instead — Godot's canonical mechanism. The bare enum
	// name lets emit.go look up the value list for the hint payload.
	HintEnum string

	PtrCallReadArg     func(bindings string, idx int) string
	PtrCallWriteReturn func(bindings, expr string) string
	CallReadArg        func(bindings string, idx int) string
	CallWriteReturn    func(bindings, expr string) string
	BuildVariant       func(bindings string, idx int, srcExpr string) string

	// WrapVariant returns an *expression* (no statement, no leading
	// `name :=`) that constructs a fresh `<bindings>.Variant` from
	// srcExpr (already the user-facing Go-side type). Used by map
	// codegen to wrap each map key and value into a Variant before
	// handing them to Dictionary.Set. Caller owns the resulting Variant
	// and must call .Destroy() once done.
	WrapVariant func(bindings, srcExpr string) string

	// UnwrapVariant returns an expression that converts varExpr (a
	// local *value* of type `<bindings>.Variant`) into the user-facing
	// Go-side type. Used by map codegen to extract the typed value from
	// a Variant returned by Array.Get / Dictionary.Get. The expression
	// references gdextension and unsafe to take the address; callers
	// must ensure varExpr is a plain identifier (not a complex
	// expression) so `&varExpr` is valid Go.
	UnwrapVariant func(bindings, varExpr string) string
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
		WrapVariant: func(b, expr string) string {
			return fmt.Sprintf("%s.NewVariantBool(%s)", b, expr)
		},
		UnwrapVariant: func(b, varExpr string) string {
			return fmt.Sprintf("%s.VariantAsBool(gdextension.VariantPtr(unsafe.Pointer(&%s)))", b, varExpr)
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
		WrapVariant: func(b, expr string) string {
			return fmt.Sprintf("%s.NewVariantInt(int64(%s))", b, expr)
		},
		UnwrapVariant: func(b, varExpr string) string {
			return fmt.Sprintf("int(%s.VariantAsInt64(gdextension.VariantPtr(unsafe.Pointer(&%s))))", b, varExpr)
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
		WrapVariant: func(b, expr string) string {
			return fmt.Sprintf("%s.NewVariantInt(int64(%s))", b, expr)
		},
		UnwrapVariant: func(b, varExpr string) string {
			return fmt.Sprintf("int32(%s.VariantAsInt64(gdextension.VariantPtr(unsafe.Pointer(&%s))))", b, varExpr)
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
		WrapVariant: func(b, expr string) string {
			return fmt.Sprintf("%s.NewVariantInt(%s)", b, expr)
		},
		UnwrapVariant: func(b, varExpr string) string {
			return fmt.Sprintf("%s.VariantAsInt64(gdextension.VariantPtr(unsafe.Pointer(&%s)))", b, varExpr)
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
		WrapVariant: func(b, expr string) string {
			return fmt.Sprintf("%s.NewVariantFloat(float64(%s))", b, expr)
		},
		UnwrapVariant: func(b, varExpr string) string {
			return fmt.Sprintf("float32(%s.VariantAsFloat64(gdextension.VariantPtr(unsafe.Pointer(&%s))))", b, varExpr)
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
		WrapVariant: func(b, expr string) string {
			return fmt.Sprintf("%s.NewVariantFloat(%s)", b, expr)
		},
		UnwrapVariant: func(b, varExpr string) string {
			return fmt.Sprintf("%s.VariantAsFloat64(gdextension.VariantPtr(unsafe.Pointer(&%s)))", b, varExpr)
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
		WrapVariant: func(b, expr string) string {
			return fmt.Sprintf("%s.NewVariantString(%s)", b, expr)
		},
		UnwrapVariant: func(b, varExpr string) string {
			return fmt.Sprintf("%s.VariantAsString(gdextension.VariantPtr(unsafe.Pointer(&%s)))", b, varExpr)
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
// Pointer types `*T` are supported in two same-package shapes:
//
//   - T names the file's @class — a self-reference like `func (n
//     *MyNode) Echo(other *MyNode) *MyNode`.
//   - T names another @class declared in a sibling file in the same
//     package — a cross-file user-class reference, e.g. `func (n
//     *Words) SampleLetter(rng *Rand)`.
//
// Both route through the per-class engine-pointer side table
// (`lookup<Class>ByEngine`) the emitter synthesizes for every @class
// in the package. Cross-package user classes are still out of scope
// (the side table only exists in the file each class is generated
// alongside, which today is the same package's bindings.gen.go).
func resolveType(expr ast.Expr, enums map[string]*enumInfo, userClasses map[string]bool, mainClass, bindings string) (*typeInfo, error) {
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
			info := enumTypeInfo(t.Name, exposedName)
			// Cross-file enum: the owning @class isn't this file's
			// mainClass. Pre-qualify ClassName so classIdentity uses
			// the right "<OwningClass>.<EnumName>" qualifier rather
			// than the current file's class. For same-file refs
			// (OwningClass == mainClass or empty) the existing
			// qualifyEnum path produces the same string, so leaving
			// ClassName empty is fine.
			if exposedName != "" && e.OwningClass != "" && e.OwningClass != mainClass {
				info.ClassName = e.OwningClass + "." + e.Name
				info.EnumName = "" // already encoded in ClassName; don't double-qualify
			}
			return info, nil
		}
		return nil, fmt.Errorf("unsupported type %q (supported: bool, int, int32, int64, float32, float64, string, or a user @enum-int type declared in this package)", t.Name)
	case *ast.StarExpr:
		// `*T` — three cases:
		//   - *ast.Ident naming the file's @class or any @class declared
		//     in a sibling file of the same package: user-class
		//     pointer. Routes through that class's side table.
		//   - *ast.SelectorExpr where the package alias matches the
		//     bindings import: engine-class pointer (Phase 6c). Codegen
		//     wraps borrowed-view-style — no refcount handling. RefCounted-
		//     derived classes still work but the user owns lifecycle.
		//   - Anything else: rejected.
		switch x := t.X.(type) {
		case *ast.Ident:
			if x.Name == mainClass || userClasses[x.Name] {
				return userClassTypeInfo(x.Name), nil
			}
			return nil, fmt.Errorf("unsupported pointer type %q (only @class structs from this package are recognized as user-class pointers; cross-package user classes go through the bindings package alias as `*<bindings>.<Class>`)", x.Name)
		case *ast.SelectorExpr:
			pkg, ok := x.X.(*ast.Ident)
			if !ok {
				return nil, fmt.Errorf("unsupported pointer selector %T (engine class pointers must take the bare `<bindings>.<Class>` form)", x.X)
			}
			if bindings == "" || pkg.Name != bindings {
				return nil, fmt.Errorf("unsupported pointer type %q.%q (only the bindings package alias %q is recognized for engine class pointers; cross-package types must be vendored through the bindings)", pkg.Name, x.Sel.Name, bindings)
			}
			return engineClassTypeInfo(bindings, x.Sel.Name), nil
		default:
			return nil, fmt.Errorf("unsupported pointer type %T (only `*<MainClass>` and `*<bindings>.<EngineClass>` are supported)", t.X)
		}
	case *ast.ArrayType:
		if t.Len != nil {
			return nil, fmt.Errorf("fixed-size arrays unsupported (only Go slices `[]T` cross the @class boundary)")
		}
		if _, nested := t.Elt.(*ast.ArrayType); nested {
			return nil, fmt.Errorf("nested slices `[][]T` are not supported at the @class boundary")
		}
		elem, err := resolveType(t.Elt, enums, userClasses, mainClass, bindings)
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
		if _, nested := t.Elt.(*ast.ArrayType); nested {
			return nil, fmt.Errorf("nested slices `...[]T` are not supported at the @class boundary")
		}
		elem, err := resolveType(t.Elt, enums, userClasses, mainClass, bindings)
		if err != nil {
			return nil, fmt.Errorf("variadic element: %w", err)
		}
		return sliceTypeInfo(elem)
	case *ast.MapType:
		key, err := resolveType(t.Key, enums, userClasses, mainClass, bindings)
		if err != nil {
			return nil, fmt.Errorf("map key: %w", err)
		}
		value, err := resolveType(t.Value, enums, userClasses, mainClass, bindings)
		if err != nil {
			return nil, fmt.Errorf("map value: %w", err)
		}
		return mapTypeInfo(key, value)
	case *ast.FuncType:
		return nil, fmt.Errorf("function types are not supported at the @class boundary (use @signals for callback contracts)")
	case *ast.ChanType:
		return nil, fmt.Errorf("channel types are not supported at the @class boundary")
	case *ast.InterfaceType:
		return nil, fmt.Errorf("interface types are not supported at the @class boundary (only @signals interfaces are recognized, declared at file scope)")
	case *ast.SelectorExpr:
		// Bare cross-package type used as a method arg / return — like
		// `godot.Variant` or `godot.Vector2`. Today only the pointer
		// form `*<bindings>.<EngineClass>` is recognized for cross-
		// package types (Phase 6c). Surface a clear error rather than
		// hitting the generic default.
		pkgName := "<unknown>"
		if id, ok := t.X.(*ast.Ident); ok {
			pkgName = id.Name
		}
		return nil, fmt.Errorf("bare cross-package type %s.%s is not supported at the @class boundary (cross-package types are only recognized via pointer — e.g. *%s.<EngineClass> — wrap or qualify accordingly)", pkgName, t.Sel.Name, bindings)
	default:
		return nil, fmt.Errorf("unsupported type %T (only primitive, user-enum, slice, variadic, and pointer-to-class types are supported)", expr)
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
	if elem.VariantType == "VariantTypeObject" && elem.ClassName != "" {
		// `[]*<UserClass>` (Phase 6b) or `[]*<bindings>.<EngineClass>`
		// (Phase 6c). Both ride on Array[<Class>] (TypedArray with
		// class_name set — the one filtered Array shape Godot supports).
		// Element handling differs: user classes route through the
		// engine-pointer side table (foreign-instance check); engine
		// classes wrap borrowed-view via &<bindings>.<Class>{} + BindPtr
		// (no foreign-instance concept — every ObjectPtr is "valid"
		// from the framework's perspective).
		if elem.IsEngineClass {
			return engineClassSliceTypeInfo(elem), nil
		}
		return userClassSliceTypeInfo(elem), nil
	}
	return nil, fmt.Errorf("unsupported slice element type %q (supported: bool, byte, int, int32, int64, float32, string, *<MainClass>, or a user int / @enum / @bitfield type declared in this file)", elem.GoType)
}

// engineClassTypeInfo builds the marshal fragments for `*<bindings>.<EngineClass>`
// — Phase 6c borrowed-view semantics. Wrapper construction is inline:
// `&<bindings>.<Class>{}` + `.BindPtr(p)` for any engine class, no
// inheritance-chain knowledge required. nil engine ObjectPtr maps to a
// nil Go pointer (so user methods can branch on `if other == nil`).
//
// Lifecycle caveat for RefCounted-derived classes (Resource, Image,
// RegEx, …): the wrapper is a borrowed view. Godot's owner — the
// caller of our method — manages the underlying refcount; if the user
// retains the wrapper past the call (stores on the struct, captures
// in a goroutine), they must call `Reference()` themselves to extend
// the lifetime. This is the documented limitation of Phase 6c.
func engineClassTypeInfo(bindings, className string) *typeInfo {
	return &typeInfo{
		GoType:        "*" + bindings + "." + className,
		VariantType:   "VariantTypeObject",
		ArgMeta:       "ArgMetaNone",
		ClassName:     className,
		IsEngineClass: true,
		CallReadArg: func(b string, idx int) string {
			return fmt.Sprintf(`arg%d_ptr := %s.VariantAsObject(args[%d])
var arg%d *%s.%s
if arg%d_ptr != nil {
	arg%d = &%s.%s{}
	arg%d.BindPtr(arg%d_ptr)
}`, idx, b, idx, idx, b, className, idx, idx, b, className, idx, idx)
		},
		CallWriteReturn: func(b, expr string) string {
			return fmt.Sprintf(`var result_ptr gdextension.ObjectPtr
if %s != nil {
	result_ptr = %s.Ptr()
}
%s.VariantSetObject(ret, result_ptr)`, expr, expr, b)
		},
		PtrCallReadArg: func(b string, idx int) string {
			return fmt.Sprintf(`arg%d_ptr := *(*gdextension.ObjectPtr)(gdextension.PtrCallArg(args, %d))
var arg%d *%s.%s
if arg%d_ptr != nil {
	arg%d = &%s.%s{}
	arg%d.BindPtr(arg%d_ptr)
}`, idx, idx, idx, b, className, idx, idx, b, className, idx, idx)
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

// engineClassSliceTypeInfo builds the marshal fragments for slices of an
// engine class — `[]*<bindings>.<EngineClass>`. Wire form is
// Array[<EngineClass>] (TypedArray with class_name set). Per-element
// wrapping uses the same borrowed-view construction as the scalar path;
// nil ObjectPtr elements produce nil Go pointers in the slice.
func engineClassSliceTypeInfo(elem *typeInfo) *typeInfo {
	className := elem.ClassName
	// elem.GoType is "*<bindings>.<Class>"; trim the leading "*" to
	// preserve the bindings alias the engine class typeInfo was built
	// with. We don't have bindings handy as a separate field, but the
	// GoType already encodes it.
	goType := "[]" + elem.GoType
	return &typeInfo{
		GoType:        goType,
		VariantType:   "VariantTypeArray",
		ArgMeta:       "ArgMetaNone",
		ClassName:     className,
		IsEngineClass: true,
		CallReadArg: func(b string, idx int) string {
			return fmt.Sprintf(`arg%d_arr := %s.VariantAsArray(args[%d])
defer arg%d_arr.Destroy()
arg%d_ptrs := arg%d_arr.ToObjectSlice()
arg%d := make([]*%s.%s, len(arg%d_ptrs))
for i, p := range arg%d_ptrs {
	if p != nil {
		arg%d[i] = &%s.%s{}
		arg%d[i].BindPtr(p)
	}
}`, idx, b, idx, idx, idx, idx, idx, b, className, idx, idx, idx, b, className, idx)
		},
		CallWriteReturn: func(b, expr string) string {
			return fmt.Sprintf(`result_ptrs := make([]gdextension.ObjectPtr, len(%s))
for i, v := range %s {
	if v != nil {
		result_ptrs[i] = v.Ptr()
	}
}
result_arr := %s.MakeArrayOfObjects(%q, result_ptrs...)
%s.VariantSetArray(ret, result_arr)
result_arr.Destroy()`, expr, expr, b, className, b)
		},
		PtrCallReadArg: func(b string, idx int) string {
			return fmt.Sprintf(`arg%d_arr := *(*%s.Array)(gdextension.PtrCallArg(args, %d))
arg%d_ptrs := arg%d_arr.ToObjectSlice()
arg%d := make([]*%s.%s, len(arg%d_ptrs))
for i, p := range arg%d_ptrs {
	if p != nil {
		arg%d[i] = &%s.%s{}
		arg%d[i].BindPtr(p)
	}
}`, idx, b, idx, idx, idx, idx, b, className, idx, idx, idx, b, className, idx)
		},
		PtrCallWriteReturn: func(b, expr string) string {
			return fmt.Sprintf(`result_ptrs := make([]gdextension.ObjectPtr, len(%s))
for i, v := range %s {
	if v != nil {
		result_ptrs[i] = v.Ptr()
	}
}
result_arr := %s.MakeArrayOfObjects(%q, result_ptrs...)
*(*%s.Array)(ret) = result_arr`, expr, expr, b, className, b)
		},
		BuildVariant: func(b string, idx int, srcExpr string) string {
			return fmt.Sprintf(`arg%d_ptrs := make([]gdextension.ObjectPtr, len(%s))
for i, v := range %s {
	if v != nil {
		arg%d_ptrs[i] = v.Ptr()
	}
}
arg%d_arr := %s.MakeArrayOfObjects(%q, arg%d_ptrs...)
arg%d := %s.NewVariantArray(arg%d_arr)
arg%d_arr.Destroy()`, idx, srcExpr, srcExpr, idx, idx, b, className, idx, idx, b, idx, idx)
		},
	}
}

// userClassSliceTypeInfo builds the marshal fragments for slices of the
// file's @class — `[]*<MainClass>`. Wire form is Array[<MainClass>]
// (TypedArray, OBJECT type with the user class name as the class_name
// filter). Per-element foreign-instance handling matches Phase 6a:
// the lookup against our engine-pointer side table returns nil for
// instances Godot didn't get from our Construct hook, and codegen
// short-circuits per decision (B).
func userClassSliceTypeInfo(elem *typeInfo) *typeInfo {
	className := elem.ClassName
	lookupName := "lookup" + className + "ByEngine"
	goType := "[]*" + className
	return &typeInfo{
		GoType:      goType,
		VariantType: "VariantTypeArray",
		ArgMeta:     "ArgMetaNone",
		ClassName:   className, // editor surfaces the typed-element identity through ArgClassNames / XML enum=
		CallReadArg: func(b string, idx int) string {
			return fmt.Sprintf(`arg%d_arr := %s.VariantAsArray(args[%d])
defer arg%d_arr.Destroy()
arg%d_ptrs := arg%d_arr.ToObjectSlice()
arg%d := make([]*%s, len(arg%d_ptrs))
for i, p := range arg%d_ptrs {
	arg%d[i] = %s(p)
	if arg%d[i] == nil {
		return gdextension.CallErrorInvalidArgument
	}
}`, idx, b, idx, idx, idx, idx, idx, className, idx, idx, idx, lookupName, idx)
		},
		CallWriteReturn: func(b, expr string) string {
			return fmt.Sprintf(`result_ptrs := make([]gdextension.ObjectPtr, len(%s))
for i, v := range %s {
	if v != nil {
		result_ptrs[i] = v.Ptr()
	}
}
result_arr := %s.MakeArrayOfObjects(%q, result_ptrs...)
%s.VariantSetArray(ret, result_arr)
result_arr.Destroy()`, expr, expr, b, className, b)
		},
		PtrCallReadArg: func(b string, idx int) string {
			return fmt.Sprintf(`arg%d_arr := *(*%s.Array)(gdextension.PtrCallArg(args, %d))
arg%d_ptrs := arg%d_arr.ToObjectSlice()
arg%d := make([]*%s, len(arg%d_ptrs))
for i, p := range arg%d_ptrs {
	arg%d[i] = %s(p)
	if arg%d[i] == nil {
		return
	}
}`, idx, b, idx, idx, idx, idx, className, idx, idx, idx, lookupName, idx)
		},
		PtrCallWriteReturn: func(b, expr string) string {
			return fmt.Sprintf(`result_ptrs := make([]gdextension.ObjectPtr, len(%s))
for i, v := range %s {
	if v != nil {
		result_ptrs[i] = v.Ptr()
	}
}
result_arr := %s.MakeArrayOfObjects(%q, result_ptrs...)
*(*%s.Array)(ret) = result_arr`, expr, expr, b, className, b)
		},
		BuildVariant: func(b string, idx int, srcExpr string) string {
			return fmt.Sprintf(`arg%d_ptrs := make([]gdextension.ObjectPtr, len(%s))
for i, v := range %s {
	if v != nil {
		arg%d_ptrs[i] = v.Ptr()
	}
}
arg%d_arr := %s.MakeArrayOfObjects(%q, arg%d_ptrs...)
arg%d := %s.NewVariantArray(arg%d_arr)
arg%d_arr.Destroy()`, idx, srcExpr, srcExpr, idx, idx, b, className, idx, idx, b, idx, idx)
		},
	}
}

// userIntSliceTypeInfo builds the marshal fragments for slices of any
// user `type X int` alias. Two wire forms by tag status:
//
//   - Tagged @enum / @bitfield: wire is untyped Variant::ARRAY with
//     PROPERTY_HINT_TYPE_STRING carrying the element identity. This
//     is the only shape Godot's editor honors for typed-element
//     rendering on method args/returns — Packed types ignore the
//     hint, and `set_typed(TYPE_INT, class_name, ...)` is rejected at
//     runtime. Per-element marshaling boxes each int through a
//     Variant, slightly slower than Packed but unlocks the
//     `Array[Kind]`-style rendering in the editor.
//
//   - Untagged: wire stays as PackedInt64Array — no enum identity to
//     preserve, contiguous storage and faster element marshaling.
//
// Both routes propagate the EnumName as HintEnum so the emitter
// builds the hint string. For Packed-wire-form (untagged) the hint
// is harmless but ignored by the editor; for untyped-Array (tagged)
// the editor reads it and renders the typed-element identity.
func userIntSliceTypeInfo(elem *typeInfo) *typeInfo {
	if elem.EnumName != "" {
		return taggedEnumSliceTypeInfo(elem)
	}
	cat := sliceCategory{
		PackedTypeName: "PackedInt64Array",
		VariantType:    "VariantTypePackedInt64Array",
		CastTo:         "int64",      // user-type → wire
		CastFrom:       elem.GoType,  // wire → user-type
	}
	return slicePackedTypeInfo("[]"+elem.GoType, elem.GoType, cat)
}

// taggedEnumSliceTypeInfo builds the marshal fragments for
// `[]<TaggedEnum>` — untyped Variant::ARRAY at the wire boundary so
// PROPERTY_HINT_TYPE_STRING fires the editor's typed-element
// rendering. Per-element marshal boxes each int through a Variant
// (Phase 3's MakeArrayOfInts on the write side; Variant.AsInt on the
// read side).
func taggedEnumSliceTypeInfo(elem *typeInfo) *typeInfo {
	goType := "[]" + elem.GoType
	enumGoType := elem.GoType
	return &typeInfo{
		GoType:      goType,
		VariantType: "VariantTypeArray",
		ArgMeta:     "ArgMetaNone",
		HintEnum:    elem.EnumName,
		CallReadArg: func(b string, idx int) string {
			return fmt.Sprintf(`arg%d_arr := %s.VariantAsArray(args[%d])
defer arg%d_arr.Destroy()
arg%d_n := arg%d_arr.Size()
arg%d := make([]%s, arg%d_n)
for i := int64(0); i < arg%d_n; i++ {
	v := arg%d_arr.Get(i)
	arg%d[i] = %s(v.AsInt())
	v.Destroy()
}`, idx, b, idx, idx, idx, idx, idx, enumGoType, idx, idx, idx, idx, enumGoType)
		},
		CallWriteReturn: func(b, expr string) string {
			return fmt.Sprintf(`result_i64 := make([]int64, len(%s))
for i, v := range %s {
	result_i64[i] = int64(v)
}
result_arr := %s.MakeArrayOfInts(result_i64...)
%s.VariantSetArray(ret, result_arr)
result_arr.Destroy()`, expr, expr, b, b)
		},
		PtrCallReadArg: func(b string, idx int) string {
			return fmt.Sprintf(`arg%d_arr := *(*%s.Array)(gdextension.PtrCallArg(args, %d))
arg%d_n := arg%d_arr.Size()
arg%d := make([]%s, arg%d_n)
for i := int64(0); i < arg%d_n; i++ {
	v := arg%d_arr.Get(i)
	arg%d[i] = %s(v.AsInt())
	v.Destroy()
}`, idx, b, idx, idx, idx, idx, enumGoType, idx, idx, idx, idx, enumGoType)
		},
		PtrCallWriteReturn: func(b, expr string) string {
			return fmt.Sprintf(`result_i64 := make([]int64, len(%s))
for i, v := range %s {
	result_i64[i] = int64(v)
}
result_arr := %s.MakeArrayOfInts(result_i64...)
*(*%s.Array)(ret) = result_arr`, expr, expr, b, b)
		},
		BuildVariant: func(b string, idx int, srcExpr string) string {
			return fmt.Sprintf(`arg%d_i64 := make([]int64, len(%s))
for i, v := range %s {
	arg%d_i64[i] = int64(v)
}
arg%d_arr := %s.MakeArrayOfInts(arg%d_i64...)
arg%d := %s.NewVariantArray(arg%d_arr)
arg%d_arr.Destroy()`, idx, srcExpr, srcExpr, idx, idx, b, idx, idx, b, idx, idx)
		},
	}
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

// mapTypeInfo builds the marshal fragments for `map[K]V` at the
// @class boundary. Wire form is Godot's untyped Dictionary — typed
// dictionaries (Dictionary[K, V] in 4.4+) are deferred since they
// require additional registration metadata. Iteration order is Go's
// randomized map order on the way out; Dictionary preserves insertion
// order, but the framework doesn't promise stable ordering at the
// boundary either way.
//
// v1 supports primitive K and V only — keys and values must come from
// the typeTable (bool, int, int32, int64, float32, float64, string).
// Slice values, user enums, user-class pointers as K/V are deferred
// to later phases. The check is "WrapVariant + UnwrapVariant
// populated" rather than "in typeTable" so Phase 2 work that adds
// these expression helpers to other typeInfos lights up automatically.
//
// Each fragment uses the typeInfo's WrapVariant / UnwrapVariant to
// box / unbox values across the Variant boundary. Dictionary.Get
// requires a default Variant for missing keys — we declare a
// stack-local zero-value Variant rather than allocating, since every
// key we iterate came from Dictionary.Keys() and is guaranteed
// present.
func mapTypeInfo(key, value *typeInfo) (*typeInfo, error) {
	if key.WrapVariant == nil || key.UnwrapVariant == nil {
		return nil, fmt.Errorf("map key type %q is not supported (only primitive types are supported as map keys today: bool, int, int32, int64, float32, float64, string)", key.GoType)
	}
	if value.WrapVariant == nil || value.UnwrapVariant == nil {
		return nil, fmt.Errorf("map value type %q is not supported (only primitive types are supported as map values today: bool, int, int32, int64, float32, float64, string)", value.GoType)
	}

	goType := "map[" + key.GoType + "]" + value.GoType

	readBody := func(b string, idx int) string {
		return fmt.Sprintf(`arg%d_keys := arg%d_dict.Keys()
defer arg%d_keys.Destroy()
arg%d_n := arg%d_keys.Size()
arg%d := make(%s, arg%d_n)
var arg%d_def %s.Variant
for arg%d_i := int64(0); arg%d_i < arg%d_n; arg%d_i++ {
	arg%d_kv := arg%d_keys.Get(arg%d_i)
	arg%d_vv := arg%d_dict.Get(arg%d_kv, arg%d_def)
	arg%d[%s] = %s
	arg%d_kv.Destroy()
	arg%d_vv.Destroy()
}`,
			idx, idx,
			idx,
			idx, idx,
			idx, goType, idx,
			idx, b,
			idx, idx, idx, idx,
			idx, idx, idx,
			idx, idx, idx, idx,
			idx,
			key.UnwrapVariant(b, fmt.Sprintf("arg%d_kv", idx)),
			value.UnwrapVariant(b, fmt.Sprintf("arg%d_vv", idx)),
			idx, idx)
	}

	writeBody := func(b, expr, dstName string) string {
		return fmt.Sprintf(`%s := %s.NewDictionary()
for k_, v_ := range %s {
	k_v := %s
	v_v := %s
	%s.Set(k_v, v_v)
	k_v.Destroy()
	v_v.Destroy()
}`,
			dstName, b,
			expr,
			key.WrapVariant(b, "k_"),
			value.WrapVariant(b, "v_"),
			dstName)
	}

	return &typeInfo{
		GoType:      goType,
		VariantType: "VariantTypeDictionary",
		ArgMeta:     "ArgMetaNone",
		CallReadArg: func(b string, idx int) string {
			return fmt.Sprintf(`arg%d_dict := %s.VariantAsDictionary(args[%d])
defer arg%d_dict.Destroy()
%s`, idx, b, idx, idx, readBody(b, idx))
		},
		CallWriteReturn: func(b, expr string) string {
			return writeBody(b, expr, "result_dict") + fmt.Sprintf(`
%s.VariantSetDictionary(ret, result_dict)
result_dict.Destroy()`, b)
		},
		PtrCallReadArg: func(b string, idx int) string {
			return fmt.Sprintf(`arg%d_dict := *(*%s.Dictionary)(gdextension.PtrCallArg(args, %d))
%s`, idx, b, idx, readBody(b, idx))
		},
		PtrCallWriteReturn: func(b, expr string) string {
			return writeBody(b, expr, "result_dict") + fmt.Sprintf(`
*(*%s.Dictionary)(ret) = result_dict`, b)
		},
		BuildVariant: func(b string, idx int, srcExpr string) string {
			dst := fmt.Sprintf("arg%d_dict", idx)
			return writeBody(b, srcExpr, dst) + fmt.Sprintf(`
arg%d := %s.NewVariantDictionary(%s)
%s.Destroy()`, idx, b, dst, dst)
		},
	}, nil
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
