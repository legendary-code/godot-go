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

	Methods []*methodInfo
}

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
	// classes) from named-int types (candidate enums); other type aliases
	// are out of scope here.
	structs := map[string]*classInfo{}
	intTypes := map[string]*enumInfo{}
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
				ci := &classInfo{
					Name:        ts.Name.Name,
					Pos:         ts.Pos(),
					Decl:        ts,
					Doc:         tags,
					Description: synthDescription(ts.Name.Name, doc, tags),
					IsAbstract:  doctag.Has(tags, "abstract"),
					IsInner:     doctag.Has(tags, "innerclass"),
				}
				if parent, parentImport, err := findEmbeddedParent(t, d.Imports); err != nil {
					return nil, fmt.Errorf("%s: %w", posStr(fset, ts.Pos()), err)
				} else {
					ci.Parent = parent
					ci.ParentImport = parentImport
				}
				structs[ts.Name.Name] = ci
			case *ast.Ident:
				if t.Name == "int" || t.Name == "int32" || t.Name == "int64" {
					intTypes[ts.Name.Name] = &enumInfo{Name: ts.Name.Name, Pos: ts.Pos()}
				}
			}
		}
	}

	// Identify the main class — the unique struct without @innerclass. Zero
	// or multiple top-level non-inner structs is a configuration error.
	var mainCandidates []*classInfo
	for _, ci := range structs {
		if ci.IsInner {
			d.InnerClasses = append(d.InnerClasses, ci)
			continue
		}
		mainCandidates = append(mainCandidates, ci)
	}
	switch len(mainCandidates) {
	case 0:
		return nil, fmt.Errorf("%s: no top-level extension class found (need exactly one struct without @innerclass)", file.Name.Name)
	case 1:
		d.MainClass = mainCandidates[0]
	default:
		names := make([]string, 0, len(mainCandidates))
		for _, c := range mainCandidates {
			names = append(names, c.Name+"@"+posStr(fset, c.Pos))
		}
		return nil, fmt.Errorf("multiple top-level extension classes (need exactly one without @innerclass): %s", strings.Join(names, ", "))
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

	// Validate: a non-inner class must have a parent. Inner classes don't.
	if d.MainClass.Parent == "" {
		return nil, fmt.Errorf("%s: class %s has no recognized base class — embed a framework type (e.g. core.Node) to inherit from a Godot class",
			posStr(fset, d.MainClass.Pos), d.MainClass.Name)
	}

	return d, nil
}

// findEmbeddedParent walks the struct fields looking for a *single*
// anonymous (embedded) field whose type lives in a framework package.
// Returns ("", "", nil) when no embedded framework field is present —
// the caller decides whether that's an error (main class) or fine
// (inner class).
//
// More than one framework embed → error (Godot is single-inheritance).
func findEmbeddedParent(st *ast.StructType, imports map[string]string) (parent, parentImport string, err error) {
	if st.Fields == nil {
		return "", "", nil
	}
	hits := 0
	for _, f := range st.Fields.List {
		if len(f.Names) != 0 {
			continue // named field, not embedded
		}
		sel, ok := f.Type.(*ast.SelectorExpr)
		if !ok {
			continue
		}
		pkgIdent, ok := sel.X.(*ast.Ident)
		if !ok {
			continue
		}
		path, ok := imports[pkgIdent.Name]
		if !ok || !frameworkPkgs[path] {
			continue
		}
		hits++
		parent = sel.Sel.Name
		parentImport = path
	}
	if hits > 1 {
		return "", "", fmt.Errorf("multiple embedded framework types — Godot is single-inheritance, pick one")
	}
	return parent, parentImport, nil
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
