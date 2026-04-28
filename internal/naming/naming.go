// Package naming centralizes the Go ↔ Godot identifier conventions
// the framework's two codegen tools both rely on. Putting them in one
// place keeps the transformations consistent — what
// `cmd/godot-go-bindgen` emits as a Go identifier is exactly what
// `cmd/godot-go` would convert back to a Godot identifier.
//
// Conventions:
//
//   - **Godot uses snake_case** for class members (methods,
//     properties, signals, virtuals — `_process`, `set_position`,
//     `damage_taken`).
//   - **Go uses PascalCase** for exported identifiers
//     (`SetPosition`) and lowercase first letter for unexported
//     ones (`damageTaken`).
//   - **Class type names** are PascalCase on both sides
//     (`Node2D`, `EditorPlugin`, `MyNode`).
//
// The two transformations the framework applies:
//
//   - PascalToSnake: user's exported Go method name → Godot's
//     ClassDB-visible name (`SetPosition` → `set_position`,
//     `HTTPServer` → `http_server`).
//   - SnakeToPascal: bindgen-side, Godot's snake_case identifier →
//     a Go-style PascalCase identifier (`set_position` →
//     `SetPosition`, `audio_stream_player_2d` →
//     `AudioStreamPlayer2D`).
//
// The two are inverses for the typical cases. They're not strict
// inverses for acronyms (`http_server` → `HttpServer` going one way,
// `HTTPServer` → `http_server` going the other) — that asymmetry is
// inherent to snake_case, not a bug here.
package naming

import (
	"strings"
	"unicode"
)

// PascalToSnake converts Go-style PascalCase to Godot-style
// snake_case. Acronyms collapse into a single word and a separator
// is inserted at acronym→word boundaries.
//
// Examples:
//
//	PascalToSnake("Hello")        // → "hello"
//	PascalToSnake("HelloWorld")   // → "hello_world"
//	PascalToSnake("HTTPServer")   // → "http_server"
//	PascalToSnake("GetURL")       // → "get_url"
//	PascalToSnake("Vector2")      // → "vector2"
//	PascalToSnake("X")            // → "x"
//	PascalToSnake("")             // → ""
//
// Used by `cmd/godot-go` to derive Godot method/property/signal
// names from the user's Go identifiers.
func PascalToSnake(s string) string {
	var b strings.Builder
	prevUpper := false
	for i, r := range s {
		isUpper := unicode.IsUpper(r)
		if i > 0 && isUpper && !prevUpper {
			b.WriteByte('_')
		}
		// Acronym → word boundary: at the `S` in `HTTPServer` after
		// the `P`, prevUpper is true but the *next* rune is lower,
		// so a separator belongs before this char.
		if i > 0 && isUpper && prevUpper {
			next, ok := nextRune(s, i, r)
			if ok && unicode.IsLower(next) {
				b.WriteByte('_')
			}
		}
		b.WriteRune(unicode.ToLower(r))
		prevUpper = isUpper
	}
	return b.String()
}

// SnakeToPascal converts Godot-style snake_case to a Go-style
// PascalCase identifier. Each underscore-separated segment becomes a
// capitalized word; embedded digits stay where they are.
//
// Examples:
//
//	SnakeToPascal("hello")              // → "Hello"
//	SnakeToPascal("hello_world")        // → "HelloWorld"
//	SnakeToPascal("audio_stream_2d")    // → "AudioStream2d"
//	SnakeToPascal("vector2")            // → "Vector2"
//	SnakeToPascal("x")                  // → "X"
//	SnakeToPascal("")                   // → ""
//
// Used by `cmd/godot-go-bindgen` when emitting Go wrappers for
// Godot's engine classes and methods.
func SnakeToPascal(s string) string {
	if s == "" {
		return ""
	}
	parts := strings.Split(s, "_")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, "")
}

// nextRune returns the rune immediately following position i in s,
// or (0, false) if i was the last rune. Helper for PascalToSnake's
// acronym-boundary lookahead.
func nextRune(s string, i int, cur rune) (rune, bool) {
	off := i + len(string(cur))
	if off >= len(s) {
		return 0, false
	}
	for _, r := range s[off:] {
		return r, true
	}
	return 0, false
}
