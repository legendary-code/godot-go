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

	enumSet := map[string]*enumInfo{}
	for _, e := range d.Enums {
		enumSet[e.Name] = e
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

	initLevel := "InitLevelScene"
	if d.MainClass.IsEditor {
		initLevel = "InitLevelEditor"
	}

	ci := d.MainClass
	supported, needsVariant, err := classifyForEmit(fset, ci.Methods, enumSet, bindingsAlias, ci.Name)
	if err != nil {
		return err
	}

	accessors, propMethods, properties, err := buildEmitProperties(ci.Properties, enumSet, bindingsAlias, ci.Name)
	if err != nil {
		return err
	}
	supported = append(supported, propMethods...)
	if len(accessors) > 0 || len(properties) > 0 {
		needsVariant = true
	}

	signals, err := buildEmitSignals(ci.Signals, enumSet, bindingsAlias, ci.Name)
	if err != nil {
		return err
	}
	if len(signals) > 0 {
		needsVariant = true
	}

	data := emitData{
		PackageName:    d.PackageName,
		SourceFile:     filepath.Base(d.FilePath),
		InitLevel:      initLevel,
		Class:          ci.Name,
		Parent:         ci.Parent,
		Lower:          lowerFirst(ci.Name),
		IsAbstract:     ci.IsAbstract,
		Methods:        supported,
		Accessors:      accessors,
		Properties:     properties,
		Signals:        signals,
		Enums:          buildEmitEnums(d.Enums),
		NeedsVariant:   needsVariant,
		BindingsImport: bindingsImport,
		BindingsAlias:  bindingsAlias,
	}

	// Render the class XML now that the per-class data is populated.
	// Classes with no documentation surface at all skip emit — the
	// template skips the LoadEditorDocXML call when DocXML is empty.
	if emitHasDocSurface(ci, data) {
		docXML, xerr := buildClassXML(data, ci.docInfo, ci.Brief, ci.Tutorials)
		if xerr != nil {
			return fmt.Errorf("build doc XML: %w", xerr)
		}
		data.DocXML = docXML
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

// emitData is the single-class payload threaded through the bindings
// template. One file = one @class = one emitData.
type emitData struct {
	PackageName string
	SourceFile  string
	InitLevel   string // bare InitializationLevel const name — "InitLevelScene" or "InitLevelEditor"

	Class      string
	Parent     string
	Lower      string // class name with first rune lowercased — used for unexported helpers
	IsAbstract bool   // @abstract on this class — passed through to RegisterClass
	Methods    []emitMethod
	Accessors  []emitAccessor // synthesized GetX/SetX Go methods for field-form @property declarations
	Properties []emitProperty // RegisterClassProperty calls (one per @property, both forms)
	Signals    []emitSignal   // RegisterClassSignal + synthesized emit method per @signals method
	Enums      []emitEnum     // @enum / @bitfield-tagged user enums registered as class-scoped integer constants
	// DocXML is the rendered <class>…</class> document baked into the
	// bindings as a const string. Empty when the class has no doc-able
	// content (no description, no docs on any declaration); the
	// template skips the LoadEditorDocXML call when empty.
	DocXML string

	NeedsVariant bool // any method touches a primitive (gates the bindings-package import)

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
	docInfo

	GoName    string // method identifier on the @signals interface ("Damaged")
	GodotName string // snake_case for ClassDB ("damaged")

	// ArgGodotTypes carries Godot-XML type names per arg, parallel to
	// ArgTypes. Used by the XML emitter on the signal's `<param>` rows.
	ArgGodotTypes []string
	// PerArg holds the rendered fragments needed to construct one Variant
	// per signal arg before dispatch. Each entry contributes a `var
	// argN variant.Variant = ...` statement and a Destroy via defer.
	PerArg []emitSignalArg
	// Registration metadata — parallel to ArgTypes / ArgMetadata on
	// emitMethod so RegisterClassSignal sees the same shape. ArgNames
	// carries the source-level identifier per arg; quoted as a Go string
	// literal in the template. ArgClassNames carries typed-enum identities
	// the same way (empty for primitives / untagged ints).
	ArgTypes      []string
	ArgMetas      []string
	ArgNames      []string
	ArgClassNames []string
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

// emitEnum carries one user-defined int enum tagged @enum or
// @bitfield. Each value gets a RegisterClassIntegerConstant call at
// SCENE init so the editor recognizes the typed-enum identity that
// the method/property/signal registrations reference via their
// ArgClassNames / ReturnClassName / ClassName fields.
type emitEnum struct {
	docInfo

	GoName     string // bare Go type identifier (e.g. "Mode")
	IsBitfield bool   // @bitfield instead of @enum — registers with is_bitfield=true
	Values     []emitEnumValue
}

type emitEnumValue struct {
	docInfo

	GoName    string // Go const identifier (e.g. "ModeIdle")
	GodotName string // SCREAMING_SNAKE form Godot expects (e.g. "IDLE", prefix stripped)
	Value     int64  // the int value the Go const evaluates to (resolved by go/types)
}

// emitProperty is one RegisterClassProperty literal. Setter is empty for
// read-only properties.
type emitProperty struct {
	docInfo

	GodotName string // snake_case property name as Godot sees it
	Type      string // bare const name, e.g. "VariantTypeInt"
	GodotType string // Godot-XML type name ("int", "String", "bool", …) for <member type="...">
	ClassName string // typed-enum identity ("<MainClass>.<EnumName>") or empty
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
	docInfo

	GoName    string
	GodotName string

	// IsConst is reserved for future use — Godot's XML schema has a
	// `qualifiers="const"` attribute on methods that can be invoked on
	// const objects. Go has no const-method concept, so the codegen
	// always leaves this false today.
	IsConst bool

	// ArgGodotTypes / ReturnGodotType carry the Godot-XML type name
	// for each arg / the return ("int", "float", "bool", "String",
	// etc.) — used by the XML emitter to fill in `type="..."` on
	// `<param>` and `<return>`. Parallel to ArgTypes (the variant-type
	// const name); the XML side wants the human-readable form.
	ArgGodotTypes   []string
	ReturnGodotType string

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
	// ArgNames carries the source-level identifier for each arg (one
	// entry per positional slot, parallel to ArgTypes / ArgMetas). The
	// template renders these as Go string literals into the
	// RegisterClassMethod ArgNames field; safeIdent runs on them upstream
	// so reserved Go keywords don't collide with locals.
	ArgNames []string
	// ArgClassNames carries the typed-enum identity for each arg
	// (parallel to ArgTypes). "<MainClass>.<EnumName>" for @enum/@bitfield
	// types; empty otherwise. ReturnClassName is the same for the return
	// type. Empty entries register as untyped — fine for primitives.
	ArgClassNames   []string
	ReturnClassName string

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

	// IsVariadic is true when the source method's last parameter is
	// declared with Go's `...T` ellipsis syntax. The wire boundary still
	// passes a single Packed<X>Array / Array[T] arg — Godot has no
	// vararg concept on extension methods — but the dispatch site spreads
	// the slice with `argN...` so the user method receives the variadic
	// it expects.
	IsVariadic bool
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
func classifyForEmit(fset *token.FileSet, methods []*methodInfo, enums map[string]*enumInfo, bindings, mainClass string) ([]emitMethod, bool, error) {
	var out []emitMethod
	needsVariant := false
	for _, m := range methods {
		em, err := buildEmitMethod(m, enums, bindings, mainClass)
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
func buildEmitMethod(m *methodInfo, enums map[string]*enumInfo, bindings, mainClass string) (emitMethod, error) {
	em := emitMethod{
		docInfo:   m.docInfo,
		GoName:    m.GoName,
		GodotName: m.GodotName,
		IsStatic:  m.Kind == methodStatic,
		IsVirtual: m.Kind == methodOverride,
	}

	// Args: every parameter spec can declare multiple names sharing one
	// type (Go's `func f(a, b int)` shape), so flatten to one logical
	// arg per name. A variadic param (`...T`) — only valid as the last
	// param — looks like a single slot at the wire boundary; the
	// dispatch site spreads it back into the call with `argN...`.
	idx := 0
	paramFields := fieldsOf(m.Decl.Type.Params)
	for fIdx, field := range paramFields {
		info, err := resolveType(field.Type, enums)
		if err != nil {
			return em, fmt.Errorf("arg %d: %w", idx, err)
		}
		_, isVariadic := field.Type.(*ast.Ellipsis)
		if isVariadic {
			if fIdx != len(paramFields)-1 {
				return em, fmt.Errorf("variadic parameter must be last (Go enforces this at parse time, but the codegen guards against AST oddities)")
			}
			em.IsVariadic = true
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
			em.ArgGodotTypes = append(em.ArgGodotTypes, godotXMLType(info.VariantType))
			// Arg names: take the source identifier when available so
			// the editor renders `take_damage(amount: int)` instead of
			// `take_damage(arg0: int)`. Unnamed params (`func F(int)`)
			// get an empty entry, which the registration call surfaces
			// as an anonymous slot.
			argName := ""
			if k < len(field.Names) {
				if n := field.Names[k].Name; n != "_" {
					argName = n
				}
			}
			em.ArgNames = append(em.ArgNames, argName)
			em.ArgClassNames = append(em.ArgClassNames, qualifyEnum(mainClass, info.EnumName))
			idx++
		}
	}
	em.HasArgs = idx > 0

	// Dispatch arg list: arg0, arg1, ... — the last arg gets a `...`
	// suffix when the source method is variadic so Go's call-site
	// spread reaches the user method correctly.
	if em.HasArgs {
		names := make([]string, idx)
		for i := 0; i < idx; i++ {
			names[i] = fmt.Sprintf("arg%d", i)
		}
		if em.IsVariadic {
			names[idx-1] += "..."
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
		em.ReturnGodotType = godotXMLType(info.VariantType)
		em.ReturnClassName = qualifyEnum(mainClass, info.EnumName)
		em.PtrCallReturn = info.PtrCallWriteReturn(bindings, "result")
		em.CallReturn = info.CallWriteReturn(bindings, "result")
	default:
		return em, fmt.Errorf("multi-return methods unsupported (Godot exposes a single return slot)")
	}

	return em, nil
}

// qualifyEnum joins the main class and an exposed enum name so the
// resulting "<Class>.<Enum>" matches what RegisterClassIntegerConstant
// uses for its (Class, Enum) registrations — that's the identity the
// editor's autocomplete resolves against. Empty enum names (the common
// primitive case) come back as empty so the registration's class_name
// field stays empty and the editor treats the slot as untyped.
func qualifyEnum(mainClass, enumName string) string {
	if enumName == "" {
		return ""
	}
	return mainClass + "." + enumName
}

// emitHasDocSurface reports whether the class has anything worth
// emitting an XML doc for. Classes without a description and no
// declarations carrying docs skip the XML entirely so the bindings
// don't carry a dead `const xml = "..."` string the editor would
// just load empty. Conservative: we emit XML when the class has a
// brief, description, deprecated/experimental marker, tutorials, or
// any method/property/signal/enum at all (since those produce their
// own rows in Godot's docs page even with no description text).
func emitHasDocSurface(ci *classInfo, d emitData) bool {
	if ci.Description != "" || ci.Brief != "" || ci.Deprecated != nil || ci.Experimental != nil ||
		ci.Since != "" || len(ci.See) > 0 || len(ci.Tutorials) > 0 {
		return true
	}
	return len(d.Methods) > 0 || len(d.Properties) > 0 ||
		len(d.Signals) > 0 || len(d.Enums) > 0
}

// godotXMLType maps a bare gdextension.VariantType const name (the
// `info.VariantType` the typeTable carries) to the Godot type name
// used in class-XML `<param type="...">`, `<return type="...">`,
// and `<member type="...">` attributes. Empty for the void return.
//
// The framework's user-method boundary supports a small primitive
// set; anything outside it (typed-enum returns, future Vector2 args,
// etc.) is handled by the caller — Variant types not in the map
// fall back to "int" for typed enums (which carry the variant type
// of the underlying int) or empty for unsupported.
func godotXMLType(variantType string) string {
	switch variantType {
	case "VariantTypeBool":
		return "bool"
	case "VariantTypeInt":
		return "int"
	case "VariantTypeFloat":
		return "float"
	case "VariantTypeString":
		return "String"
	case "":
		return ""
	}
	// Fallback: take the const-name suffix as the Godot type name (so
	// future supported types Just Work without table updates).
	const prefix = "VariantType"
	if strings.HasPrefix(variantType, prefix) {
		return variantType[len(prefix):]
	}
	return variantType
}

// godotEnumValueName converts a Go enum-constant identifier to the
// SCREAMING_SNAKE form Godot expects on registered class-scoped enum
// values. When the Go constant carries the enum's type as a prefix
// (e.g. `LanguageEnglish` under `type Language int`), the prefix is
// stripped so GDScript callers write `Language.ENGLISH`, not
// `Language.LANGUAGE_ENGLISH`. Constants without the prefix
// (`Idle` under `type Mode int`) pass through with just the
// SCREAMING_SNAKE conversion (`IDLE`).
func godotEnumValueName(enumType, valueName string) string {
	v := valueName
	if strings.HasPrefix(v, enumType) && len(v) > len(enumType) {
		v = v[len(enumType):]
	}
	return strings.ToUpper(naming.PascalToSnake(v))
}

// buildEmitEnums filters discovered user-int types down to those tagged
// @enum or @bitfield, converts each value's Go identifier to the
// SCREAMING_SNAKE form Godot expects (with type-prefix stripping), and
// returns the slice the template uses to emit one
// RegisterClassIntegerConstant call per value.
func buildEmitEnums(enums []*enumInfo) []emitEnum {
	if len(enums) == 0 {
		return nil
	}
	out := make([]emitEnum, 0, len(enums))
	for _, e := range enums {
		if !e.IsExposed {
			continue
		}
		ee := emitEnum{
			docInfo:    e.docInfo,
			GoName:     e.Name,
			IsBitfield: e.IsBitfield,
		}
		for _, v := range e.Values {
			ee.Values = append(ee.Values, emitEnumValue{
				docInfo:   v.docInfo,
				GoName:    v.Name,
				GodotName: godotEnumValueName(e.Name, v.Name),
				Value:     v.Value,
			})
		}
		out = append(out, ee)
	}
	return out
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
func buildEmitProperties(props []*propertyInfo, enums map[string]*enumInfo, bindings, mainClass string) (
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
			docInfo:    p.docInfo,
			GodotName:  propGodotName,
			Type:       info.VariantType,
			GodotType:  godotXMLType(info.VariantType),
			ClassName:  qualifyEnum(mainClass, info.EnumName),
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
		methods = append(methods, syntheticGetterMethod(getterGoName, getterGodotName, info, bindings, mainClass))

		if p.ReadOnly {
			continue
		}
		accessors = append(accessors, emitAccessor{
			Name:     setterGoName,
			Field:    p.Name,
			GoType:   info.GoType,
			IsSetter: true,
		})
		methods = append(methods, syntheticSetterMethod(setterGoName, setterGodotName, info, bindings, mainClass))
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
func buildEmitSignals(signals []*signalInfo, enums map[string]*enumInfo, bindings, mainClass string) ([]emitSignal, error) {
	if len(signals) == 0 {
		return nil, nil
	}
	out := make([]emitSignal, 0, len(signals))
	for _, s := range signals {
		es := emitSignal{
			docInfo:   s.docInfo,
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
				registeredName := ""
				if k < len(field.Names) && field.Names[k].Name != "" && field.Names[k].Name != "_" {
					goArgName = field.Names[k].Name
					registeredName = field.Names[k].Name
				}
				es.PerArg = append(es.PerArg, emitSignalArg{
					Index:    idx,
					GoName:   goArgName,
					GoType:   info.GoType,
					BuildVar: info.BuildVariant(bindings, idx, goArgName),
				})
				es.ArgTypes = append(es.ArgTypes, info.VariantType)
				es.ArgMetas = append(es.ArgMetas, info.ArgMeta)
				es.ArgNames = append(es.ArgNames, registeredName)
				es.ArgClassNames = append(es.ArgClassNames, qualifyEnum(mainClass, info.EnumName))
				es.ArgGodotTypes = append(es.ArgGodotTypes, godotXMLType(info.VariantType))
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
func syntheticGetterMethod(goName, godotName string, info *typeInfo, bindings, mainClass string) emitMethod {
	return emitMethod{
		GoName:          goName,
		GodotName:       godotName,
		HasReturn:       true,
		ReturnType:      info.VariantType,
		ReturnMeta:      info.ArgMeta,
		ReturnClassName: qualifyEnum(mainClass, info.EnumName),
		PtrCallReturn:   info.PtrCallWriteReturn(bindings, "result"),
		CallReturn:      info.CallWriteReturn(bindings, "result"),
	}
}

// syntheticSetterMethod hand-builds an emitMethod equivalent to
// `func (n *T) Set<Name>(v <Type>)` — one arg, no return.
func syntheticSetterMethod(goName, godotName string, info *typeInfo, bindings, mainClass string) emitMethod {
	return emitMethod{
		GoName:          goName,
		GodotName:       godotName,
		PtrCallArgReads: []string{info.PtrCallReadArg(bindings, 0)},
		CallArgReads:    []string{info.CallReadArg(bindings, 0)},
		DispatchArgs:    "arg0",
		HasArgs:         true,
		ArgTypes:        []string{info.VariantType},
		ArgMetas:        []string{info.ArgMeta},
		// Synthesized field-form setters take a single argument; "value"
		// is the conventional name and matches how Godot's own
		// PropertyInfo entries label property setter parameters in docs.
		ArgNames:      []string{"value"},
		ArgClassNames: []string{qualifyEnum(mainClass, info.EnumName)},
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
// goRawString renders s as a Go raw-string-literal expression. The
// common case (no backticks) is just `s` wrapped in backticks; if s
// itself contains backticks they get spliced out using a quoted-
// backtick concatenation so the generated source still parses.
// Used by the bindings template to embed the rendered class XML
// without escaping every special character via strconv.Quote.
func goRawString(s string) string {
	if !strings.Contains(s, "`") {
		return "`" + s + "`"
	}
	// Splice each backtick out of the raw form by closing the raw
	// string, concatenating a quoted backtick, then reopening — yields
	// a Go expression like ``"…"+ "`" + "…"``.
	parts := strings.Split(s, "`")
	var pieces []string
	for _, p := range parts {
		pieces = append(pieces, "`"+p+"`")
	}
	return strings.Join(pieces, "+\"`\"+")
}

var bindingTemplate = template.Must(template.New("binding").Funcs(template.FuncMap{
	"godotGoBacktick": goRawString,
}).Parse(`// Code generated by godot-go; DO NOT EDIT.
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
		{{- if .ReturnClassName}}
		ReturnClassName: {{printf "%q" .ReturnClassName}},
		{{- end}}
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
		ArgNames: []string{
			{{- range .ArgNames}}
			{{printf "%q" .}},
			{{- end}}
		},
		ArgClassNames: []string{
			{{- range .ArgClassNames}}
			{{printf "%q" .}},
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
		{{- if .ReturnClassName}}
		ReturnClassName: {{printf "%q" .ReturnClassName}},
		{{- end}}
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
		ArgNames: []string{
			{{- range .ArgNames}}
			{{printf "%q" .}},
			{{- end}}
		},
		ArgClassNames: []string{
			{{- range .ArgClassNames}}
			{{printf "%q" .}},
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
		{{- if .ClassName}}
		ClassName: {{printf "%q" .ClassName}},
		{{- end}}
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
		ArgNames: []string{
			{{- range .ArgNames}}
			{{printf "%q" .}},
			{{- end}}
		},
		ArgClassNames: []string{
			{{- range .ArgClassNames}}
			{{printf "%q" .}},
			{{- end}}
		},
		{{- end}}
	})
{{end}}
{{- range .Enums}}
	{{- $enumName := .GoName}}
	{{- $isBitfield := .IsBitfield}}
	{{- range .Values}}
	gdextension.RegisterClassIntegerConstant(gdextension.ClassIntegerConstantDef{
		Class: "{{$.Class}}",
		Enum:  "{{$enumName}}",
		Name:  "{{.GodotName}}",
		Value: {{.Value}},
		{{- if $isBitfield}}
		IsBitfield: true,
		{{- end}}
	})
	{{- end}}
{{end}}
{{- if .DocXML}}
	gdextension.LoadEditorDocXML({{.Lower}}DocXML)
{{- end}}
}
{{if .DocXML}}
// {{.Lower}}DocXML is the rendered <class> document for {{.Class}}.
// Loaded at SCENE init via editor_help_load_xml_from_utf8_chars (an
// editor-only entry point — game-mode runtimes resolve the symbol to
// nil and the load call is a no-op).
const {{.Lower}}DocXML = {{.DocXML | godotGoBacktick}}
{{end}}
func init() {
	gdextension.RegisterInitCallback(gdextension.{{.InitLevel}}, register{{.Class}})
	gdextension.RegisterDeinitCallback(gdextension.{{.InitLevel}}, func() {
		gdextension.UnregisterClass("{{.Class}}")
	})
}
`))
