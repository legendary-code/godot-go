package main

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

func mustDiscover(t *testing.T, src string) *discovered {
	t.Helper()
	d, _ := mustDiscoverWithFset(t, src)
	return d
}

func mustDiscoverWithFset(t *testing.T, src string) (*discovered, *token.FileSet) {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	d, err := discover(fset, file, file.Name.Name)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	return d, fset
}

func mustFailDiscover(t *testing.T, src string, wantSubstr string) {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	_, err = discover(fset, file, file.Name.Name)
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", wantSubstr)
	}
	if !strings.Contains(err.Error(), wantSubstr) {
		t.Fatalf("error %q does not contain %q", err.Error(), wantSubstr)
	}
}

func TestDiscoverMainClassWithMethod(t *testing.T) {
	src := `package x
import "github.com/legendary-code/godot-go/core"
// @class
type MyNode struct {
	// @extends
	core.Node
}
func (n *MyNode) Hello() {}
`
	d := mustDiscover(t, src)
	if d.MainClass.Name != "MyNode" {
		t.Errorf("MainClass.Name = %q, want MyNode", d.MainClass.Name)
	}
	if d.MainClass.Parent != "Node" {
		t.Errorf("MainClass.Parent = %q, want Node", d.MainClass.Parent)
	}
	if d.MainClass.ParentImport != "github.com/legendary-code/godot-go/core" {
		t.Errorf("MainClass.ParentImport = %q", d.MainClass.ParentImport)
	}
	if len(d.MainClass.Methods) != 1 || d.MainClass.Methods[0].GoName != "Hello" {
		t.Fatalf("methods = %+v", d.MainClass.Methods)
	}
	if d.MainClass.Methods[0].GodotName != "hello" {
		t.Errorf("GodotName = %q, want hello", d.MainClass.Methods[0].GodotName)
	}
	if d.MainClass.Methods[0].Kind != methodInstance {
		t.Errorf("Kind = %v, want instance", d.MainClass.Methods[0].Kind)
	}
	if !d.MainClass.Methods[0].IsPointerRcv {
		t.Errorf("expected pointer receiver")
	}
}

func TestDiscoverStaticMethod(t *testing.T) {
	src := `package x
import "github.com/legendary-code/godot-go/core"
// @class
type MyNode struct {
	// @extends
	core.Node
}
// @static
func (MyNode) Helper() {}
`
	d := mustDiscover(t, src)
	if d.MainClass.Methods[0].Kind != methodStatic {
		t.Errorf("Kind = %v, want static (@static-tagged)", d.MainClass.Methods[0].Kind)
	}
}

func TestDiscoverUnnamedReceiverWithoutStaticRejected(t *testing.T) {
	// `func (T) Foo()` used to silently classify as static — now
	// requires explicit @static so the user's intent is visible.
	src := `package x
import "github.com/legendary-code/godot-go/core"
// @class
type MyNode struct {
	// @extends
	core.Node
}
func (MyNode) Helper() {}
`
	mustFailDiscover(t, src, "unnamed receiver but no @static")
}

func TestDiscoverStaticAndOverrideRejected(t *testing.T) {
	src := `package x
import "github.com/legendary-code/godot-go/core"
// @class
type MyNode struct {
	// @extends
	core.Node
}
// @static
// @override
func (n *MyNode) Helper() {}
`
	mustFailDiscover(t, src, "both @static and @override")
}

func TestDiscoverStaticWithNamedReceiver(t *testing.T) {
	// Named receiver + @static is allowed — the method's body just
	// doesn't use the receiver, but Godot still sees it as static
	// (registered with MethodFlagStatic).
	src := `package x
import "github.com/legendary-code/godot-go/core"
// @class
type MyNode struct {
	// @extends
	core.Node
}
// @static
func (n *MyNode) Helper() int64 { return 42 }
`
	d := mustDiscover(t, src)
	if d.MainClass.Methods[0].Kind != methodStatic {
		t.Errorf("Kind = %v, want static", d.MainClass.Methods[0].Kind)
	}
}

func TestDiscoverOverrideLowercase(t *testing.T) {
	// Lowercase Go method + @override → registered as a Godot virtual
	// with a leading underscore on the snake-case name.
	src := `package x
import "github.com/legendary-code/godot-go/core"
// @class
type MyNode struct {
	// @extends
	core.Node
}
// @override
func (n *MyNode) process(delta float64) {}
`
	d := mustDiscover(t, src)
	if len(d.MainClass.Methods) != 1 {
		t.Fatalf("methods = %+v", d.MainClass.Methods)
	}
	m := d.MainClass.Methods[0]
	if m.Kind != methodOverride {
		t.Errorf("Kind = %v, want override", m.Kind)
	}
	if m.GodotName != "_process" {
		t.Errorf("GodotName = %q, want _process", m.GodotName)
	}
}

