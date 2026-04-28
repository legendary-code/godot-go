package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"io"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/legendary-code/godot-go/internal/naming"
)

// emit renders the binding file for the discovered class to w. Phase 5d
// scope: emit registration glue + instance methods that take primitive
// args and return a primitive value (bool / int / int32 / int64 /
// float32 / float64 / string). Static and virtual-candidate methods are
// flagged as unsupported (Phase 5e); methods touching non-primitive
// types are rejected with file:line context by resolveType.
//
// The emitter writes gofmt'd source so diffs against hand-written
// references stay clean. Iteration order is deterministic — methods are
// emitted in source-position order, mirroring the discovered slice.
func emit(w io.Writer, fset *token.FileSet, d *discovered) error {
	if d.MainClass == nil {
		return fmt.Errorf("no main class to emit")
	}

	enumSet := map[string]bool{}
	for _, e := range d.Enums {
		enumSet[e.Name] = true
	}

	// Bindings alias: the local name the user's source uses for the
	// bindings package (e.g. "godot"). The parent's import comes from
	// the @extends embed; that same import IS the bindings package
	// (single-package layout means engine classes and variant helpers
	// live together). Walk the user's imports to find the local name.
	bindingsImport := d.MainClass.ParentImport
	bindingsAlias := localNameFor(d.Imports, bindingsImport)
	if bindingsAlias == "" {
		return fmt.Errorf("internal: bindings import %q not in file imports — discover should have rejected this", bindingsImport)
	}

	supported, needsVariant, err := classifyForEmit(fset, d.MainClass.Methods, enumSet, bindingsAlias)
	if err != nil {
		return err
	}

	accessors, propMethods, properties, err := buildEmitProperties(d.MainClass.Properties, enumSet, bindingsAlias)
	if err != nil {
		return err
	}
	supported = append(supported, propMethods...)
	if len(accessors) > 0 || len(properties) > 0 {
		needsVariant = true
	}

	signals, err := buildEmitSignals(d.MainClass.Signals, enumSet, bindingsAlias)
	if err != nil {
		return err
	}
	if len(signals) > 0 {
		needsVariant = true
	}

	initLevel := "InitLevelScene"
	if d.MainClass.IsEditor {
		initLevel = "InitLevelEditor"
	}

	data := emitData{
		PackageName:    d.PackageName,
		SourceFile:     filepath.Base(d.FilePath),
		Class:          d.MainClass.Name,
		Parent:         d.MainClass.Parent,
		Lower:          lowerFirst(d.MainClass.Name),
		IsAbstract:     d.MainClass.IsAbstract,
		InitLevel:      initLevel,
		Methods:        supported,
		Accessors:      accessors,
		Properties:     properties,
		Signals:        signals,
		NeedsVariant:   needsVariant,
		BindingsImport: bindingsImport,
		BindingsAlias:  bindingsAlias,
	}

	var buf bytes.Buffer
	if err := bindingTemplate.Execute(&buf, data); err != nil {
		return fmt.Errorf("template: %w", err)
	}
	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return fmt.Errorf("gofmt emitted source: %w\n--- raw ---\n%s", err, buf.String())
	}
	_, err = w.Write(formatted)
	return err
}

type emitData struct {
	PackageName  string
	SourceFile   string
	Class        string
	Parent       string
	Lower        string // class name with first rune lowercased — used for unexported helpers
	IsAbstract   bool   // @abstract on the main class — passed through to RegisterClass
	InitLevel    string // bare InitializationLevel const name — "InitLevelScene" or "InitLevelEditor"
	Methods      []emitMethod
	Accessors    []emitAccessor // synthesized GetX/SetX Go methods for field-form @property declarations
	Properties   []emitProperty // RegisterClassProperty calls (one per @property, both forms)
	Signals      []emitSignal   // RegisterClassSignal + synthesized emit method per @signals method
	NeedsVariant bool           // any method touches a primitive (gates the bindings-package import)

	// BindingsImport / BindingsAlias identify the user's bindings
	// package — the import path and the local name the source uses for
	// it (e.g. "github.com/foo/me/godot" + "godot"). Variant marshal
	// helpers (VariantAsInt64, NewVariantString, …) live there in the
	// single-package layout, so generated calls are qualified through
	// the alias.
	BindingsImport string
	BindingsAlias  string
}

