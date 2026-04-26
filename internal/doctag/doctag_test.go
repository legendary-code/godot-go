package doctag

import (
	"go/ast"
	"go/token"
	"testing"
)

// commentGroup builds a synthetic *ast.CommentGroup from one or more
// `// …` line comments, faking source positions starting at the given
// base. Test-only helper.
func commentGroup(base token.Pos, lines ...string) *ast.CommentGroup {
	cg := &ast.CommentGroup{}
	pos := base
	for _, ln := range lines {
		text := "// " + ln
		cg.List = append(cg.List, &ast.Comment{Slash: pos, Text: text})
		pos += token.Pos(len(text) + 1)
	}
	return cg
}

func TestParseExtractsTags(t *testing.T) {
	cg := commentGroup(1,
		"MyNode is a Godot extension class.",
		"@innerclass",
		"@description Custom-described node",
	)
	tags := Parse(cg)
	if len(tags) != 2 {
		t.Fatalf("got %d tags, want 2: %+v", len(tags), tags)
	}
	if tags[0].Name != "innerclass" || tags[0].Value != "" {
		t.Errorf("tag[0] = %+v, want {innerclass, \"\"}", tags[0])
	}
	if tags[1].Name != "description" || tags[1].Value != "Custom-described node" {
		t.Errorf("tag[1] = %+v, want {description, \"Custom-described node\"}", tags[1])
	}
}

func TestParseIgnoresProse(t *testing.T) {
	cg := commentGroup(1, "Just regular doc text with @-symbol@in middle")
	if got := Parse(cg); len(got) != 0 {
		t.Errorf("expected no tags, got %+v", got)
	}
}

func TestParseNilSafe(t *testing.T) {
	if got := Parse(nil); got != nil {
		t.Errorf("Parse(nil) = %+v, want nil", got)
	}
}

func TestFindAndHas(t *testing.T) {
	tags := []Tag{{Name: "name", Value: "foo"}, {Name: "deprecated"}}
	v, ok := Find(tags, "name")
	if !ok || v != "foo" {
		t.Errorf("Find(name) = %q, %v; want \"foo\", true", v, ok)
	}
	if _, ok := Find(tags, "missing"); ok {
		t.Errorf("Find(missing) ok = true; want false")
	}
	if !Has(tags, "deprecated") {
		t.Errorf("Has(deprecated) = false; want true")
	}
	if Has(tags, "missing") {
		t.Errorf("Has(missing) = true; want false")
	}
}

func TestParseTrimsLeadingWhitespaceAndStars(t *testing.T) {
	// /* … */ block comments often have leading "*" decorations on each line.
	cg := &ast.CommentGroup{
		List: []*ast.Comment{{
			Slash: 1,
			Text:  "/*\n * @abstract\n * @name custom_name\n */",
		}},
	}
	tags := Parse(cg)
	if len(tags) != 2 {
		t.Fatalf("got %d tags, want 2: %+v", len(tags), tags)
	}
	if tags[0].Name != "abstract" || tags[1].Name != "name" || tags[1].Value != "custom_name" {
		t.Errorf("unexpected tags: %+v", tags)
	}
}
