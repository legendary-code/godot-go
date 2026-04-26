// Package doctag parses `@<name>[ <value>]` directives out of Go doc
// comments. The codegen tool (cmd/godot-go) reads these to learn user
// intent that doesn't fit cleanly in the type system — e.g. `@innerclass`
// to opt a struct into nested-class registration, `@name foo` to override
// the auto-generated GDScript-visible method name.
//
// The parser is deliberately liberal: it surfaces every recognized tag,
// without checking applicability. The AST consumer enforces context (a
// `@name` on a struct doc, for instance, is a user error caught upstream).
package doctag

import (
	"go/ast"
	"go/token"
	"strings"
)

// Tag is a single parsed `@name [value]` directive. Pos points at the
// start of the line in the original source so callers can produce
// editor-jumpable diagnostics ("file.go:42: …").
type Tag struct {
	Name  string
	Value string
	Pos   token.Pos
}

// Parse extracts every `@<name>[ <value>]` directive from cg. Lines that
// don't begin (after the comment-start marker and any whitespace) with `@`
// are ignored, so prose interleaved with directives is fine.
//
// A directive's value is the rest of the line after the first whitespace
// run; trailing whitespace is trimmed but interior whitespace is preserved
// as-is (so `@description Hello, world` keeps the comma).
//
// Returns an empty slice if cg is nil.
func Parse(cg *ast.CommentGroup) []Tag {
	if cg == nil {
		return nil
	}
	var out []Tag
	for _, c := range cg.List {
		text := stripCommentMarker(c.Text)
		// One ast.Comment may span multiple source lines for a /* … */
		// block comment; split so each gets its own Pos-tracked entry.
		// For // line comments, this is a no-op single iteration.
		offset := token.Pos(0)
		for _, line := range splitLines(text) {
			tag, ok := parseLine(line, c.Slash+token.Pos(len(c.Text)-len(text))+offset)
			if ok {
				out = append(out, tag)
			}
			offset += token.Pos(len(line) + 1) // +1 for the consumed '\n'
		}
	}
	return out
}

// Find returns the first tag with the given name, or "", false if none
// matches. Convenience for callers that only care about a single tag.
func Find(tags []Tag, name string) (string, bool) {
	for _, t := range tags {
		if t.Name == name {
			return t.Value, true
		}
	}
	return "", false
}

// Has reports whether tags contains a directive with the given name.
func Has(tags []Tag, name string) bool {
	for _, t := range tags {
		if t.Name == name {
			return true
		}
	}
	return false
}

func stripCommentMarker(s string) string {
	switch {
	case strings.HasPrefix(s, "//"):
		return s[2:]
	case strings.HasPrefix(s, "/*") && strings.HasSuffix(s, "*/"):
		return s[2 : len(s)-2]
	default:
		return s
	}
}

func splitLines(s string) []string {
	// strings.Split keeps empty trailing entries which we want for accurate
	// position tracking; callers that want non-empty lines filter later.
	return strings.Split(s, "\n")
}

func parseLine(line string, pos token.Pos) (Tag, bool) {
	trimmed := strings.TrimLeft(line, " \t*")
	if !strings.HasPrefix(trimmed, "@") {
		return Tag{}, false
	}
	body := trimmed[1:]
	// Tag name is the leading run of identifier-ish chars; everything after
	// the first whitespace is the value.
	nameEnd := len(body)
	for i, r := range body {
		if r == ' ' || r == '\t' {
			nameEnd = i
			break
		}
	}
	name := body[:nameEnd]
	if name == "" {
		return Tag{}, false
	}
	value := ""
	if nameEnd < len(body) {
		value = strings.TrimSpace(body[nameEnd:])
	}
	return Tag{Name: name, Value: value, Pos: pos}, true
}
