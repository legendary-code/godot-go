package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"
	"unicode"

	"github.com/legendary-code/godot-go/internal/doctag"
)

// discovered is everything the AST pass found in one trigger file. The
// emitter (Phase 5c) consumes this; the report formatter prints it.
type discovered struct {
	PackageName string
	FilePath    string

	Imports map[string]string // local name → import path

	MainClass    *classInfo
	InnerClasses []*classInfo
	Enums        []*enumInfo
}

// classInfo is one struct flagged for class-registration. Receiver-method
// classification happens in collectMethods after structs are discovered.
type classInfo struct {
	Name        string
	Pos         token.Pos
	Decl        *ast.TypeSpec
	Doc         []doctag.Tag
	Description string

	// Parent is the framework class this struct embeds. Empty for inner
	// classes — those don't extend a Godot class.
	Parent       string
	ParentImport string // e.g. "github.com/legendary-code/godot-go/core"

	IsAbstract bool
	IsInner    bool
	// IsClass marks structs explicitly tagged `@class` — the
	// registered extension class for this file. Exactly one struct
	// per file must carry @class. Mutually exclusive with @innerclass.
	IsClass bool
	// IsEditor flips registration from INIT_LEVEL_SCENE to
	// INIT_LEVEL_EDITOR — Godot only fires editor-level init callbacks
	// when the engine is in editor mode, so the class is invisible to
	// game runtime builds. Used for EditorPlugin subclasses, custom
	// inspectors, gizmos, and other tooling that has no business
	// existing in deployed game builds.
	IsEditor bool

	Methods    []*methodInfo
	Properties []*propertyInfo
	Signals    []*signalInfo
}

// signalInfo is one signal declared on a `@signals` interface. Multiple
// such interfaces may live in the same file; their methods accumulate
// onto the main class. Codegen attaches an emit method directly to
// `*<MainClass>` for each signal — there's no embedded struct in the
// user's source.
type signalInfo struct {
	Name string // method identifier as it appeared on the interface (e.g. "Damaged")
	// GodotName is the snake_case name Godot's ClassDB sees ("damaged").
	GodotName string
	Pos       token.Pos
	// Args carry the AST type expressions for each formal parameter. The
	// emitter resolves these via the same resolveType path used for
	// method args — supported types are the framework's primitive set.
	Args []*ast.Field
	// SourceInterface is the Go name of the @signals interface this
	// signal came from, used in error messages.
	SourceInterface string
}

// propertyInfo is one @property declaration on the class. There are two
// surface forms:
//
//   - **Field form:** an exported struct field carries `@property` (and
//     optionally `@readonly`). Codegen synthesizes Get<Name> + Set<Name>
//     methods that read / write the field, registers them as ClassDB
//     methods, then registers the property linking name → those methods.
//   - **Method form:** the user already wrote a Get<Name> method (with
//     `@property` on it). If a Set<Name> method also exists in the source
//     it's wired as the setter; otherwise the property is read-only. The
//     user owns the backing — we just emit RegisterClassProperty.
//
// Conflict rule: if the same Name appears in both forms (an exported field
// `Foo` AND a `GetFoo` method), discovery errors with file:line. Pick one.
type propertyInfo struct {
	// Name is the bare property identifier in PascalCase (e.g. "Health").
	// The Godot name is snake_case(Name) ("health"); generated method
	// names are GetHealth / SetHealth.
	Name string
	// GoType is the property's Go type as it appears in the source (the
	// field type for field form, the getter return type for method form).
	GoType ast.Expr
	// Pos points to the source declaration that triggered the property —
	// the field declaration for field form, the GetX method for method
	// form. Used for error reporting.
	Pos token.Pos
	// Source distinguishes how the property was declared. Field form
	// emits synthesized Get/Set; method form just registers the property.
	Source propertySource
	// ReadOnly is true when @readonly accompanies @property (field form),
	// or when no Set<Name> method exists alongside the user's Get<Name>
	// (method form).
	ReadOnly bool

	// Group / Subgroup carry the inspector grouping for this property
	// (field form only — method-form properties don't support grouping
	// in v1). Empty strings mean "ungrouped".
	Group    string
	Subgroup string

	// Hint + HintString are the Godot PropertyHint enum value (bare
	// const name, e.g. "PropertyHintRange") and its payload string. Set
	// from `@export_range`/`@export_enum`/etc. on field-form @property.
	// Default is "PropertyHintNone" + empty.
	Hint       string
	HintString string
}

type propertySource int

const (
	propertyFromField propertySource = iota
	propertyFromMethod
)

type methodInfo struct {
	GoName       string // exact identifier in the source
	GodotName    string // snake_case unless @name overrides
	Pos          token.Pos
	Decl         *ast.FuncDecl
	Doc          []doctag.Tag
	Kind         methodKind
	IsPointerRcv bool
}

