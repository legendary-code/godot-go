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
	// Tag name is the leading run of identifier characters (alphanumeric +
	// underscore). Anything else — whitespace, '(', ',' — terminates the
	// name. This lets `@export_range(0, 100)` parse as name=export_range
	// with the value coming from the parenthesized block that follows;
	// existing `@name foo` style still works because the space is also a
	// boundary.
	nameEnd := len(body)
	for i, r := range body {
		if !isIdentRune(r) {
			nameEnd = i
			break
		}
	}
	name := body[:nameEnd]
	if name == "" {
		return Tag{}, false
	}
	rest := body[nameEnd:]
	value := ""
	switch {
	case strings.HasPrefix(rest, "("):
		// Parenthesized form: `@tag(args)`. Consume to the matching ')',
		// honoring quoted strings so a ')' inside a quote doesn't end
		// the block early. Unbalanced parens fall through to the no-op
		// case (whole rest treated as value); SplitArgs will surface
		// the malformed input downstream.
		if end, ok := findMatchingParen(rest); ok {
			value = rest[1:end]
			break
		}
		// Unbalanced — fall through to whitespace-style.
		fallthrough
	default:
		value = strings.TrimSpace(rest)
	}
	return Tag{Name: name, Value: value, Pos: pos}, true
}

// isIdentRune reports whether r can appear in a tag name. Mirrors Go's own
// identifier syntax: letters, digits, and underscore. Tag names start with
// `@` (handled upstream) and can carry digits after the first character.
func isIdentRune(r rune) bool {
	switch {
	case 'a' <= r && r <= 'z', 'A' <= r && r <= 'Z':
		return true
	case '0' <= r && r <= '9':
		return true
	case r == '_':
		return true
	}
	return false
}

// findMatchingParen returns the index of the ')' that closes the '(' at
// position 0 of s. Quoted strings inside the block are skipped so a ')'
// inside `"..."` doesn't end the block early. Returns -1, false if no
// matching ')' is found before end-of-string.
func findMatchingParen(s string) (int, bool) {
	if len(s) == 0 || s[0] != '(' {
		return -1, false
	}
	inQuote := false
	for i := 1; i < len(s); i++ {
		c := s[i]
		switch {
		case inQuote:
			if c == '\\' && i+1 < len(s) {
				i++ // skip escaped character
				continue
			}
			if c == '"' {
				inQuote = false
			}
		case c == '"':
			inQuote = true
		case c == ')':
			return i, true
		}
	}
	return -1, false
}

// SplitArgs parses a tag value rendered in the parenthesized form
// (`@foo(a, b, "quoted, with comma")` → value `a, b, "quoted, with comma"`)
// into a list of argument strings. Bare arguments are passed through with
// surrounding whitespace trimmed. Quoted arguments have surrounding quotes
// stripped and the escapes \" → " and \\ → \ decoded.
//
// Returns ([], nil) for an empty value (the no-arg parenthesized form
// `@foo()`). Returns an error for malformed input — unbalanced quotes,
// trailing escape, or an unterminated argument.
func SplitArgs(value string) ([]string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	var (
		out  []string
		buf  strings.Builder
		i    int
		inQ  bool
	)
	flush := func() {
		out = append(out, buf.String())
		buf.Reset()
	}
	// Walk character-by-character. Inside a quoted string, only `"` and
	// `\` carry meaning; outside, `,` separates args and quotes start a
	// new quoted segment.
	for ; i < len(value); i++ {
		c := value[i]
		switch {
		case inQ:
			if c == '\\' {
				if i+1 >= len(value) {
					return nil, errSplit("trailing escape")
				}
				next := value[i+1]
				switch next {
				case '"', '\\':
					buf.WriteByte(next)
				default:
					buf.WriteByte('\\')
					buf.WriteByte(next)
				}
				i++
				continue
			}
			if c == '"' {
				inQ = false
				continue
			}
			buf.WriteByte(c)
		case c == '"':
			inQ = true
		case c == ',':
			flush()
			// Skip whitespace after comma so each arg starts clean.
			for i+1 < len(value) && (value[i+1] == ' ' || value[i+1] == '\t') {
				i++
			}
		case c == ' ' || c == '\t':
			if buf.Len() == 0 {
				continue // skip leading whitespace within an arg
			}
			buf.WriteByte(c)
		default:
			buf.WriteByte(c)
		}
	}
	if inQ {
		return nil, errSplit("unterminated quoted string")
	}
	// The last argument has no trailing comma — flush it. Trim trailing
	// whitespace from bare args; quoted args already have exact contents.
	last := strings.TrimRight(buf.String(), " \t")
	out = append(out, last)
	return out, nil
}

// errSplit wraps a SplitArgs failure with a recognizable prefix so consumers
// can match in tests without depending on the message verbatim.
func errSplit(msg string) error {
	return splitError(msg)
}

// splitError is a string error type for SplitArgs failures. Defined as a
// distinct type so consumers can errors.As against it if they want, though
// the messages are stable enough for string matching too.
type splitError string

func (e splitError) Error() string { return "doctag.SplitArgs: " + string(e) }
