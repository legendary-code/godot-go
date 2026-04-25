package main

import (
	"fmt"
	"strings"
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
)

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

// pascal converts snake_case_or_lowercase to PascalCase. Numbers in the
// middle are preserved (e.g. "vector2" -> "Vector2", "a_b_c" -> "ABC").
func pascal(s string) string {
	if s == "" {
		return ""
	}
	parts := strings.Split(s, "_")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, "")
}

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
