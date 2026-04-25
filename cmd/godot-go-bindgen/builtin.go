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

// builtinView is the data the builtin-class template consumes. It's a
// flattened, name-mangled projection of BuiltinClass + size + offsets so the
// template stays free of business logic.
type builtinView struct {
	GoName       string // e.g. "Vector2"
	Size         int    // bytes (per builtin_class_sizes for the target build_config)
	VariantType  string // e.g. "VariantTypeVector2"
	HasDestructor bool

	Members      []memberView
	Constructors []constructorView
	Methods      []methodView
}

type memberView struct {
	GoName    string // PascalCase
	Offset    int
	GoType    string // Go type for the field accessor (float32, int32, etc.)
}

type constructorView struct {
	GoName string // e.g. "NewVector2", "NewVector2FromXY"
	Index  int
	VarID  string // unique Go identifier to cache the resolved fn pointer
	Args   []paramView
}

type methodView struct {
	GoName        string // PascalCase
	GodotName     string // original snake_case name (used to resolve via StringName)
	Hash          int64
	IsStatic      bool
	IsConst       bool
	VarID         string // unique Go identifier for the cached fn pointer
	ReturnGoType  string // Go return type, "" if void
	ReturnZero    string // zero-value expression for the return type, "" if void
	Args          []paramView
}

type paramView struct {
	GoName string // Go-friendly param name
	GoType string // Go param type
}

// emitBuiltinClass writes <outDir>/<lower(name)>.gen.go for the given builtin
// class. The chosen subset of methods/constructors/operators is curated for
// Phase 2b — Phase 2c lifts that restriction.
func emitBuiltinClass(api *API, buildConfig string, bc *BuiltinClass, outDir string) error {
	size, ok := api.SizeFor(buildConfig, bc.Name)
	if !ok {
		return fmt.Errorf("no size for %s under %s", bc.Name, buildConfig)
	}
	offsets := api.OffsetsFor(buildConfig, bc.Name)

	view := builtinView{
		GoName:        bc.Name,
		Size:          size,
		VariantType:   "VariantType" + bc.Name,
		HasDestructor: bc.HasDestructor,
	}

	for _, m := range offsets {
		view.Members = append(view.Members, memberView{
			GoName: pascal(m.Member),
			Offset: m.Offset,
			GoType: goTypeForMeta(m.Meta),
		})
	}

	for _, c := range bc.Constructors {
		if !shouldEmitConstructor(bc.Name, c) {
			continue
		}
		args := make([]paramView, 0, len(c.Arguments))
		for _, a := range c.Arguments {
			args = append(args, paramView{
				GoName: safeGoIdent(a.Name),
				GoType: goTypeForGodot(a.Type),
			})
		}
		view.Constructors = append(view.Constructors, constructorView{
			GoName: ctorGoName(bc.Name, c),
			Index:  c.Index,
			VarID:  fmt.Sprintf("ctor%d", c.Index),
			Args:   args,
		})
	}

	for _, m := range bc.Methods {
		if !shouldEmitMethod(bc.Name, m) {
			continue
		}
		args := make([]paramView, 0, len(m.Arguments))
		for _, a := range m.Arguments {
			args = append(args, paramView{
				GoName: safeGoIdent(a.Name),
				GoType: goTypeForGodot(a.Type),
			})
		}
		retGo := ""
		retZero := ""
		if m.ReturnType != "" && m.ReturnType != "void" {
			retGo = goTypeForGodot(m.ReturnType)
			retZero = goZeroForGodot(m.ReturnType)
		}
		view.Methods = append(view.Methods, methodView{
			GoName:       pascal(m.Name),
			GodotName:    m.Name,
			Hash:         m.Hash,
			IsStatic:     m.IsStatic,
			IsConst:      m.IsConst,
			VarID:        "method" + pascal(m.Name),
			ReturnGoType: retGo,
			ReturnZero:   retZero,
			Args:         args,
		})
	}

	var buf bytes.Buffer
	if err := builtinTmpl.Execute(&buf, view); err != nil {
		return fmt.Errorf("template: %w", err)
	}

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		// Write the unformatted output for debugging when formatting fails.
		_ = os.WriteFile(filepath.Join(outDir, strings.ToLower(bc.Name)+".gen.go.broken"), buf.Bytes(), 0o644)
		return fmt.Errorf("gofmt: %w", err)
	}

	outPath := filepath.Join(outDir, strings.ToLower(bc.Name)+".gen.go")
	return os.WriteFile(outPath, formatted, 0o644)
}

