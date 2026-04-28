package runtime

import (
	"github.com/legendary-code/godot-go/internal/gdextension"
)

// User-facing aliases for the main-thread work queue. The actual
// implementation lives in internal/gdextension/mainqueue.go because
// framework-internal code (bindgen-emitted RefCounted finalizers in
// core/) needs to reach RunOnMain without an import cycle through
// internal/runtime.
//
// Behavior, semantics, and surface are unchanged from the original
// definitions; this file is just where user-package callers reach
// for them.

// IsMainThread reports whether the calling goroutine is currently
// running on the engine's main thread.
func IsMainThread() bool { return gdextension.IsMainThread() }

// RunOnMain schedules f to execute on the engine's main thread.
// Inline if already on main; queued otherwise. See
// internal/gdextension.RunOnMain for full semantics.
func RunOnMain(f func()) { gdextension.RunOnMain(f) }

// DrainMain executes every func currently queued via RunOnMain.
// Must be called from the main thread.
func DrainMain() { gdextension.DrainMain() }