func TestDiscoverOverrideExported(t *testing.T) {
	// Exported Go method + @override → still gets the leading underscore;
	// the case decision is independent of override status.
	src := `package x
import "github.com/legendary-code/godot-go/core"
// @class
type MyNode struct {
	// @extends
	core.Node
}
// @override
func (n *MyNode) Process(delta float64) {}
`
	d := mustDiscover(t, src)
	m := d.MainClass.Methods[0]
	if m.Kind != methodOverride {
		t.Errorf("Kind = %v, want override", m.Kind)
	}
	if m.GodotName != "_process" {
		t.Errorf("GodotName = %q, want _process", m.GodotName)
	}
}

func TestDiscoverLowercaseNoTagSkipped(t *testing.T) {
	// Lowercase Go method without @override → Go-private helper, NOT
	// registered with Godot's ClassDB. Mirrors Go's own export model:
	// lowercase = unexported = invisible outside the package.
	src := `package x
import "github.com/legendary-code/godot-go/core"
// @class
type MyNode struct {
	// @extends
	core.Node
}
func (n *MyNode) Hello() {}
func (n *MyNode) helper() {}
`
	d := mustDiscover(t, src)
	if len(d.MainClass.Methods) != 1 {
		t.Fatalf("expected only Hello to register, got %d methods: %+v",
			len(d.MainClass.Methods), d.MainClass.Methods)
	}
	if d.MainClass.Methods[0].GoName != "Hello" {
		t.Errorf("Methods[0] = %q, want Hello", d.MainClass.Methods[0].GoName)
	}
}

func TestDiscoverNameOverrideOnVirtualSkipsLeadingUnderscore(t *testing.T) {
	// @name supplies the Godot name verbatim. No auto-prepend, even on
	// overrides — @name is the explicit escape hatch.
	src := `package x
import "github.com/legendary-code/godot-go/core"
// @class
type MyNode struct {
	// @extends
	core.Node
}
// @override
// @name _physics_process
func (n *MyNode) Tick(delta float64) {}
`
	d := mustDiscover(t, src)
	m := d.MainClass.Methods[0]
	if m.Kind != methodOverride {
		t.Errorf("Kind = %v, want override", m.Kind)
	}
	if m.GodotName != "_physics_process" {
		t.Errorf("GodotName = %q, want _physics_process (verbatim from @name)", m.GodotName)
	}
}

func TestDiscoverNameOverride(t *testing.T) {
	src := `package x
import "github.com/legendary-code/godot-go/core"
// @class
type MyNode struct {
	// @extends
	core.Node
}
// Hello sends a greeting.
// @name shout
func (n *MyNode) Hello() {}
`
	d := mustDiscover(t, src)
	if d.MainClass.Methods[0].GodotName != "shout" {
		t.Errorf("GodotName = %q, want shout (from @name override)", d.MainClass.Methods[0].GodotName)
	}
}

func TestDiscoverInnerClassRejected(t *testing.T) {
	// @innerclass was removed: only one @class per file is registered.
	// A struct still carrying @innerclass gets a clear error pointing at
	// the new model rather than silently going unregistered.
	src := `package x
import "github.com/legendary-code/godot-go/core"
// @class
type MyNode struct {
	// @extends
	core.Node
}

// @innerclass
type Helper struct {
	// @extends
	core.Object
}
`
	mustFailDiscover(t, src, "@innerclass")
}

func TestDiscoverExtendsOutsideClassRejected(t *testing.T) {
	// @extends only makes sense on the @class struct's embedded parent.
	// On any other struct it's a misuse — typically the user thought
	// they were declaring a registered class but forgot @class.
	src := `package x
import "github.com/legendary-code/godot-go/core"
// @class
type MyNode struct {
	// @extends
	core.Node
}

type Helper struct {
	// @extends
	core.Object
}
`
	mustFailDiscover(t, src, "not tagged @class")
}

func TestDiscoverExtendsCrossPackage(t *testing.T) {
	// @extends accepts any imported package — not just the framework
	// bindings. The runtime registration succeeds as long as the
	// parent class is in Godot's ClassDB by the time this class
	// loads (i.e. the providing extension registered first). The
	// discovery side records the parent's import path verbatim.
	src := `package x
import "example.com/coolext/godot"
import "example.com/coolext"
// @class
type MyHero struct {
	// @extends
	coolext.Player
}
`
	d := mustDiscover(t, src)
	if d.MainClass.Parent != "Player" {
		t.Errorf("Parent = %q, want Player", d.MainClass.Parent)
	}
	if d.MainClass.ParentImport != "example.com/coolext" {
		t.Errorf("ParentImport = %q, want example.com/coolext", d.MainClass.ParentImport)
	}
}

func TestDiscoverExtendsBareIdentRejected(t *testing.T) {
	// Bare-identifier parents (`@extends Outer` referring to a
	// same-file sibling) are no longer supported — only one @class
	// per file is registered, and cross-file parents take the
	// qualified `pkg.Type` form.
	src := `package x
// @class
type Lonely struct {
	// @extends
	SomeOtherType
}
`
	mustFailDiscover(t, src, "must be an embedded package-qualified type")
}

