package main

import (
	"fmt"
	"go/ast"
)

// typeInfo describes how a Go primitive maps onto Godot's GDExtension ABI
// for callee-side method args and returns. Phase 5d MVP scope: bool, int /
// int32 / int64, float32 / float64, string. Pointers to other extension
// classes, packed arrays, and variant types are deferred to later phases.
//
// Each func field is a code-gen helper, not a runtime value. The emitter
// invokes them while building per-method Call / PtrCall bodies so the
// generated bindings can read host args, dispatch to the user method, and
// write the return.
type typeInfo struct {
	// GoType is the user-facing Go type ident as it appears in the source
	// signature (e.g. "int32"). Used as the type of the argument variable
	// passed to the user method.
	GoType string

	// VariantType is the bare gdextension.VariantType constant name (no
	// package qualifier — the emitter prepends "gdextension." once, in the
	// template, to keep the table portable).
	VariantType string

	// ArgMeta is the gdextension.MethodArgumentMetadata constant name —
	// Godot uses this to display narrower-than-int64 widths in the editor
	// even though the wire always carries int64 / double.
	ArgMeta string

	// NeedsVariantImport is true for types whose helpers live in the
	// variant package. Currently every type qualifies because the Call
	// path always routes through variant.VariantAs* / VariantSet*; the
	// emitter still gates the import on whether *any* method has at
	// least one arg or a return so a no-op class file doesn't pull
	// variant in.
	NeedsVariantImport bool

	// PtrCallReadArg returns Go source for a single statement that
	// declares `arg<idx>` with the user-facing type. The expression on
	// the right reads from the PtrCall args slot and applies any
	// wire-to-Go conversion (e.g. int64 → int32).
	PtrCallReadArg func(idx int) string

	// PtrCallWriteReturn returns Go source for a single statement that
	// stores `expr` (a GoType-typed expression) into the PtrCall ret
	// pointer, applying any Go-to-wire conversion internally.
	PtrCallWriteReturn func(expr string) string

	// CallReadArg returns Go source for a single statement that declares
	// `arg<idx>` with the user-facing type by extracting from the i-th
	// host VariantPtr in `args`.
	CallReadArg func(idx int) string

	// CallWriteReturn returns Go source for a single statement that
	// writes `expr` (a GoType-typed expression) into the ret VariantPtr,
	// allocating any required temporary internally.
	CallWriteReturn func(expr string) string

	// BuildVariant returns a single Go statement that constructs a
	// fresh `variant.Variant` value, named `arg<idx>`, holding `srcExpr`
	// (a GoType-typed expression). The caller is responsible for
	// emitting `defer arg<idx>.Destroy()` separately. Used by signal-
	// emit synthesis to marshal each user-supplied arg before dispatch.
	BuildVariant func(idx int, srcExpr string) string
}