// emitSignal is one signal declared on a `@signals` interface. The emitter
// renders two pieces per signal: a method on *<Class> that constructs the
// per-arg Variants and dispatches Object::emit_signal, plus a
// RegisterClassSignal call at SCENE init.
type emitSignal struct {
	GoName    string // method identifier on the @signals interface ("Damaged")
	GodotName string // snake_case for ClassDB ("damaged")
	// PerArg holds the rendered fragments needed to construct one Variant
	// per signal arg before dispatch. Each entry contributes a `var
	// argN variant.Variant = ...` statement and a Destroy via defer.
	PerArg []emitSignalArg
	// Registration metadata — parallel to ArgTypes / ArgMetadata on
	// emitMethod so RegisterClassSignal sees the same shape.
	ArgTypes []string
	ArgMetas []string
}

type emitSignalArg struct {
	Index    int    // 0-based positional arg index (matches argN naming)
	GoName   string // identifier the user method exposes for this arg
	GoType   string // Go type as it appears in the synthesized method signature
	BuildVar string // single statement that constructs the Variant, e.g.
	// `arg0 := variant.NewVariantInt(amount)`. The Variant is destroyed
	// via `defer arg0.Destroy()` rendered separately in the template.
}

// emitAccessor is one synthesized Go method that backs an exported field
// for a field-form @property. The bindings file emits these as plain Go
// source so they pass through the existing method-registration pipeline
// — `self.GetHealth()` is just another method call from the trampoline's
// perspective.
type emitAccessor struct {
	Name     string // "GetHealth" or "SetHealth"
	Field    string // "Health"
	GoType   string // type as it should render in source — "int64", "string", "Language" etc.
	IsSetter bool
}

// emitProperty is one RegisterClassProperty literal. Setter is empty for
// read-only properties.
type emitProperty struct {
	GodotName string // snake_case property name as Godot sees it
	Type      string // bare const name, e.g. "VariantTypeInt"
	Setter    string // snake_case setter method name (empty for read-only)
	Getter    string // snake_case getter method name (always set)

	// Hint + HintString render the property's editor metadata. Hint is
	// the bare PropertyHint const name (e.g. "PropertyHintRange");
	// empty means no hint specified (codegen omits the field entirely
	// in that case so the default takes over).
	Hint       string
	HintString string

	// BeginGroup / BeginSubgroup carry the group / subgroup name when
	// THIS property is the first one of that group/subgroup. Codegen
	// emits the corresponding RegisterClassPropertyGroup /
	// RegisterClassPropertySubgroup call right before the property's
	// own registration. Empty means "no transition here".
	BeginGroup    string
	BeginSubgroup string
}

type emitMethod struct {
	GoName    string
	GodotName string

	// Pre-rendered Go fragments. Each line in *ArgReads is a complete
	// statement (e.g. "arg0 := *(*int64)(gdextension.PtrCallArg(args, 0))").
	// PtrCallReturn / CallReturn are empty strings when the method has
	// no return value; otherwise each is a single statement that stores
	// the result.
	PtrCallArgReads []string
	PtrCallReturn   string
	CallArgReads    []string
	CallReturn      string

	// DispatchArgs is the argument list passed to the user's method —
	// "arg0, arg1, ..." or "" for no-arg methods.
	DispatchArgs string

	// Registration metadata for RegisterClassMethod.
	HasReturn  bool
	ReturnType string // bare const name, e.g. "VariantTypeInt"
	ReturnMeta string // bare const name, e.g. "ArgMetaIntIsInt64"
	ArgTypes   []string
	ArgMetas   []string

	// HasArgs is true when len(ArgTypes) > 0 — the template uses this to
	// decide whether to render the ArgTypes / ArgMetadata slice fields
	// on the RegisterClassMethod literal.
	HasArgs bool

	// IsStatic is true when the source method's receiver is unnamed
	// (`func (T) Foo()` or `func (*T) Foo()`). The generated dispatch
	// skips the instance-handle lookup and invokes the method on a zero
	// value; the registered MethodFlags include MethodFlagStatic so the
	// host knows GDScript callers can use `T.foo()` syntax.
	IsStatic bool

	// IsVirtual is true when the source method's Go name starts with a
	// lowercase letter — by convention these map to engine virtuals like
	// `_process`. The codegen routes them through RegisterClassVirtual
	// (not RegisterClassMethod) and prepends an `_` to GodotName unless
	// the user already supplied one via @name.
	IsVirtual bool
}