func TestDiscoverSliceArg(t *testing.T) {
	src := `package x
import "github.com/legendary-code/godot-go/core"
// @class
type MyNode struct {
	// @extends
	core.Node
}
func (n *MyNode) Sum(values []int64) int64 { return 0 }
`
	d := mustDiscover(t, src)
	if len(d.MainClass.Methods) != 1 {
		t.Fatalf("methods = %+v", d.MainClass.Methods)
	}
	// Discovery captures the AST verbatim; type resolution happens in
	// the emitter. Here we just verify discovery accepts the slice arg
	// shape — emit_test would assert the marshal fragments separately.
	if d.MainClass.Methods[0].GoName != "Sum" {
		t.Errorf("Methods[0] = %q, want Sum", d.MainClass.Methods[0].GoName)
	}
}

func TestDiscoverSliceReturn(t *testing.T) {
	src := `package x
import "github.com/legendary-code/godot-go/core"
// @class
type MyNode struct {
	// @extends
	core.Node
}
func (n *MyNode) Names() []string { return nil }
`
	d := mustDiscover(t, src)
	if len(d.MainClass.Methods) != 1 || d.MainClass.Methods[0].GoName != "Names" {
		t.Fatalf("methods = %+v", d.MainClass.Methods)
	}
}

func TestDiscoverVariadicArg(t *testing.T) {
	// Variadic params are Go-side syntactic sugar over a slice — the
	// codegen treats them identically at the wire boundary.
	src := `package x
import "github.com/legendary-code/godot-go/core"
// @class
type MyNode struct {
	// @extends
	core.Node
}
func (n *MyNode) Sum(values ...int64) int64 { return 0 }
`
	d := mustDiscover(t, src)
	if len(d.MainClass.Methods) != 1 || d.MainClass.Methods[0].GoName != "Sum" {
		t.Fatalf("methods = %+v", d.MainClass.Methods)
	}
}

func TestDiscoverEnumSliceArg(t *testing.T) {
	// `[]Mode` where Mode is a tagged @enum should route through the
	// TypedArray path (class_name preserved for editor identity).
	src := `package x
import "github.com/legendary-code/godot-go/core"
// @class
type MyNode struct {
	// @extends
	core.Node
}
// @enum
type Mode int
const (
	ModeIdle Mode = iota
	ModeRun
)
func (n *MyNode) Filter(modes []Mode) []Mode { return nil }
`
	d := mustDiscover(t, src)
	if len(d.MainClass.Methods) != 1 {
		t.Fatalf("methods = %+v", d.MainClass.Methods)
	}
}

func TestDiscoverUntaggedUserIntSliceArg(t *testing.T) {
	// `[]Counter` where Counter is an untagged user int alias should
	// route through PackedInt64Array (no enum identity to preserve).
	src := `package x
import "github.com/legendary-code/godot-go/core"
// @class
type MyNode struct {
	// @extends
	core.Node
}
type Counter int
func (n *MyNode) Tally(counters []Counter) {}
`
	d := mustDiscover(t, src)
	if len(d.MainClass.Methods) != 1 {
		t.Fatalf("methods = %+v", d.MainClass.Methods)
	}
}

func TestDiscoverSelfClassPointerArg(t *testing.T) {
	// `*<MainClass>` as a method arg / return type — Phase 6a. The codegen
	// looks the engine ObjectPtr up in the parallel side table to recover
	// the *<MainClass> Go wrapper.
	src := `package x
import "github.com/legendary-code/godot-go/core"
// @class
type MyNode struct {
	// @extends
	core.Node
}
func (n *MyNode) Echo(other *MyNode) *MyNode { return other }
`
	d := mustDiscover(t, src)
	if len(d.MainClass.Methods) != 1 || d.MainClass.Methods[0].GoName != "Echo" {
		t.Fatalf("methods = %+v", d.MainClass.Methods)
	}
}

func TestDiscoverSelfClassSlice(t *testing.T) {
	// `[]*<MainClass>` slice — Phase 6b. Wire form is Array[MyNode]
	// (TypedArray of OBJECT with the user class name as class_name).
	src := `package x
import "github.com/legendary-code/godot-go/core"
// @class
type MyNode struct {
	// @extends
	core.Node
}
func (n *MyNode) EchoMany(others []*MyNode) []*MyNode { return others }
`
	d := mustDiscover(t, src)
	if len(d.MainClass.Methods) != 1 || d.MainClass.Methods[0].GoName != "EchoMany" {
		t.Fatalf("methods = %+v", d.MainClass.Methods)
	}
}

func TestDiscoverSelfClassVariadic(t *testing.T) {
	// Variadic `...*<MainClass>` should route through the same slice
	// typeInfo — wire form is Array[MyNode].
	src := `package x
import "github.com/legendary-code/godot-go/core"
// @class
type MyNode struct {
	// @extends
	core.Node
}
func (n *MyNode) Combine(others ...*MyNode) *MyNode {
	if len(others) > 0 {
		return others[0]
	}
	return n
}
`
	d := mustDiscover(t, src)
	if len(d.MainClass.Methods) != 1 {
		t.Fatalf("methods = %+v", d.MainClass.Methods)
	}
}