// typeTable indexes typeInfo by Go type ident. Keys must match exactly
// what appears in the source AST — `int` is distinct from `int64` even
// though they're often the same width on a 64-bit platform.
var typeTable = map[string]*typeInfo{
	"bool": {
		GoType:             "bool",
		VariantType:        "VariantTypeBool",
		ArgMeta:            "ArgMetaNone",
		NeedsVariantImport: true,
		PtrCallReadArg: func(idx int) string {
			return fmt.Sprintf("arg%d := *(*bool)(gdextension.PtrCallArg(args, %d))", idx, idx)
		},
		PtrCallWriteReturn: func(expr string) string {
			return fmt.Sprintf("*(*bool)(ret) = %s", expr)
		},
		CallReadArg: func(idx int) string {
			return fmt.Sprintf("arg%d := variant.VariantAsBool(args[%d])", idx, idx)
		},
		CallWriteReturn: func(expr string) string {
			return fmt.Sprintf("variant.VariantSetBool(ret, %s)", expr)
		},
		BuildVariant: func(idx int, srcExpr string) string {
			return fmt.Sprintf("arg%d := variant.NewVariantBool(%s)", idx, srcExpr)
		},
	},
	"int": {
		GoType:             "int",
		VariantType:        "VariantTypeInt",
		ArgMeta:            "ArgMetaIntIsInt64",
		NeedsVariantImport: true,
		PtrCallReadArg: func(idx int) string {
			return fmt.Sprintf("arg%d := int(*(*int64)(gdextension.PtrCallArg(args, %d)))", idx, idx)
		},
		PtrCallWriteReturn: func(expr string) string {
			return fmt.Sprintf("*(*int64)(ret) = int64(%s)", expr)
		},
		CallReadArg: func(idx int) string {
			return fmt.Sprintf("arg%d := int(variant.VariantAsInt64(args[%d]))", idx, idx)
		},
		CallWriteReturn: func(expr string) string {
			return fmt.Sprintf("variant.VariantSetInt64(ret, int64(%s))", expr)
		},
		BuildVariant: func(idx int, srcExpr string) string {
			return fmt.Sprintf("arg%d := variant.NewVariantInt(int64(%s))", idx, srcExpr)
		},
	},
	"int32": {
		GoType:             "int32",
		VariantType:        "VariantTypeInt",
		ArgMeta:            "ArgMetaIntIsInt32",
		NeedsVariantImport: true,
		PtrCallReadArg: func(idx int) string {
			return fmt.Sprintf("arg%d := int32(*(*int64)(gdextension.PtrCallArg(args, %d)))", idx, idx)
		},
		PtrCallWriteReturn: func(expr string) string {
			return fmt.Sprintf("*(*int64)(ret) = int64(%s)", expr)
		},
		CallReadArg: func(idx int) string {
			return fmt.Sprintf("arg%d := int32(variant.VariantAsInt64(args[%d]))", idx, idx)
		},
		CallWriteReturn: func(expr string) string {
			return fmt.Sprintf("variant.VariantSetInt64(ret, int64(%s))", expr)
		},
		BuildVariant: func(idx int, srcExpr string) string {
			return fmt.Sprintf("arg%d := variant.NewVariantInt(int64(%s))", idx, srcExpr)
		},
	},
	"int64": {
		GoType:             "int64",
		VariantType:        "VariantTypeInt",
		ArgMeta:            "ArgMetaIntIsInt64",
		NeedsVariantImport: true,
		PtrCallReadArg: func(idx int) string {
			return fmt.Sprintf("arg%d := *(*int64)(gdextension.PtrCallArg(args, %d))", idx, idx)
		},
		PtrCallWriteReturn: func(expr string) string {
			return fmt.Sprintf("*(*int64)(ret) = %s", expr)
		},
		CallReadArg: func(idx int) string {
			return fmt.Sprintf("arg%d := variant.VariantAsInt64(args[%d])", idx, idx)
		},
		CallWriteReturn: func(expr string) string {
			return fmt.Sprintf("variant.VariantSetInt64(ret, %s)", expr)
		},
		BuildVariant: func(idx int, srcExpr string) string {
			return fmt.Sprintf("arg%d := variant.NewVariantInt(%s)", idx, srcExpr)
		},
	},
	"float32": {
		GoType:             "float32",
		VariantType:        "VariantTypeFloat",
		ArgMeta:            "ArgMetaRealIsFloat",
		NeedsVariantImport: true,
		PtrCallReadArg: func(idx int) string {
			return fmt.Sprintf("arg%d := float32(*(*float64)(gdextension.PtrCallArg(args, %d)))", idx, idx)
		},
		PtrCallWriteReturn: func(expr string) string {
			return fmt.Sprintf("*(*float64)(ret) = float64(%s)", expr)
		},
		CallReadArg: func(idx int) string {
			return fmt.Sprintf("arg%d := float32(variant.VariantAsFloat64(args[%d]))", idx, idx)
		},
		CallWriteReturn: func(expr string) string {
			return fmt.Sprintf("variant.VariantSetFloat64(ret, float64(%s))", expr)
		},
		BuildVariant: func(idx int, srcExpr string) string {
			return fmt.Sprintf("arg%d := variant.NewVariantFloat(float64(%s))", idx, srcExpr)
		},
	},
	"float64": {
		GoType:             "float64",
		VariantType:        "VariantTypeFloat",
		ArgMeta:            "ArgMetaRealIsDouble",
		NeedsVariantImport: true,
		PtrCallReadArg: func(idx int) string {
			return fmt.Sprintf("arg%d := *(*float64)(gdextension.PtrCallArg(args, %d))", idx, idx)
		},
		PtrCallWriteReturn: func(expr string) string {
			return fmt.Sprintf("*(*float64)(ret) = %s", expr)
		},
		CallReadArg: func(idx int) string {
			return fmt.Sprintf("arg%d := variant.VariantAsFloat64(args[%d])", idx, idx)
		},
		CallWriteReturn: func(expr string) string {
			return fmt.Sprintf("variant.VariantSetFloat64(ret, %s)", expr)
		},
		BuildVariant: func(idx int, srcExpr string) string {
			return fmt.Sprintf("arg%d := variant.NewVariantFloat(%s)", idx, srcExpr)
		},
	},
	"string": {
		GoType:             "string",
		VariantType:        "VariantTypeString",
		ArgMeta:            "ArgMetaNone",
		NeedsVariantImport: true,
		PtrCallReadArg: func(idx int) string {
			return fmt.Sprintf("arg%d := variant.PtrCallArgString(args, %d)", idx, idx)
		},
		PtrCallWriteReturn: func(expr string) string {
			return fmt.Sprintf("variant.PtrCallStoreString(ret, %s)", expr)
		},
		CallReadArg: func(idx int) string {
			return fmt.Sprintf("arg%d := variant.VariantAsString(args[%d])", idx, idx)
		},
		CallWriteReturn: func(expr string) string {
			return fmt.Sprintf("variant.VariantSetString(ret, %s)", expr)
		},
		BuildVariant: func(idx int, srcExpr string) string {
			return fmt.Sprintf("arg%d := variant.NewVariantString(%s)", idx, srcExpr)
		},
	},
}

