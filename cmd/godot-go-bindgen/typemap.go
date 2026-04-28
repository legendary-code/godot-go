package main

import (
	"fmt"
	"strings"

	"github.com/legendary-code/godot-go/internal/naming"
)

// typeKind classifies a Godot type for codegen purposes. It controls the Go
// type the user sees, the local-temp prep code emitted at the boundary, and
// whether the generated method needs a deferred destructor call.
type typeKind int

const (
	kindUnsupported typeKind = iota
	kindBool                  // bool
	kindInt                   // int -> int64
	kindFloat                 // float -> float32 (single precision build)
	kindString                // String -> string (transparent boundary)
	kindStringName            // StringName -> string
	kindNodePath              // NodePath -> string
	kindVariant               // Variant -> variant.Variant
	kindBuiltin               // any other builtin class -> variant.X by-value
	kindObject                // engine class -> *core.X / *editor.X (opaque)
)

// engineClassNames is the set of engine-class names the API exposes.
// Populated by registerEngineClasses before any emit* call. Used by the
// type resolver to recognize cross-class references (every emitted file
// lands in the same package now, so we just need to know whether a name
// is a class — not where it lives).
var engineClassNames = map[string]bool{}

// registerEngineClasses fills engineClassNames from the loaded API. Must run
// before any emit* call so type lookups resolve correctly.
func registerEngineClasses(api *API) {
	for i := range api.Classes {
		engineClassNames[api.Classes[i].Name] = true
	}
}

// classGoType returns the user-facing Go type for an engine class reference.
// All bindings live in one package, so the result is the bare `*ClassName`.
// Returns ("", false) if the class isn't a known engine class.
func classGoType(className string) (string, bool) {
	if !engineClassNames[className] {
		return "", false
	}
	return "*" + className, true
}

// resolveType is the engine-class emitter's analogue of goType: same return
// shape, but it also knows about engine-class refs and class-scoped enums.
// Single-package output means class refs are bare names (no pkg qualifier).
func resolveType(godot string) (goTypeName string, kind typeKind, ok bool) {
	// Native-structure pointer types (e.g. "const GDExtensionInterface*")
	// come straight from the Godot C ABI's native_structures list — the
	// engine-class methods that reference them are dropped.
	if strings.ContainsAny(godot, " *") {
		return "", kindUnsupported, false
	}
	if ct, k := classGoType(godot); k {
		return ct, kindObject, true
	}
	if strings.HasPrefix(godot, "enum::") || strings.HasPrefix(godot, "bitfield::") {
		return enumGoIdent(godot), kindInt, true
	}
	g, k := goType(godot)
	if k == kindUnsupported {
		return "", k, false
	}
	return g, k, true
}

// enumGoIdent renders an `enum::ClassName.EnumName` (or bare top-level enum)
// reference as a Go identifier. Bitfields use the same encoding. The leaf
// type is always int64 underneath; this just gives nicer-looking method
// signatures.
//
// All enums live in the same package now, so the qualified reference
// "ClassName" + "EnumName" (e.g. NodeProcessMode) is always usable; bare
// top-level enums collapse to int64 since their typed Go names live in the
// global enums sweep and matching them to method-signature shape is more
// trouble than it's worth.
func enumGoIdent(godotEnumType string) string {
	body := strings.TrimPrefix(strings.TrimPrefix(godotEnumType, "enum::"), "bitfield::")
	dot := strings.IndexByte(body, '.')
	if dot < 0 {
		return "int64"
	}
	owner, enum := body[:dot], body[dot+1:]
	if !engineClassNames[owner] {
		return "int64"
	}
	return owner + enum
}

// goType returns the user-facing Go type for a Godot type, plus a typeKind
// classifying it. Returns (_, kindUnsupported) for types we can't emit yet
// (Object, typed arrays, etc. — Phase 3+).
func goType(godot string) (string, typeKind) {
	switch godot {
	case "bool":
		return "bool", kindBool
	case "int":
		return "int64", kindInt
	case "float":
		return "float32", kindFloat
	case "String":
		return "string", kindString
	case "StringName":
		return "string", kindStringName
	case "NodePath":
		return "string", kindNodePath
	case "Variant":
		return "Variant", kindVariant
	case "Object":
		return "", kindUnsupported
	}
	if strings.HasPrefix(godot, "typedarray::") {
		// Typed arrays collapse to plain Array for now; element-type
		// preservation is Phase 3 work.
		return "Array", kindBuiltin
	}
	if strings.HasPrefix(godot, "enum::") || strings.HasPrefix(godot, "bitfield::") {
		return "int64", kindInt
	}
	// Anything else is presumed to be a builtin class. The caller is
	// responsible for confirming the class actually exists in the spec.
	return godot, kindBuiltin
}