type methodKind int

const (
	methodInstance methodKind = iota
	methodStatic
	methodOverride // @override on a method — register as a Godot virtual
)

func (k methodKind) String() string {
	switch k {
	case methodInstance:
		return "instance"
	case methodStatic:
		return "static"
	case methodOverride:
		return "override"
	default:
		return "unknown"
	}
}

type enumInfo struct {
	Name   string
	Pos    token.Pos
	Values []enumValue
}

type enumValue struct {
	Name string
	Pos  token.Pos
}

// frameworkPkgs lists the import paths whose exported types are recognized
// as candidate base classes for a user extension. User-defined parents
// (extension classes from another module) aren't supported in Phase 5b —
// see PLAN_EXECUTION.md Phase 5 step 3.
var frameworkPkgs = map[string]bool{
	"github.com/legendary-code/godot-go/core":    true,
	"github.com/legendary-code/godot-go/editor":  true,
	"github.com/legendary-code/godot-go/godot":   true,
	"github.com/legendary-code/godot-go/variant": true,
}

func discover(fset *token.FileSet, file *ast.File, pkgName string) (*discovered, error) {
	d := &discovered{
		PackageName: pkgName,
		FilePath:    fset.Position(file.Pos()).Filename,
		Imports:     map[string]string{},
	}

	// Index imports first — both implicit (last segment of path) and aliased.
	// Used by parent-class resolution to confirm the embedded type lives in a
	// framework package.
	for _, imp := range file.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		var local string
		switch {
		case imp.Name != nil:
			local = imp.Name.Name
		default:
			if i := strings.LastIndex(path, "/"); i >= 0 {
				local = path[i+1:]
			} else {
				local = path
			}
		}
		d.Imports[local] = path
	}

	// First pass: top-level types. Distinguish struct types (candidate
	// classes), named-int types (candidate enums), and interface types
	// tagged @signals (signal contracts). Other type aliases are out of
	// scope here.
	structs := map[string]*classInfo{}
	intTypes := map[string]*enumInfo{}
	signalIfaces := []*ast.TypeSpec{}
	for _, decl := range file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, spec := range gd.Specs {
			ts := spec.(*ast.TypeSpec)
			doc := docOf(gd, ts)
			tags := doctag.Parse(doc)
			switch t := ts.Type.(type) {
			case *ast.StructType:
				isClass := doctag.Has(tags, "class")
				isInner := doctag.Has(tags, "innerclass")
				if isClass && isInner {
					return nil, fmt.Errorf("%s: %s carries both @class and @innerclass — pick one (the framework registers @class structs and discovers @innerclass structs as nested types)",
						posStr(fset, ts.Pos()), ts.Name.Name)
				}
				ci := &classInfo{
					Name:        ts.Name.Name,
					Pos:         ts.Pos(),
					Decl:        ts,
					Doc:         tags,
					Description: synthDescription(ts.Name.Name, doc, tags),
					IsAbstract:  doctag.Has(tags, "abstract"),
					IsInner:     isInner,
					IsClass:     isClass,
					IsEditor:    doctag.Has(tags, "editor"),
				}
				// Parent resolution: only @class structs need (and get)
				// a parent. Inner classes embed framework types
				// without that meaning "extends" — they're just nested
				// declarations the codegen records but doesn't
				// register. For @class structs, exactly one embedded
				// field must carry @extends; that's the parent.
				if isClass {
					parent, parentImport, perr := findExtendsParent(fset, t, d.Imports)
					if perr != nil {
						return nil, perr
					}
					ci.Parent = parent
					ci.ParentImport = parentImport
				}
				structs[ts.Name.Name] = ci
			case *ast.Ident:
				if t.Name == "int" || t.Name == "int32" || t.Name == "int64" {
					intTypes[ts.Name.Name] = &enumInfo{Name: ts.Name.Name, Pos: ts.Pos()}
				}
			case *ast.InterfaceType:
				if doctag.Has(tags, "signals") {
					signalIfaces = append(signalIfaces, ts)
				}
			}
		}
	}

	// Identify the main class — the struct explicitly tagged @class.
	// Inner classes get bucketed by @innerclass; everything else (no
	// tag at all) is a regular Go struct that codegen ignores.
	var mainCandidates []*classInfo
	for _, ci := range structs {
		if ci.IsInner {
			if ci.IsEditor {
				return nil, fmt.Errorf("%s: @editor on inner class %s — inner classes aren't registered with Godot, so the init-level distinction has no effect",
					posStr(fset, ci.Pos), ci.Name)
			}
			d.InnerClasses = append(d.InnerClasses, ci)
			continue
		}
		if ci.IsClass {
			mainCandidates = append(mainCandidates, ci)
		}
	}
	switch len(mainCandidates) {
	case 0:
		return nil, fmt.Errorf("%s: no @class struct found — tag exactly one top-level struct with @class to register it as a Godot extension class",
			file.Name.Name)
	case 1:
		d.MainClass = mainCandidates[0]
	default:
		names := make([]string, 0, len(mainCandidates))
		for _, c := range mainCandidates {
			names = append(names, c.Name+"@"+posStr(fset, c.Pos))
		}
		return nil, fmt.Errorf("multiple @class structs (one per file is allowed): %s", strings.Join(names, ", "))
	}

	// Methods: walk all FuncDecls with named receivers matching one of the
	// discovered struct types. Static (unnamed receiver) methods are
	// identified by an empty Names slice on the receiver field.
	//
	// Classification rule (Phase 6):
	//   - unnamed receiver           → static
	//   - @override doctag           → override (registered as a Godot virtual)
	//   - else                       → regular instance method
	// Lowercase-first methods with no @override are treated as Go-private
	// helpers and skipped — they don't get registered with Godot at all,
	// matching Go's own export model. The @name doctag overrides the
	// derived Godot name verbatim; otherwise the name is snake_case(GoName)
	// with a leading `_` for overrides (or for cases where @name already
	// supplies a leading underscore explicitly).
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || len(fn.Recv.List) != 1 {
			continue
		}
		recvType, isPtr := receiverTypeName(fn.Recv.List[0].Type)
		ci, ok := structs[recvType]
		if !ok {
			continue
		}
		isStatic := len(fn.Recv.List[0].Names) == 0
		tags := doctag.Parse(fn.Doc)
		hasOverride := doctag.Has(tags, "override")
		// Skip Go-private methods that aren't explicit overrides — they
		// belong to the user's package, not to Godot's ClassDB.
		if !isStatic && !hasOverride && isLowerFirst(fn.Name.Name) {
			continue
		}
		mi := &methodInfo{
			GoName:       fn.Name.Name,
			Pos:          fn.Pos(),
			Decl:         fn,
			Doc:          tags,
			IsPointerRcv: isPtr,
		}
		switch {
		case isStatic:
			mi.Kind = methodStatic
		case hasOverride:
			mi.Kind = methodOverride
		default:
			mi.Kind = methodInstance
		}
		if name, ok := doctag.Find(mi.Doc, "name"); ok {
			mi.GodotName = name
		} else {
			mi.GodotName = pascalToSnake(fn.Name.Name)
			if mi.Kind == methodOverride && !strings.HasPrefix(mi.GodotName, "_") {
				mi.GodotName = "_" + mi.GodotName
			}
		}
		ci.Methods = append(ci.Methods, mi)
	}

	// Enums: a const block whose entries are typed as one of the discovered
	// int types becomes that enum's value list. The same const block can
	// hold values for multiple types; they get bucketed by declared type.
	for _, decl := range file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		// Within a `const ( … )` block, a spec with no explicit type carries
		// forward the previous spec's type — that's how `iota` continuation
		// entries work (only the first line names the type).
		carriedType := ""
		for _, spec := range gd.Specs {
			vs := spec.(*ast.ValueSpec)
			if id, ok := vs.Type.(*ast.Ident); ok {
				carriedType = id.Name
			}
			ei, ok := intTypes[carriedType]
			if !ok {
				continue
			}
			for _, name := range vs.Names {
				ei.Values = append(ei.Values, enumValue{Name: name.Name, Pos: name.Pos()})
			}
		}
	}
	for _, ei := range intTypes {
		if len(ei.Values) > 0 {
			d.Enums = append(d.Enums, ei)
		}
	}

	// Properties: walk the main class's struct fields and methods for
	// @property declarations. Two surface forms (see propertyInfo):
	//   - field form: an exported field with @property. We synthesize
	//     getter/setter methods at emit time.
	//   - method form: a Get<Name> method with @property. The user owns
	//     the getter (and optional setter); we just register.
	// The two forms must not collide for the same property name — that
	// would leave the binding ambiguous.
	if err := collectProperties(fset, d.MainClass); err != nil {
		return nil, err
	}

	// Signals: walk @signals-tagged interfaces. Each interface method
	// becomes a signal on the main class. Codegen attaches an emit
	// method directly to *<MainClass> for each signal — no embedded
	// struct in the user's source, no boilerplate beyond the interface
	// declaration itself.
	if err := collectSignals(fset, d.MainClass, signalIfaces); err != nil {
		return nil, err
	}

	// Parent is guaranteed non-empty here — findExtendsParent errors
	// out at type-collection time if the @class struct is missing
	// @extends or has more than one. No further validation needed.

	return d, nil
}

