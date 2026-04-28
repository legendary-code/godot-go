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
// way out. The Godot ABI doesn't carry the typed enum identity so the
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
		GoType:      name,
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