// goZero returns a zero-value expression for the Go type that goType would
// produce for a given Godot type. Used to emit `var ret T` declarations.
func goZero(godot string) string {
	g, k := goType(godot)
	switch k {
	case kindBool:
		return "false"
	case kindInt, kindFloat:
		return "0"
	case kindString, kindStringName, kindNodePath:
		return `""`
	default:
		return "var ret " + g
	}
}

// suffixFor returns the operator-disambiguator suffix for a right_type.
// Primitives get title-cased; named types pass through.
func suffixFor(godot string) string {
	switch godot {
	case "bool":
		return "Bool"
	case "int":
		return "Int"
	case "float":
		return "Float"
	}
	return godot
}

// pascal is a thin alias to keep call sites concise. The actual
// snake → Pascal logic lives in internal/naming alongside its inverse
// (PascalToSnake), so the user-facing codegen and the framework
// bindgen agree on the convention.
func pascal(s string) string { return naming.SnakeToPascal(s) }

// safeIdent rewrites Godot argument names that collide with Go keywords or
// commonly-shadowed builtin identifiers.
func safeIdent(name string) string {
	switch name {
	case "type":
		return "typ"
	case "func":
		return "fn"
	case "len":
		return "length"
	case "string":
		return "s"
	case "default":
		return "def"
	case "range":
		return "rng"
	case "map":
		return "m"
	case "select":
		return "sel"
	case "var":
		return "v"
	case "package":
		return "pkg"
	case "interface":
		return "iface"
	case "chan":
		return "ch"
	case "go":
		return "g"
	case "switch":
		return "sw"
	case "case":
		return "c"
	// Names below collide with locals or imports the bindgen emits.
	case "args":
		return "a" // collides with the args[...] literal
	case "ret":
		return "r" // collides with the return-slot local
	case "variant":
		return "v" // shadows the variant package import
	case "self":
		return "s" // can't shadow the receiver name
	case "core":
		return "c2" // shadows the core import in editor/
	case "editor":
		return "e" // shadows the editor import (future use)
	case "gdextension":
		return "gd" // shadows the gdextension import
	}
	return name
}

// operatorMap is the Godot operator name -> Go method-name root. Suffixed by
// the right_type for disambiguation when right_type != self_type and != "".
var operatorMap = map[string]string{
	"==":      "Eq",
	"!=":      "Ne",
	"<":       "Lt",
	"<=":      "Le",
	">":       "Gt",
	">=":      "Ge",
	"+":       "Add",
	"-":       "Sub",
	"*":       "Mul",
	"/":       "Div",
	"%":       "Mod",
	"**":      "Pow",
	"<<":      "Shl",
	">>":      "Shr",
	"&":       "BitAnd",
	"|":       "BitOr",
	"^":       "BitXor",
	"~":       "BitNot",
	"unary-":  "Neg",
	"unary+":  "Pos",
	"and":     "And",
	"or":      "Or",
	"xor":     "Xor",
	"not":     "Not",
	"in":      "In",
}

// operatorEnumMap maps Godot operator name to the gdextension.OpXxx constant
// name (without the gdextension. prefix).
var operatorEnumMap = map[string]string{
	"==":      "OpEqual",
	"!=":      "OpNotEqual",
	"<":       "OpLess",
	"<=":      "OpLessEqual",
	">":       "OpGreater",
	">=":      "OpGreaterEqual",
	"+":       "OpAdd",
	"-":       "OpSubtract",
	"*":       "OpMultiply",
	"/":       "OpDivide",
	"%":       "OpModule",
	"**":      "OpPower",
	"<<":      "OpShiftLeft",
	">>":      "OpShiftRight",
	"&":       "OpBitAnd",
	"|":       "OpBitOr",
	"^":       "OpBitXor",
	"~":       "OpBitNegate",
	"unary-":  "OpNegate",
	"unary+":  "OpPositive",
	"and":     "OpAnd",
	"or":      "OpOr",
	"xor":     "OpXor",
	"not":     "OpNot",
	"in":      "OpIn",
}

// opGoName builds the Go method name for an operator on `selfClass` with a
// right_type of `right` (empty for unary). Same-type binary operators drop
// the suffix; mixed-type ops carry the suffix.
func opGoName(selfClass, godotOp, right string) (string, error) {
	root, ok := operatorMap[godotOp]
	if !ok {
		return "", fmt.Errorf("unknown operator %q", godotOp)
	}
	if right == "" || right == selfClass {
		return root, nil
	}
	return root + suffixFor(right), nil
}
