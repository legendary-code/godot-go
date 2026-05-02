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

// builtinView is the data the builtin-class template consumes. Each entry's
// Body is pre-rendered into Go source by the build* helpers below — the
// template only stitches the file together.
type builtinView struct {
	Pkg           string // target package name
	ModulePath    string // framework module path (for the gdextension import)
	GoName        string // e.g. "Vector2"
	Size          int    // bytes (per builtin_class_sizes)
	VariantType   string // e.g. "VariantTypeVector2"
	HasDestructor bool

	Members      []memberView
	Constructors []callableView
	Methods      []callableView
	Operators    []callableView
	Indexed      *indexedView
	Keyed        *keyedView

	// CacheVars carries every cached resolved-fn-ptr we declare at file scope.
	// Each cacheVar is rendered as `var <Name> <Type>` — for the lazy pattern,
	// Type starts with "= sync.OnceValue(...)" so the var becomes a
	// `sync.OnceValue[T]` and call sites invoke it via `<Name>()`.
	CacheVars []cacheVar
}

type memberView struct {
	GoName string // PascalCase
	Offset int
	GoType string // Go type for the field accessor
}

type cacheVar struct {
	Name string
	// Type is whatever follows the variable name in `var <Name> <Type>`.
	// For the lazy pattern, this is a `= sync.OnceValue(...)` initializer
	// (with the type inferred from the closure's return).
	Type string
}

// callableView is the shared shape for emitted constructors/methods/operators.
// Sig is the Go function signature *without* `func` and *with* the receiver
// (or empty for package-level functions). Body is the function body without
// the surrounding braces.
type callableView struct {
	GoDoc string
	Sig   string
	Body  string
}

type indexedView struct {
	GoName    string // emitted Get/Set names
	GoType    string // Go return type of Get / arg type of Set
	GetSig    string
	GetBody   string
	HasSetter bool
	SetSig    string
	SetBody   string
}

type keyedView struct {
	GetBody string
	SetBody string
}

// emitBuiltinClass writes <cfg.Dir>/<lower(name)>.gen.go for the given builtin
// class. Methods/constructors/operators that reference unsupported types
// (Object, missing builtins) are silently skipped; the rest are emitted.
// emitBuiltinClasses renders every supported builtin class into a single
// consolidated file at <cfg.Dir>/builtins.gen.go. Each builtin's body is
// rendered in api.BuiltinClasses order; the shared package header
// (preamble + imports) is written once at the top.
//
// Replaces the per-builtin `<lower(name)>.gen.go` output to keep the
// bindings directory navigable. Imports are uniform across builtins
// (sync, unsafe, gdextension) so no per-class import accumulation is
// needed.
func emitBuiltinClasses(api *API, buildConfig string, cfg *genConfig) (emittedNames []string, err error) {
	imports := map[string]bool{
		"sync":                            true,
		"unsafe":                          true,
		cfg.ModulePath + "/gdextension":   true,
	}
	var bodies bytes.Buffer
	for i := range api.BuiltinClasses {
		bc := &api.BuiltinClasses[i]
		if goNativePrimitives[bc.Name] {
			continue
		}
		if err := emitBuiltinClassBody(api, buildConfig, bc, cfg, &bodies); err != nil {
			return nil, fmt.Errorf("builtin %s: %w", bc.Name, err)
		}
		bodies.WriteByte('\n')
		emittedNames = append(emittedNames, bc.Name)
	}

	var out bytes.Buffer
	writeFileHeader(&out, cfg.Package, imports)
	out.Write(bodies.Bytes())

	formatted, ferr := format.Source(out.Bytes())
	if ferr != nil {
		broken := filepath.Join(cfg.Dir, "builtins.gen.go.broken")
		_ = os.WriteFile(broken, out.Bytes(), 0o644)
		return nil, fmt.Errorf("gofmt builtins.gen.go: %w (raw at %s)", ferr, broken)
	}
	if werr := os.WriteFile(filepath.Join(cfg.Dir, "builtins.gen.go"), formatted, 0o644); werr != nil {
		return nil, werr
	}
	return emittedNames, nil
}

