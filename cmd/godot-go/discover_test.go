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

func TestDiscoverVirtualCandidate(t *testing.T) {
	src := `package x
import "github.com/legendary-code/godot-go/core"
type MyNode struct { core.Node }
func (n *MyNode) process(delta float64) {}
`
	d := mustDiscover(t, src)
	if d.MainClass.Methods[0].Kind != methodVirtualCandidate {
		t.Errorf("Kind = %v, want virtual candidate (lowercase first letter)", d.MainClass.Methods[0].Kind)
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