// classifyForEmit walks the discovered methods and returns:
//   - the supported methods, with arg/return marshalling fragments pre-rendered;
//   - whether the bindings package import is needed (true iff any supported
//     method has at least one arg or a return);
//   - the first reason a method couldn't be supported, with file:line.
//
// Static methods (unnamed receiver) and virtual-candidate methods
// (lowercase Go name → engine `_name` virtual) are folded in. Any
// unsupported arg/return type or multi-result return propagates as an
// error. The enums set lets resolveType accept user-defined int enum
// types declared in the same file. `bindings` is the local alias the
// user's source uses for the bindings package; it qualifies every
// VariantAs* / VariantSet* call in the rendered fragments.
func classifyForEmit(fset *token.FileSet, methods []*methodInfo, enums map[string]bool, bindings string) ([]emitMethod, bool, error) {
	var out []emitMethod
	needsVariant := false
	for _, m := range methods {
		em, err := buildEmitMethod(m, enums, bindings)
		if err != nil {
			return nil, false, fmt.Errorf("%s: %s: %w", posStr(fset, m.Pos), m.GoName, err)
		}
		if em.HasArgs || em.HasReturn {
			needsVariant = true
		}
		out = append(out, em)
	}
	return out, needsVariant, nil
}

// buildEmitMethod resolves arg / return types via the type table and
// pre-renders the marshalling fragments. The caller is responsible for
// rejecting non-instance method kinds before calling.
func buildEmitMethod(m *methodInfo, enums map[string]bool, bindings string) (emitMethod, error) {
	em := emitMethod{
		GoName:    m.GoName,
		GodotName: m.GodotName,
		IsStatic:  m.Kind == methodStatic,
		IsVirtual: m.Kind == methodOverride,
	}

	// Args: every parameter spec can declare multiple names sharing one
	// type (Go's `func f(a, b int)` shape), so flatten to one logical
	// arg per name.
	idx := 0
	for _, field := range fieldsOf(m.Decl.Type.Params) {
		info, err := resolveType(field.Type, enums)
		if err != nil {
			return em, fmt.Errorf("arg %d: %w", idx, err)
		}
		count := len(field.Names)
		if count == 0 {
			count = 1 // unnamed param — still counts as one positional slot
		}
		for k := 0; k < count; k++ {
			em.PtrCallArgReads = append(em.PtrCallArgReads, info.PtrCallReadArg(bindings, idx))
			em.CallArgReads = append(em.CallArgReads, info.CallReadArg(bindings, idx))
			em.ArgTypes = append(em.ArgTypes, info.VariantType)
			em.ArgMetas = append(em.ArgMetas, info.ArgMeta)
			idx++
		}
	}
	em.HasArgs = idx > 0

	// Dispatch arg list: arg0, arg1, ...
	if em.HasArgs {
		names := make([]string, idx)
		for i := 0; i < idx; i++ {
			names[i] = fmt.Sprintf("arg%d", i)
		}
		em.DispatchArgs = strings.Join(names, ", ")
	}

	// Return: at most one named-or-unnamed result. Multi-return is
	// disallowed (Godot methods have a single return slot).
	results := fieldsOf(m.Decl.Type.Results)
	switch len(results) {
	case 0:
		// no-op
	case 1:
		ret := results[0]
		if len(ret.Names) > 1 {
			return em, fmt.Errorf("multi-name return field unsupported")
		}
		info, err := resolveType(ret.Type, enums)
		if err != nil {
			return em, fmt.Errorf("return: %w", err)
		}
		em.HasReturn = true
		em.ReturnType = info.VariantType
		em.ReturnMeta = info.ArgMeta
		em.PtrCallReturn = info.PtrCallWriteReturn(bindings, "result")
		em.CallReturn = info.CallWriteReturn(bindings, "result")
	default:
		return em, fmt.Errorf("multi-return methods unsupported (Godot exposes a single return slot)")
	}

	return em, nil
}

// localNameFor returns the local alias for an import path in the user's
// file, falling back to the path's last segment when no explicit alias
// is set. Empty if the path isn't imported by the file at all.
func localNameFor(imports map[string]string, path string) string {
	for local, p := range imports {
		if p == path {
			return local
		}
	}
	return ""
}