// collectSignals walks @signals-tagged interfaces and accumulates a flat
// list of signalInfo on the main class. Each interface method becomes one
// signal. Validation:
//   - signal name must not collide with a regular method registered on
//     the class (Go itself would reject the duplicate method declaration
//     at compile time, but catching it here gives the user a clearer
//     error pointing at both source positions).
//   - signal names across all @signals interfaces must be unique (same
//     reasoning).
//
// Argument types are not resolved here — the emitter does that via the
// shared resolveType path so unsupported types fail with the same
// diagnostic shape as method args.
func collectSignals(fset *token.FileSet, ci *classInfo, ifaces []*ast.TypeSpec) error {
	if len(ifaces) == 0 {
		return nil
	}
	// Build a name set of the class's regular methods for collision
	// checking. Use GoName (the source identifier) since that's what
	// would clash if codegen synthesizes `func (n *T) <SignalName>(...)`.
	methodNames := map[string]token.Pos{}
	for _, m := range ci.Methods {
		methodNames[m.GoName] = m.Pos
	}

	signalNames := map[string]token.Pos{}
	for _, iface := range ifaces {
		it := iface.Type.(*ast.InterfaceType)
		if it.Methods == nil {
			continue
		}
		for _, m := range it.Methods.List {
			if len(m.Names) == 0 {
				// Embedded interface — not supported as a signal carrier.
				return fmt.Errorf("%s: @signals interface %s embeds another interface; only direct method declarations become signals",
					posStr(fset, m.Pos()), iface.Name.Name)
			}
			ft, ok := m.Type.(*ast.FuncType)
			if !ok {
				continue
			}
			if ft.Results != nil && len(ft.Results.List) > 0 {
				return fmt.Errorf("%s: signal %s on @signals interface %s has a return value; signals don't return values",
					posStr(fset, m.Pos()), m.Names[0].Name, iface.Name.Name)
			}
			for _, name := range m.Names {
				if existing, dup := signalNames[name.Name]; dup {
					return fmt.Errorf("%s: duplicate signal %q (also at %s) — names must be unique across all @signals interfaces on a class",
						posStr(fset, name.Pos()), name.Name, posStr(fset, existing))
				}
				if existing, dup := methodNames[name.Name]; dup {
					return fmt.Errorf("%s: signal %q collides with regular method %s (at %s) — codegen would synthesize a method of that name on *%s, which Go would reject as a duplicate declaration",
						posStr(fset, name.Pos()), name.Name, name.Name, posStr(fset, existing), ci.Name)
				}
				signalNames[name.Name] = name.Pos()
				ci.Signals = append(ci.Signals, &signalInfo{
					Name:            name.Name,
					GodotName:       pascalToSnake(name.Name),
					Pos:             name.Pos(),
					Args:            ft.Params.List,
					SourceInterface: iface.Name.Name,
				})
			}
		}
	}
	return nil
}

