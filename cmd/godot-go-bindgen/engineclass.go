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
	GoName    string // e.g. "Node3D"
	Pkg       string // "core" or "editor" — also the directory the file lands in.
	BaseRef   string // Go type to embed; "" for Object (root).
	IsRefcounted bool

	// Imports tracks Go import paths the rendered methods reference.
	Imports map[string]bool

	Constants []constantView
	Enums     []enumView
	Methods   []callableView // reuses the builtin callableView shape

	// Singleton, if non-empty, names the host registration string. We emit a
	// `<Class>Singleton()` accessor backed by sync.OnceValue.
	Singleton string

	// CacheVars + InitLines kept around in case future passes want eager init.
	// Phase 3b uses sync.OnceValue inline instead, so these stay empty for now.
	CacheVars []cacheVar
	InitLines []string
}

type constantView struct {
	GoName string
	Value  int64
}

type enumView struct {
	TypeName string // "ObjectConnectFlags"
	Values   []constantView
	IsBitfield bool
}

// emitEngineClass writes <pkgDir>/<lower(name)>.gen.go for one engine class.
// Methods that reference unsupported types (cross-package forbidden, missing
// builtins, etc.) are silently skipped; the rest are emitted.
func emitEngineClass(api *API, c *Class, pkgRoot string) error {
	pkg := c.APIType
	if pkg == "" {
		pkg = "core"
	}
	view := &classView{
		GoName:       c.Name,
		Pkg:          pkg,
		IsRefcounted: c.IsRefcounted,
		Imports: map[string]bool{
			// Every generated class file references gdextension.ObjectPtr
			// in its <Class>FromPtr helper, regardless of method content.
			"github.com/legendary-code/godot-go/internal/gdextension": true,
		},
	}
	// RefCounted-derived classes attach a Go finalizer in FromPtr so
	// the engine reference drops when GC marks the wrapper. Pull
	// stdlib runtime into the imports for those classes.
	if view.IsRefcounted {
		view.Imports["runtime"] = true
	}

	if c.Inherits != "" {
		basePkg := classPkg(c.Inherits)
		switch {
		case basePkg == "" :
			return fmt.Errorf("class %s inherits unknown class %s", c.Name, c.Inherits)
		case basePkg == pkg:
			view.BaseRef = c.Inherits
		default:
			// editor classes can inherit core classes; the reverse is forbidden.
			if pkg == "core" && basePkg == "editor" {
				return fmt.Errorf("core class %s would have to inherit editor class %s", c.Name, c.Inherits)
			}
			view.BaseRef = basePkg + "." + c.Inherits
			view.Imports["github.com/legendary-code/godot-go/"+basePkg] = true
		}
	}

	if s := api.SingletonForType(c.Name); s != nil {
		view.Singleton = s.Name
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
			// Phase 5 (user-class codegen) handles these.
			continue
		}
		mv, ok := buildClassMethodView(api, c, m, view)
		if !ok {
			continue
		}
		view.Methods = append(view.Methods, mv)
	}

	pkgDir := filepath.Join(pkgRoot, pkg)
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		return err
	}

	var buf bytes.Buffer
	if err := writeClassFile(&buf, view); err != nil {
		return fmt.Errorf("render %s: %w", c.Name, err)
	}
	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		_ = os.WriteFile(filepath.Join(pkgDir, strings.ToLower(c.Name)+".gen.go.broken"), buf.Bytes(), 0o644)
		return fmt.Errorf("gofmt %s: %w", c.Name, err)
	}
	out := filepath.Join(pkgDir, strings.ToLower(c.Name)+".gen.go")
	return os.WriteFile(out, formatted, 0o644)
}

// buildClassMethodView is the engine-class twin of buildMethodView. It uses
// resolveType so engine-class refs and class-scoped enums work, and routes
// dispatch through CallMethodBindPtrCall (or CallMethodBindCall for vararg —
// 3b skips vararg methods until the user-facing Variant slice API is in).
func buildClassMethodView(api *API, c *Class, m ClassMethod, view *classView) (callableView, bool) {
	if m.IsVararg {
		// Vararg engine-class methods exist (Object.call, etc.) — defer
		// these until we have a tidy `...variant.Variant` boundary.
		return callableView{}, false
	}

	// Return planning.
	rPlan, rImports, ok := classReturnPlan(view.Pkg, m.ReturnVal)
	if !ok {
		return callableView{}, false
	}
	for ip := range rImports {
		view.Imports[ip] = true
	}

	// Argument planning.
	prep, argsExpr, aImports, ok := buildClassArgs(view.Pkg, m.Arguments)
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
		sig = fmt.Sprintf("%s%s(%s)", c.Name, goMethod, paramSigCtx(view.Pkg, m.Arguments))
		instanceExpr = "nil"
	} else {
		sig = fmt.Sprintf("(self *%s) %s(%s)", c.Name, goMethod, paramSigCtx(view.Pkg, m.Arguments))
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
	view.Imports["github.com/legendary-code/godot-go/internal/gdextension"] = true
	// Void+nullary methods don't reference unsafe — only add it when a
	// method actually does. Same idea for variant: rPlan/aImports already
	// signal it, but a few paths (e.g. variant.Variant in a sig) only
	// surface here.
	if strings.Contains(body, "unsafe.") || strings.Contains(sig, "unsafe.") {
		view.Imports["unsafe"] = true
	}
	if strings.Contains(body, "variant.") || strings.Contains(sig, "variant.") {
		view.Imports["github.com/legendary-code/godot-go/variant"] = true
	}
	return callableView{GoDoc: doc, Sig: sig, Body: body}, true
}