// buildEmitProperties walks the discovered @property declarations and
// returns:
//   - the synthesized accessor source for field-form properties (renders
//     as plain Go in the bindings file before the registrations);
//   - the synthesized emitMethods for those accessors (so they go through
//     the same RegisterClassMethod path user methods do — no special
//     dispatch in the template);
//   - the per-property RegisterClassProperty entries (both forms).
//
// Method-form properties contribute a property entry only — the user's
// Get/Set methods already live in the source file and were classified by
// classifyForEmit upstream.
func buildEmitProperties(props []*propertyInfo, enums map[string]bool, bindings string) (
	accessors []emitAccessor,
	methods []emitMethod,
	out []emitProperty,
	err error,
) {
	// Track the active group/subgroup as we walk in source order so
	// each transition emits exactly one RegisterClassPropertyGroup or
	// RegisterClassPropertySubgroup call, attached to the first
	// property of the new group/subgroup. Discovery already validated
	// that ordering is contiguous, so a simple "did the name change"
	// check suffices here.
	curGroup := ""
	curSubgroup := ""
	for _, p := range props {
		info, terr := resolveType(p.GoType, enums)
		if terr != nil {
			return nil, nil, nil, fmt.Errorf("@property %s: %w", p.Name, terr)
		}
		getterGoName := "Get" + p.Name
		getterGodotName := naming.PascalToSnake(getterGoName)
		setterGoName := "Set" + p.Name
		setterGodotName := naming.PascalToSnake(setterGoName)
		propGodotName := naming.PascalToSnake(p.Name)

		entry := emitProperty{
			GodotName:  propGodotName,
			Type:       info.VariantType,
			Getter:     getterGodotName,
			Hint:       p.Hint,
			HintString: p.HintString,
		}
		if !p.ReadOnly {
			entry.Setter = setterGodotName
		}
		// Group transition: emit a RegisterClassPropertyGroup call
		// before this property's registration if we're entering a new
		// group. A new group also resets the subgroup tracker because
		// subgroups nest under their group.
		if p.Group != curGroup {
			entry.BeginGroup = p.Group
			curGroup = p.Group
			curSubgroup = ""
		}
		if p.Subgroup != curSubgroup {
			entry.BeginSubgroup = p.Subgroup
			curSubgroup = p.Subgroup
		}
		out = append(out, entry)

		if p.Source != propertyFromField {
			continue
		}
		// Field form: synthesize the Go accessors and the matching
		// emitMethods. The synthesized methods dispatch via direct field
		// access — `return n.<Field>` and `n.<Field> = v` — so we don't
		// need a template branch; the emit pipeline's `self.GetX()` /
		// `self.SetX(arg0)` calls just resolve to the synthesized methods
		// in the same package.
		accessors = append(accessors, emitAccessor{
			Name:   getterGoName,
			Field:  p.Name,
			GoType: info.GoType,
		})
		methods = append(methods, syntheticGetterMethod(getterGoName, getterGodotName, info, bindings))

		if p.ReadOnly {
			continue
		}
		accessors = append(accessors, emitAccessor{
			Name:     setterGoName,
			Field:    p.Name,
			GoType:   info.GoType,
			IsSetter: true,
		})
		methods = append(methods, syntheticSetterMethod(setterGoName, setterGodotName, info, bindings))
	}
	return accessors, methods, out, nil
}

// buildEmitSignals walks the discovered @signals declarations and produces
// the emitter-side records: per-signal arg marshaling fragments and the
// type/metadata slices that go into RegisterClassSignal.
//
// Each signal's arguments are resolved through the same type table used
// by regular methods, so signal arg type support tracks method arg type
// support — adding a new arg type to types.go automatically lights it up
// for signals too.
//
// Note: the synthesized `func (n *T) <SignalName>(args...)` method is a
// Go-only API for the user's own code to fire signals from inside class
// methods. We do NOT register it with ClassDB; GDScript callers use the
// standard `emit_signal("name", args...)` to trigger emission from
// outside the class, matching Godot's idiomatic model.
func buildEmitSignals(signals []*signalInfo, enums map[string]bool, bindings string) ([]emitSignal, error) {
	if len(signals) == 0 {
		return nil, nil
	}
	out := make([]emitSignal, 0, len(signals))
	for _, s := range signals {
		es := emitSignal{
			GoName:    s.Name,
			GodotName: s.GodotName,
		}
		idx := 0
		for _, field := range s.Args {
			info, err := resolveType(field.Type, enums)
			if err != nil {
				return nil, fmt.Errorf("@signals %s arg %d: %w", s.Name, idx, err)
			}
			count := len(field.Names)
			if count == 0 {
				count = 1
			}
			for k := 0; k < count; k++ {
				goArgName := fmt.Sprintf("arg%d", idx)
				if k < len(field.Names) && field.Names[k].Name != "" && field.Names[k].Name != "_" {
					goArgName = field.Names[k].Name
				}
				es.PerArg = append(es.PerArg, emitSignalArg{
					Index:    idx,
					GoName:   goArgName,
					GoType:   info.GoType,
					BuildVar: info.BuildVariant(bindings, idx, goArgName),
				})
				es.ArgTypes = append(es.ArgTypes, info.VariantType)
				es.ArgMetas = append(es.ArgMetas, info.ArgMeta)
				idx++
			}
		}
		out = append(out, es)
	}
	return out, nil
}