// hintTagNames lists the export-hint doctag names recognized on field-form
// `@property` declarations. Each maps to a Godot PropertyHint enum value
// + a hint_string format described in the per-tag handler below. The list
// is used both at parse time (parsePropertyHints) and at validation time
// (rejecting hints on method-form properties).
var hintTagNames = []string{
	"export_range",
	"export_enum",
	"export_file",
	"export_dir",
	"export_multiline",
	"export_placeholder",
}

// parsePropertyHints walks a field's doctags and extracts @group,
// @subgroup, and @export_* hint info. Returns (group, subgroup, hint,
// hintString, err). hint is the bare PropertyHint enum const name
// (e.g. "PropertyHintRange"); empty means no hint specified.
//
// At most one @export_* tag may appear per field. @subgroup without
// @group is rejected — Godot's inspector nests subgroups under their
// parent group, and a free-floating subgroup degrades to a plain group
// in confusing ways.
func parsePropertyHints(fset *token.FileSet, tags []doctag.Tag, pos token.Pos) (group, subgroup, hint, hintString string, err error) {
	if v, ok := doctag.Find(tags, "group"); ok {
		args, perr := doctag.SplitArgs(v)
		if perr != nil {
			return "", "", "", "", fmt.Errorf("%s: @group: %w", posStr(fset, pos), perr)
		}
		if len(args) != 1 {
			return "", "", "", "", fmt.Errorf("%s: @group expects one quoted argument, got %d", posStr(fset, pos), len(args))
		}
		group = args[0]
	}
	if v, ok := doctag.Find(tags, "subgroup"); ok {
		args, perr := doctag.SplitArgs(v)
		if perr != nil {
			return "", "", "", "", fmt.Errorf("%s: @subgroup: %w", posStr(fset, pos), perr)
		}
		if len(args) != 1 {
			return "", "", "", "", fmt.Errorf("%s: @subgroup expects one quoted argument, got %d", posStr(fset, pos), len(args))
		}
		subgroup = args[0]
		if group == "" {
			return "", "", "", "", fmt.Errorf("%s: @subgroup without @group — subgroups must be nested under a group", posStr(fset, pos))
		}
	}

	// At most one export hint per field. Multiple is ambiguous and
	// would silently let one win; reject so the user picks.
	hintCount := 0
	for _, name := range hintTagNames {
		if doctag.Has(tags, name) {
			hintCount++
		}
	}
	if hintCount > 1 {
		return "", "", "", "", fmt.Errorf("%s: multiple @export_* hints on one field — pick one", posStr(fset, pos))
	}

	switch {
	case doctag.Has(tags, "export_range"):
		v, _ := doctag.Find(tags, "export_range")
		args, perr := doctag.SplitArgs(v)
		if perr != nil {
			return "", "", "", "", fmt.Errorf("%s: @export_range: %w", posStr(fset, pos), perr)
		}
		if len(args) != 2 && len(args) != 3 {
			return "", "", "", "", fmt.Errorf("%s: @export_range expects (min, max) or (min, max, step), got %d args", posStr(fset, pos), len(args))
		}
		hint = "PropertyHintRange"
		hintString = strings.Join(args, ",")
	case doctag.Has(tags, "export_enum"):
		v, _ := doctag.Find(tags, "export_enum")
		args, perr := doctag.SplitArgs(v)
		if perr != nil {
			return "", "", "", "", fmt.Errorf("%s: @export_enum: %w", posStr(fset, pos), perr)
		}
		if len(args) == 0 {
			return "", "", "", "", fmt.Errorf("%s: @export_enum requires at least one value", posStr(fset, pos))
		}
		hint = "PropertyHintEnum"
		hintString = strings.Join(args, ",")
	case doctag.Has(tags, "export_file"):
		v, _ := doctag.Find(tags, "export_file")
		args, perr := doctag.SplitArgs(v)
		if perr != nil {
			return "", "", "", "", fmt.Errorf("%s: @export_file: %w", posStr(fset, pos), perr)
		}
		hint = "PropertyHintFile"
		// @export_file with no args opens the picker without a filter.
		// @export_file("*.png") supplies the filter; multiple filters
		// are passed as comma-separated bare or quoted args. Either
		// form joins to a single comma-separated payload.
		hintString = strings.Join(args, ",")
	case doctag.Has(tags, "export_dir"):
		// Directory picker — payload is empty.
		hint = "PropertyHintDir"
	case doctag.Has(tags, "export_multiline"):
		// Multi-line text editor — payload is empty.
		hint = "PropertyHintMultilineText"
	case doctag.Has(tags, "export_placeholder"):
		v, _ := doctag.Find(tags, "export_placeholder")
		args, perr := doctag.SplitArgs(v)
		if perr != nil {
			return "", "", "", "", fmt.Errorf("%s: @export_placeholder: %w", posStr(fset, pos), perr)
		}
		if len(args) != 1 {
			return "", "", "", "", fmt.Errorf("%s: @export_placeholder expects one quoted argument, got %d", posStr(fset, pos), len(args))
		}
		hint = "PropertyHintPlaceholderText"
		hintString = args[0]
	}
	return group, subgroup, hint, hintString, nil
}