func TestDiscoverEngineClassPointer(t *testing.T) {
	// `*<bindings>.<EngineClass>` — Phase 6c. The codegen wraps the
	// borrowed engine ObjectPtr via &<bindings>.<Class>{} + BindPtr.
	src := `package x
import "github.com/legendary-code/godot-go/core"
// @class
type MyNode struct {
	// @extends
	core.Node
}
func (n *MyNode) EchoNode(other *core.Node) *core.Node { return other }
`
	d := mustDiscover(t, src)
	if len(d.MainClass.Methods) != 1 || d.MainClass.Methods[0].GoName != "EchoNode" {
		t.Fatalf("methods = %+v", d.MainClass.Methods)
	}
}

func TestDiscoverEngineClassSlice(t *testing.T) {
	// `[]*<bindings>.<EngineClass>` — Phase 6c slice variant.
	src := `package x
import "github.com/legendary-code/godot-go/core"
// @class
type MyNode struct {
	// @extends
	core.Node
}
func (n *MyNode) EchoNodes(others []*core.Node) []*core.Node { return others }
`
	d := mustDiscover(t, src)
	if len(d.MainClass.Methods) != 1 {
		t.Fatalf("methods = %+v", d.MainClass.Methods)
	}
}

func TestEmitNonBindingsPackagePointerRejected(t *testing.T) {
	// `*<other_pkg>.<Class>` is rejected — Phase 6c only recognizes
	// pointers through the user's bindings package alias.
	src := `package x
import (
	"github.com/legendary-code/godot-go/core"
	"some/other/pkg"
)
// @class
type MyNode struct {
	// @extends
	core.Node
}
func (n *MyNode) Echo(other *pkg.Thing) *pkg.Thing { return other }
`
	d, fset := mustDiscoverWithFset(t, src)
	var buf strings.Builder
	err := emit(&buf, fset, []*discovered{d})
	if err == nil {
		t.Fatalf("expected emit error for non-bindings pointer, got nil")
	}
	if !strings.Contains(err.Error(), "bindings package alias") {
		t.Fatalf("error %q does not mention bindings package alias", err.Error())
	}
}

func TestEmitForeignClassPointerRejected(t *testing.T) {
	// `*<OtherClass>` is rejected — Phase 6a only supports same-class
	// self-references. Cross-file user classes and engine class pointers
	// are deferred to later phases.
	src := `package x
import "github.com/legendary-code/godot-go/core"
// @class
type MyNode struct {
	// @extends
	core.Node
}
type OtherClass struct{}
func (n *MyNode) Echo(other *OtherClass) *OtherClass { return other }
`
	d, fset := mustDiscoverWithFset(t, src)
	var buf strings.Builder
	err := emit(&buf, fset, []*discovered{d})
	if err == nil {
		t.Fatalf("expected emit error for *OtherClass, got nil; output: %s", buf.String())
	}
	if !strings.Contains(err.Error(), "self-reference") {
		t.Fatalf("error %q does not mention self-reference", err.Error())
	}
}

func TestEmitNestedSliceRejected(t *testing.T) {
	// Discovery accepts any AST shape; rejection happens at emit time
	// via resolveType. Phase 7 sharpened the message — instead of
	// bubbling up via the generic slice-element error, the codegen now
	// recognizes `[][]T` directly and tells the user "nested slices".
	src := `package x
import "github.com/legendary-code/godot-go/core"
// @class
type MyNode struct {
	// @extends
	core.Node
}
func (n *MyNode) Bad(values [][]int64) {}
`
	d, fset := mustDiscoverWithFset(t, src)
	var buf strings.Builder
	err := emit(&buf, fset, []*discovered{d})
	if err == nil {
		t.Fatalf("expected emit error for nested slice, got nil; output: %s", buf.String())
	}
	if !strings.Contains(err.Error(), "nested slices") {
		t.Fatalf("error %q does not mention nested slices", err.Error())
	}
}

func TestEmitMapArgRejected(t *testing.T) {
	src := `package x
import "github.com/legendary-code/godot-go/core"
// @class
type MyNode struct {
	// @extends
	core.Node
}
func (n *MyNode) Bad(m map[string]int) {}
`
	d, fset := mustDiscoverWithFset(t, src)
	var buf strings.Builder
	err := emit(&buf, fset, []*discovered{d})
	if err == nil || !strings.Contains(err.Error(), "map types") {
		t.Fatalf("expected map-types error, got %v", err)
	}
}

func TestEmitFuncArgRejected(t *testing.T) {
	src := `package x
import "github.com/legendary-code/godot-go/core"
// @class
type MyNode struct {
	// @extends
	core.Node
}
func (n *MyNode) Bad(cb func()) {}
`
	d, fset := mustDiscoverWithFset(t, src)
	var buf strings.Builder
	err := emit(&buf, fset, []*discovered{d})
	if err == nil || !strings.Contains(err.Error(), "function types") {
		t.Fatalf("expected function-types error, got %v", err)
	}
}

