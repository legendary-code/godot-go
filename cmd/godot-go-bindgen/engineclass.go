package main

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"strings"
)

// classView is the data the engine-class template consumes. Each method's
// Body is pre-rendered to keep the template short.
type classView struct {
	GoName       string // e.g. "Node3D"
	BaseRef      string // Go type to embed; "" for Object (root).
	IsRefcounted bool

	// Imports tracks Go import paths the rendered methods reference.
	Imports map[string]bool

	Constants []constantView
	Enums     []enumView
	Methods   []callableView // reuses the builtin callableView shape

	// Singleton, if non-empty, names the host registration string. We emit a
	// `<Class>Singleton()` accessor backed by sync.OnceValue.
	Singleton string

	// CacheVars holds method-bind sync.OnceValue declarations rendered at
	// file scope, one per emitted method.
	CacheVars []cacheVar
}

type constantView struct {
	GoName string
	Value  int64
}

type enumView struct {
	TypeName   string // "ObjectConnectFlags"
	Values     []constantView
	IsBitfield bool
}

// emitEngineClasses renders every engine class into a single consolidated
// file at <cfg.Dir>/engineclasses.gen.go. Each class's body is rendered
// in source order (matching api.Classes), with the union of all classes'
// imports going into the file's import block. Methods that reference
// unsupported types are silently skipped per buildClassMethodView.
//
// Replaces the per-class `<lower(name)>.gen.go` output to keep the
// bindings directory navigable — 1023 small files become one large
// file (~250K lines, gofmt-stable).
func emitEngineClasses(api *API, cfg *genConfig) error {
	registerEngineClasses(api)
	imports := map[string]bool{
		// Every class's FromPtr helper references gdextension.ObjectPtr.
		cfg.ModulePath + "/gdextension": true,
	}
	var bodies bytes.Buffer
	for i := range api.Classes {
		c := &api.Classes[i]
		view, err := buildClassView(api, c, cfg)
		if err != nil {
			return fmt.Errorf("class %s: %w", c.Name, err)
		}
		if err := writeClassBody(&bodies, view, cfg); err != nil {
			return fmt.Errorf("render class %s: %w", c.Name, err)
		}
		bodies.WriteByte('\n')
		for ip := range view.Imports {
			imports[ip] = true
		}
	}

	var out bytes.Buffer
	writeFileHeader(&out, cfg.Package, imports)
	out.Write(bodies.Bytes())

	formatted, err := format.Source(out.Bytes())
	if err != nil {
		broken := filepath.Join(cfg.Dir, "engineclasses.gen.go.broken")
		_ = os.WriteFile(broken, out.Bytes(), 0o644)
		return fmt.Errorf("gofmt engineclasses.gen.go: %w (raw at %s)", err, broken)
	}
	return os.WriteFile(filepath.Join(cfg.Dir, "engineclasses.gen.go"), formatted, 0o644)
}

// buildClassView assembles the rendering metadata for one class —
// pulled out of emitEngineClass so emitEngineClasses can iterate
// without per-class file writes.
func buildClassView(api *API, c *Class, cfg *genConfig) (*classView, error) {
	view := &classView{
		GoName:       c.Name,
		IsRefcounted: c.IsRefcounted,
		Imports: map[string]bool{
			// Every generated class file references gdextension.ObjectPtr
			// in its <Class>FromPtr helper, regardless of method content.
			cfg.ModulePath + "/gdextension": true,
		},
	}
	// RefCounted-derived classes attach a Go finalizer in FromPtr so
	// the engine reference drops when GC marks the wrapper. Pull
	// stdlib runtime into the imports for those classes.
	if view.IsRefcounted {
		view.Imports["runtime"] = true
	}

	if c.Inherits != "" {
		if !engineClassNames[c.Inherits] {
			return nil, fmt.Errorf("class %s inherits unknown class %s", c.Name, c.Inherits)
		}
		view.BaseRef = c.Inherits
	}

	if s := api.SingletonForType(c.Name); s != nil {
		view.Singleton = s.Name
		view.Imports["sync"] = true
	}

	for _, k := range c.Constants {
		view.Constants = append(view.Constants, constantView{
			GoName: c.Name + "_" + k.Name,
			Value:  k.Value,
		})
	}

	for _, e := range c.Enums {
		ev := enumView{
			TypeName:   c.Name + e.Name,
			IsBitfield: e.IsBitfield,
		}
		for _, v := range e.Values {
			ev.Values = append(ev.Values, constantView{
				GoName: c.Name + "_" + v.Name,
				Value:  v.Value,
			})
		}
		view.Enums = append(view.Enums, ev)
	}

	for _, m := range c.Methods {
		if m.IsVirtual {
			// User-overridable virtuals don't have callable method binds.
			continue
		}
		mv, ok := buildClassMethodView(api, c, m, view, cfg)
		if !ok {
			continue
		}
		view.Methods = append(view.Methods, mv)
	}

	return view, nil
}

