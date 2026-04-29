package main

import (
	"fmt"
	"go/ast"
	"strings"
	"unicode"

	"github.com/legendary-code/godot-go/internal/doctag"
)

// parseDocInfo extracts a docInfo from a doc comment + already-parsed
// tag list. typeName is the leading identifier the prose-extractor
// strips when present (Go's standard doc-comment convention starts the
// description with the type name; Godot's docs page renders that as
// noise so we drop it).
//
// Description defaults to the doc comment's prose body (multi-paragraph,
// `@`-prefixed tag lines removed). `@description "..."` overrides
// outright. Deprecated/Experimental are pointer-tristate per the
// docInfo definition. Since/See pass through verbatim.
func parseDocInfo(cg *ast.CommentGroup, tags []doctag.Tag, typeName string) docInfo {
	info := docInfo{}
	if v, ok := doctag.Find(tags, "description"); ok {
		info.Description = v
	} else {
		info.Description = extractDocProse(cg, typeName)
	}
	if v, ok := doctag.Find(tags, "deprecated"); ok {
		info.Deprecated = strPtr(v)
	}
	if v, ok := doctag.Find(tags, "experimental"); ok {
		info.Experimental = strPtr(v)
	}
	if v, ok := doctag.Find(tags, "since"); ok {
		info.Since = v
	}
	for _, t := range tags {
		if t.Name == "see" {
			info.See = append(info.See, t.Value)
		}
	}
	return info
}

// parseTutorials walks the tag list and returns every `@tutorial(title,
// url)` entry. Class-only — Godot's schema doesn't carry tutorials on
// other declaration kinds, so the discover pass uses this only when
// building classInfo. Errors carry a position-less message; the caller
// wraps with file:line.
func parseTutorials(tags []doctag.Tag) ([]tutorialInfo, error) {
	var out []tutorialInfo
	for _, t := range tags {
		if t.Name != "tutorial" {
			continue
		}
		args, err := doctag.SplitArgs(t.Value)
		if err != nil {
			return nil, fmt.Errorf("@tutorial: %w", err)
		}
		if len(args) != 2 {
			return nil, fmt.Errorf("@tutorial expects (title, url), got %d args", len(args))
		}
		out = append(out, tutorialInfo{Title: args[0], URL: args[1]})
	}
	return out, nil
}

// extractDocProse pulls the plain-text prose out of a doc comment,
// dropping any line that begins with `@` (those are doctags) and any
// blank lines at the start / end. The Go convention of "TypeName foo
// is …" gets the type-name prefix stripped from the leading line so the
// result reads naturally on Godot's docs page.
//
// Multi-line prose is joined with newlines preserved; downstream code
// can collapse whitespace at render time as needed.
func extractDocProse(cg *ast.CommentGroup, typeName string) string {
	if cg == nil {
		return ""
	}
	var lines []string
	for _, c := range cg.List {
		text := stripCommentMarker(c.Text)
		for _, line := range strings.Split(text, "\n") {
			// Skip @-prefixed doctag lines.
			trimmed := strings.TrimLeft(line, " \t*")
			if strings.HasPrefix(trimmed, "@") {
				continue
			}
			// Strip the conventional single space after `// ` or `/* `
			// so the prose isn't indented in the rendered docs.
			if strings.HasPrefix(line, " ") {
				line = line[1:]
			}
			lines = append(lines, line)
		}
	}

	// Trim leading and trailing empty lines so the prose body sits
	// flush against its boundaries.
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return ""
	}

	// Drop the "TypeName " prefix from the leading line and uppercase
	// the next rune so the prose reads as a proper sentence.
	if typeName != "" && strings.HasPrefix(lines[0], typeName+" ") {
		rest := lines[0][len(typeName)+1:]
		if rest != "" {
			r := []rune(rest)
			r[0] = unicode.ToUpper(r[0])
			lines[0] = string(r)
		}
	}
	return strings.Join(lines, "\n")
}

// extractBrief returns the one-line summary suitable for
// <brief_description>: the leading sentence of the prose, terminated
// at the first ". " (period + space). If the prose has no such
// terminator, the entire first paragraph is returned. Newlines
// within the first paragraph are collapsed to spaces so the brief
// reads as one line.
func extractBrief(prose string) string {
	if prose == "" {
		return ""
	}
	first := prose
	if i := strings.Index(first, "\n\n"); i >= 0 {
		first = first[:i]
	}
	first = strings.ReplaceAll(first, "\n", " ")
	first = strings.TrimSpace(first)
	// Truncate at the first sentence boundary if the leading
	// paragraph is multi-sentence.
	if i := strings.Index(first, ". "); i >= 0 {
		return first[:i+1]
	}
	return first
}

// stripCommentMarker drops the `//` or `/* … */` decorations so the
// remaining text is just the comment body.
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

// strPtr returns &s. Tiny helper used for the Deprecated/Experimental
// pointer-tristate so the call sites stay readable.
func strPtr(s string) *string { return &s }