func TestEmitChanArgRejected(t *testing.T) {
	src := `package x
import "github.com/legendary-code/godot-go/core"
// @class
type MyNode struct {
	// @extends
	core.Node
}
func (n *MyNode) Bad(ch chan int) {}
`
	d, fset := mustDiscoverWithFset(t, src)
	var buf strings.Builder
	err := emit(&buf, fset, []*discovered{d})
	if err == nil || !strings.Contains(err.Error(), "channel types") {
		t.Fatalf("expected channel-types error, got %v", err)
	}
}

func TestEmitInterfaceArgRejected(t *testing.T) {
	src := `package x
import "github.com/legendary-code/godot-go/core"
// @class
type MyNode struct {
	// @extends
	core.Node
}
func (n *MyNode) Bad(x interface{}) {}
`
	d, fset := mustDiscoverWithFset(t, src)
	var buf strings.Builder
	err := emit(&buf, fset, []*discovered{d})
	if err == nil || !strings.Contains(err.Error(), "interface types") {
		t.Fatalf("expected interface-types error, got %v", err)
	}
}

func TestEmitBareSelectorArgRejected(t *testing.T) {
	// Bare `<bindings>.<Type>` (no pointer) — like passing a Variant or
	// Vector2 directly. Today only the pointer form is recognized for
	// cross-package types.
	src := `package x
import "github.com/legendary-code/godot-go/core"
// @class
type MyNode struct {
	// @extends
	core.Node
}
func (n *MyNode) Bad(v core.Variant) {}
`
	d, fset := mustDiscoverWithFset(t, src)
	var buf strings.Builder
	err := emit(&buf, fset, []*discovered{d})
	if err == nil || !strings.Contains(err.Error(), "bare cross-package type") {
		t.Fatalf("expected bare cross-package error, got %v", err)
	}
}

func TestEmitUnsupportedSliceElementRejected(t *testing.T) {
	src := `package x
import "github.com/legendary-code/godot-go/core"
// @class
type MyNode struct {
	// @extends
	core.Node
}
func (n *MyNode) Bad(values []float64) {}
`
	d, fset := mustDiscoverWithFset(t, src)
	var buf strings.Builder
	err := emit(&buf, fset, []*discovered{d})
	if err == nil {
		t.Fatalf("expected emit error for []float64, got nil; output: %s", buf.String())
	}
	// Phase 4 ships seven element types; float64 is deferred.
	if !strings.Contains(err.Error(), "unsupported slice element") {
		t.Fatalf("error %q does not mention unsupported slice element", err.Error())
	}
}

func TestDiscoverEnum(t *testing.T) {
	src := `package x
import "github.com/legendary-code/godot-go/core"
// @class
type MyNode struct {
	// @extends
	core.Node
}
type Mode int
const (
	ModeA Mode = iota
	ModeB
	ModeC
)
`
	d := mustDiscover(t, src)
	if len(d.Enums) != 1 {
		t.Fatalf("enums = %+v", d.Enums)
	}
	if d.Enums[0].Name != "Mode" || len(d.Enums[0].Values) != 3 {
		t.Errorf("enum = %+v", d.Enums[0])
	}
}

func TestDiscoverErrorOnNoMainClass(t *testing.T) {
	// File has no @class struct at all — codegen has nothing to
	// register. The diagnostic should point the user at the missing
	// tag, not just say "no class found."
	src := `package x
type SomethingElse struct {}
`
	mustFailDiscover(t, src, "no @class struct found")
}

func TestDiscoverErrorOnMultipleMainClasses(t *testing.T) {
	src := `package x
import "github.com/legendary-code/godot-go/core"
// @class
type A struct {
	// @extends
	core.Node
}
// @class
type B struct {
	// @extends
	core.Node
}
`
	mustFailDiscover(t, src, "multiple @class structs")
}

func TestDiscoverErrorOnClassWithoutExtends(t *testing.T) {
	// @class struct but no @extends — codegen has no parent to
	// register. The error should say the class is missing @extends,
	// not that it has "no recognized base class".
	src := `package x
import "github.com/legendary-code/godot-go/core"
// @class
type Lonely struct {
	core.Node
}
`
	mustFailDiscover(t, src, "no @extends")
}

func TestDiscoverErrorOnMultipleExtends(t *testing.T) {
	src := `package x
import (
	"github.com/legendary-code/godot-go/core"
	"github.com/legendary-code/godot-go/editor"
)
// @class
type Wrong struct {
	// @extends
	core.Node
	// @extends
	editor.EditorPlugin
}
`
	mustFailDiscover(t, src, "single-inheritance")
}

func TestDiscoverErrorOnExtendsOnNamedField(t *testing.T) {
	// @extends only makes sense on an embedded (anonymous) field —
	// it's the GDScript-equivalent of `extends Node`. A named field
	// is composition, not inheritance.
	src := `package x
import "github.com/legendary-code/godot-go/core"
// @class
type Wrong struct {
	// @extends
	parent core.Node
}
`
	mustFailDiscover(t, src, "@extends on named field")
}