// collectProperties walks the main class's struct fields (for the field
// form) and its already-collected methods (for the method form), producing
// the consolidated propertyInfo slice on classInfo.
//
// Field form:  exported field tagged with @property → synthesize get/set.
//              An additional @readonly tag drops the setter.
// Method form: a method named GetX with @property → register; if SetX
//              exists alongside it, wire as setter, else read-only is
//              inferred. @readonly is NOT honored on methods — it's
//              redundant with the absence of SetX, and accepting it
//              would just create two ways to say the same thing.
//
// Conflict: a property name appearing in both forms is rejected here.
//
// Misuse caught here:
//   - @readonly on a method (any method, regardless of @property)
//   - @readonly on a field without @property
//   - @property on an unexported field
//   - field- and method-form colliding on the same name
func collectProperties(fset *token.FileSet, ci *classInfo) error {
	st, ok := ci.Decl.Type.(*ast.StructType)
	if !ok || st.Fields == nil {
		return nil
	}

	byName := map[string]*propertyInfo{}

	// Field form. Each ast.Field can declare multiple names sharing one
	// type; iterate names so each property has its own propertyInfo.
	for _, f := range st.Fields.List {
		if f.Doc == nil || len(f.Names) == 0 {
			continue
		}
		tags := doctag.Parse(f.Doc)
		hasProperty := doctag.Has(tags, "property")
		hasReadOnly := doctag.Has(tags, "readonly")
		if hasReadOnly && !hasProperty {
			return fmt.Errorf("%s: @readonly without @property — the @readonly tag only applies to fields also tagged @property",
				posStr(fset, f.Pos()))
		}
		if !hasProperty {
			continue
		}
		group, subgroup, hint, hintString, perr := parsePropertyHints(fset, tags, f.Pos())
		if perr != nil {
			return perr
		}
		for _, name := range f.Names {
			if !name.IsExported() {
				return fmt.Errorf("%s: field %q has @property but is unexported — properties must be on exported fields (capitalize) or on a Get<Name> method",
					posStr(fset, name.Pos()), name.Name)
			}
			if existing, dup := byName[name.Name]; dup {
				return fmt.Errorf("%s: duplicate @property %q (also at %s)",
					posStr(fset, name.Pos()), name.Name, posStr(fset, existing.Pos))
			}
			byName[name.Name] = &propertyInfo{
				Name:       name.Name,
				GoType:     f.Type,
				Pos:        name.Pos(),
				Source:     propertyFromField,
				ReadOnly:   hasReadOnly,
				Group:      group,
				Subgroup:   subgroup,
				Hint:       hint,
				HintString: hintString,
			}
		}
	}

	// Method form. A property is keyed off Get<Name> with @property; the
	// matching Set<Name> ALSO needs @property to be wired as the setter
	// — explicit on both sides, no silent pairing-by-name. @readonly on
	// any method is rejected: it's either redundant (when no SetX exists,
	// the property is already read-only) or contradictory (when SetX
	// exists), so it never carries useful information here.
	getters := map[string]*methodInfo{}
	setters := map[string]*methodInfo{}
	for _, m := range ci.Methods {
		if doctag.Has(m.Doc, "readonly") {
			return fmt.Errorf("%s: @readonly on method %s — @readonly only applies to fields tagged @property; method-form properties are read-only by virtue of having no matching Set<Name> (also tagged @property)",
				posStr(fset, m.Pos), m.GoName)
		}
		switch {
		case strings.HasPrefix(m.GoName, "Get") && len(m.GoName) > 3:
			getters[strings.TrimPrefix(m.GoName, "Get")] = m
		case strings.HasPrefix(m.GoName, "Set") && len(m.GoName) > 3:
			setters[strings.TrimPrefix(m.GoName, "Set")] = m
		}
	}

	// Orphan setters: @property on Set<X> without a matching Get<X>
	// also tagged @property is meaningless — there's no property to
	// attach it to. Treating it as a regular method would silently
	// drop the user's intent, so we surface it.
	for propName, setter := range setters {
		if !doctag.Has(setter.Doc, "property") {
			continue
		}
		getter, hasGetter := getters[propName]
		if !hasGetter || !doctag.Has(getter.Doc, "property") {
			return fmt.Errorf("%s: @property on Set%s but no matching Get%s with @property — both halves of a method-form property must be tagged @property",
				posStr(fset, setter.Pos), propName, propName)
		}
	}

	for propName, getter := range getters {
		if !doctag.Has(getter.Doc, "property") {
			continue
		}
		// Method-form properties don't support grouping or export hints
		// in v1 — those tags only make sense on a field, where codegen
		// has full control of the synthesized accessor. Reject early so
		// the user gets a clear error rather than silently-dropped tags.
		for _, banned := range append([]string{"group", "subgroup"}, hintTagNames...) {
			if doctag.Has(getter.Doc, banned) {
				return fmt.Errorf("%s: @%s on method-form @property %s — grouping and export hints are field-form only in this release",
					posStr(fset, getter.Pos), banned, getter.GoName)
			}
		}
		if existing, dup := byName[propName]; dup {
			// Conflict: the same Name appears as a field property AND a
			// method property. The user has to pick one to avoid the
			// binding being ambiguous about which getter to call.
			return fmt.Errorf("%s: ambiguous @property %q — declared as both an exported field (at %s) and a Get%s method; pick one form",
				posStr(fset, getter.Pos), propName, posStr(fset, existing.Pos), propName)
		}
		// Determine the property type from the getter's return signature.
		results := fieldsOf(getter.Decl.Type.Results)
		if len(results) != 1 || len(results[0].Names) > 1 {
			return fmt.Errorf("%s: @property method Get%s must return exactly one value",
				posStr(fset, getter.Pos), propName)
		}
		// The setter only counts when it ALSO has @property — explicit
		// on both sides. A Set<X> without @property is just a regular
		// method named set_<x> that happens to share the property's
		// stem; it gets registered through the normal method path but
		// is not wired as the property's setter.
		setter, hasSetter := setters[propName]
		setterIsTagged := hasSetter && doctag.Has(setter.Doc, "property")
		byName[propName] = &propertyInfo{
			Name:     propName,
			GoType:   results[0].Type,
			Pos:      getter.Pos,
			Source:   propertyFromMethod,
			ReadOnly: !setterIsTagged,
		}
	}

	if len(byName) == 0 {
		return nil
	}
	// Source-order iteration with a tier split: ungrouped properties
	// are emitted before grouped ones. Within each tier, source order
	// is preserved. The split lets users freely intermix
	// (method-form properties, which can't be grouped in v1, fall in
	// the ungrouped tier without forcing a particular struct layout)
	// while still meeting Godot's positional requirement that ungrouped
	// properties not visually fall under a previously-registered group.
	props := make([]*propertyInfo, 0, len(byName))
	for _, p := range byName {
		props = append(props, p)
	}
	sortByPos(props)

	var ungrouped, grouped []*propertyInfo
	for _, p := range props {
		if p.Group == "" {
			ungrouped = append(ungrouped, p)
		} else {
			grouped = append(grouped, p)
		}
	}
	if err := validateGroupOrdering(fset, grouped); err != nil {
		return err
	}
	ci.Properties = append(ci.Properties, ungrouped...)
	ci.Properties = append(ci.Properties, grouped...)
	return nil
}