// buildClassMethodView is the engine-class twin of buildMethodView. It uses
// resolveType so engine-class refs and class-scoped enums work, and routes
// dispatch through CallMethodBindPtrCall (vararg methods are skipped until
// the user-facing Variant slice API is in).
func buildClassMethodView(api *API, c *Class, m ClassMethod, view *classView, cfg *genConfig) (callableView, bool) {
	if m.IsVararg {
		return callableView{}, false
	}

	// Return planning.
	rPlan, rImports, ok := classReturnPlan(cfg.Package, m.ReturnVal)
	if !ok {
		return callableView{}, false
	}
	for ip := range rImports {
		view.Imports[ip] = true
	}

	// Argument planning.
	prep, argsExpr, aImports, ok := buildClassArgs(cfg.Package, m.Arguments)
	if !ok {
		return callableView{}, false
	}
	for ip := range aImports {
		view.Imports[ip] = true
	}

	goMethod := pascal(m.Name)
	cacheName := fmt.Sprintf("%sMethod%s", lowerFirst(c.Name), goMethod)

	// Lazy resolver: each method gets its own sync.OnceValue. The first call
	// pays for the classdb lookup; later calls are a plain pointer load.
	view.CacheVars = append(view.CacheVars, cacheVar{
		Name: cacheName,
		Type: "= sync.OnceValue(func() gdextension.MethodBindPtr {\n" +
			"\treturn gdextension.GetMethodBind(\n" +
			fmt.Sprintf("\t\tgdextension.InternStringName(%q),\n", c.Name) +
			fmt.Sprintf("\t\tgdextension.InternStringName(%q),\n", m.Name) +
			fmt.Sprintf("\t\t%d,\n", m.Hash) +
			"\t)\n" +
			"})",
	})

	var sig string
	var instanceExpr string
	if m.IsStatic {
		sig = fmt.Sprintf("%s%s(%s)", c.Name, goMethod, paramSigCtx(cfg.Package, m.Arguments))
		instanceExpr = "nil"
	} else {
		sig = fmt.Sprintf("(self *%s) %s(%s)", c.Name, goMethod, paramSigCtx(cfg.Package, m.Arguments))
		instanceExpr = "self.Ptr()"
	}
	if rPlan.GoType != "" {
		sig += " " + rPlan.GoType
	}

	var b strings.Builder
	if rPlan.DeclRet != "" {
		b.WriteString(rPlan.DeclRet)
		b.WriteString("\n\t")
	}
	if rPlan.Finalize != "" {
		b.WriteString(rPlan.Finalize)
		b.WriteString("\n\t")
	}
	if prep != "" {
		b.WriteString(prep)
	}
	fmt.Fprintf(&b, "gdextension.CallMethodBindPtrCall(%s(), %s, %s, %s)",
		cacheName, instanceExpr, argsExpr, rPlan.RetArg)
	if rPlan.RetExpr != "" {
		b.WriteString("\n\t")
		b.WriteString(rPlan.RetExpr)
	}

	doc := fmt.Sprintf("// %s mirrors the Godot %s.%s method.", goMethod, c.Name, m.Name)
	body := b.String()
	view.Imports["sync"] = true
	view.Imports[cfg.ModulePath+"/gdextension"] = true
	if strings.Contains(body, "unsafe.") || strings.Contains(sig, "unsafe.") {
		view.Imports["unsafe"] = true
	}
	return callableView{GoDoc: doc, Sig: sig, Body: body}, true
}

