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

func TestParseParenthesizedValue(t *testing.T) {
	// `@tag(args)` puts the inner text in Value; SplitArgs splits it
	// further into a list when the consumer asks.
	cg := commentGroup(1,
		"@group(\"Movement\")",
		"@export_range(0, 100, 5)",
		"@export_placeholder(\"Hello, world\")",
	)
	tags := Parse(cg)
	if len(tags) != 3 {
		t.Fatalf("got %d tags, want 3: %+v", len(tags), tags)
	}
	if tags[0].Name != "group" || tags[0].Value != `"Movement"` {
		t.Errorf("tag[0] = %+v, want {group, \"Movement\"}", tags[0])
	}
	if tags[1].Name != "export_range" || tags[1].Value != "0, 100, 5" {
		t.Errorf("tag[1] = %+v, want {export_range, 0, 100, 5}", tags[1])
	}
	if tags[2].Name != "export_placeholder" || tags[2].Value != `"Hello, world"` {
		t.Errorf("tag[2] = %+v, want {export_placeholder, \"Hello, world\"}", tags[2])
	}
}

func TestSplitArgsBare(t *testing.T) {
	got, err := SplitArgs("0, 100, 5")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := []string{"0", "100", "5"}
	if !equalStrings(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSplitArgsQuoted(t *testing.T) {
	got, err := SplitArgs(`"Movement", "Combat"`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := []string{"Movement", "Combat"}
	if !equalStrings(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSplitArgsEscapedQuote(t *testing.T) {
	// `"Quote: \"foo\""` decodes to `Quote: "foo"`.
	got, err := SplitArgs(`"Quote: \"foo\""`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := []string{`Quote: "foo"`}
	if !equalStrings(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSplitArgsCommaInsideQuote(t *testing.T) {
	got, err := SplitArgs(`"Hello, world", 42`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := []string{"Hello, world", "42"}
	if !equalStrings(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSplitArgsEmpty(t *testing.T) {
	got, err := SplitArgs("")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %q, want []", got)
	}
}

func TestSplitArgsUnterminatedQuote(t *testing.T) {
	if _, err := SplitArgs(`"unterminated`); err == nil {
		t.Errorf("expected error, got nil")
	}
}

func TestParseUnclosedParenFallsBack(t *testing.T) {
	// Unbalanced `(` should not silently swallow the rest of the file —
	// fall back to whitespace-style so the consumer at least sees the
	// raw text and can flag it.
	cg := commentGroup(1, "@export_range(0, 100")
	tags := Parse(cg)
	if len(tags) != 1 || tags[0].Name != "export_range" {
		t.Fatalf("expected one tag, got %+v", tags)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
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
