# Threading

**Short answer:** Most Godot engine calls require the main thread.
Use `runtime.RunOnMain(func)` from goroutines to bounce engine work
back to a safe thread, and call `runtime.DrainMain()` from a per-
frame hook (typically a `Process` override) so queued work actually
runs.

```go
import "github.com/legendary-code/godot-go/internal/runtime"

func (n *MyNode) FetchScore() {
    go func() {
        score := slowHTTPCall()
        runtime.RunOnMain(func() {
            n.scoreLabel.SetText(score)  // safe — runs on main thread
        })
    }()
}

// @override
func (n *MyNode) Process(delta float64) {
    runtime.DrainMain()  // execute queued main-thread work this frame
    // ... your per-frame logic ...
}
```

If you don't need goroutines, you don't need any of this — the
engine drives everything from the main thread by default.

## Why this exists

Godot 4 is largely single-threaded. The scene tree, ClassDB, signal
emission, virtually every engine method assumes exclusive access
from the engine's main thread. Calling those from a goroutine
running on a worker thread crashes (engine writes to structures it
assumes own exclusively) or, worse, silently corrupts state with
heisenbugs that surface much later.

GDScript users rarely hit this because GDScript exposes threads
explicitly via the `Thread` class — opt-in and obvious. Go users
hit it constantly because goroutines are everywhere and idiomatic.
Without framework support, every Go-Godot extension is one goroutine
away from a hard-to-debug crash.

## What's safe on any thread

Operations that don't touch engine state are safe to use from any
goroutine:

- **Pure Go logic** — your own data structures, computation, channels.
- **Network I/O** — `net/http`, gRPC, database drivers. The engine
  doesn't know or care.
- **File I/O** through Go's `os` / `io` packages (not via Godot's
  `FileAccess`).
- **Variant arithmetic that doesn't touch a Godot Object** — math
  on bare numbers, constructing `variant.Vector2` from primitives.
  The variant package documents which methods are safe; in general,
  anything that doesn't dereference an `ObjectPtr` is fine.

## What requires the main thread

If you're not sure, assume main-thread only:

- **All engine method calls.** Anything that crosses an
  `ObjectPtr` boundary — `node.SetText(…)`, `obj.CallDeferred(…)`,
  reading a node's `position`.
- **ClassDB queries** — `ClassDB.class_exists(…)`, etc.
- **Signal emission and connection** —
  `n.damaged.emit(…)`, `n.damaged.connect(…)`, anything that
  routes through `Object::emit_signal`.
- **Scene tree mutations** — `add_child`, `queue_free`, instancing,
  loading scenes/resources.
- **Anything else not explicitly documented as thread-safe.**
  Godot's documentation is the source of truth; when in doubt,
  bounce to main.

## The framework's threading helpers

Three exported functions in `internal/runtime`:

### `runtime.IsMainThread() bool`

Reports whether the calling code is currently running on Godot's
main thread. The framework captures the main-thread ID on the first
engine→Go callback (extension load) and compares against the
calling OS thread.

Returns `false` before the framework has been called by the engine
at least once (rare in practice — by the time your code is running,
SCENE init has fired).

### `runtime.RunOnMain(f func())`

Schedules `f` for main-thread execution.

- If the caller is **already on the main thread** (`IsMainThread`
  returns true), `f` runs synchronously and `RunOnMain` returns
  when `f` returns. Use this defensively when a code path might
  run from main or from a goroutine.
- Otherwise `f` is appended to the main-thread queue and runs on
  the next `DrainMain`. `RunOnMain` returns immediately in that
  case.

`f` must not panic. A panic inside `f` when `DrainMain` executes it
will take down the main thread and the entire process — there is no
per-func recovery. Wrap with `defer/recover` inside `f` if you need
fault tolerance.

`f == nil` is a silent no-op.

### `runtime.DrainMain()`

Executes every func currently queued via `RunOnMain`. Must be
called from the main thread; calling from a worker is a logic bug
(the funcs were queued precisely to avoid running off-main).