func TestDiscoverPropertyFieldForm(t *testing.T) {
	src := `package x
import "github.com/legendary-code/godot-go/core"
// @class
type MyNode struct {
	// @extends
	core.Node
	// @property
	Health int64
}
`
	d := mustDiscover(t, src)
	if len(d.MainClass.Properties) != 1 {
		t.Fatalf("properties = %+v", d.MainClass.Properties)
	}
	p := d.MainClass.Properties[0]
	if p.Name != "Health" {
		t.Errorf("Name = %q, want Health", p.Name)
	}
	if p.Source != propertyFromField {
		t.Errorf("Source = %v, want field-form", p.Source)
	}
	if p.ReadOnly {
		t.Errorf("ReadOnly = true, want false (no @readonly)")
	}
}

func TestDiscoverPropertyFieldFormReadOnly(t *testing.T) {
	src := `package x
import "github.com/legendary-code/godot-go/core"
// @class
type MyNode struct {
	// @extends
	core.Node
	// @readonly
	// @property
	Health int64
}
`
	d := mustDiscover(t, src)
	if len(d.MainClass.Properties) != 1 || !d.MainClass.Properties[0].ReadOnly {
		t.Fatalf("expected one read-only property, got %+v", d.MainClass.Properties)
	}
}

func TestDiscoverPropertyMethodForm(t *testing.T) {
	// Read-write method form: BOTH GetX and SetX must carry @property.
	src := `package x
import "github.com/legendary-code/godot-go/core"
// @class
type MyNode struct {
	// @extends
	core.Node
}
// @property
func (n *MyNode) GetHealth() int64 { return 0 }
// @property
func (n *MyNode) SetHealth(v int64) {}
`
	d := mustDiscover(t, src)
	if len(d.MainClass.Properties) != 1 {
		t.Fatalf("properties = %+v", d.MainClass.Properties)
	}
	p := d.MainClass.Properties[0]
	if p.Name != "Health" {
		t.Errorf("Name = %q, want Health", p.Name)
	}
	if p.Source != propertyFromMethod {
		t.Errorf("Source = %v, want method-form", p.Source)
	}
	if p.ReadOnly {
		t.Errorf("ReadOnly = true, want false (Set<X> with @property exists)")
	}
}

func TestDiscoverPropertyMethodFormSetterUntagged(t *testing.T) {
	// GetX has @property but SetX does NOT — pairing requires both. The
	// property is read-only; SetX is a regular method (registered, but
	// not wired as the property's setter).
	src := `package x
import "github.com/legendary-code/godot-go/core"
// @class
type MyNode struct {
	// @extends
	core.Node
}
// @property
func (n *MyNode) GetHealth() int64 { return 0 }
func (n *MyNode) SetHealth(v int64) {}
`
	d := mustDiscover(t, src)
	if len(d.MainClass.Properties) != 1 {
		t.Fatalf("properties = %+v", d.MainClass.Properties)
	}
	if !d.MainClass.Properties[0].ReadOnly {
		t.Errorf("ReadOnly = false, want true (SetHealth lacks @property)")
	}
}

func TestDiscoverPropertyMethodFormReadOnly(t *testing.T) {
	// No matching SetHealth → read-only inferred.
	src := `package x
import "github.com/legendary-code/godot-go/core"
// @class
type MyNode struct {
	// @extends
	core.Node
}
// @property
func (n *MyNode) GetHealth() int64 { return 0 }
`
	d := mustDiscover(t, src)
	if len(d.MainClass.Properties) != 1 || !d.MainClass.Properties[0].ReadOnly {
		t.Fatalf("expected read-only inferred, got %+v", d.MainClass.Properties)
	}
}

func TestDiscoverPropertyConflict(t *testing.T) {
	// Same name as field-form AND method-form → ambiguous.
	src := `package x
import "github.com/legendary-code/godot-go/core"
// @class
type MyNode struct {
	// @extends
	core.Node
	// @property
	Health int64
}
// @property
func (n *MyNode) GetHealth() int64 { return 0 }
`
	mustFailDiscover(t, src, "ambiguous @property")
}

func TestDiscoverPropertyLowercaseFieldRejected(t *testing.T) {
	src := `package x
import "github.com/legendary-code/godot-go/core"
// @class
type MyNode struct {
	// @extends
	core.Node
	// @property
	health int64
}
`
	mustFailDiscover(t, src, "unexported")
}

func TestDiscoverReadOnlyOnFieldWithoutPropertyRejected(t *testing.T) {
	src := `package x
import "github.com/legendary-code/godot-go/core"
// @class
type MyNode struct {
	// @extends
	core.Node
	// @readonly
	Foo int64
}
`
	mustFailDiscover(t, src, "@readonly without @property")
}

