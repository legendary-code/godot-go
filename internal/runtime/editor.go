package runtime

import (
	"sync"

	"github.com/legendary-code/godot-go/core"
)

// IsEditorHint reports whether the calling code is currently running
// inside the Godot editor's "edit-time" execution context — for
// example, while the inspector is rebuilding a `@tool`-equivalent
// extension class or evaluating editor-only code paths.
//
// Mirrors GDScript's `Engine.is_editor_hint()`. Returns false in
// deployed game builds and false during play-mode runs from inside
// the editor (the engine flips the hint off when scenes are actually
// running). Use this for branching: editor-only debug visuals,
// tooling helpers, parameter defaults, etc.
//
// Distinct from the `@editor` codegen doctag, which controls *whether
// a class is registered at all* outside the editor (that's the
// init-level decision). `IsEditorHint` is per-call: a normally-
// registered class can use it to vary behavior between editor and
// runtime.
//
// Cheap — Engine singleton is resolved once and cached. Backed by
// the engine's MethodBindPtr cache so the call itself is a single
// ptrcall.
func IsEditorHint() bool {
	return engineSingleton().IsEditorHint()
}

// engineSingleton caches the Engine singleton resolution. Lazy so the
// runtime package doesn't depend on the engine being available at Go
// init time.
var engineSingleton = sync.OnceValue(func() *core.Engine {
	return core.EngineSingleton()
})