// Phase 2b method/constructor curation. These predicates exist only to keep
// the first emitted file small enough to eyeball; Phase 2c removes them and
// emits everything in extension_api.json.
func shouldEmitConstructor(class string, c Constructor) bool {
	switch class {
	case "Vector2":
		return c.Index == 0 || c.Index == 3 // default + (x,y) — skips copy/Vector2i
	}
	return true
}

func shouldEmitMethod(class string, m Method) bool {
	if m.IsVararg {
		return false
	}
	switch class {
	case "Vector2":
		switch m.Name {
		case "length", "length_squared", "is_normalized", "normalized", "dot", "distance_to":
			return true
		default:
			return false
		}
	}
	return true
}

func ctorGoName(class string, c Constructor) string {
	if len(c.Arguments) == 0 {
		return "New" + class
	}
	parts := []string{"New", class}
	for _, a := range c.Arguments {
		parts = append(parts, pascal(a.Name))
	}
	return strings.Join(parts, "")
}

// goTypeForMeta maps a member-offset `meta` field to a Go type. Members of
// composite types (other Vector*, etc.) are returned as the Go type name —
// the template emits raw `unsafe.Pointer`-cast accessors for them later.
func goTypeForMeta(meta string) string {
	switch meta {
	case "float":
		return "float32"
	case "double":
		return "float64"
	case "int8":
		return "int8"
	case "int16":
		return "int16"
	case "int32":
		return "int32"
	case "int64":
		return "int64"
	case "uint8":
		return "uint8"
	case "uint16":
		return "uint16"
	case "uint32":
		return "uint32"
	case "uint64":
		return "uint64"
	default:
		// Composite member types (e.g. "Vector2") fall through to the type
		// name as-is. Phase 2c handles cross-type field accessors.
		return meta
	}
}

// goTypeForGodot maps a Godot type name (as it appears in extension_api.json)
// to the Go type used in user-facing APIs.
//
// Phase 2b only handles the primitive types that show up in the curated
// Vector2 method set. Phase 2c extends this for composite + ref-counted types.
func goTypeForGodot(godotType string) string {
	switch godotType {
	case "bool":
		return "bool"
	case "int":
		return "int64"
	case "float":
		return "float32" // single precision; flips to float64 under double_* configs in Phase 2c
	case "String", "StringName", "NodePath":
		return "string"
	default:
		return godotType
	}
}

// goZeroForGodot returns the zero-value expression for the type
// goTypeForGodot would produce.
func goZeroForGodot(godotType string) string {
	switch godotType {
	case "bool":
		return "false"
	case "int":
		return "0"
	case "float":
		return "0"
	case "String", "StringName", "NodePath":
		return "\"\""
	default:
		// Builtin struct types are zero-initialized via `var v Vector2; return v`
		// in the template; no expression needed.
		return ""
	}
}

func pascal(snake string) string {
	if snake == "" {
		return ""
	}
	parts := strings.Split(snake, "_")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, "")
}

// safeGoIdent rewrites Godot argument names that collide with Go keywords or
// predeclared identifiers.
func safeGoIdent(name string) string {
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
	}
	return name
}