// classReturnPlan is the engine-class returnPrep. It supports kindObject in
// addition to the builtin path. retImports is the set of import paths the
// generated code needs.
//
// Engine-class returns are tricky: the host writes a raw GDExtensionObjectPtr
// into the slot. We declare a local pointer slot, then wrap it in the typed
// Go struct after the call. nil returns are returned as a typed-nil Go
// pointer (the caller can check with the IsNil method on Object).
func classReturnPlan(_ string, rv *ClassReturnVal) (retPlan, map[string]bool, bool) {
	imports := map[string]bool{}
	if rv == nil || rv.Type == "" || rv.Type == "void" {
		return retPlan{IsVoid: true, RetArg: "nil"}, imports, true
	}

	g, kind, ok := resolveType(rv.Type)
	if !ok {
		return retPlan{}, imports, false
	}

	switch kind {
	case kindBool:
		return retPlan{GoType: "bool",
			DeclRet: "var ret bool",
			RetArg:  "gdextension.TypePtr(unsafe.Pointer(&ret))",
			RetExpr: "return ret"}, imports, true
	case kindInt:
		return retPlan{GoType: g, // may be "int64" or a typed enum (still int64 underneath)
			DeclRet: "var ret int64",
			RetArg:  "gdextension.TypePtr(unsafe.Pointer(&ret))",
			RetExpr: "return " + maybeCast(g, "ret")}, imports, true
	case kindFloat:
		return retPlan{GoType: "float32",
			DeclRet: "var raw float64",
			RetArg:  "gdextension.TypePtr(unsafe.Pointer(&raw))",
			RetExpr: "return float32(raw)"}, imports, true
	case kindString:
		return retPlan{GoType: "string",
			DeclRet:  "var raw String",
			RetArg:   "gdextension.TypePtr(unsafe.Pointer(&raw))",
			Finalize: "defer stringDestroy(&raw)",
			RetExpr:  "return stringToGo(&raw)"}, imports, true
	case kindStringName:
		return retPlan{GoType: "string",
			DeclRet:  "var raw StringName",
			RetArg:   "gdextension.TypePtr(unsafe.Pointer(&raw))",
			Finalize: "defer stringNameDestroy(&raw)",
			RetExpr:  "return stringNameToGo(&raw)"}, imports, true
	case kindNodePath:
		return retPlan{GoType: "string",
			DeclRet:  "var raw NodePath",
			RetArg:   "gdextension.TypePtr(unsafe.Pointer(&raw))",
			Finalize: "defer nodePathDestroy(&raw)",
			RetExpr:  "return nodePathToGo(&raw)"}, imports, true
	case kindVariant:
		return retPlan{GoType: "Variant",
			DeclRet: "var ret Variant",
			RetArg:  "gdextension.TypePtr(unsafe.Pointer(&ret))",
			RetExpr: "return ret"}, imports, true
	case kindBuiltin:
		return retPlan{GoType: g,
			DeclRet: "var ret " + g,
			RetArg:  "gdextension.TypePtr(unsafe.Pointer(&ret))",
			RetExpr: "return ret"}, imports, true
	case kindObject:
		// g is "*ClassName". The wrapper-construction path goes through
		// <ClassName>FromPtr in the same package.
		bare := strings.TrimPrefix(g, "*")
		return retPlan{GoType: g,
			DeclRet: "var ret gdextension.ObjectPtr",
			RetArg:  "gdextension.TypePtr(unsafe.Pointer(&ret))",
			RetExpr: "return " + bare + "FromPtr(ret)",
		}, imports, true
	}
	return retPlan{}, imports, false
}