// sortByPos orders props by source token.Pos. Insertion sort is fine
// here — property counts are tiny and the generated binding stays
// byte-stable.
func sortByPos(props []*propertyInfo) {
	for i := 1; i < len(props); i++ {
		for j := i; j > 0 && props[j-1].Pos > props[j].Pos; j-- {
			props[j-1], props[j] = props[j], props[j-1]
		}
	}
}

// validateGroupOrdering enforces contiguity for grouped properties:
//   - Properties of the same group must be contiguous (no coming back
//     to a group after switching away).
//   - Subgroups cycle the same way within their parent group.
//
// Receives only grouped properties (ungrouped tier is split out
// upstream). Rationale: Godot's inspector treats group registration
// positionally — RegisterClassPropertyGroup("X") makes every
// subsequent property belong to X until another group call. Allowing
// non-contiguous grouping would force codegen to either re-register
// groups (creating duplicate inspector headers) or reorder properties
// (losing source-order intent within a group). The strict rule keeps
// the user's source visually matching what they get in the inspector.
func validateGroupOrdering(fset *token.FileSet, props []*propertyInfo) error {
	var (
		seenGroup    = map[string]token.Pos{}
		seenSubgroup = map[string]token.Pos{}
		curGroup     string
		curSubgroup  string
	)
	for _, p := range props {
		if p.Group != curGroup {
			// Switching to a new group. If we've seen this group name
			// before, the user is going back — reject so the inspector
			// doesn't get duplicate headers.
			if existing, ok := seenGroup[p.Group]; ok && existing != p.Pos {
				return fmt.Errorf("%s: @group(%q) on %q reuses a group already left at %s — properties of the same group must be contiguous in source",
					posStr(fset, p.Pos), p.Group, p.Name, posStr(fset, existing))
			}
			seenGroup[p.Group] = p.Pos
			curGroup = p.Group
			// New group resets subgroup tracking.
			curSubgroup = ""
			seenSubgroup = map[string]token.Pos{}
		}
		if p.Subgroup != curSubgroup {
			if existing, ok := seenSubgroup[p.Subgroup]; ok && p.Subgroup != "" && existing != p.Pos {
				return fmt.Errorf("%s: @subgroup(%q) on %q reuses a subgroup already left at %s — properties of the same subgroup must be contiguous",
					posStr(fset, p.Pos), p.Subgroup, p.Name, posStr(fset, existing))
			}
			if p.Subgroup != "" {
				seenSubgroup[p.Subgroup] = p.Pos
			}
			curSubgroup = p.Subgroup
		}
	}
	return nil
}