// classReturnPlan is the engine-class returnPrep. It supports kindObject in
// addition to the builtin path. retImports is the set of import paths the
// generated code needs (e.g. "github.com/legendary-code/godot-go/variant"
// when the return type is a builtin).
//
// Engine-class returns are tricky: the host writes a raw GDExtensionObjectPtr
// into the slot. We declare a local pointer slot, then wrap it in the typed
// Go struct after the call. nil returns are returned as a typed-nil Go
// pointer (the caller can check with the IsNil method on Object).
func classReturnPlan(currentPkg string, rv *ClassReturnVal) (retPlan, map[string]bool, bool) {
	imports := map[string]bool{}
	if rv == nil || rv.Type == "" || rv.Type == "void" {
		return retPlan{IsVoid: true, RetArg: "nil"}, imports, true
	}

	g, kind, ok := resolveType(rv.Type, currentPkg)
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
		imports["github.com/legendary-code/godot-go/variant"] = true
		return retPlan{GoType: "string",
			DeclRet:  "var raw variant.String",
			RetArg:   "gdextension.TypePtr(unsafe.Pointer(&raw))",
			Finalize: "defer raw.Destroy()",
			RetExpr:  "return variant.StringToGo(&raw)"}, imports, true
	case kindStringName:
		imports["github.com/legendary-code/godot-go/variant"] = true
		return retPlan{GoType: "string",
			DeclRet:  "var raw variant.StringName",
			RetArg:   "gdextension.TypePtr(unsafe.Pointer(&raw))",
			Finalize: "defer raw.Destroy()",
			RetExpr:  "return variant.StringNameToGo(&raw)"}, imports, true
	case kindNodePath:
		imports["github.com/legendary-code/godot-go/variant"] = true
		return retPlan{GoType: "string",
			DeclRet:  "var raw variant.NodePath",
			RetArg:   "gdextension.TypePtr(unsafe.Pointer(&raw))",
			Finalize: "defer raw.Destroy()",
			RetExpr:  "return variant.NodePathToGo(&raw)"}, imports, true
	case kindVariant:
		imports["github.com/legendary-code/godot-go/variant"] = true
		return retPlan{GoType: "variant.Variant",
			DeclRet: "var ret variant.Variant",
			RetArg:  "gdextension.TypePtr(unsafe.Pointer(&ret))",
			RetExpr: "return ret"}, imports, true
	case kindBuiltin:
		imports["github.com/legendary-code/godot-go/variant"] = true
		ret := "variant." + g
		return retPlan{GoType: ret,
			DeclRet: "var ret " + ret,
			RetArg:  "gdextension.TypePtr(unsafe.Pointer(&ret))",
			RetExpr: "return ret"}, imports, true
	case kindObject:
		// g is "*ClassName" or "*pkg.ClassName". The wrapper-construction
		// path goes through <ClassName>FromPtr in whichever package owns it.
		bare := strings.TrimPrefix(g, "*")
		var fromPtrCall string
		if dot := strings.IndexByte(bare, '.'); dot >= 0 {
			pkgPath := bare[:dot]
			imports["github.com/legendary-code/godot-go/"+pkgPath] = true
			fromPtrCall = pkgPath + "." + bare[dot+1:] + "FromPtr(ret)"
		} else {
			fromPtrCall = bare + "FromPtr(ret)"
		}
		return retPlan{GoType: g,
			DeclRet: "var ret gdextension.ObjectPtr",
			RetArg:  "gdextension.TypePtr(unsafe.Pointer(&ret))",
			RetExpr: "return " + fromPtrCall,
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
func classArgPrep(currentPkg, godotType, goName string) (prep, passExpr string, imports map[string]bool, ok bool) {
	imports = map[string]bool{}
	g, kind, resolved := resolveType(godotType, currentPkg)
	if !resolved {
		return "", "", imports, false
	}
	switch kind {
	case kindObject:
		_ = g
		tmp := "tmp_" + goName
		prep = fmt.Sprintf("var %s gdextension.ObjectPtr\n\tif %s != nil { %s = %s.Ptr() }\n\t",
			tmp, goName, tmp, goName)
		passExpr = fmt.Sprintf("gdextension.TypePtr(unsafe.Pointer(&%s))", tmp)
		// If g is qualified, ensure import.
		if dot := strings.IndexByte(strings.TrimPrefix(g, "*"), '.'); dot >= 0 {
			pkgPath := strings.TrimPrefix(g, "*")[:dot]
			imports["github.com/legendary-code/godot-go/"+pkgPath] = true
		}
		return prep, passExpr, imports, true
	case kindString:
		tmp := "tmp_" + goName
		prep = fmt.Sprintf("%s := variant.NewStringFromString(%s)\n\tdefer %s.Destroy()\n\t", tmp, goName, tmp)
		passExpr = fmt.Sprintf("gdextension.TypePtr(unsafe.Pointer(&%s))", tmp)
		imports["github.com/legendary-code/godot-go/variant"] = true
		return prep, passExpr, imports, true
	case kindStringName:
		tmp := "tmp_" + goName
		prep = fmt.Sprintf("%s := variant.NewStringNameFromString(%s)\n\tdefer %s.Destroy()\n\t", tmp, goName, tmp)
		passExpr = fmt.Sprintf("gdextension.TypePtr(unsafe.Pointer(&%s))", tmp)
		imports["github.com/legendary-code/godot-go/variant"] = true
		return prep, passExpr, imports, true
	case kindNodePath:
		tmp := "tmp_" + goName
		prep = fmt.Sprintf("%s := variant.NewNodePathFromString(%s)\n\tdefer %s.Destroy()\n\t", tmp, goName, tmp)
		passExpr = fmt.Sprintf("gdextension.TypePtr(unsafe.Pointer(&%s))", tmp)
		imports["github.com/legendary-code/godot-go/variant"] = true
		return prep, passExpr, imports, true
	case kindFloat:
		tmp := "tmp_" + goName
		prep = fmt.Sprintf("%s := float64(%s)\n\t", tmp, goName)
		passExpr = fmt.Sprintf("gdextension.TypePtr(unsafe.Pointer(&%s))", tmp)
		return prep, passExpr, imports, true
	case kindInt:
		// Typed enum args land here (g != "int64"). The slot still needs
		// an int64 underneath, so widen via a temporary.
		if g != "int64" {
			tmp := "tmp_" + goName
			prep = fmt.Sprintf("%s := int64(%s)\n\t", tmp, goName)
			passExpr = fmt.Sprintf("gdextension.TypePtr(unsafe.Pointer(&%s))", tmp)
		} else {
			passExpr = fmt.Sprintf("gdextension.TypePtr(unsafe.Pointer(&%s))", goName)
		}
		return prep, passExpr, imports, true
	case kindVariant:
		passExpr = fmt.Sprintf("gdextension.TypePtr(unsafe.Pointer(&%s))", goName)
		imports["github.com/legendary-code/godot-go/variant"] = true
		return prep, passExpr, imports, true
	case kindBuiltin:
		passExpr = fmt.Sprintf("gdextension.TypePtr(unsafe.Pointer(&%s))", goName)
		imports["github.com/legendary-code/godot-go/variant"] = true
		return prep, passExpr, imports, true
	default:
		passExpr = fmt.Sprintf("gdextension.TypePtr(unsafe.Pointer(&%s))", goName)
		return prep, passExpr, imports, true
	}
}

// buildClassArgs renders the prep code + args[...] literal + import set for
// an engine-class method's argument list. ("", "nil", _, true) for the empty
// list.
func buildClassArgs(currentPkg string, args []ClassArgument) (string, string, map[string]bool, bool) {
	imports := map[string]bool{}
	if len(args) == 0 {
		return "", "nil", imports, true
	}
	for _, a := range args {
		if _, _, ok := resolveType(a.Type, currentPkg); !ok {
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

// paramSigCtx is paramSig with a current-package context so engine-class arg
// types get the right `*core.X` / `*X` form, and builtin-class refs get the
// `variant.` prefix.
func paramSigCtx(currentPkg string, args []ClassArgument) string {
	if len(args) == 0 {
		return ""
	}
	parts := make([]string, 0, len(args))
	for _, a := range args {
		g, k, _ := resolveType(a.Type, currentPkg)
		switch k {
		case kindBuiltin:
			g = "variant." + g
		case kindVariant:
			g = "variant.Variant"
		}
		parts = append(parts, safeIdent(a.Name)+" "+g)
	}
	return strings.Join(parts, ", ")
}

// writeClassFile renders the .gen.go body. We don't use text/template because
// we need to interleave imports computed mid-render.
func writeClassFile(buf *bytes.Buffer, view *classView) error {
	// Preamble + imports.
	fmt.Fprintf(buf, "// Code generated by godot-go-bindgen. DO NOT EDIT.\n\npackage %s\n\n", view.Pkg)
	if len(view.Imports) > 0 {
		buf.WriteString("import (\n")
		// Sort imports for stable output: stdlib first, then ours.
		stdlib := []string{}
		ours := []string{}
		for ip := range view.Imports {
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
	//
	// Plain Object subclasses (Node, Node2D, etc.) keep the simpler
	// FromPtr — those are host-managed via the scene tree.
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