// maybeCast wraps the expression in a type conversion if goType isn't bare
// int64 (i.e. it's a typed enum that's still int64 underneath).
func maybeCast(goType, expr string) string {
	if goType == "int64" {
		return expr
	}
	return goType + "(" + expr + ")"
}

// classArgPrep is the engine-class twin of argPrep. It handles the kindObject
// case (a single Object pointer slot) and reuses the builtin path for the
// rest.
func classArgPrep(_, godotType, goName string) (prep, passExpr string, imports map[string]bool, ok bool) {
	imports = map[string]bool{}
	_, kind, resolved := resolveType(godotType)
	if !resolved {
		return "", "", imports, false
	}
	switch kind {
	case kindObject:
		tmp := "tmp_" + goName
		prep = fmt.Sprintf("var %s gdextension.ObjectPtr\n\tif %s != nil { %s = %s.Ptr() }\n\t",
			tmp, goName, tmp, goName)
		passExpr = fmt.Sprintf("gdextension.TypePtr(unsafe.Pointer(&%s))", tmp)
		return prep, passExpr, imports, true
	case kindString:
		tmp := "tmp_" + goName
		prep = fmt.Sprintf("var %s String\n\tstringFromGo(&%s, %s)\n\tdefer stringDestroy(&%s)\n\t", tmp, tmp, goName, tmp)
		passExpr = fmt.Sprintf("gdextension.TypePtr(unsafe.Pointer(&%s))", tmp)
		return prep, passExpr, imports, true
	case kindStringName:
		tmp := "tmp_" + goName
		prep = fmt.Sprintf("var %s StringName\n\tstringNameFromGo(&%s, %s)\n\tdefer stringNameDestroy(&%s)\n\t", tmp, tmp, goName, tmp)
		passExpr = fmt.Sprintf("gdextension.TypePtr(unsafe.Pointer(&%s))", tmp)
		return prep, passExpr, imports, true
	case kindNodePath:
		tmp := "tmp_" + goName
		prep = fmt.Sprintf("var %s NodePath\n\tnodePathFromGo(&%s, %s)\n\tdefer nodePathDestroy(&%s)\n\t", tmp, tmp, goName, tmp)
		passExpr = fmt.Sprintf("gdextension.TypePtr(unsafe.Pointer(&%s))", tmp)
		return prep, passExpr, imports, true
	case kindFloat:
		tmp := "tmp_" + goName
		prep = fmt.Sprintf("%s := float64(%s)\n\t", tmp, goName)
		passExpr = fmt.Sprintf("gdextension.TypePtr(unsafe.Pointer(&%s))", tmp)
		return prep, passExpr, imports, true
	case kindInt:
		// Typed enum args land here (the Go type isn't int64). The slot
		// still needs an int64 underneath, so widen via a temporary.
		g, _, _ := resolveType(godotType)
		if g != "int64" {
			tmp := "tmp_" + goName
			prep = fmt.Sprintf("%s := int64(%s)\n\t", tmp, goName)
			passExpr = fmt.Sprintf("gdextension.TypePtr(unsafe.Pointer(&%s))", tmp)
		} else {
			passExpr = fmt.Sprintf("gdextension.TypePtr(unsafe.Pointer(&%s))", goName)
		}
		return prep, passExpr, imports, true
	default:
		passExpr = fmt.Sprintf("gdextension.TypePtr(unsafe.Pointer(&%s))", goName)
		return prep, passExpr, imports, true
	}
}