// syntheticGetterMethod hand-builds an emitMethod equivalent to what
// classifyForEmit would produce for `func (n *T) Get<Name>() <Type>`.
// Dispatch uses the same `result := self.<GoName>()` shape the template
// already emits — the synthesized Go method we render alongside is what
// closes the loop.
func syntheticGetterMethod(goName, godotName string, info *typeInfo, bindings string) emitMethod {
	return emitMethod{
		GoName:        goName,
		GodotName:     godotName,
		HasReturn:     true,
		ReturnType:    info.VariantType,
		ReturnMeta:    info.ArgMeta,
		PtrCallReturn: info.PtrCallWriteReturn(bindings, "result"),
		CallReturn:    info.CallWriteReturn(bindings, "result"),
	}
}

// syntheticSetterMethod hand-builds an emitMethod equivalent to
// `func (n *T) Set<Name>(v <Type>)` — one arg, no return.
func syntheticSetterMethod(goName, godotName string, info *typeInfo, bindings string) emitMethod {
	return emitMethod{
		GoName:          goName,
		GodotName:       godotName,
		PtrCallArgReads: []string{info.PtrCallReadArg(bindings, 0)},
		CallArgReads:    []string{info.CallReadArg(bindings, 0)},
		DispatchArgs:    "arg0",
		HasArgs:         true,
		ArgTypes:        []string{info.VariantType},
		ArgMetas:        []string{info.ArgMeta},
	}
}