func TestDiscoverReadOnlyOnMethodRejected(t *testing.T) {
	// @readonly on a method-form @property is redundant: read-only is
	// already inferred from the absence of a matching Set<X>. Discovery
	// rejects it so users don't end up with two ways to say the same
	// thing.
	src := `package x
import "github.com/legendary-code/godot-go/core"
// @class
type MyNode struct {
	// @extends
	core.Node
}
// @property
// @readonly
func (n *MyNode) GetHealth() int64 { return 0 }
`
	mustFailDiscover(t, src, "@readonly on method")
}

func TestDiscoverPropertyMethodFormReadWrite(t *testing.T) {
	// User writes both Get and Set, both tagged @property. Codegen
	// pairs them and registers a read-write property.
	src := `package x
import "github.com/legendary-code/godot-go/core"
// @class
type MyNode struct {
	// @extends
	core.Node
}
// @property
func (n *MyNode) GetTag() string { return "" }
// @property
func (n *MyNode) SetTag(v string) {}
`
	d := mustDiscover(t, src)
	if len(d.MainClass.Properties) != 1 {
		t.Fatalf("properties = %+v", d.MainClass.Properties)
	}
	p := d.MainClass.Properties[0]
	if p.Source != propertyFromMethod {
		t.Errorf("Source = %v, want method-form", p.Source)
	}
	if p.ReadOnly {
		t.Errorf("ReadOnly = true, want false (SetTag tagged @property)")
	}
}

func TestDiscoverPropertyOrphanSetter(t *testing.T) {
	// @property on Set<X> with no matching Get<X> @property is an error
	// — there's no property to attach the setter to.
	src := `package x
import "github.com/legendary-code/godot-go/core"
// @class
type MyNode struct {
	// @extends
	core.Node
}
// @property
func (n *MyNode) SetHealth(v int64) {}
`
	mustFailDiscover(t, src, "@property on SetHealth")
}

func TestDiscoverSignalsInterface(t *testing.T) {
	src := `package x
import "github.com/legendary-code/godot-go/core"
// @class
type MyNode struct {
	// @extends
	core.Node
}

// @signals
type Signals interface {
	Damaged(amount int64)
	LeveledUp()
	Tagged(label string)
}
`
	d := mustDiscover(t, src)
	if len(d.MainClass.Signals) != 3 {
		t.Fatalf("signals = %+v", d.MainClass.Signals)
	}
	got := map[string]string{}
	for _, s := range d.MainClass.Signals {
		got[s.Name] = s.GodotName
	}
	want := map[string]string{
		"Damaged":   "damaged",
		"LeveledUp": "leveled_up",
		"Tagged":    "tagged",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("signal %q: GodotName = %q, want %q", k, got[k], v)
		}
	}
}

func TestDiscoverSignalsCollidesWithMethod(t *testing.T) {
	// Signal method name on a @signals interface collides with a regular
	// method on the class — Go would reject the duplicate at compile
	// time; discover catches it earlier with a clearer message.
	src := `package x
import "github.com/legendary-code/godot-go/core"
// @class
type MyNode struct {
	// @extends
	core.Node
}
func (n *MyNode) Damaged(x int64) {}

// @signals
type Signals interface {
	Damaged(amount int64)
}
`
	mustFailDiscover(t, src, "collides with regular method")
}

func TestDiscoverSignalsDuplicateAcrossInterfaces(t *testing.T) {
	src := `package x
import "github.com/legendary-code/godot-go/core"
// @class
type MyNode struct {
	// @extends
	core.Node
}

// @signals
type SignalsA interface {
	Damaged(amount int64)
}

// @signals
type SignalsB interface {
	Damaged(amount int64)
}
`
	mustFailDiscover(t, src, "duplicate signal")
}

func TestDiscoverSignalsRejectsReturn(t *testing.T) {
	// Signals don't return values. Reject return types so users don't
	// get silent drops.
	src := `package x
import "github.com/legendary-code/godot-go/core"
// @class
type MyNode struct {
	// @extends
	core.Node
}

// @signals
type Signals interface {
	Damaged(amount int64) bool
}
`
	mustFailDiscover(t, src, "return value")
}

func TestDiscoverPropertyGroup(t *testing.T) {
	src := `package x
import "github.com/legendary-code/godot-go/core"
// @class
type MyNode struct {
	// @extends
	core.Node
	// @group("Combat")
	// @property
	Damage int64
}
`
	d := mustDiscover(t, src)
	if len(d.MainClass.Properties) != 1 {
		t.Fatalf("properties = %+v", d.MainClass.Properties)
	}
	p := d.MainClass.Properties[0]
	if p.Group != "Combat" {
		t.Errorf("Group = %q, want Combat", p.Group)
	}
}

func TestDiscoverPropertyExportRange(t *testing.T) {
	src := `package x
import "github.com/legendary-code/godot-go/core"
// @class
type MyNode struct {
	// @extends
	core.Node
	// @export_range(0, 100)
	// @property
	Health int64
}
`
	d := mustDiscover(t, src)
	p := d.MainClass.Properties[0]
	if p.Hint != "PropertyHintRange" {
		t.Errorf("Hint = %q, want PropertyHintRange", p.Hint)
	}
	if p.HintString != "0,100" {
		t.Errorf("HintString = %q, want 0,100", p.HintString)
	}
}