// buildClassArgs renders the prep code + args[...] literal + import set for
// an engine-class method's argument list.
func buildClassArgs(currentPkg string, args []ClassArgument) (string, string, map[string]bool, bool) {
	imports := map[string]bool{}
	if len(args) == 0 {
		return "", "nil", imports, true
	}
	for _, a := range args {
		if _, _, ok := resolveType(a.Type); !ok {
			return "", "", imports, false
		}
	}
	var prepB strings.Builder
	var elemsB strings.Builder
	elemsB.WriteString("args := [...]gdextension.TypePtr{\n")
	for _, a := range args {
		gname := safeIdent(a.Name)
		prep, expr, imps, ok := classArgPrep(currentPkg, a.Type, gname)
		if !ok {
			return "", "", imports, false
		}
		for ip := range imps {
			imports[ip] = true
		}
		if prep != "" {
			prepB.WriteString(prep)
			prepB.WriteString("\n\t")
		}
		fmt.Fprintf(&elemsB, "\t\t%s,\n", expr)
	}
	elemsB.WriteString("\t}\n\t")
	return prepB.String() + elemsB.String(), "args[:]", imports, true
}

// paramSigCtx renders a Go parameter list. currentPkg is unused now (single
// package), but kept as a parameter so the utility-function emitter can call
// in with no behavior change while we migrate.
func paramSigCtx(_ string, args []ClassArgument) string {
	if len(args) == 0 {
		return ""
	}
	parts := make([]string, 0, len(args))
	for _, a := range args {
		g, _, _ := resolveType(a.Type)
		parts = append(parts, safeIdent(a.Name)+" "+g)
	}
	return strings.Join(parts, ", ")
}

// writeClassFile renders one class's full .gen.go (preamble + imports +
// body). Used when a single-class output is wanted; the consolidated
// emitEngineClasses path renders bodies directly via writeClassBody and
// assembles the package header itself.
func writeClassFile(buf *bytes.Buffer, view *classView, cfg *genConfig) error {
	writeFileHeader(buf, cfg.Package, view.Imports)
	return writeClassBody(buf, view, cfg)
}

// writeFileHeader emits the standard `// Code generated...` banner, the
// package declaration, and a sorted import block (stdlib then external).
// Empty imports map is allowed — no import block is written.
func writeFileHeader(buf *bytes.Buffer, pkg string, imports map[string]bool) {
	fmt.Fprintf(buf, "// Code generated by godot-go-bindgen. DO NOT EDIT.\n\npackage %s\n\n", pkg)
	if len(imports) == 0 {
		return
	}
	buf.WriteString("import (\n")
	stdlib := []string{}
	ours := []string{}
	for ip := range imports {
		if strings.Contains(ip, ".") {
			ours = append(ours, ip)
		} else {
			stdlib = append(stdlib, ip)
		}
	}
	sortStrings(stdlib)
	sortStrings(ours)
	for _, ip := range stdlib {
		fmt.Fprintf(buf, "\t%q\n", ip)
	}
	if len(stdlib) > 0 && len(ours) > 0 {
		buf.WriteString("\n")
	}
	for _, ip := range ours {
		fmt.Fprintf(buf, "\t%q\n", ip)
	}
	buf.WriteString(")\n\n")
}

