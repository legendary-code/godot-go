package main

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

func mustDiscover(t *testing.T, src string) *discovered {
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
	return d
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
type MyNode struct { core.Node }
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
type MyNode struct { core.Node }
func (MyNode) Helper() {}
`
	d := mustDiscover(t, src)
	if d.MainClass.Methods[0].Kind != methodStatic {
		t.Errorf("Kind = %v, want static (unnamed receiver)", d.MainClass.Methods[0].Kind)
	}
}

func TestDiscoverOverrideLowercase(t *testing.T) {
	// Lowercase Go method + @override → registered as a Godot virtual
	// with a leading underscore on the snake-case name.
	src := `package x
import "github.com/legendary-code/godot-go/core"
type MyNode struct { core.Node }
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
type MyNode struct { core.Node }
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
type MyNode struct { core.Node }
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
type MyNode struct { core.Node }
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
type MyNode struct { core.Node }
// Hello sends a greeting.
// @name shout
func (n *MyNode) Hello() {}
`
	d := mustDiscover(t, src)
	if d.MainClass.Methods[0].GodotName != "shout" {
		t.Errorf("GodotName = %q, want shout (from @name override)", d.MainClass.Methods[0].GodotName)
	}
}

func TestDiscoverInnerClass(t *testing.T) {
	src := `package x
import "github.com/legendary-code/godot-go/core"
type MyNode struct { core.Node }

// @innerclass
type Helper struct {}
`
	d := mustDiscover(t, src)
	if len(d.InnerClasses) != 1 || d.InnerClasses[0].Name != "Helper" {
		t.Fatalf("inner classes = %+v", d.InnerClasses)
	}
}

func TestDiscoverEnum(t *testing.T) {
	src := `package x
import "github.com/legendary-code/godot-go/core"
type MyNode struct { core.Node }
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
	src := `package x
// @innerclass
type Only struct {}
`
	mustFailDiscover(t, src, "no top-level extension class found")
}

func TestDiscoverErrorOnMultipleMainClasses(t *testing.T) {
	src := `package x
import "github.com/legendary-code/godot-go/core"
type A struct { core.Node }
type B struct { core.Node }
`
	mustFailDiscover(t, src, "multiple top-level extension classes")
}

func TestDiscoverErrorOnMissingParent(t *testing.T) {
	src := `package x
type Lonely struct {}
`
	mustFailDiscover(t, src, "no recognized base class")
}

func TestDiscoverErrorOnMultipleEmbeddedFramework(t *testing.T) {
	src := `package x
import (
	"github.com/legendary-code/godot-go/core"
	"github.com/legendary-code/godot-go/editor"
)
type Wrong struct {
	core.Node
	editor.EditorPlugin
}
`
	mustFailDiscover(t, src, "single-inheritance")
}

func TestDiscoverPropertyFieldForm(t *testing.T) {
	src := `package x
import "github.com/legendary-code/godot-go/core"
type MyNode struct {
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
type MyNode struct {
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
type MyNode struct { core.Node }
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
type MyNode struct { core.Node }
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
type MyNode struct { core.Node }
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
type MyNode struct {
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
type MyNode struct {
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
type MyNode struct {
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
type MyNode struct { core.Node }
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
type MyNode struct { core.Node }
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
type MyNode struct { core.Node }
// @property
func (n *MyNode) SetHealth(v int64) {}
`
	mustFailDiscover(t, src, "@property on SetHealth")
}

func TestDiscoverSignalsInterface(t *testing.T) {
	src := `package x
import "github.com/legendary-code/godot-go/core"
type MyNode struct { core.Node }

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
type MyNode struct { core.Node }
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
type MyNode struct { core.Node }

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
type MyNode struct { core.Node }

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
type MyNode struct {
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
type MyNode struct {
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
type MyNode struct {
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
type MyNode struct {
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
type MyNode struct {
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
type MyNode struct {
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
type MyNode struct { core.Node }
// @export_range(0, 100)
// @property
func (n *MyNode) GetHealth() int64 { return 0 }
`
	mustFailDiscover(t, src, "field-form only")
}

func TestDiscoverPropertyGroupNonContiguousRejected(t *testing.T) {
	// Going back to group "A" after switching to group "B" would
	// require re-registering A's header — produces a duplicate
	// inspector entry. Reject.
	src := `package x
import "github.com/legendary-code/godot-go/core"
type MyNode struct {
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
	mustFailDiscover(t, src, "reuses a group already left")
}

func TestDiscoverPropertyUngroupedFirstAcrossForms(t *testing.T) {
	// Method-form @property is inherently ungrouped; codegen places
	// ungrouped properties before grouped ones regardless of source
	// position so source layout doesn't have to mirror the strict
	// register-ungrouped-first ordering Godot requires.
	src := `package x
import "github.com/legendary-code/godot-go/core"
type MyNode struct {
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

func TestPascalToSnake(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"Hello", "hello"},
		{"HelloWorld", "hello_world"},
		{"HTTPServer", "http_server"},
		{"GetURL", "get_url"},
		{"X", "x"},
	}
	for _, c := range cases {
		if got := pascalToSnake(c.in); got != c.want {
			t.Errorf("pascalToSnake(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
