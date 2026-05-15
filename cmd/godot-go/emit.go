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

// emit renders the consolidated bindings file to w. Takes every
// `@class` discovered across the package (one per source file —
// discover enforces that at the file level) and emits a single
// bindings.gen.go containing the registration glue for all of them.
//
// All classes in the package must agree on the bindings import path —
// they all extend from the same user bindings package. Mixed bindings
// imports across @class files in one package surface as an explicit
// error.
//
// The emitter writes gofmt'd source so diffs against hand-written
// references stay clean. Iteration order matches the slice order from
// main (sorted by source filename, then source position within each
// file) so the emitted file is deterministic across runs.
func emit(w io.Writer, fset *token.FileSet, classes []*discovered) error {
	if len(classes) == 0 {
		return fmt.Errorf("no @class structs to emit")
	}

	// All @class files in a package extend from the same bindings
	// package; otherwise the user's source wouldn't compile (every
	// package can have only one local name per import path).
	// Same-package extends (parentImport == "") means this class
	// extends another @class in the same package — those don't
	// contribute to bindings-import resolution; we look at non-empty
	// imports across the package and require they all match.
	bindingsImport := ""
	bindingsImportFile := ""
	for _, d := range classes {
		if d.MainClass.ParentImport == "" {
			continue
		}
		if bindingsImport == "" {
			bindingsImport = d.MainClass.ParentImport
			bindingsImportFile = d.FilePath
			continue
		}
		if d.MainClass.ParentImport != bindingsImport {
			return fmt.Errorf("mixed bindings imports in package: @class in %s extends %q but @class in %s extends %q — all classes that extend through a package alias must use the same bindings import",
				bindingsImportFile, bindingsImport,
				d.FilePath, d.MainClass.ParentImport)
		}
	}
	if bindingsImport == "" {
		return fmt.Errorf("no @class in this package extends through a bindings package — at least one class must @extends an engine class (e.g. godot.Node, godot.RefCounted) so the framework knows which bindings package to import")
	}
	// localNameFor needs to look in a file that actually imports the
	// bindings package — find it (the same one we recorded when we
	// first observed this import).
	var bindingsFileImports map[string]string
	for _, d := range classes {
		if d.FilePath == bindingsImportFile {
			bindingsFileImports = d.Imports
			break
		}
	}
	bindingsAlias := localNameFor(bindingsFileImports, bindingsImport)
	if bindingsAlias == "" {
		return fmt.Errorf("internal: bindings import %q not in file imports — discover should have rejected this", bindingsImport)
	}

	// Validate uniqueness — two @class files declaring the same class
	// name would collide at registration. Cheap check; surface a
	// clear error rather than letting the host's classdb panic.
	seen := map[string]string{}
	for _, d := range classes {
		if prev, ok := seen[d.MainClass.Name]; ok {
			return fmt.Errorf("duplicate @class name %q (declared in %s and %s)",
				d.MainClass.Name, prev, d.FilePath)
		}
		seen[d.MainClass.Name] = d.FilePath
	}

	// Package-level enum map: aggregate every @class file's enums into
	// one lookup so cross-file enum references resolve correctly. Each
	// entry's OwningClass field already carries the qualifier; here we
	// just check for collisions across files.
	packageEnums := map[string]*enumInfo{}
	enumOrigin := map[string]string{}
	for _, d := range classes {
		for _, e := range d.Enums {
			if prev, ok := enumOrigin[e.Name]; ok {
				return fmt.Errorf("duplicate enum name %q (declared in %s and %s) — package-level enum names must be unique",
					e.Name, prev, d.FilePath)
			}
			packageEnums[e.Name] = e
			enumOrigin[e.Name] = d.FilePath
		}
	}

	// Package-level user-class set: every @class declared in this
	// package's bindings.gen.go is a candidate target for `*<Class>`
	// references at the @class boundary. resolveType uses this set
	// to recognize cross-file user classes — same package means the
	// side-table lookup function `lookup<Class>ByEngine` exists in
	// the same generated file, so the same marshalling path that
	// handles self-references works without modification.
	packageClasses := map[string]bool{}
	for _, d := range classes {
		packageClasses[d.MainClass.Name] = true
	}

	// Same-package @extends validation. discover.go accepts a bare
	// ident on @extends without confirming the name exists; we close
	// that loop here, where we have the package-wide class set built
	// up. ParentImport=="" signals "@extends references a same-package
	// class" — its name must be in packageClasses.
	classByName := map[string]*classInfo{}
	for _, d := range classes {
		classByName[d.MainClass.Name] = d.MainClass
	}
	for _, d := range classes {
		if d.MainClass.ParentImport != "" {
			continue
		}
		if !packageClasses[d.MainClass.Parent] {
			return fmt.Errorf("@class %q extends %q (bare-ident, same-package), but no @class with that name was discovered in this package — declare it as a @class struct in a sibling file or use the qualified `<bindings>.<Parent>` form for an engine class",
				d.MainClass.Name, d.MainClass.Parent)
		}
	}

	// Engine-root resolution: for every user class, walk up the
	// `@extends` chain until we reach a cross-package parent
	// (ParentImport != ""). That cross-package class is the engine
	// root — what we pass to gdextension.ConstructObject in the
	// Construct hook. We can't ConstructObject same-package
	// intermediates: they may be @abstract (fails outright) or fire
	// their own Construct hooks that try to allocate a SECOND
	// engine ptr. The engine root is the only stable thing to
	// allocate; ClassDB's class hierarchy carries the inheritance
	// chain via the per-class `Parent` registrations.
	engineRoots := map[string]string{}
	for _, d := range classes {
		ci := d.MainClass
		root := ci.Parent
		visited := map[string]bool{ci.Name: true}
		for {
			pc, ok := classByName[root]
			if !ok {
				break // cross-package parent — stop walking
			}
			if visited[pc.Name] {
				return fmt.Errorf("@extends cycle detected involving %q — same-package extends must form a tree", pc.Name)
			}
			visited[pc.Name] = true
			root = pc.Parent
		}
		engineRoots[ci.Name] = root
	}

	// Build per-class emit payloads. Each @class file's enums scope to
	// its own MainClass — they're isolated per-file in the discover
	// result, so each class's enumSet is built from its own discovered
	// struct, not unified across the package.
	emitClasses := make([]emitClass, 0, len(classes))
	needsVariant := false
	for _, d := range classes {
		ec, classNeedsVariant, err := buildEmitClass(fset, d, packageEnums, packageClasses, bindingsAlias)
		if err != nil {
			return err
		}
		ec.EngineRoot = engineRoots[d.MainClass.Name]
		if classNeedsVariant {
			needsVariant = true
		}
		emitClasses = append(emitClasses, ec)
	}

	hasDocXML := false
	for _, ec := range emitClasses {
		if ec.DocXML != "" {
			hasDocXML = true
			break
		}
	}

	data := emitData{
		PackageName:    classes[0].PackageName,
		Classes:        emitClasses,
		NeedsVariant:   needsVariant,
		HasAnyDocXML:   hasDocXML,
		BindingsImport: bindingsImport,
		BindingsAlias:  bindingsAlias,
		RuntimeImport:  bindingsImport + "/runtime",
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

// buildEmitClass produces the emitClass for one discovered file —
// builds the methods / properties / signals / enums lists with their
// rendered marshal fragments, plus the class-XML doc string when the
// class has any doc surface.
//
// packageEnums is the package-wide enum map (every @class file's
// enums aggregated, with OwningClass populated). packageClasses is
// the set of @class names declared in the package — both feed into
// resolveType so cross-file enum and user-class references at this
// class's @class boundary resolve to the correct registration
// metadata and the side-table lookup of the right sibling class.
func buildEmitClass(fset *token.FileSet, d *discovered, packageEnums map[string]*enumInfo, packageClasses map[string]bool, bindingsAlias string) (emitClass, bool, error) {
	ci := d.MainClass

	initLevel := "InitLevelScene"
	if ci.IsEditor {
		initLevel = "InitLevelEditor"
	}

	supported, needsVariant, err := classifyForEmit(fset, ci.Methods, packageEnums, packageClasses, bindingsAlias, ci.Name)
	if err != nil {
		return emitClass{}, false, err
	}

	accessors, propMethods, properties, err := buildEmitProperties(ci.Properties, packageEnums, packageClasses, bindingsAlias, ci.Name)
	if err != nil {
		return emitClass{}, false, err
	}
	supported = append(supported, propMethods...)
	if len(accessors) > 0 || len(properties) > 0 {
		needsVariant = true
	}

	signals, err := buildEmitSignals(ci.Signals, packageEnums, packageClasses, bindingsAlias, ci.Name)
	if err != nil {
		return emitClass{}, false, err
	}
	if len(signals) > 0 {
		needsVariant = true
	}

	abstractMethods, err := buildEmitAbstractMethods(ci.AbstractMethods, packageEnums, packageClasses, bindingsAlias, ci.Name)
	if err != nil {
		return emitClass{}, false, err
	}
	if len(abstractMethods) > 0 {
		needsVariant = true
	}

	ec := emitClass{
		SourceFile:      filepath.Base(d.FilePath),
		Class:           ci.Name,
		Parent:          ci.Parent,
		Lower:           lowerFirst(ci.Name),
		InitLevel:       initLevel,
		IsAbstract:      ci.IsAbstract,
		HasConstructor:  ci.HasConstructor,
		Methods:         supported,
		Accessors:       accessors,
		Properties:      properties,
		Signals:         signals,
		AbstractMethods: abstractMethods,
		Enums:           buildEmitEnums(d.Enums),
	}

	if emitHasDocSurface(ci, ec) {
		docXML, xerr := buildClassXML(ec, ci.docInfo, ci.Brief, ci.Tutorials)
		if xerr != nil {
			return emitClass{}, false, fmt.Errorf("build doc XML for %s: %w", ci.Name, xerr)
		}
		ec.DocXML = docXML
	}

	return ec, needsVariant, nil
}

// emitData is the package-level payload threaded through the bindings
// template. Classes carries one emitClass per @class file in the
// package, in deterministic order.
type emitData struct {
	PackageName string

	Classes []emitClass

	NeedsVariant bool // any class's method touches a primitive (gates the bindings-package import)
	// HasAnyDocXML is true when at least one class has a non-empty
	// DocXML string. Gates the runtime sub-package import — no need
	// to pull `<bindings>/runtime` in for a class that won't call
	// runtime.LoadEditorDocXML.
	HasAnyDocXML bool

	// BindingsImport / BindingsAlias identify the user's bindings
	// package — the import path and the local name the source uses for
	// it (e.g. "github.com/foo/me/godot" + "godot"). Variant marshal
	// helpers (VariantAsInt64, NewVariantString, …) live there in the
	// single-package layout, so generated calls are qualified through
	// the alias.
	BindingsImport string
	BindingsAlias  string

	// RuntimeImport is BindingsImport + "/runtime"; aliased to
	// godotruntime in the emitted source to avoid clashing with Go's
	// stdlib `runtime` package. Used for runtime.LoadEditorDocXML —
	// gated wrapper around gdextension.LoadEditorDocXML that
	// suppresses the engine's "editor_help_load_xml_buffer is null"
	// noise in headless / deployed-game runs.
	RuntimeImport string
}

// emitClass is the per-@class payload. The bindings template ranges
// over emitData.Classes and stamps out a side table + register<X>()
// function + DocXML const per entry.
type emitClass struct {
	SourceFile string // filepath.Base of the source file — used in the per-class header comment
	InitLevel  string // bare InitializationLevel const name — "InitLevelScene" or "InitLevelEditor"

	Class      string
	Parent     string
	// EngineRoot is the first cross-package ancestor (engine class)
	// up the @extends chain. For a class that directly extends an
	// engine class (Parent == EngineRoot), the two are the same.
	// For a class that extends another same-package @class, EngineRoot
	// is the eventual engine ancestor — that's what the Construct hook
	// passes to ConstructObject (constructing an intermediate user
	// @class would re-fire its Construct hook or fail outright if it's
	// @abstract). Class hierarchy registration still uses Parent.
	EngineRoot string
	Lower      string // class name with first rune lowercased — used for unexported helpers
	IsAbstract bool   // @abstract on this class — passed through to RegisterClass
	Methods    []emitMethod
	Accessors  []emitAccessor // synthesized GetX/SetX Go methods for field-form @property declarations
	Properties []emitProperty // RegisterClassProperty calls (one per @property, both forms)
	Signals         []emitSignal   // RegisterClassSignal + synthesized emit method per @signals method
	AbstractMethods []emitAbstractMethod // synthesized dispatcher method per @abstract_methods declaration — routes via Object::call to whatever subclasses register
	Enums      []emitEnum     // @enum / @bitfield-tagged user enums registered as class-scoped integer constants
	// DocXML is the rendered <class>…</class> document baked into the
	// bindings as a const string. Empty when the class has no doc-able
	// content (no description, no docs on any declaration); the
	// template skips the LoadEditorDocXML call when empty.
	DocXML string

	// HasConstructor is true when the source defines an unexported
	// `func new<Class>() *<Class>` default-init hook. The Construct
	// callback then routes through it instead of the zero-value
	// `&<Class>{}` literal.
	HasConstructor bool
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

	// ArgHints / ArgHintStrings carry per-arg PropertyHint + payload
	// (parallel to ArgTypes). Currently populated for slice-of-tagged-
	// enum args via PROPERTY_HINT_TYPE_STRING — element-level enum
	// identity that can't ride on class_name (Godot rejects class_name
	// on non-OBJECT slots). Empty entries register as no-hint.
	ArgHints       []string // bare PropertyHint const names ("PropertyHintTypeString" etc.)
	ArgHintStrings []string

	// ReturnHint / ReturnHintString carry the return value's hint
	// metadata — see ArgHints for the typed-array element identity
	// use case.
	ReturnHint       string
	ReturnHintString string

	// IsVariadic flag — see below. (HasAnyArgHint method follows the
	// struct definition.)
	//
	// IsVariadic is true when the source method's last parameter is
	// declared with Go's `...T` ellipsis syntax. The wire boundary still
	// passes a single Packed<X>Array / Array[T] arg — Godot has no
	// vararg concept on extension methods — but the dispatch site spreads
	// the slice with `argN...` so the user method receives the variadic
	// it expects.
	IsVariadic bool
}

// HasAnyArgHint reports whether at least one positional arg has a
// non-empty PropertyHint constant set. Used by the bindings template
// to decide whether to render the ArgHints / ArgHintStrings slices —
// most methods don't carry per-arg hints, and emitting empty slices
// every time would clutter the registration call.
func (m emitMethod) HasAnyArgHint() bool {
	for _, h := range m.ArgHints {
		if h != "" {
			return true
		}
	}
	return false
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
func classifyForEmit(fset *token.FileSet, methods []*methodInfo, enums map[string]*enumInfo, userClasses map[string]bool, bindings, mainClass string) ([]emitMethod, bool, error) {
	var out []emitMethod
	needsVariant := false
	for _, m := range methods {
		em, err := buildEmitMethod(m, enums, userClasses, bindings, mainClass)
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
func buildEmitMethod(m *methodInfo, enums map[string]*enumInfo, userClasses map[string]bool, bindings, mainClass string) (emitMethod, error) {
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
		info, err := resolveType(field.Type, enums, userClasses, mainClass, bindings)
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
			em.ArgClassNames = append(em.ArgClassNames, classIdentity(mainClass, info))
			argHint, argHintStr := hintForTypeInfo(info, enums)
			em.ArgHints = append(em.ArgHints, argHint)
			em.ArgHintStrings = append(em.ArgHintStrings, argHintStr)
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
		info, err := resolveType(ret.Type, enums, userClasses, mainClass, bindings)
		if err != nil {
			return em, fmt.Errorf("return: %w", err)
		}
		em.HasReturn = true
		em.ReturnType = info.VariantType
		em.ReturnMeta = info.ArgMeta
		em.ReturnGodotType = godotXMLType(info.VariantType)
		em.ReturnClassName = classIdentity(mainClass, info)
		em.ReturnHint, em.ReturnHintString = hintForTypeInfo(info, enums)
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

// classIdentity returns the value to put in ArgClassNames /
// ReturnClassName for a typeInfo. User-class / engine-class boundaries
// carry a bare class name in the typeInfo's ClassName field; enum
// boundaries carry the unqualified enum name in EnumName and need
// the "<MainClass>.<EnumName>" prefix at the use site. Returns empty
// for primitives.
func classIdentity(mainClass string, info *typeInfo) string {
	if info.ClassName != "" {
		return info.ClassName
	}
	return qualifyEnum(mainClass, info.EnumName)
}

// hintForTypeInfo computes the PropertyHint const name and hint string
// for a typeInfo. Two paths:
//
//   - Slice-of-tagged-enum (HintEnum != ""): element identity rides on
//     PROPERTY_HINT_TYPE_STRING with an "<elem_type>/<elem_hint>:names"
//     payload — this is Godot's mechanism for `Array[<Enum>]` rendering.
//
//   - Typed Dictionary (DictHintString != ""): K/V identity rides on
//     PROPERTY_HINT_DICTIONARY_TYPE with the precomputed
//     "<K_var>/<K_hint>:<K_str>;<V_var>/<V_hint>:<V_str>" payload built
//     by mapTypeInfo. Best-effort — Godot 4.4+'s typed-Dictionary
//     editor surface is less stable than typed arrays.
//
// Returns empty pair for typeInfos that don't need hint metadata.
//
// Numeric values are baked at codegen time so the framework doesn't
// have to import gdextension for these constants — they're stable
// across Godot 4.x. INT = 2, PROPERTY_HINT_ENUM = 2.
func hintForTypeInfo(info *typeInfo, enums map[string]*enumInfo) (hint, hintString string) {
	if info.DictHintString != "" {
		return "PropertyHintDictionaryType", info.DictHintString
	}
	if info.HintEnum == "" {
		return "", ""
	}
	e, ok := enums[info.HintEnum]
	if !ok || !e.IsExposed {
		return "", ""
	}
	names := make([]string, 0, len(e.Values))
	for _, v := range e.Values {
		names = append(names, godotEnumValueName(e.Name, v.Name))
	}
	return "PropertyHintTypeString", "2/2:" + strings.Join(names, ",")
}

// emitHasDocSurface reports whether the class has anything worth
// emitting an XML doc for. Classes without a description and no
// declarations carrying docs skip the XML entirely so the bindings
// don't carry a dead `const xml = "..."` string the editor would
// just load empty. Conservative: we emit XML when the class has a
// brief, description, deprecated/experimental marker, tutorials, or
// any method/property/signal/enum at all (since those produce their
// own rows in Godot's docs page even with no description text).
func emitHasDocSurface(ci *classInfo, ec emitClass) bool {
	if ci.Description != "" || ci.Brief != "" || ci.Deprecated != nil || ci.Experimental != nil ||
		ci.Since != "" || len(ci.See) > 0 || len(ci.Tutorials) > 0 {
		return true
	}
	return len(ec.Methods) > 0 || len(ec.Properties) > 0 ||
		len(ec.Signals) > 0 || len(ec.Enums) > 0
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
func buildEmitProperties(props []*propertyInfo, enums map[string]*enumInfo, userClasses map[string]bool, bindings, mainClass string) (
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
		info, terr := resolveType(p.GoType, enums, userClasses, mainClass, bindings)
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
			ClassName:  classIdentity(mainClass, info),
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
func buildEmitSignals(signals []*signalInfo, enums map[string]*enumInfo, userClasses map[string]bool, bindings, mainClass string) ([]emitSignal, error) {
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
			info, err := resolveType(field.Type, enums, userClasses, mainClass, bindings)
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
				es.ArgClassNames = append(es.ArgClassNames, classIdentity(mainClass, info))
				es.ArgGodotTypes = append(es.ArgGodotTypes, godotXMLType(info.VariantType))
				idx++
			}
		}
		out = append(out, es)
	}
	return out, nil
}

// emitAbstractMethod is one method declared on a `@abstract_methods`
// interface. Renders to a Go-side dispatcher on `*<MainClass>` that
// builds per-arg Variants, invokes Object::call against the engine
// pointer, and unwraps the return — plus a ClassDB registration on
// the parent class with MethodFlagVirtualRequired so Godot's editor
// surfaces the contract in docs and the engine can warn about
// missing subclass overrides.
type emitAbstractMethod struct {
	docInfo

	GoName    string // Go identifier the dispatcher exposes to user code
	GodotName string // snake_case name passed to Object::call ("speak", etc.)

	// Args carry the per-arg metadata the template needs to render the
	// dispatcher's signature and the per-arg Variant build/destroy
	// fragments.
	Args []emitAbstractMethodArg

	// HasReturn / Return* describe the return-Variant unwrap. If
	// HasReturn is false, the dispatcher discards the return slot.
	HasReturn bool
	// ReturnGoType is the user-facing Go return type ("string",
	// "int64", "*MainClass", etc.) — exactly what we'd render in the
	// method signature.
	ReturnGoType string
	// ReturnUnwrap is a single statement that converts a Variant value
	// named `result_var` (the Variant filled by Object::call) into
	// a typed value bound to the local identifier `result`. Multi-step
	// types (object pointers) emit multi-line statements ending with
	// the `result :=` assignment.
	ReturnUnwrap string

	// Registration metadata — parallel to emitMethod's ArgTypes /
	// ArgMetas / ReturnType. The parent's RegisterClassMethod call
	// uses these to advertise the contract; the ClassDB lookup that
	// happens at subclass-registration time matches override
	// signatures against this metadata.
	ArgTypes        []string
	ArgMetas        []string
	ArgNames        []string
	ArgClassNames   []string
	ArgGodotTypes   []string
	ReturnType      string
	ReturnMeta      string
	ReturnClassName string
	ReturnGodotType string

	// SourceInterface is the Go name of the @abstract_methods interface
	// this method came from. Currently informational; future work may
	// surface it in the XML doc.
	SourceInterface string
}

// emitAbstractMethodArg is one positional slot of an abstract method
// dispatcher. Renders both the Go signature parameter and the per-arg
// Variant construction fragment.
type emitAbstractMethodArg struct {
	Index    int    // 0-based positional arg index (matches argN naming)
	GoName   string // identifier the dispatcher signature exposes
	GoType   string // Go source-rendered type expression
	BuildVar string // single statement that constructs the Variant, e.g.
	// `arg0 := bindings.NewVariantInt(distance)`. The Variant is
	// destroyed via `defer arg0.Destroy()` rendered separately in
	// the template.
}

// buildEmitAbstractMethods walks the discovered @abstract_methods
// declarations and produces emitAbstractMethod records the template
// renders into per-method dispatchers on the parent struct. Arg /
// return types resolve through the same resolveType path as regular
// methods, so abstract method type support tracks method arg/return
// type support.
func buildEmitAbstractMethods(abstracts []*abstractMethodInfo, enums map[string]*enumInfo, userClasses map[string]bool, bindings, mainClass string) ([]emitAbstractMethod, error) {
	if len(abstracts) == 0 {
		return nil, nil
	}
	out := make([]emitAbstractMethod, 0, len(abstracts))
	for _, a := range abstracts {
		em := emitAbstractMethod{
			docInfo:         a.docInfo,
			GoName:          a.Name,
			GodotName:       a.GodotName,
			SourceInterface: a.SourceInterface,
		}
		idx := 0
		for _, field := range a.Args {
			info, err := resolveType(field.Type, enums, userClasses, mainClass, bindings)
			if err != nil {
				return nil, fmt.Errorf("@abstract_methods %s arg %d: %w", a.Name, idx, err)
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
				em.Args = append(em.Args, emitAbstractMethodArg{
					Index:    idx,
					GoName:   goArgName,
					GoType:   info.GoType,
					BuildVar: info.BuildVariant(bindings, idx, goArgName),
				})
				em.ArgTypes = append(em.ArgTypes, info.VariantType)
				em.ArgMetas = append(em.ArgMetas, info.ArgMeta)
				em.ArgNames = append(em.ArgNames, registeredName)
				em.ArgClassNames = append(em.ArgClassNames, classIdentity(mainClass, info))
				em.ArgGodotTypes = append(em.ArgGodotTypes, godotXMLType(info.VariantType))
				idx++
			}
		}
		if a.Result != nil {
			info, err := resolveType(a.Result.Type, enums, userClasses, mainClass, bindings)
			if err != nil {
				return nil, fmt.Errorf("@abstract_methods %s return: %w", a.Name, err)
			}
			em.HasReturn = true
			em.ReturnGoType = info.GoType
			em.ReturnType = info.VariantType
			em.ReturnMeta = info.ArgMeta
			em.ReturnClassName = classIdentity(mainClass, info)
			em.ReturnGodotType = godotXMLType(info.VariantType)
			// UnwrapVariant gives us a multi-statement form that ends
			// with `result := <typed value>`. mapTypeInfo introduced this
			// helper; primitives + enums + class pointers all populate
			// it. If the caller wired a type without UnwrapVariant
			// (slice, etc.), surface a clear error rather than emitting
			// invalid source.
			if info.UnwrapVariant == nil {
				return nil, fmt.Errorf("@abstract_methods %s return type %q is not supported (only types with variant unwrap helpers can flow back through Object::call)", a.Name, info.GoType)
			}
			em.ReturnUnwrap = info.UnwrapVariant(bindings, "result", "result_var")
		}
		out = append(out, em)
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
		ReturnClassName: classIdentity(mainClass, info),
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
		ArgClassNames: []string{classIdentity(mainClass, info)},
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

package {{.PackageName}}

import (
	"sync"
	"unsafe"

	"github.com/legendary-code/godot-go/gdextension"
{{if .NeedsVariant}}	{{.BindingsAlias}} "{{.BindingsImport}}"
{{end}}{{if .HasAnyDocXML}}	godotruntime "{{.RuntimeImport}}"
{{end}})

{{range .Classes}}{{$class := .}}
// === {{$class.Class}} ({{$class.SourceFile}}) ===

// Per-instance side tables for {{$class.Class}}. The void* value the host
// hands back to us as p_instance is the small integer id we returned from
// Construct, not a Go pointer (cgo forbids storing Go pointers in
// C-visible memory). The parallel <X>ByEngine map is keyed by the engine
// ObjectPtr Godot uses for object identity — populated by Construct so
// methods that take *{{$class.Class}} args can recover the Go wrapper
// from the ObjectPtr Godot passes through Variant arg slots.
var (
	{{$class.Lower}}InstancesMu sync.Mutex
	{{$class.Lower}}Instances   = map[uintptr]*{{$class.Class}}{}
	{{$class.Lower}}ByEngine    = map[uintptr]*{{$class.Class}}{}
	{{$class.Lower}}NextID      uintptr
)

func register{{$class.Class}}Instance(n *{{$class.Class}}, parent gdextension.ObjectPtr) unsafe.Pointer {
	{{$class.Lower}}InstancesMu.Lock()
	{{$class.Lower}}NextID++
	id := {{$class.Lower}}NextID
	{{$class.Lower}}Instances[id] = n
	{{$class.Lower}}ByEngine[uintptr(parent)] = n
	{{$class.Lower}}InstancesMu.Unlock()
	return unsafe.Pointer(id)
}

func lookup{{$class.Class}}Instance(handle unsafe.Pointer) *{{$class.Class}} {
	{{$class.Lower}}InstancesMu.Lock()
	defer {{$class.Lower}}InstancesMu.Unlock()
	return {{$class.Lower}}Instances[uintptr(handle)]
}

// lookup{{$class.Class}}ByEngine resolves an engine ObjectPtr to the
// *{{$class.Class}} we registered at Construct time. Returns nil for
// foreign instances — see the foreign-instance policy in
// docs/reference.md.
func lookup{{$class.Class}}ByEngine(p gdextension.ObjectPtr) *{{$class.Class}} {
	{{$class.Lower}}InstancesMu.Lock()
	defer {{$class.Lower}}InstancesMu.Unlock()
	return {{$class.Lower}}ByEngine[uintptr(p)]
}

{{if not $class.IsAbstract}}
// New{{$class.Class}} constructs a fresh {{$class.Class}} instance via
// Godot's ClassDB. Routes through gdextension.ConstructObject, which
// fires the framework's Construct hook (creating the Go wrapper and
// registering it in the side table).
{{- if $class.HasConstructor}}
// The wrapper itself comes from the user's package-level
// new{{$class.Class}}() factory, so default field values seeded there
// flow into both Go-side construction and GDScript-driven
// {{$class.Class}}.new() calls.
{{- end}}
//
// Use this instead of plain &{{$class.Class}}{} when you need an
// engine-backed instance. Hollow struct literals have no engine
// pointer and serialize as nil at the @class boundary.
//
// RefCounted-derived classes start the new instance at refcount 1
// (per Godot's ConstructObject semantics). The variant boundary
// increments when the wrapper crosses into Godot. The factory's
// initial reference is the caller's responsibility — call
// (*{{$class.Class}}).Unreference() if you need explicit cleanup
// before relying on Go GC; finalizer-driven release for user-class
// wrappers is not yet wired.
func New{{$class.Class}}() *{{$class.Class}} {
	parent := gdextension.ConstructObject(gdextension.InternStringName({{printf "%q" $class.Class}}))
	n := lookup{{$class.Class}}ByEngine(parent)
	// RefCounted-derived user classes need init_ref called once on
	// the freshly-constructed instance — godot-cpp does this implicitly
	// via Ref<T>; godot-go has to do it explicitly. Without it, the
	// first Variant(Object*) construction (e.g. inside emit_signal's
	// per-callable boxing) trips Godot's refcount_init self-balance
	// asymmetrically, draining one ref. Has to happen AFTER
	// object_set_instance has wired the extension instance handle —
	// calling InitRef from inside the Construct hook (before
	// set_instance fires) doesn't take. Type-assert against an
	// inline interface holding InitRef so non-RefCounted user classes
	// (Node-derived, Object-derived) skip silently.
	if rc, ok := any(n).(interface{ InitRef() bool }); ok {
		rc.InitRef()
	}
	return n
}
{{end}}

func release{{$class.Class}}Instance(handle unsafe.Pointer) {
	{{$class.Lower}}InstancesMu.Lock()
	defer {{$class.Lower}}InstancesMu.Unlock()
	n, ok := {{$class.Lower}}Instances[uintptr(handle)]
	if !ok {
		return
	}
	delete({{$class.Lower}}Instances, uintptr(handle))
	delete({{$class.Lower}}ByEngine, uintptr(n.Ptr()))
}
{{range .Accessors}}
{{if .IsSetter -}}
func (n *{{$class.Class}}) {{.Name}}(v {{.GoType}}) { n.{{.Field}} = v }
{{else -}}
func (n *{{$class.Class}}) {{.Name}}() {{.GoType}} { return n.{{.Field}} }
{{end -}}
{{end}}
{{range .Signals}}
// {{.GoName}} emits the {{.GodotName}} signal on this instance. Synthesized
// from a @signals interface declaration; per-arg Variants are constructed
// from the typed parameters and dispatched via Object::emit_signal.
func (n *{{$class.Class}}) {{.GoName}}({{range $i, $a := .PerArg}}{{if $i}}, {{end}}{{$a.GoName}} {{$a.GoType}}{{end}}) {
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
{{range .AbstractMethods}}
// {{.GoName}} dispatches polymorphically to whatever subclass registered
// "{{.GodotName}}". Synthesized from a @abstract_methods interface
// declaration; per-arg Variants are built from the typed parameters
// and routed through gdextension.ObjectCall (Godot's variant call
// protocol). Subclass implementations register the concrete method
// under the same Godot name, so ClassDB's hierarchy lookup picks
// the right override at call time. Calling on an instance of the
// declaring class itself (when not overridden) surfaces Godot's
// "method not found" CallError.
func (n *{{$class.Class}}) {{.GoName}}({{range $i, $a := .Args}}{{if $i}}, {{end}}{{$a.GoName}} {{$a.GoType}}{{end}}){{if .HasReturn}} {{.ReturnGoType}}{{end}} {
	{{- range .Args}}
	{{.BuildVar}}
	defer arg{{.Index}}.Destroy()
	{{- end}}
	{{- if .Args}}
	args := []gdextension.VariantPtr{
		{{- range .Args}}
		gdextension.VariantPtr(unsafe.Pointer(&arg{{.Index}})),
		{{- end}}
	}
	{{- end}}
	var result_var {{$.BindingsAlias}}.Variant
	defer result_var.Destroy()
	gdextension.ObjectCall(n.Ptr(), gdextension.InternStringName("{{.GodotName}}"), {{if .Args}}args{{else}}nil{{end}}, gdextension.VariantPtr(unsafe.Pointer(&result_var)))
	{{- if .HasReturn}}
	{{.ReturnUnwrap}}
	return result
	{{- end}}
}
{{end}}
func register{{$class.Class}}() {
	gdextension.RegisterClass(gdextension.ClassDef{
		Name:      "{{$class.Class}}",
		Parent:    "{{$class.Parent}}",
		IsExposed: true,
		{{- if $class.IsAbstract}}
		// godot-cpp parity: GDREGISTER_ABSTRACT_CLASS sets is_abstract=true,
		// which leaves the registration's creation_func null. Godot's
		// _instantiate_internal then refuses to construct the class —
		// MyAbstract.new() errors out, AND "class_name X extends
		// MyAbstract; X.new()" errors out (Godot has no engine flag that
		// separates the two paths). If you need GDScript subclasses, drop
		// @abstract and rely on the New<Class>() suppression alone.
		IsAbstract: true,
		{{- end}}

		Construct: func() (gdextension.ObjectPtr, unsafe.Pointer) {
			{{- if $class.HasConstructor}}
			n := new{{$class.Class}}()
			{{- else}}
			n := &{{$class.Class}}{}
			{{- end}}
			parent := gdextension.ConstructObject(gdextension.InternStringName("{{$class.EngineRoot}}"))
			n.BindPtr(parent)
			return parent, register{{$class.Class}}Instance(n, parent)
		},

		Free: func(instance unsafe.Pointer) {
			release{{$class.Class}}Instance(instance)
		},
	})
{{range .Methods}}
	{{- if .IsVirtual}}
	gdextension.RegisterClassVirtual(gdextension.ClassVirtualDef{
		Class: "{{$class.Class}}",
		Name:  "{{.GodotName}}",
		Call: func(instance unsafe.Pointer, args []gdextension.VariantPtr, ret gdextension.VariantPtr) gdextension.CallErrorType {
			self := lookup{{$class.Class}}Instance(instance)
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
			self := lookup{{$class.Class}}Instance(instance)
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
		Class: "{{$class.Class}}",
		Name:  "{{.GodotName}}",
		Call: func(instance unsafe.Pointer, args []gdextension.VariantPtr, ret gdextension.VariantPtr) gdextension.CallErrorType {
			{{- if .IsStatic}}
			_ = instance
			var self {{$class.Class}}
			{{- else}}
			self := lookup{{$class.Class}}Instance(instance)
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
			var self {{$class.Class}}
			{{- else}}
			self := lookup{{$class.Class}}Instance(instance)
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
		{{- if .ReturnHint}}
		ReturnHint:       gdextension.{{.ReturnHint}},
		ReturnHintString: {{printf "%q" .ReturnHintString}},
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
		{{- if .HasAnyArgHint}}
		ArgHints: []gdextension.PropertyHint{
			{{- range .ArgHints}}
			{{- if .}}
			gdextension.{{.}},
			{{- else}}
			gdextension.PropertyHintNone,
			{{- end}}
			{{- end}}
		},
		ArgHintStrings: []string{
			{{- range .ArgHintStrings}}
			{{printf "%q" .}},
			{{- end}}
		},
		{{- end}}
		{{- end}}
	})
	{{- end}}
{{end}}
{{- range .Properties}}
	{{- if .BeginGroup}}
	gdextension.RegisterClassPropertyGroup(gdextension.ClassPropertyGroupDef{
		Class: "{{$class.Class}}",
		Name:  "{{.BeginGroup}}",
	})
	{{- else if and (eq .BeginGroup "") (ne .BeginSubgroup "")}}
	{{- /* Subgroup transition without a group transition — only emit
	       the subgroup call. The group is unchanged. */}}
	{{- end}}
	{{- if .BeginSubgroup}}
	gdextension.RegisterClassPropertySubgroup(gdextension.ClassPropertyGroupDef{
		Class: "{{$class.Class}}",
		Name:  "{{.BeginSubgroup}}",
	})
	{{- end}}
	gdextension.RegisterClassProperty(gdextension.ClassPropertyDef{
		Class:  "{{$class.Class}}",
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
		Class: "{{$class.Class}}",
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
{{- range .AbstractMethods}}
	// {{.GoName}} is registered on {{$class.Class}} as a virtual-method
	// declaration (no implementation). Routing through
	// classdb_register_extension_class_virtual_method places it in the
	// engine's virtual_methods_map rather than the regular MethodBind
	// table — so GDScript's NATIVE_METHOD_OVERRIDE warning stays silent
	// when subclasses override, and MethodFlagVirtualRequired makes the
	// parser refuse subclasses that skip the override. Real dispatch
	// happens through subclasses' regular method registrations (matched
	// by name) and via the synthesized *{{$class.Class}} dispatcher's
	// Object::call route.
	gdextension.RegisterClassAbstractMethod(gdextension.ClassAbstractMethodDef{
		Class: "{{$class.Class}}",
		Name:  "{{.GodotName}}",
		Flags: gdextension.MethodFlagsDefault | gdextension.MethodFlagVirtual | gdextension.MethodFlagVirtualRequired,
		{{- if .HasReturn}}
		HasReturn:      true,
		ReturnType:     gdextension.{{.ReturnType}},
		ReturnMetadata: gdextension.{{.ReturnMeta}},
		{{- if .ReturnClassName}}
		ReturnClassName: {{printf "%q" .ReturnClassName}},
		{{- end}}
		{{- end}}
		{{- if .Args}}
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
		Class: "{{$class.Class}}",
		Enum:  "{{$enumName}}",
		Name:  "{{.GodotName}}",
		Value: {{.Value}},
		{{- if $isBitfield}}
		IsBitfield: true,
		{{- end}}
	})
	{{- end}}
{{end}}
{{- if $class.DocXML}}
	godotruntime.LoadEditorDocXML({{$class.Lower}}DocXML)
{{- end}}
}
{{if $class.DocXML}}
// {{$class.Lower}}DocXML is the rendered <class> document for {{$class.Class}}.
// Loaded at SCENE init via editor_help_load_xml_from_utf8_chars (an
// editor-only entry point — game-mode runtimes resolve the symbol to
// nil and the load call is a no-op).
const {{$class.Lower}}DocXML = {{$class.DocXML | godotGoBacktick}}
{{end}}
{{end}}
func init() {
{{- range .Classes}}
	gdextension.RegisterInitCallback(gdextension.{{.InitLevel}}, register{{.Class}})
	gdextension.RegisterDeinitCallback(gdextension.{{.InitLevel}}, func() {
		gdextension.UnregisterClass("{{.Class}}")
	})
{{- end}}
}
`))