Drain is **single-pass**. Funcs queued by other goroutines while a
drain is running stay in the queue for the next `DrainMain`. This
prevents pathological loops where each drained func enqueues
another and the main thread never makes progress on its own
per-frame work.

Funcs run in FIFO order. Calling `DrainMain` on an empty queue is a
cheap no-op.

## Wiring the drain into your game loop

The framework does not auto-drain — you choose when. The standard
recipe is to call `DrainMain` once per frame from a `Process`
override:

```go
// @override
func (n *MyNode) Process(delta float64) {
    runtime.DrainMain()
    // ... per-frame game logic ...
}
```

Reasons to vary this:

- **Multiple drainage points.** If you have several long-lived
  nodes, you only need ONE of them to drain — the queue is global.
  Pick the most stable one (your top-level controller, etc.).
- **Physics-tick drain.** Use `PhysicsProcess` instead of `Process`
  if your queued work needs to influence physics in the same step
  it was queued.
- **Manual drain.** For one-off work where you know the queue
  state, call `DrainMain` directly anywhere on the main thread.

If your extension has no per-frame node (e.g., editor plugins or
singleton-only extensions), call `DrainMain` from whatever main-
thread hook you do have — typically a tool method invoked from
GDScript or an editor button.

## Common patterns

### Bounce a result from a goroutine

```go
func (n *MyNode) StartLoading() {
    go func() {
        data, err := doSlowWork()
        runtime.RunOnMain(func() {
            if err != nil {
                n.statusLabel.SetText("Failed: " + err.Error())
                return
            }
            n.applyData(data)
        })
    }()
}
```

### Be safe regardless of caller thread

```go
func (n *MyNode) UpdateLabel(text string) {
    runtime.RunOnMain(func() {
        n.label.SetText(text)
    })
}
```

`UpdateLabel` is now safe to call from any thread. If the caller is
on main, the inline path makes it a normal synchronous call. If
not, it queues.

### Block a goroutine until the main thread runs your work

The framework doesn't ship a `RunOnMainSync` (deferred — see
`PLAN_EXECUTION.md`), but you can compose one:

```go
done := make(chan struct{})
runtime.RunOnMain(func() {
    n.applyData(data)
    close(done)
})
<-done  // goroutine blocks until main drains the queue
```

Be careful with this — if main is blocked waiting for the
goroutine you're calling from, you'll deadlock.

## What this doesn't solve

- **Re-entrant engine calls from goroutines.** If goroutine A
  calls into engine state (even via `RunOnMain`) and that engine
  call somehow ends up running goroutine B's `RunOnMain` callback
  too, the ordering between A and B is loosely defined. Don't
  rely on inter-goroutine ordering through main-thread funcs;
  use channels or synchronization primitives between goroutines.
- **Engine state your code shares with goroutines without a
  bounce.** The bounce only works if you actually use it. A
  goroutine that touches `n.someChild.GetPosition()` directly is
  still racing engine state regardless of `IsMainThread` checks
  elsewhere.
- **Hot-reload.** Independent question; see `docs/hot-reload.md`.

## Future work

- **Auto-drain.** A future iteration may inject `DrainMain` calls
  into codegen-generated `Process` methods, or provide a hidden
  framework Node that drains automatically. Both have non-trivial
  trade-offs (codegen surprises users; hidden nodes don't auto-
  instantiate). The MVP requires the user to be intentional.
- **`RunOnMainSync(f func()) error`** that blocks until the func
  has run on main, returning any panic as an error. Useful for
  goroutines that genuinely need a main-thread result before
  proceeding. Deadlock-prone if used carelessly; deferred until
  there's a clear need.
- **Long-lived background workers.** A more structured "worker
  pool" pattern (start N goroutines that pull from a job channel,
  bounce results to main) is straightforward to build on top of
  these primitives — left to user code rather than baked in.

## See also

- `internal/runtime/thread.go` — the implementation. Worth a read
  if you want to understand the queue's lock semantics.
- `internal/gdextension/thread.go` — main-thread ID capture.
- `examples/smoke/main.go` — the framework's own threading
  exercises (search for `IsMainThread`).