// findExtendsParent locates the parent class for a `@class` struct by
// walking its fields for the one tagged with `@extends`. The tag must
// sit on an *embedded* (anonymous) field whose type resolves to a
// framework package — that's the GDScript-equivalent `extends Node`
// declaration.
//
// Validation surfaces:
//   - no field carries @extends → error (the class needs a parent).
//   - multiple fields carry @extends → error (single inheritance).
//   - @extends on a named (non-embedded) field → error (parent
//     inheritance must come from an embed; named fields are
//     composition).
//   - @extends on an embed whose type isn't a framework package →
//     error (cross-module class inheritance isn't supported yet).
//
// Returns the bare class name (e.g. "Node") + the framework import
// path it came from.
func findExtendsParent(fset *token.FileSet, st *ast.StructType, imports map[string]string) (parent, parentImport string, err error) {
	if st.Fields == nil {
		return "", "", fmt.Errorf("@class struct has no fields — it needs an embedded framework type tagged @extends")
	}
	hits := 0
	for _, f := range st.Fields.List {
		if !doctag.Has(doctag.Parse(f.Doc), "extends") {
			continue
		}
		hits++
		if len(f.Names) != 0 {
			return "", "", fmt.Errorf("%s: @extends on named field %q — @extends marks the *embedded* parent type (anonymous field), not a regular composition field",
				posStr(fset, f.Pos()), f.Names[0].Name)
		}
		sel, ok := f.Type.(*ast.SelectorExpr)
		if !ok {
			return "", "", fmt.Errorf("%s: @extends on field with type %T — must be an embedded framework type like core.Node",
				posStr(fset, f.Pos()), f.Type)
		}
		pkgIdent, ok := sel.X.(*ast.Ident)
		if !ok {
			return "", "", fmt.Errorf("%s: @extends on field with non-package selector — must be an embedded framework type like core.Node",
				posStr(fset, f.Pos()))
		}
		path, ok := imports[pkgIdent.Name]
		if !ok || !frameworkPkgs[path] {
			return "", "", fmt.Errorf("%s: @extends on %s.%s — type must come from a framework package (core, editor, godot, variant); cross-module class inheritance isn't supported",
				posStr(fset, f.Pos()), pkgIdent.Name, sel.Sel.Name)
		}
		parent = sel.Sel.Name
		parentImport = path
	}
	switch hits {
	case 0:
		return "", "", fmt.Errorf("@class struct has no @extends — tag the embedded framework type that this class extends (Godot's single-inheritance equivalent of `extends Node` in GDScript)")
	case 1:
		return parent, parentImport, nil
	default:
		return "", "", fmt.Errorf("multiple @extends fields — Godot is single-inheritance, pick one")
	}
}

