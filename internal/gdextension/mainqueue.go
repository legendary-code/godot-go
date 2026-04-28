package gdextension

import "sync"

// Main-thread work queue.
//
// Godot's engine state is single-threaded — most engine method calls
// assume exclusive access from the engine's main thread, and racing
// them with goroutine-spawned work produces crashes or silent
// corruption. The framework provides:
//
//   - IsMainThread()  — runtime check (in thread.go).
//   - RunOnMain(f)    — schedule f for main-thread execution. Inline
//                       if already on main; queued otherwise.
//   - DrainMain()     — main-thread caller pulls all queued work and
//                       runs it. Typically called from the user's
//                       per-frame Process override.
//
// The primitives live in internal/gdextension (not internal/runtime,
// the user-facing convenience layer) because framework-internal code
// — particularly the bindgen-emitted RefCounted finalizers in
// core/ — needs to call RunOnMain without taking on internal/runtime
// as a dependency, which would create an import cycle.
// internal/runtime/thread.go re-exports the same names as a thin
// pass-through so user code can keep using `runtime.RunOnMain` etc.
//
// The queue is unbounded (slice + mutex). Bounded would risk
// deadlocking — a producer waiting for the main thread to drain,
// when main is waiting on the producer, is the worst kind of bug.
// Practical workloads post a handful of funcs per frame; the slice
// grows as needed and shrinks back to a small cap on each drain.

var (
	mainQueueMu sync.Mutex
	mainQueue   []func()
)

// RunOnMain schedules f to execute on the engine's main thread.
//
// If the caller is already on the main thread (IsMainThread is true),
// f runs synchronously and RunOnMain returns when f returns. Use
// this defensively when a code path might run from main or from a
// goroutine and you want the same behavior either way.
//
// Otherwise f is appended to the main-thread queue and runs on the
// next DrainMain. RunOnMain returns immediately in that case;
// callers that need to wait for f's completion should compose with
// a sync.WaitGroup or a result channel themselves.
//
// f must not panic. A panic in f, when DrainMain executes it, will
// take down the main thread and the entire process — there is no
// per-func recovery. Wrap with defer/recover inside f if you need
// fault tolerance.
func RunOnMain(f func()) {
	if f == nil {
		return
	}
	if IsMainThread() {
		f()
		return
	}
	mainQueueMu.Lock()
	mainQueue = append(mainQueue, f)
	mainQueueMu.Unlock()
}

// DrainMain executes every func currently queued via RunOnMain. Must
// be called from the main thread; calling from a worker is a logic
// bug (the funcs were queued precisely to avoid running off-main).
//
// Drain is single-pass — funcs queued by other goroutines DURING
// drain stay in the queue for the next DrainMain. This avoids
// pathological loops where each drained func enqueues another and
// the main thread never makes progress on its own per-frame work.
//
// Funcs are run in the order they were queued. The mutex is released
// while running each func so RunOnMain calls from inside a drained
// func don't deadlock — they just append to the queue and run on
// the NEXT drain.
//
// Calling DrainMain on an empty queue is a cheap no-op.
func DrainMain() {
	mainQueueMu.Lock()
	if len(mainQueue) == 0 {
		mainQueueMu.Unlock()
		return
	}
	// Atomic swap: take everything that's queued right now into a
	// local slice, leave the queue empty (with cap preserved for
	// the next round to avoid re-allocating). Anything posted by
	// the funcs we're about to run lands in the next batch.
	batch := mainQueue
	mainQueue = mainQueue[:0:0]
	mainQueueMu.Unlock()

	for _, fn := range batch {
		fn()
	}
}