// fieldsOf is a nil-safe accessor for *ast.FieldList.List.
func fieldsOf(fl *ast.FieldList) []*ast.Field {
	if fl == nil {
		return nil
	}
	return fl.List
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

// bindingTemplate produces a Go file with three sections: per-class side
// table (handle ↔ *T map), a register<Class>() function that calls
// RegisterClass and RegisterClassMethod for each method, and an init()
// that wires SCENE init/deinit so the user's main package doesn't need
// to know about it.
//
// Per-method Call / PtrCall bodies inline arg reads and return writes
// when the method takes args or returns a value; for no-arg / no-return
// methods the bodies stay tight (no `args` / `ret` accesses, fewer
// allocations).
var bindingTemplate = template.Must(template.New("binding").Parse(`// Code generated by godot-go; DO NOT EDIT.
// Source: {{.SourceFile}}

package {{.PackageName}}

import (
	"sync"
	"unsafe"

	"github.com/legendary-code/godot-go/gdextension"
{{if .NeedsVariant}}	{{.BindingsAlias}} "{{.BindingsImport}}"
{{end}})

// Per-instance side table. The void* value the host hands back to us as
// p_instance is the small integer id we returned from Construct, not a
// Go pointer (cgo forbids storing Go pointers in C-visible memory).
var (
	{{.Lower}}InstancesMu sync.Mutex
	{{.Lower}}Instances   = map[uintptr]*{{.Class}}{}
	{{.Lower}}NextID      uintptr
)

func register{{.Class}}Instance(n *{{.Class}}) unsafe.Pointer {
	{{.Lower}}InstancesMu.Lock()
	{{.Lower}}NextID++
	id := {{.Lower}}NextID
	{{.Lower}}Instances[id] = n
	{{.Lower}}InstancesMu.Unlock()
	return unsafe.Pointer(id)
}

func lookup{{.Class}}Instance(handle unsafe.Pointer) *{{.Class}} {
	{{.Lower}}InstancesMu.Lock()
	defer {{.Lower}}InstancesMu.Unlock()
	return {{.Lower}}Instances[uintptr(handle)]
}

func release{{.Class}}Instance(handle unsafe.Pointer) {
	{{.Lower}}InstancesMu.Lock()
	defer {{.Lower}}InstancesMu.Unlock()
	delete({{.Lower}}Instances, uintptr(handle))
}
{{range .Accessors}}
{{if .IsSetter -}}
func (n *{{$.Class}}) {{.Name}}(v {{.GoType}}) { n.{{.Field}} = v }
{{else -}}
func (n *{{$.Class}}) {{.Name}}() {{.GoType}} { return n.{{.Field}} }
{{end -}}
{{end}}
{{range .Signals}}
// {{.GoName}} emits the {{.GodotName}} signal on this instance. Synthesized
// from a @signals interface declaration; per-arg Variants are constructed
// from the typed parameters and dispatched via Object::emit_signal.
func (n *{{$.Class}}) {{.GoName}}({{range $i, $a := .PerArg}}{{if $i}}, {{end}}{{$a.GoName}} {{$a.GoType}}{{end}}) {
	{{- range .PerArg}}
	{{.BuildVar}}
	defer arg{{.Index}}.Destroy()
	{{- end}}
	{{- if .PerArg}}
	args := []gdextension.VariantPtr{
		{{- range .PerArg}}
		gdextension.VariantPtr(unsafe.Pointer(&arg{{.Index}})),
		{{- end}}
	}
	gdextension.EmitSignal(n.Ptr(), gdextension.InternStringName("{{.GodotName}}"), args)
	{{- else}}
	gdextension.EmitSignal(n.Ptr(), gdextension.InternStringName("{{.GodotName}}"), nil)
	{{- end}}
}
{{end}}
func register{{.Class}}() {
	gdextension.RegisterClass(gdextension.ClassDef{
		Name:      "{{.Class}}",
		Parent:    "{{.Parent}}",
		IsExposed: true,
{{- if .IsAbstract}}
		IsAbstract: true,
{{- end}}

		Construct: func() (gdextension.ObjectPtr, unsafe.Pointer) {
			n := &{{.Class}}{}
			parent := gdextension.ConstructObject(gdextension.InternStringName("{{.Parent}}"))
			n.BindPtr(parent)
			return parent, register{{.Class}}Instance(n)
		},

		Free: func(instance unsafe.Pointer) {
			release{{.Class}}Instance(instance)
		},
	})
{{range .Methods}}
	{{- if .IsVirtual}}
	gdextension.RegisterClassVirtual(gdextension.ClassVirtualDef{
		Class: "{{$.Class}}",
		Name:  "{{.GodotName}}",
		Call: func(instance unsafe.Pointer, args []gdextension.VariantPtr, ret gdextension.VariantPtr) gdextension.CallErrorType {
			self := lookup{{$.Class}}Instance(instance)
			if self == nil {
				return gdextension.CallErrorInstanceIsNull
			}
			{{- range .CallArgReads}}
			{{.}}
			{{- end}}
			{{- if .HasReturn}}
			result := self.{{.GoName}}({{.DispatchArgs}})
			{{.CallReturn}}
			{{- else}}
			self.{{.GoName}}({{.DispatchArgs}})
			{{- end}}
			return gdextension.CallErrorOK
		},
		PtrCall: func(instance unsafe.Pointer, args unsafe.Pointer, ret unsafe.Pointer) {
			self := lookup{{$.Class}}Instance(instance)
			if self == nil {
				return
			}
			{{- range .PtrCallArgReads}}
			{{.}}
			{{- end}}
			{{- if .HasReturn}}
			result := self.{{.GoName}}({{.DispatchArgs}})
			{{.PtrCallReturn}}
			{{- else}}
			self.{{.GoName}}({{.DispatchArgs}})
			{{- end}}
		},
		{{- if .HasReturn}}
		HasReturn:      true,
		ReturnType:     gdextension.{{.ReturnType}},
		ReturnMetadata: gdextension.{{.ReturnMeta}},
		{{- end}}
		{{- if .HasArgs}}
		ArgTypes: []gdextension.VariantType{
			{{- range .ArgTypes}}
			gdextension.{{.}},
			{{- end}}
		},
		ArgMetadata: []gdextension.MethodArgumentMetadata{
			{{- range .ArgMetas}}
			gdextension.{{.}},
			{{- end}}
		},
		{{- end}}
	})
	{{- else}}
	gdextension.RegisterClassMethod(gdextension.ClassMethodDef{
		Class: "{{$.Class}}",
		Name:  "{{.GodotName}}",
		Call: func(instance unsafe.Pointer, args []gdextension.VariantPtr, ret gdextension.VariantPtr) gdextension.CallErrorType {
			{{- if .IsStatic}}
			_ = instance
			var self {{$.Class}}
			{{- else}}
			self := lookup{{$.Class}}Instance(instance)
			if self == nil {
				return gdextension.CallErrorInstanceIsNull
			}
			{{- end}}
			{{- range .CallArgReads}}
			{{.}}
			{{- end}}
			{{- if .HasReturn}}
			result := self.{{.GoName}}({{.DispatchArgs}})
			{{.CallReturn}}
			{{- else}}
			self.{{.GoName}}({{.DispatchArgs}})
			{{- end}}
			return gdextension.CallErrorOK
		},
		PtrCall: func(instance unsafe.Pointer, args unsafe.Pointer, ret unsafe.Pointer) {
			{{- if .IsStatic}}
			_ = instance
			var self {{$.Class}}
			{{- else}}
			self := lookup{{$.Class}}Instance(instance)
			if self == nil {
				return
			}
			{{- end}}
			{{- range .PtrCallArgReads}}
			{{.}}
			{{- end}}
			{{- if .HasReturn}}
			result := self.{{.GoName}}({{.DispatchArgs}})
			{{.PtrCallReturn}}
			{{- else}}
			self.{{.GoName}}({{.DispatchArgs}})
			{{- end}}
		},
		{{- if .IsStatic}}
		Flags: gdextension.MethodFlagsDefault | gdextension.MethodFlagStatic,
		{{- end}}
		{{- if .HasReturn}}
		HasReturn:      true,
		ReturnType:     gdextension.{{.ReturnType}},
		ReturnMetadata: gdextension.{{.ReturnMeta}},
		{{- end}}
		{{- if .HasArgs}}
		ArgTypes: []gdextension.VariantType{
			{{- range .ArgTypes}}
			gdextension.{{.}},
			{{- end}}
		},
		ArgMetadata: []gdextension.MethodArgumentMetadata{
			{{- range .ArgMetas}}
			gdextension.{{.}},
			{{- end}}
		},
		{{- end}}
	})
	{{- end}}
{{end}}
{{- range .Properties}}
	{{- if .BeginGroup}}
	gdextension.RegisterClassPropertyGroup(gdextension.ClassPropertyGroupDef{
		Class: "{{$.Class}}",
		Name:  "{{.BeginGroup}}",
	})
	{{- else if and (eq .BeginGroup "") (ne .BeginSubgroup "")}}
	{{- /* Subgroup transition without a group transition — only emit
	       the subgroup call. The group is unchanged. */}}
	{{- end}}
	{{- if .BeginSubgroup}}
	gdextension.RegisterClassPropertySubgroup(gdextension.ClassPropertyGroupDef{
		Class: "{{$.Class}}",
		Name:  "{{.BeginSubgroup}}",
	})
	{{- end}}
	gdextension.RegisterClassProperty(gdextension.ClassPropertyDef{
		Class:  "{{$.Class}}",
		Name:   "{{.GodotName}}",
		Type:   gdextension.{{.Type}},
		{{- if .Setter}}
		Setter: "{{.Setter}}",
		{{- end}}
		Getter: "{{.Getter}}",
		{{- if .Hint}}
		Hint:       gdextension.{{.Hint}},
		{{- end}}
		{{- if .HintString}}
		HintString: {{printf "%q" .HintString}},
		{{- end}}
	})
{{end}}
{{- range .Signals}}
	gdextension.RegisterClassSignal(gdextension.ClassSignalDef{
		Class: "{{$.Class}}",
		Name:  "{{.GodotName}}",
		{{- if .ArgTypes}}
		ArgTypes: []gdextension.VariantType{
			{{- range .ArgTypes}}
			gdextension.{{.}},
			{{- end}}
		},
		ArgMetadata: []gdextension.MethodArgumentMetadata{
			{{- range .ArgMetas}}
			gdextension.{{.}},
			{{- end}}
		},
		{{- end}}
	})
{{end}}}

func init() {
	gdextension.RegisterInitCallback(gdextension.{{.InitLevel}}, register{{.Class}})
	gdextension.RegisterDeinitCallback(gdextension.{{.InitLevel}}, func() {
		gdextension.UnregisterClass("{{.Class}}")
	})
}
`))