// receiverTypeName returns the bare type name of a method receiver and
// whether it was declared as a pointer (`*T`). Receivers of generic types
// (`T[X]`) aren't supported here — Godot extension classes can't be
// generic anyway, so we just bail with an empty name.
func receiverTypeName(expr ast.Expr) (name string, isPointer bool) {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name, false
	case *ast.StarExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			return id.Name, true
		}
	}
	return "", false
}

// docOf returns the doc comment that "belongs" to a TypeSpec. Single-spec
// `type X struct{…}` declarations attach the doc to the parent GenDecl;
// multi-spec ones attach per-TypeSpec. We check both.
func docOf(gd *ast.GenDecl, ts *ast.TypeSpec) *ast.CommentGroup {
	if ts.Doc != nil {
		return ts.Doc
	}
	return gd.Doc
}

// synthDescription mirrors PLAN_EXECUTION.md item 8: drop the identifier
// prefix from the leading sentence, uppercase the first surviving word,
// unless an explicit @description tag overrides.
func synthDescription(typeName string, cg *ast.CommentGroup, tags []doctag.Tag) string {
	if v, ok := doctag.Find(tags, "description"); ok {
		return v
	}
	if cg == nil {
		return ""
	}
	// Take the first non-tag, non-empty line of prose.
	for _, c := range cg.List {
		text := strings.TrimSpace(strings.TrimPrefix(c.Text, "//"))
		text = strings.TrimSpace(strings.TrimPrefix(text, "*"))
		if text == "" || strings.HasPrefix(text, "@") {
			continue
		}
		// Drop "TypeName " prefix if present.
		if strings.HasPrefix(text, typeName+" ") {
			text = text[len(typeName)+1:]
		}
		// Uppercase first surviving rune.
		if text == "" {
			continue
		}
		runes := []rune(text)
		runes[0] = unicode.ToUpper(runes[0])
		return string(runes)
	}
	return ""
}

func isLowerFirst(name string) bool {
	if name == "" {
		return false
	}
	r := []rune(name)[0]
	return unicode.IsLower(r)
}

// pascalToSnake converts Go-style PascalCase to Godot-style snake_case.
// Acronyms run together (e.g. `HTTPServer` → `http_server`). The mapping
// matches what the engine bindgen does when going the other way — see
// cmd/godot-go-bindgen/naming.go.
func pascalToSnake(s string) string {
	var b strings.Builder
	prevUpper := false
	for i, r := range s {
		isUpper := unicode.IsUpper(r)
		if i > 0 && isUpper && !prevUpper {
			b.WriteByte('_')
		}
		// Handle acronym → word boundary, e.g. `HTTPServer`: at the `S`
		// after `HTTP`, prevUpper is true but the *next* rune is lower,
		// so a separator belongs before this char.
		if i > 0 && isUpper && prevUpper {
			next, ok := nextRune(s, i, r)
			if ok && unicode.IsLower(next) {
				b.WriteByte('_')
			}
		}
		b.WriteRune(unicode.ToLower(r))
		prevUpper = isUpper
	}
	return b.String()
}

func nextRune(s string, i int, cur rune) (rune, bool) {
	off := i + len(string(cur))
	if off >= len(s) {
		return 0, false
	}
	for _, r := range s[off:] {
		return r, true
	}
	return 0, false
}

func posStr(fset *token.FileSet, pos token.Pos) string {
	p := fset.Position(pos)
	if p.Filename == "" {
		return fmt.Sprintf("%d:%d", p.Line, p.Column)
	}
	return fmt.Sprintf("%s:%d:%d", p.Filename, p.Line, p.Column)
}