// emitBuiltinClassBody renders one builtin's body (no header) into buf.
// Pulled out of the previous emitBuiltinClass entry point so
// emitBuiltinClasses can stream every builtin into the same file.
func emitBuiltinClassBody(api *API, buildConfig string, bc *BuiltinClass, cfg *genConfig, buf *bytes.Buffer) error {
	size, ok := api.SizeFor(buildConfig, bc.Name)
	if !ok {
		return fmt.Errorf("no size for %s under %s", bc.Name, buildConfig)
	}
	offsets := api.OffsetsFor(buildConfig, bc.Name)

	view := &builtinView{
		Pkg:           cfg.Package,
		ModulePath:    cfg.ModulePath,
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

	// Variant from/to type — every emitted builtin gets these.
	addLazyVar(view, lowerFirst(bc.Name)+"FromType", "gdextension.VariantFromTypeFunc",
		fmt.Sprintf("gdextension.GetVariantFromTypeConstructor(gdextension.%s)", view.VariantType))
	addLazyVar(view, lowerFirst(bc.Name)+"ToType", "gdextension.VariantToTypeFunc",
		fmt.Sprintf("gdextension.GetVariantToTypeConstructor(gdextension.%s)", view.VariantType))
	if bc.HasDestructor {
		dvar := lowerFirst(bc.Name) + "Dtor"
		addLazyVar(view, dvar, "gdextension.PtrDestructor",
			fmt.Sprintf("gdextension.GetPtrDestructor(gdextension.%s)", view.VariantType))
	}

	for _, c := range bc.Constructors {
		cv, ok := buildConstructorView(api, bc, c, view)
		if !ok {
			continue
		}
		view.Constructors = append(view.Constructors, cv)
	}

	for _, m := range bc.Methods {
		if m.IsVararg {
			continue
		}
		mv, ok := buildMethodView(api, bc, m, view)
		if !ok {
			continue
		}
		view.Methods = append(view.Methods, mv)
	}

	for _, op := range bc.Operators {
		ov, ok := buildOperatorView(api, bc, op, view)
		if !ok {
			continue
		}
		view.Operators = append(view.Operators, ov)
	}

	if bc.IndexingReturnType != "" && isSupportedType(api, bc.IndexingReturnType) {
		if iv, ok := buildIndexedView(api, bc, view); ok {
			view.Indexed = &iv
		}
	}

	if err := builtinTmpl.Execute(buf, view); err != nil {
		return fmt.Errorf("template: %w", err)
	}
	return nil
}

// addLazyVar appends a `<name> = sync.OnceValue(func() <retType> { return <body> })`
// declaration to view.CacheVars. Call sites reference these as `<name>()` so
// the resolution happens on first use rather than at package init.
func addLazyVar(view *builtinView, name, retType, body string) {
	init := "= sync.OnceValue(func() " + retType + " {\n\treturn " + body + "\n})"
	view.CacheVars = append(view.CacheVars, cacheVar{Name: name, Type: init})
}

// argPrep returns the (prep, passExpr) pair for an argument. prep is Go code
// that declares any temp boundary value and arranges destruction; passExpr
// is the expression handed to `gdextension.TypePtr(...)`.
//
// PtrCall ABI: Godot's GDExtension encodes "float" as double (8 bytes) for
// binary stability across single/double precision builds — even when the
// underlying real_t is float32. So float args/returns travel through a
// float64 slot at the boundary even though the user-facing Go type stays
// float32. Same trick won't be needed for "int" (already int64) or "bool"
// (Godot encodes as 1-byte bool).
func argPrep(godotType, goName string) (prep, passExpr string) {
	_, kind := goType(godotType)
	switch kind {
	case kindString:
		tmp := "tmp_" + goName
		prep = fmt.Sprintf("var %s String\n\tstringFromGo(&%s, %s)\n\tdefer stringDestroy(&%s)\n\t", tmp, tmp, goName, tmp)
		passExpr = fmt.Sprintf("gdextension.TypePtr(unsafe.Pointer(&%s))", tmp)
	case kindStringName:
		tmp := "tmp_" + goName
		prep = fmt.Sprintf("var %s StringName\n\tstringNameFromGo(&%s, %s)\n\tdefer stringNameDestroy(&%s)\n\t", tmp, tmp, goName, tmp)
		passExpr = fmt.Sprintf("gdextension.TypePtr(unsafe.Pointer(&%s))", tmp)
	case kindNodePath:
		tmp := "tmp_" + goName
		prep = fmt.Sprintf("var %s NodePath\n\tnodePathFromGo(&%s, %s)\n\tdefer nodePathDestroy(&%s)\n\t", tmp, tmp, goName, tmp)
		passExpr = fmt.Sprintf("gdextension.TypePtr(unsafe.Pointer(&%s))", tmp)
	case kindFloat:
		tmp := "tmp_" + goName
		prep = fmt.Sprintf("%s := float64(%s)\n\t", tmp, goName)
		passExpr = fmt.Sprintf("gdextension.TypePtr(unsafe.Pointer(&%s))", tmp)
	default:
		passExpr = fmt.Sprintf("gdextension.TypePtr(unsafe.Pointer(&%s))", goName)
	}
	return
}

// returnPrep computes the prologue/epilogue for a return value. retGoType is
// the user-facing Go return type (empty for void). retArgExpr is the pointer
// passed to the call; it's "nil" for void. For boundary types, declRet
// declares the raw temp, finalize converts/destroys, and retExpr is the
// final return expression.
type retPlan struct {
	GoType    string // "" for void
	DeclRet   string // e.g. "var ret float32" or "var raw String"
	RetArg    string // pointer expression passed to the call, or "nil"
	Finalize  string // any defer/cleanup line(s) before the return, or ""
	RetExpr   string // expression returned, "" for void
	IsVoid    bool
}

func returnPrep(godotType string) (retPlan, bool) {
	if godotType == "" || godotType == "void" {
		return retPlan{IsVoid: true, RetArg: "nil"}, true
	}
	g, kind := goType(godotType)
	if kind == kindUnsupported {
		return retPlan{}, false
	}
	switch kind {
	case kindBool:
		return retPlan{GoType: "bool", DeclRet: "var ret bool", RetArg: "gdextension.TypePtr(unsafe.Pointer(&ret))", RetExpr: "ret"}, true
	case kindInt:
		return retPlan{GoType: "int64", DeclRet: "var ret int64", RetArg: "gdextension.TypePtr(unsafe.Pointer(&ret))", RetExpr: "ret"}, true
	case kindFloat:
		// PtrCall returns a "float" as 8-byte double; widen the buffer and
		// narrow at the user-facing boundary. See argPrep for the same rule.
		return retPlan{GoType: "float32",
			DeclRet: "var raw float64",
			RetArg:  "gdextension.TypePtr(unsafe.Pointer(&raw))",
			RetExpr: "float32(raw)"}, true
	case kindString:
		return retPlan{GoType: "string",
			DeclRet:  "var raw String",
			RetArg:   "gdextension.TypePtr(unsafe.Pointer(&raw))",
			Finalize: "defer stringDestroy(&raw)",
			RetExpr:  "stringToGo(&raw)"}, true
	case kindStringName:
		return retPlan{GoType: "string",
			DeclRet:  "var raw StringName",
			RetArg:   "gdextension.TypePtr(unsafe.Pointer(&raw))",
			Finalize: "defer stringNameDestroy(&raw)",
			RetExpr:  "stringNameToGo(&raw)"}, true
	case kindNodePath:
		return retPlan{GoType: "string",
			DeclRet:  "var raw NodePath",
			RetArg:   "gdextension.TypePtr(unsafe.Pointer(&raw))",
			Finalize: "defer nodePathDestroy(&raw)",
			RetExpr:  "nodePathToGo(&raw)"}, true
	case kindVariant:
		return retPlan{GoType: "Variant",
			DeclRet: "var ret Variant",
			RetArg:  "gdextension.TypePtr(unsafe.Pointer(&ret))",
			RetExpr: "ret"}, true
	case kindBuiltin:
		return retPlan{GoType: g,
			DeclRet: "var ret " + g,
			RetArg:  "gdextension.TypePtr(unsafe.Pointer(&ret))",
			RetExpr: "ret"}, true
	}
	return retPlan{}, false
}

// isSupportedType returns true if the bindgen can emit code referencing this
// Godot type. For builtin classes it confirms the class is also in
// extension_api.json#builtin_classes — we don't emit references to engine
// classes (Object, etc.) until Phase 3.
func isSupportedType(api *API, godotType string) bool {
	g, kind := goType(godotType)
	if kind == kindUnsupported {
		return false
	}
	if kind == kindBuiltin {
		// Strip array element types — typed arrays collapse to "Array" which
		// is itself a builtin. The "Array" class exists, so this is fine.
		return api.FindBuiltin(g) != nil
	}
	return true
}

// buildArgs renders prep code and the args[...] literal for a list of
// arguments. Returns ("", "nil", true) for the empty list. The third return
// value is false if any argument is unsupported.
func buildArgs(api *API, args []Argument) (string, string, bool) {
	if len(args) == 0 {
		return "", "nil", true
	}
	for _, a := range args {
		if !isSupportedType(api, a.Type) {
			return "", "", false
		}
	}
	var prepB strings.Builder
	var elemsB strings.Builder
	elemsB.WriteString("args := [...]gdextension.TypePtr{\n")
	for _, a := range args {
		gname := safeIdent(a.Name)
		prep, expr := argPrep(a.Type, gname)
		if prep != "" {
			prepB.WriteString(prep)
			prepB.WriteString("\n\t")
		}
		fmt.Fprintf(&elemsB, "\t\t%s,\n", expr)
	}
	elemsB.WriteString("\t}\n\t")
	return prepB.String() + elemsB.String(), "args[:]", true
}

// paramSig builds the Go parameter list for a function signature.
func paramSig(args []Argument) string {
	if len(args) == 0 {
		return ""
	}
	parts := make([]string, 0, len(args))
	for _, a := range args {
		gtype, _ := goType(a.Type)
		parts = append(parts, safeIdent(a.Name)+" "+gtype)
	}
	return strings.Join(parts, ", ")
}

func ctorGoName(class string, c Constructor) string {
	if len(c.Arguments) == 0 {
		return "New" + class
	}
	// Single-arg ctors are named after the arg type ("NewArrayFromPackedByteArray"
	// rather than "NewArrayFrom") because Godot reuses "from" as the arg name
	// across overloads — the type is what distinguishes them. Multi-arg
	// ctors stick with arg names ("NewVector2XY") since named axes read better.
	if len(c.Arguments) == 1 {
		// Use the *Godot* type, not the mapped Go type — "String", "StringName",
		// and "NodePath" all flatten to Go's "string" in the user-facing API,
		// so the function name has to carry the original distinction to remain
		// unique.
		return "New" + class + "From" + suffixFor(c.Arguments[0].Type)
	}
	parts := []string{"New", class}
	for _, a := range c.Arguments {
		parts = append(parts, pascal(a.Name))
	}
	return strings.Join(parts, "")
}

func buildConstructorView(api *API, bc *BuiltinClass, c Constructor, view *builtinView) (callableView, bool) {
	for _, a := range c.Arguments {
		if !isSupportedType(api, a.Type) {
			return callableView{}, false
		}
	}
	cacheName := fmt.Sprintf("%sCtor%d", lowerFirst(bc.Name), c.Index)
	addLazyVar(view, cacheName, "gdextension.PtrConstructor",
		fmt.Sprintf("gdextension.GetPtrConstructor(gdextension.%s, %d)", view.VariantType, c.Index))

	name := ctorGoName(bc.Name, c)
	sig := fmt.Sprintf("%s(%s) %s", name, paramSig(c.Arguments), bc.Name)
	prep, argsExpr, ok := buildArgs(api, c.Arguments)
	if !ok {
		return callableView{}, false
	}

	var b strings.Builder
	fmt.Fprintf(&b, "var v %s\n\t", bc.Name)
	if prep != "" {
		b.WriteString(prep)
	}
	fmt.Fprintf(&b, "gdextension.CallPtrConstructor(%s(), gdextension.TypePtr(unsafe.Pointer(&v)), %s)\n\treturn v",
		cacheName, argsExpr)

	doc := fmt.Sprintf("// %s constructs a %s via the host (constructor index %d).", name, bc.Name, c.Index)
	return callableView{GoDoc: doc, Sig: sig, Body: b.String()}, true
}

func buildMethodView(api *API, bc *BuiltinClass, m Method, view *builtinView) (callableView, bool) {
	if !isSupportedType(api, "Variant") && false {
		// placeholder — Variant is always supported
	}
	if m.ReturnType != "" && m.ReturnType != "void" && !isSupportedType(api, m.ReturnType) {
		return callableView{}, false
	}
	for _, a := range m.Arguments {
		if !isSupportedType(api, a.Type) {
			return callableView{}, false
		}
	}

	goMethod := pascal(m.Name)
	cacheName := fmt.Sprintf("%sMethod%s", lowerFirst(bc.Name), goMethod)
	addLazyVar(view, cacheName, "gdextension.PtrBuiltInMethod",
		fmt.Sprintf("gdextension.GetPtrBuiltinMethod(gdextension.%s, internStringName(%q), %d)",
			view.VariantType, m.Name, m.Hash))

	plan, ok := returnPrep(m.ReturnType)
	if !ok {
		return callableView{}, false
	}

	prep, argsExpr, ok := buildArgs(api, m.Arguments)
	if !ok {
		return callableView{}, false
	}

	var sig string
	var basePtrExpr string
	if m.IsStatic {
		sig = fmt.Sprintf("%s%s(%s)", bc.Name, goMethod, paramSig(m.Arguments))
		basePtrExpr = "nil"
	} else {
		sig = fmt.Sprintf("(self *%s) %s(%s)", bc.Name, goMethod, paramSig(m.Arguments))
		basePtrExpr = "gdextension.TypePtr(unsafe.Pointer(self))"
	}
	if plan.GoType != "" {
		sig += " " + plan.GoType
	}

	var b strings.Builder
	if plan.DeclRet != "" {
		b.WriteString(plan.DeclRet)
		b.WriteString("\n\t")
	}
	if plan.Finalize != "" {
		b.WriteString(plan.Finalize)
		b.WriteString("\n\t")
	}
	if prep != "" {
		b.WriteString(prep)
	}
	fmt.Fprintf(&b, "gdextension.CallPtrBuiltinMethod(%s(), %s, %s, %s)",
		cacheName, basePtrExpr, argsExpr, plan.RetArg)
	if plan.RetExpr != "" {
		b.WriteString("\n\treturn ")
		b.WriteString(plan.RetExpr)
	}

	doc := fmt.Sprintf("// %s mirrors the Godot %s.%s method.", goMethod, bc.Name, m.Name)
	return callableView{GoDoc: doc, Sig: sig, Body: b.String()}, true
}

func buildOperatorView(api *API, bc *BuiltinClass, op Operator, view *builtinView) (callableView, bool) {
	// op.RightType=="Variant" means "any Variant on the right" — the operator
	// evaluator handles that via the Nil-typed right slot, but the user-facing
	// surface gets noisy fast (every type gains an Eq(Variant), Ne(Variant), …).
	// Skip them in Phase 2c; we still emit the typed fast paths.
	if op.RightType == "Variant" {
		return callableView{}, false
	}
	if op.RightType != "" && !isSupportedType(api, op.RightType) {
		return callableView{}, false
	}
	if !isSupportedType(api, op.ReturnType) {
		return callableView{}, false
	}
	enum, ok := operatorEnumMap[op.Name]
	if !ok {
		return callableView{}, false
	}
	goName, err := opGoName(bc.Name, op.Name, op.RightType)
	if err != nil {
		return callableView{}, false
	}

	rightVariantType := "VariantTypeNil"
	if op.RightType != "" {
		rightVariantType = "VariantType" + suffixFor(op.RightType)
	}

	cacheName := fmt.Sprintf("%sOp%s", lowerFirst(bc.Name), goName)
	addLazyVar(view, cacheName, "gdextension.PtrOperatorEvaluator",
		fmt.Sprintf("gdextension.GetPtrOperatorEvaluator(gdextension.%s, gdextension.%s, gdextension.%s)",
			enum, view.VariantType, rightVariantType))

	plan, ok := returnPrep(op.ReturnType)
	if !ok {
		return callableView{}, false
	}
	if plan.IsVoid {
		// Operators always produce a value.
		return callableView{}, false
	}

	// Sig: (v *Self) Name(rhs RhsType) RetType
	var paramList string
	var rightExpr string
	if op.RightType == "" {
		// unary
		rightExpr = "nil"
	} else {
		argName := "rhs"
		gtype, _ := goType(op.RightType)
		paramList = argName + " " + gtype
		_, kind := goType(op.RightType)
		switch kind {
		case kindString:
			// Add prep above the call
			rightExpr = "gdextension.TypePtr(unsafe.Pointer(&tmp_rhs))"
		case kindStringName:
			rightExpr = "gdextension.TypePtr(unsafe.Pointer(&tmp_rhs))"
		case kindNodePath:
			rightExpr = "gdextension.TypePtr(unsafe.Pointer(&tmp_rhs))"
		default:
			rightExpr = "gdextension.TypePtr(unsafe.Pointer(&rhs))"
		}
	}
	sig := fmt.Sprintf("(self *%s) %s(%s) %s", bc.Name, goName, paramList, plan.GoType)

	var b strings.Builder
	b.WriteString(plan.DeclRet)
	b.WriteString("\n\t")
	if plan.Finalize != "" {
		b.WriteString(plan.Finalize)
		b.WriteString("\n\t")
	}
	if op.RightType != "" {
		// Boundary prep on rhs.
		_, kind := goType(op.RightType)
		switch kind {
		case kindString:
			b.WriteString("var tmp_rhs String\n\tstringFromGo(&tmp_rhs, rhs)\n\tdefer stringDestroy(&tmp_rhs)\n\t")
		case kindStringName:
			b.WriteString("var tmp_rhs StringName\n\tstringNameFromGo(&tmp_rhs, rhs)\n\tdefer stringNameDestroy(&tmp_rhs)\n\t")
		case kindNodePath:
			b.WriteString("var tmp_rhs NodePath\n\tnodePathFromGo(&tmp_rhs, rhs)\n\tdefer nodePathDestroy(&tmp_rhs)\n\t")
		}
	}
	fmt.Fprintf(&b, "gdextension.CallPtrOperatorEvaluator(%s(), gdextension.TypePtr(unsafe.Pointer(self)), %s, %s)\n\treturn %s",
		cacheName, rightExpr, plan.RetArg, plan.RetExpr)

	doc := fmt.Sprintf("// %s mirrors the Godot %s %s operator.", goName, bc.Name, op.Name)
	return callableView{GoDoc: doc, Sig: sig, Body: b.String()}, true
}

func buildIndexedView(api *API, bc *BuiltinClass, view *builtinView) (indexedView, bool) {
	if !isSupportedType(api, bc.IndexingReturnType) {
		return indexedView{}, false
	}
	plan, ok := returnPrep(bc.IndexingReturnType)
	if !ok || plan.IsVoid {
		return indexedView{}, false
	}

	getCache := lowerFirst(bc.Name) + "IndexedGetter"
	setCache := lowerFirst(bc.Name) + "IndexedSetter"
	addLazyVar(view, getCache, "gdextension.PtrIndexedGetter",
		fmt.Sprintf("gdextension.GetPtrIndexedGetter(gdextension.%s)", view.VariantType))
	addLazyVar(view, setCache, "gdextension.PtrIndexedSetter",
		fmt.Sprintf("gdextension.GetPtrIndexedSetter(gdextension.%s)", view.VariantType))

	var getB strings.Builder
	getB.WriteString(plan.DeclRet)
	getB.WriteString("\n\t")
	if plan.Finalize != "" {
		getB.WriteString(plan.Finalize)
		getB.WriteString("\n\t")
	}
	fmt.Fprintf(&getB, "gdextension.CallPtrIndexedGetter(%s(), gdextension.TypePtr(unsafe.Pointer(self)), index, %s)\n\treturn %s",
		getCache, plan.RetArg, plan.RetExpr)

	var setB strings.Builder
	_, kind := goType(bc.IndexingReturnType)
	switch kind {
	case kindString:
		setB.WriteString("var tmp_value String\n\tstringFromGo(&tmp_value, value)\n\tdefer stringDestroy(&tmp_value)\n\t")
		fmt.Fprintf(&setB, "gdextension.CallPtrIndexedSetter(%s(), gdextension.TypePtr(unsafe.Pointer(self)), index, gdextension.TypePtr(unsafe.Pointer(&tmp_value)))",
			setCache)
	case kindStringName:
		setB.WriteString("var tmp_value StringName\n\tstringNameFromGo(&tmp_value, value)\n\tdefer stringNameDestroy(&tmp_value)\n\t")
		fmt.Fprintf(&setB, "gdextension.CallPtrIndexedSetter(%s(), gdextension.TypePtr(unsafe.Pointer(self)), index, gdextension.TypePtr(unsafe.Pointer(&tmp_value)))",
			setCache)
	case kindNodePath:
		setB.WriteString("var tmp_value NodePath\n\tnodePathFromGo(&tmp_value, value)\n\tdefer nodePathDestroy(&tmp_value)\n\t")
		fmt.Fprintf(&setB, "gdextension.CallPtrIndexedSetter(%s(), gdextension.TypePtr(unsafe.Pointer(self)), index, gdextension.TypePtr(unsafe.Pointer(&tmp_value)))",
			setCache)
	default:
		fmt.Fprintf(&setB, "gdextension.CallPtrIndexedSetter(%s(), gdextension.TypePtr(unsafe.Pointer(self)), index, gdextension.TypePtr(unsafe.Pointer(&value)))",
			setCache)
	}

	// The Godot JSON exposes `get`/`set` for indexable classes as both regular
	// methods AND via the indexed-accessor codepath. To avoid colliding with
	// the method-named Get/Set the bindgen emits, the indexed wrappers go out
	// as Index/SetIndex. Same C entry point, slightly different name.
	getSig := fmt.Sprintf("(self *%s) Index(index int64) %s", bc.Name, plan.GoType)
	setSig := fmt.Sprintf("(self *%s) SetIndex(index int64, value %s)", bc.Name, plan.GoType)

	return indexedView{
		GoName:    "Index/SetIndex",
		GoType:    plan.GoType,
		GetSig:    getSig,
		GetBody:   getB.String(),
		HasSetter: true,
		SetSig:    setSig,
		SetBody:   setB.String(),
	}, true
}

// goTypeForMeta maps a member-offset `meta` field to a Go type. Composite
// member types (e.g. "Vector2") flow through unchanged; the template emits
// a memcpy via unsafe.Pointer cast for them.
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
		return meta
	}
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