var builtinTmpl = template.Must(template.New("builtin").Funcs(template.FuncMap{
	"lower":  strings.ToLower,
	"pascal": pascal,
	"quote":  func(s string) string { return fmt.Sprintf("%q", s) },
}).Parse(`// Code generated by godot-go-bindgen. DO NOT EDIT.

package variant

import (
	"unsafe"

	"github.com/legendary-code/godot-go/internal/gdextension"
)

// {{.GoName}} is the opaque Godot builtin {{.GoName}} ({{.Size}} bytes under
// the framework's float_32 build config). The backing storage is an opaque
// byte array; field reads/writes go through offset accessors below.
type {{.GoName}} [{{.Size}}]byte

{{range .Members}}
// {{.GoName}} reads the {{.GoName}} field.
func (v *{{$.GoName}}) {{.GoName}}() {{.GoType}} {
	return *(*{{.GoType}})(unsafe.Pointer(&v[{{.Offset}}]))
}

// Set{{.GoName}} writes the {{.GoName}} field.
func (v *{{$.GoName}}) Set{{.GoName}}(value {{.GoType}}) {
	*(*{{.GoType}})(unsafe.Pointer(&v[{{.Offset}}])) = value
}
{{end}}

// Cached resolved function pointers. Populated by {{.GoName | lower}}Init at
// CORE init level (the host's interface table is loaded before then).
var (
{{range .Constructors}}	{{$.GoName | lower}}{{.VarID | pascal}} gdextension.PtrConstructor
{{end -}}
{{range .Methods}}	{{$.GoName | lower}}{{.VarID | pascal}} gdextension.PtrBuiltInMethod
{{end -}}
	{{.GoName | lower}}FromType gdextension.VariantFromTypeFunc
	{{.GoName | lower}}ToType   gdextension.VariantToTypeFunc
{{if .HasDestructor}}	{{.GoName | lower}}Dtor    gdextension.PtrDestructor
{{end -}}
)

func init() {
	gdextension.RegisterInitCallback(gdextension.InitLevelCore, {{.GoName | lower}}Init)
}

func {{.GoName | lower}}Init() {
{{range .Constructors}}	{{$.GoName | lower}}{{.VarID | pascal}} = gdextension.GetPtrConstructor(gdextension.{{$.VariantType}}, {{.Index}})
{{end -}}
{{range .Methods}}	{{$.GoName | lower}}{{.VarID | pascal}} = gdextension.GetPtrBuiltinMethod(gdextension.{{$.VariantType}}, internStringName({{.GodotName | quote}}), {{.Hash}})
{{end -}}
	{{.GoName | lower}}FromType = gdextension.GetVariantFromTypeConstructor(gdextension.{{.VariantType}})
	{{.GoName | lower}}ToType = gdextension.GetVariantToTypeConstructor(gdextension.{{.VariantType}})
{{if .HasDestructor}}	{{.GoName | lower}}Dtor = gdextension.GetPtrDestructor(gdextension.{{.VariantType}})
{{end -}}
}

{{range .Constructors}}
// {{.GoName}} constructs a {{$.GoName}} via the host (constructor index {{.Index}}).
func {{.GoName}}({{range $i, $a := .Args}}{{if $i}}, {{end}}{{$a.GoName}} {{$a.GoType}}{{end}}) {{$.GoName}} {
	var v {{$.GoName}}
{{- if .Args}}
	args := [...]gdextension.TypePtr{
{{- range .Args}}
		gdextension.TypePtr(unsafe.Pointer(&{{.GoName}})),
{{- end}}
	}
	gdextension.CallPtrConstructor({{$.GoName | lower}}{{.VarID | pascal}}, gdextension.TypePtr(unsafe.Pointer(&v)), args[:])
{{- else}}
	gdextension.CallPtrConstructor({{$.GoName | lower}}{{.VarID | pascal}}, gdextension.TypePtr(unsafe.Pointer(&v)), nil)
{{- end}}
	return v
}
{{end}}

{{range .Methods}}
// {{.GoName}} mirrors the Godot {{$.GoName}}.{{.GodotName}} method.
func (v *{{$.GoName}}) {{.GoName}}({{range $i, $a := .Args}}{{if $i}}, {{end}}{{$a.GoName}} {{$a.GoType}}{{end}}){{if .ReturnGoType}} {{.ReturnGoType}}{{end}} {
{{- if .ReturnGoType}}
	var ret {{.ReturnGoType}}
{{- end}}
{{- if .Args}}
	args := [...]gdextension.TypePtr{
{{- range .Args}}
		gdextension.TypePtr(unsafe.Pointer(&{{.GoName}})),
{{- end}}
	}
	gdextension.CallPtrBuiltinMethod({{$.GoName | lower}}{{.VarID | pascal}}, gdextension.TypePtr(unsafe.Pointer(v)), args[:],{{if .ReturnGoType}} gdextension.TypePtr(unsafe.Pointer(&ret)){{else}} nil{{end}})
{{- else}}
	gdextension.CallPtrBuiltinMethod({{$.GoName | lower}}{{.VarID | pascal}}, gdextension.TypePtr(unsafe.Pointer(v)), nil,{{if .ReturnGoType}} gdextension.TypePtr(unsafe.Pointer(&ret)){{else}} nil{{end}})
{{- end}}
{{- if .ReturnGoType}}
	return ret
{{- end}}
}
{{end}}

// ToVariant copies v into the uninitialized Variant slot dst. dst must point
// to variantSize bytes (16 under float_32, sized by builtin_class_sizes).
func (v *{{.GoName}}) ToVariant(dst gdextension.VariantPtr) {
	gdextension.CallVariantFromType({{.GoName | lower}}FromType, dst, gdextension.TypePtr(unsafe.Pointer(v)))
}

// {{.GoName}}FromVariant unwraps a Variant slot into a fresh {{.GoName}}.
func {{.GoName}}FromVariant(src gdextension.VariantPtr) {{.GoName}} {
	var v {{.GoName}}
	gdextension.CallTypeFromVariant({{.GoName | lower}}ToType, gdextension.TypePtr(unsafe.Pointer(&v)), src)
	return v
}
`))

