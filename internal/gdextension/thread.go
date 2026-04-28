package gdextension

/*
#cgo CFLAGS: -I${SRCDIR} -I${SRCDIR}/../../godot

#include "shim.h"
*/
import "C"

import (
	"sync/atomic"
)

// Main-thread identification.
//
// Godot's engine state is single-threaded by default — the scene
// tree, ClassDB, signal emission, virtually every engine method
// call assumes exclusive access from the engine's main thread.
// Calling those from a goroutine running on a different OS thread
// is a data race that crashes or silently corrupts state.
//
// Go programs frequently use goroutines, which the Go scheduler
// places on whatever worker thread is convenient. The framework
// captures the engine's main-thread ID on first entry and exposes
// it so user code can decide between executing inline (already on
// main) and queueing for later main-thread drainage.
//
// The captured value isn't a portable number; it's only meaningful
// for `==` comparisons within the same process, on the same OS,
// against another value obtained the same way.

var capturedMainThreadID atomic.Uint64

// CaptureMainThread records the current OS thread as the engine's
// main thread. Idempotent — only the first call has effect, so
// subsequent invocations from different threads can't clobber the
// captured ID. Called automatically from godotGoOnInitialize on
// the first engine→Go callback (which runs on the engine main
// thread by Godot's contract).
func CaptureMainThread() {
	tid := uint64(C.godot_go_current_thread_id())
	// CompareAndSwap with zero — only the first caller wins. A non-
	// zero existing value means a previous call already captured
	// the ID; we leave it alone rather than risk overwriting with
	// a worker-thread ID if someone calls this from the wrong place.
	capturedMainThreadID.CompareAndSwap(0, tid)
}

// MainThreadID returns the captured engine-main-thread identifier.
// Returns 0 before CaptureMainThread has run; callers should treat
// 0 as "not yet known" rather than as a real thread ID.
func MainThreadID() uint64 {
	return capturedMainThreadID.Load()
}

// CurrentThreadID returns the OS thread identifier of the goroutine
// currently calling. Cgo round-trip; cheap but not free, so callers
// in hot loops should cache where reasonable.
func CurrentThreadID() uint64 {
	return uint64(C.godot_go_current_thread_id())
}

// IsMainThread reports whether the calling code is currently
// executing on the engine's main thread. Returns false if the
// main thread hasn't been captured yet (defensive — safer to
// queue work than to assume).
func IsMainThread() bool {
	main := capturedMainThreadID.Load()
	if main == 0 {
		return false
	}
	return uint64(C.godot_go_current_thread_id()) == main
}