// resolveType maps an AST type expression to a typeInfo entry. Returns
// (nil, error) for unsupported types — the caller (emit) reports the
// error with file:line context.
//
// User-defined int-backed enums (Phase 5b discovery) are accepted as if
// they were int64 over the wire: read with a typed cast on the way in,
// stored as int64 on the way out. The Godot ABI doesn't carry the typed
// enum identity — VariantTypeInt is what crosses the boundary — so the
// editor still sees an int and GDScript callers pass plain numbers.
func resolveType(expr ast.Expr, enums map[string]bool) (*typeInfo, error) {
	id, ok := expr.(*ast.Ident)
	if !ok {
		return nil, fmt.Errorf("unsupported type %T (only primitive and user-enum types are supported)", expr)
	}
	if info, ok := typeTable[id.Name]; ok {
		return info, nil
	}
	if enums[id.Name] {
		return enumTypeInfo(id.Name), nil
	}
	return nil, fmt.Errorf("unsupported type %q (supported: bool, int, int32, int64, float32, float64, string, or a user @enum-int type declared in this file)", id.Name)
}

// enumTypeInfo builds the marshalling helpers for a user int enum. The
// Godot wire type is int64 (just like `int`), but the user-facing arg /
// return uses the enum's named type so the generated binding compiles
// cleanly against the user's source.
func enumTypeInfo(name string) *typeInfo {
	return &typeInfo{
		GoType:             name,
		VariantType:        "VariantTypeInt",
		ArgMeta:            "ArgMetaIntIsInt64",
		NeedsVariantImport: true,
		PtrCallReadArg: func(idx int) string {
			return fmt.Sprintf("arg%d := %s(*(*int64)(gdextension.PtrCallArg(args, %d)))", idx, name, idx)
		},
		PtrCallWriteReturn: func(expr string) string {
			return fmt.Sprintf("*(*int64)(ret) = int64(%s)", expr)
		},
		CallReadArg: func(idx int) string {
			return fmt.Sprintf("arg%d := %s(variant.VariantAsInt64(args[%d]))", idx, name, idx)
		},
		CallWriteReturn: func(expr string) string {
			return fmt.Sprintf("variant.VariantSetInt64(ret, int64(%s))", expr)
		},
		BuildVariant: func(idx int, srcExpr string) string {
			return fmt.Sprintf("arg%d := variant.NewVariantInt(int64(%s))", idx, srcExpr)
		},
	}
}
