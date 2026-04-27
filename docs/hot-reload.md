# Hot-reload

**Short answer:** godot-go does not support hot-reloading extensions.
To pick up code changes, restart Godot.

This document explains why, what partial workflows still work, and
what an alternative architecture would look like for projects that
truly need it.

## The TL;DR

Godot 4.2+ can reload a GDExtension at runtime — replace the DLL,
re-init, recreate live instances onto the new code. The mechanism
relies on:

- `recreate_instance_func` per class for upgrading live instances.
- `NOTIFICATION_EXTENSION_RELOADED` (Object notification 2) so existing
  instances can rebuild their internal state.
- A clean tear-down/re-init cycle the engine can trigger from the
  editor's "Reload Extensions" command (or after an automatic
  recompile during development).

Godot expects all of that to be transparent to GDExtension authors.
For Rust (godot-rs) or C++ (godot-cpp), it largely is. For Go, it
isn't — and the obstacles are fundamental to how Go's runtime works,
not bugs in this framework.

## Why it doesn't work

### 1. The Go runtime can't be cleanly unloaded

Once `runtime.Init` (the implicit one C-shared DLLs run on first
load) executes inside a process, Go's scheduler, garbage collector,
sysmon thread, per-P workers, and global runtime state are all
permanent for that process's lifetime. There is no inverse — no
`runtime.Shutdown`, no way to dispose of the m/p/g pools, no way to
release the runtime's own thread-local storage. The Go team has
explicitly designed it this way: Go assumes it owns the process.

When Godot calls `FreeLibrary` (Windows) / `dlclose` (POSIX) on an
extension DLL during reload, the OS unmaps our code pages, but
Go's runtime threads are still scheduled, still GC'ing, still
calling functions that no longer exist. The result is an immediate
crash on the next sysmon tick or GC cycle.

A second `LoadLibrary` of the new DLL doesn't help: the new DLL
ships with its own copy of the Go runtime, but the *first* runtime
is still live and attempting to drive memory it no longer owns.
There's no way to merge or hand off.

### 2. Our shutdown hook calls `os.Exit(0)`

The framework's `internal/gdextension/shutdown.go` registers a
Core-level deinit callback that calls `os.Exit(0)`. This was the
fix for a process-shutdown segfault under the c-shared model on
Windows — Go's auxiliary threads race the engine's late teardown
during `DLL_PROCESS_DETACH`, and `os.Exit` from the very last
deinit callback skips the rest of engine teardown to terminate
cleanly via `ExitProcess`.

`os.Exit` is incompatible with reload by definition: the process
doesn't survive long enough to be re-initialized. Even if we
removed the hook for reload-mode runs, problem #1 would surface
first.

### 3. Engine deinit during reload doesn't reach Core level

A subtle observation worth flagging: Godot's reload typically only
fires deinit at Scene level (and Editor, if in editor mode). Core
deinit fires only at process exit. So our `os.Exit` hook may not
even trigger during reload — but problem #1 still applies, and the
engine still expects a working extension afterward.

## What still works without hot-reload

- **GDScript reload.** Unrelated to extensions; works fine.
- **Resource changes.** Textures, scenes, shaders all reload
  without touching the DLL.
- **Project reopens.** Each launch loads the extension fresh.
- **Inspector / scene editing.** Editing scenes that use Go
  extension classes is fine as long as you don't change the
  extension code.

## Workflow recommendation

The development loop:

1. Edit Go source.
2. `task build:example` (or your project's equivalent).
3. Restart Godot.

This is slower than the GDScript edit-test loop, but unavoidable.
A few tips that help:

- **Keep tests headless when possible.** Most behavior can be
  verified through `godot --headless --script test.gd`, which is
  much faster than a full editor restart.
- **Lean on properties + signals to push state out of Go.** The
  more your scene structure carries state in Godot resources, the
  less a Go restart costs you (the scene reopens with state intact).
- **Restart from the command line.** Closing and re-launching Godot
  from a terminal is a couple seconds; closing the editor window
  via the GUI and reopening the project is slower.

## What would have to change to support it

The only architecture that fundamentally accommodates Go's runtime
constraints is the **invert-the-host model**:

- Build the project as a Go binary, not a c-shared DLL.
- The Go program loads `libgodot.dll` via `LoadLibrary` /
  `dlopen` and drives it directly.
- Go owns the process lifetime; the engine is a passenger.
- Hot-reload becomes a question of "rebuild the Go binary and
  relaunch", which is fast because it doesn't go through Godot's
  reload path at all.

This is the model used by [graphics.gd][graphics-gd] (a sibling Go
extension framework). It changes the deployment story significantly:

- Production builds become Go binaries that ship with `libgodot`
  alongside, instead of `.gdextension` DLLs distributed inside a
  Godot project.
- Editor integration is more complex; graphics.gd runs Godot from
  Go for development and uses the c-shared model for editor mode.
- The whole framework would need redesign — class registration
  flow, scene loading, the relationship between user Go code and
  engine init.

godot-go is committed to the c-shared "Godot loads our DLL" model
because it integrates more naturally with Godot project structure
and mirrors how Rust/C++ extensions deploy. We accept the
no-hot-reload trade-off as the cost of that integration.

## Background reading

- [Godot GDExtension docs — Hot-reload section][godot-docs]
- [Go issue #11100 — runtime cleanup on dlclose][go-issue]
- [graphics.gd's host-inversion design][graphics-gd]
- The framework's own `internal/gdextension/shutdown.go` —
  documents the Core-deinit `os.Exit(0)` choice and the bisection
  that led to it.

[godot-docs]: https://docs.godotengine.org/en/stable/tutorials/scripting/gdextension/index.html
[go-issue]: https://github.com/golang/go/issues/11100
[graphics-gd]: https://github.com/quaadgras/graphics.gd