var builtinTmpl = template.Must(template.New("builtin").Funcs(template.FuncMap{
	"lower": lowerFirst,
}).Parse(`// {{.GoName}} is the opaque Godot builtin {{.GoName}} ({{.Size}} bytes under
// the framework's float_64 build config). The backing storage is an opaque
// byte array; field reads/writes go through offset accessors below.
type {{.GoName}} [{{.Size}}]byte

{{range .Members}}
// {{.GoName}} reads the {{.GoName}} field.
func (self *{{$.GoName}}) {{.GoName}}() {{.GoType}} {
	return *(*{{.GoType}})(unsafe.Pointer(&self[{{.Offset}}]))
}

// Set{{.GoName}} writes the {{.GoName}} field.
func (self *{{$.GoName}}) Set{{.GoName}}(value {{.GoType}}) {
	*(*{{.GoType}})(unsafe.Pointer(&self[{{.Offset}}])) = value
}
{{end}}

// Lazily-resolved function pointers. Each is a sync.OnceValue that performs
// the host lookup on first call — the host's interface table is loaded by
// the time any user code runs, so the lookup always succeeds.
var (
{{range .CacheVars}}	{{.Name}} {{.Type}}
{{end}}
)

{{range .Constructors}}
{{.GoDoc}}
func {{.Sig}} {
	{{.Body}}
}
{{end}}

{{range .Methods}}
{{.GoDoc}}
func {{.Sig}} {
	{{.Body}}
}
{{end}}

{{range .Operators}}
{{.GoDoc}}
func {{.Sig}} {
	{{.Body}}
}
{{end}}

{{with .Indexed}}
// Index reads element [index] from the receiver.
func {{.GetSig}} {
	{{.GetBody}}
}

{{if .HasSetter}}
// SetIndex writes value into element [index] of the receiver.
func {{.SetSig}} {
	{{.SetBody}}
}
{{end}}
{{end}}

{{if .HasDestructor}}
// Destroy releases the resources owned by the receiver. Safe to call on a
// zero value.
func (self *{{.GoName}}) Destroy() {
	gdextension.CallPtrDestructor({{.GoName | lower}}Dtor(), gdextension.TypePtr(unsafe.Pointer(self)))
}
{{end}}

// ToVariant copies the receiver into a freshly-initialized Variant slot. The
// caller owns the returned slot and must call (*Variant).Destroy() once done.
func (self *{{.GoName}}) ToVariant() *Variant {
	ret := new(Variant)
	gdextension.CallVariantFromType({{.GoName | lower}}FromType(),
		gdextension.VariantPtr(unsafe.Pointer(ret)),
		gdextension.TypePtr(unsafe.Pointer(self)))
	return ret
}

// {{.GoName}}FromVariant unwraps a Variant slot into a fresh {{.GoName}}. The
// source slot is not destroyed; the caller still owns it.
func {{.GoName}}FromVariant(src *Variant) {{.GoName}} {
	var v {{.GoName}}
	gdextension.CallTypeFromVariant({{.GoName | lower}}ToType(),
		gdextension.TypePtr(unsafe.Pointer(&v)),
		gdextension.VariantPtr(unsafe.Pointer(src)))
	return v
}
`))