// writeClassBody renders a single class's content (type decl, FromPtr
// helper, constants, enums, method binds, method bodies). No file-level
// preamble — caller writes the package + imports.
func writeClassBody(buf *bytes.Buffer, view *classView, cfg *genConfig) error {
	// Type declaration. Object (no base) keeps the raw ptr; everything else
	// embeds its base.
	if view.GoName == "Object" {
		buf.WriteString(`// Object is the root of Godot's engine-class hierarchy. Wrapper structs
// for every other engine class embed Object (directly or transitively), so
// promoted methods and the underlying pointer reach the same slot.
type Object struct {
	ptr gdextension.ObjectPtr
}

// Ptr returns the underlying GDExtensionObjectPtr. Useful for ABI calls
// against host APIs the bindgen hasn't covered yet.
func (self *Object) Ptr() gdextension.ObjectPtr {
	if self == nil {
		return nil
	}
	return self.ptr
}

// IsNil reports whether the wrapped pointer is null.
func (self *Object) IsNil() bool {
	return self == nil || self.ptr == nil
}

// BindPtr installs a host-allocated GDExtensionObjectPtr in the receiver.
// Intended for the bindgen-generated <Class>FromPtr helpers; user code
// should never need to call it directly.
func (self *Object) BindPtr(p gdextension.ObjectPtr) { self.ptr = p }

`)
	} else {
		fmt.Fprintf(buf, "// %s mirrors the Godot engine class %s.\ntype %s struct {\n\t%s\n}\n\n", view.GoName, view.GoName, view.GoName, view.BaseRef)
	}

	// fromPtr helper — used by method returns to wrap a ObjectPtr.
	// For RefCounted-derived classes, the wrapper picks up a Go-side
	// finalizer so the engine reference drops automatically when the
	// last Go reference is gone. Engine method returns of Ref<T> are
	// already +1; the finalizer just calls Unreference. The
	// Unreference call dispatches through gdextension.RunOnMain so
	// the engine call lands on the main thread regardless of which
	// goroutine the Go GC happens to use for finalizer dispatch.
	if view.IsRefcounted {
		fmt.Fprintf(buf, `// %sFromPtr wraps an existing host-allocated GDExtensionObjectPtr in a
// *%s. Returns nil on a nil input. %s descends from RefCounted, so the
// returned wrapper carries a Go finalizer that drops one engine
// reference (via Unreference, dispatched on the main thread) when
// Go's GC determines the wrapper is unreachable. Users who want
// deterministic free can call ret.Unreference() directly — the
// finalizer is harmless after the refcount hits zero.
func %sFromPtr(p gdextension.ObjectPtr) *%s {
	if p == nil { return nil }
	ret := &%s{}
	ret.BindPtr(p)
	runtime.SetFinalizer(ret, func(r *%s) {
		gdextension.RunOnMain(func() { r.Unreference() })
	})
	return ret
}

`, view.GoName, view.GoName, view.GoName, view.GoName, view.GoName, view.GoName, view.GoName)
	} else {
		fmt.Fprintf(buf, "// %sFromPtr wraps an existing host-allocated GDExtensionObjectPtr in a\n// *%s. Returns nil on a nil input.\nfunc %sFromPtr(p gdextension.ObjectPtr) *%s {\n\tif p == nil { return nil }\n\tret := &%s{}\n\tret.BindPtr(p)\n\treturn ret\n}\n\n", view.GoName, view.GoName, view.GoName, view.GoName, view.GoName)
	}

	// Constants.
	if len(view.Constants) > 0 {
		buf.WriteString("const (\n")
		for _, k := range view.Constants {
			fmt.Fprintf(buf, "\t%s = %d\n", k.GoName, k.Value)
		}
		buf.WriteString(")\n\n")
	}

	// Enums.
	for _, e := range view.Enums {
		fmt.Fprintf(buf, "// %s is the Godot %s enum.\ntype %s int64\n\n", e.TypeName, e.TypeName, e.TypeName)
		if len(e.Values) > 0 {
			buf.WriteString("const (\n")
			for _, v := range e.Values {
				fmt.Fprintf(buf, "\t%s %s = %d\n", v.GoName, e.TypeName, v.Value)
			}
			buf.WriteString(")\n\n")
		}
	}

	// Lazy method-bind cache vars.
	if len(view.CacheVars) > 0 {
		buf.WriteString("var (\n")
		for _, cv := range view.CacheVars {
			fmt.Fprintf(buf, "\t%s %s\n", cv.Name, cv.Type)
		}
		buf.WriteString(")\n\n")
	}

	// Singleton accessor.
	if view.Singleton != "" {
		fmt.Fprintf(buf, "// %sSingleton returns the global %s singleton registered under %q.\n// The host owns the lifetime; do not call any destructor on it.\nvar %sSingleton = sync.OnceValue(func() *%s {\n\treturn %sFromPtr(gdextension.GetSingleton(gdextension.InternStringName(%q)))\n})\n\n",
			view.GoName, view.GoName, view.Singleton, view.GoName, view.GoName, view.GoName, view.Singleton)
	}

	// Methods.
	for _, m := range view.Methods {
		fmt.Fprintf(buf, "%s\nfunc %s {\n\t%s\n}\n\n", m.GoDoc, m.Sig, m.Body)
	}

	return nil
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}