func TestDiscoverPropertyExportEnum(t *testing.T) {
	src := `package x
import "github.com/legendary-code/godot-go/core"
// @class
type MyNode struct {
	// @extends
	core.Node
	// @export_enum("Idle", "Run", "Jump")
	// @property
	Mode int64
}
`
	d := mustDiscover(t, src)
	p := d.MainClass.Properties[0]
	if p.Hint != "PropertyHintEnum" {
		t.Errorf("Hint = %q, want PropertyHintEnum", p.Hint)
	}
	if p.HintString != "Idle,Run,Jump" {
		t.Errorf("HintString = %q, want Idle,Run,Jump", p.HintString)
	}
}

func TestDiscoverPropertyExportPlaceholderWithComma(t *testing.T) {
	// Quoted-string handling: a comma inside the quoted arg must NOT
	// split it into two args.
	src := `package x
import "github.com/legendary-code/godot-go/core"
// @class
type MyNode struct {
	// @extends
	core.Node
	// @export_placeholder("Hello, world")
	// @property
	Display string
}
`
	d := mustDiscover(t, src)
	p := d.MainClass.Properties[0]
	if p.HintString != "Hello, world" {
		t.Errorf("HintString = %q, want Hello, world", p.HintString)
	}
}

func TestDiscoverPropertySubgroupRequiresGroup(t *testing.T) {
	src := `package x
import "github.com/legendary-code/godot-go/core"
// @class
type MyNode struct {
	// @extends
	core.Node
	// @subgroup("Texture")
	// @property
	Skin string
}
`
	mustFailDiscover(t, src, "@subgroup without @group")
}

func TestDiscoverPropertyMultipleHintsRejected(t *testing.T) {
	src := `package x
import "github.com/legendary-code/godot-go/core"
// @class
type MyNode struct {
	// @extends
	core.Node
	// @export_range(0, 100)
	// @export_enum("A", "B")
	// @property
	Foo int64
}
`
	mustFailDiscover(t, src, "multiple @export_*")
}

func TestDiscoverPropertyHintsRejectedOnMethodForm(t *testing.T) {
	src := `package x
import "github.com/legendary-code/godot-go/core"
// @class
type MyNode struct {
	// @extends
	core.Node
}
// @export_range(0, 100)
// @property
func (n *MyNode) GetHealth() int64 { return 0 }
`
	mustFailDiscover(t, src, "field-form only")
}

func TestDiscoverPropertyGroupNonContiguousReorders(t *testing.T) {
	// Each @property carries its @group("X") explicitly — there's no
	// inheritance from a previous declaration — so a user can write
	// groups in any order. The codegen reorders so each group's
	// properties register contiguously without producing duplicate
	// inspector headers. Source-order matching within a group is
	// preserved; groups themselves are ordered by first-appearance.
	src := `package x
import "github.com/legendary-code/godot-go/core"
// @class
type MyNode struct {
	// @extends
	core.Node
	// @group("A")
	// @property
	One int64
	// @group("B")
	// @property
	Two int64
	// @group("A")
	// @property
	Three int64
}
`
	d := mustDiscover(t, src)
	props := d.MainClass.Properties
	if len(props) != 3 {
		t.Fatalf("expected 3 properties, got %d", len(props))
	}
	got := []string{props[0].Name, props[1].Name, props[2].Name}
	want := []string{"One", "Three", "Two"}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("property %d: got %q, want %q (full order %v, want %v)", i, got[i], want[i], got, want)
		}
	}
}

func TestDiscoverPropertyUngroupedFirstAcrossForms(t *testing.T) {
	// Method-form @property is inherently ungrouped; codegen places
	// ungrouped properties before grouped ones regardless of source
	// position so source layout doesn't have to mirror the strict
	// register-ungrouped-first ordering Godot requires.
	src := `package x
import "github.com/legendary-code/godot-go/core"
// @class
type MyNode struct {
	// @extends
	core.Node
	// @group("Combat")
	// @property
	Damage int64
	score int64
}
// @property
func (n *MyNode) GetScore() int64 { return 0 }
`
	d := mustDiscover(t, src)
	if len(d.MainClass.Properties) != 2 {
		t.Fatalf("properties = %+v", d.MainClass.Properties)
	}
	if d.MainClass.Properties[0].Name != "Score" {
		t.Errorf("first property = %q, want Score (ungrouped, comes first)", d.MainClass.Properties[0].Name)
	}
	if d.MainClass.Properties[1].Name != "Damage" {
		t.Errorf("second property = %q, want Damage (grouped)", d.MainClass.Properties[1].Name)
	}
}

func TestDiscoverEditorClass(t *testing.T) {
	src := `package x
import "github.com/legendary-code/godot-go/core"
// @class
// @editor
type MyEditorPlugin struct {
	// @extends
	core.Node
}
`
	d := mustDiscover(t, src)
	if !d.MainClass.IsEditor {
		t.Errorf("IsEditor = false, want true")
	}
}


