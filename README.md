# godot-go

[![CI](https://github.com/legendary-code/godot-go/actions/workflows/ci.yml/badge.svg)](https://github.com/legendary-code/godot-go/actions/workflows/ci.yml)

A Go framework for writing [Godot 4](https://godotengine.org/)
GDExtensions. Define your engine classes as plain Go structs; a code
generator turns doc-tagged declarations into the C ABI Godot expects.

```go
package main

//go:generate godot-go

import (
    "github.com/legendary-code/godot-go/godot"
    "github.com/legendary-code/godot-go/godot/runtime"
)

// Player is exposed to Godot as a `Player` Node.
//
// @class
type Player struct {
    // @extends — Player extends Node, the same as `extends Node` in GDScript.
    godot.Node

    // @property
    // @export_range(0, 100)
    Health int64
}

// @signals
type Signals interface {
    Damaged(amount int64)
}

// @override — registers as the engine virtual `_process`.
func (p *Player) Process(delta float64) {
    runtime.DrainMain()
    // ... per-frame logic ...
}

// TakeDamage is callable from GDScript as `player.take_damage(10)`.
func (p *Player) TakeDamage(amount int64) {
    p.Health -= amount
    p.Damaged(amount)
}
```

`go generate` produces a sibling `<file>_bindings.go` that registers
the class, its property (with the slider hint Godot's inspector
shows), the signal, the virtual override, and the regular method.
The user's source above is the entire ergonomic surface.

## What's supported

- Class registration with `@abstract`, `@editor`, and inner-class
  declarations.
- Method binding for primitive types (`bool`, `int`/`int32`/`int64`,
  `float32`/`float64`, `string`) and user-defined int enums.
- Static methods (unnamed receiver), virtual overrides (`@override`),
  and regular instance methods.
- Properties via `@property`/`@readonly`, both field-form (codegen
  synthesizes getter/setter) and method-form (user owns getter and
  optional setter).
- Property inspector polish — `@group`/`@subgroup`,
  `@export_range`/`@export_enum`/`@export_file`/`@export_dir`/
  `@export_multiline`/`@export_placeholder`.
- Signals via a `@signals`-tagged interface — codegen synthesizes
  typed Go-side emit methods; emission from outside the class uses
  Godot's standard Signal API.
- Goroutine ↔ main-thread bridge: `runtime.IsMainThread()`,
  `runtime.RunOnMain(f)`, `runtime.DrainMain()`.
- Editor-mode detection: `runtime.IsEditorHint()`.
- Automatic refcount management for `RefCounted`-derived classes
  (`Resource`, `Image`, `RegEx`, ...) via Go finalizers — drop the
  Go reference and the engine cleanup happens automatically. See
  [`docs/lifecycle.md`](./docs/lifecycle.md).

## Known limitations

- **Hot-reload** of extensions doesn't work because Go's runtime
  can't be cleanly unloaded; restart Godot to apply code changes.
  See [`docs/hot-reload.md`](./docs/hot-reload.md) for the full
  explanation and recommended dev loop.
- **Property hint coverage** — the seven most-used `PropertyHint`
  values are wired (RANGE, ENUM, FILE, DIR, MULTILINE_TEXT,
  PLACEHOLDER_TEXT, plus default NONE). LAYERS_*, RESOURCE_TYPE,
  COLOR_NO_ALPHA, FLAGS, etc. aren't reachable from doctags yet.
- **Method-form `@property`** doesn't accept export hints (those
  are field-form only in the current release).

## Quickstart

### Prerequisites

- **Go 1.26+** (the project's `go.mod` pins 1.26.1).
- **Godot 4.6** to load the resulting extension.
- A **C toolchain reachable by cgo**: MinGW-w64 on Windows, the
  standard clang/gcc on macOS / Linux.
- [`go-task`](https://taskfile.dev) v3 for the build orchestration
  (optional — `go build` works directly too).

### Build the smoke example

The repo ships with a smoke test that exercises every feature:

```sh
task build:example
```

This produces `examples/smoke/godot_project/bin/godot_go_smoke.dll`
(or `.so` / `.dylib`).

### Run it headless

The first time, import the project so Godot recognizes the
`.gdextension`:

```sh
godot --headless --editor --path examples/smoke/godot_project --quit-after 5
```

(One-shot. Creates `examples/smoke/godot_project/.godot/`. Skip on
subsequent runs.)

Then the verification script:

```sh
godot --headless --path examples/smoke/godot_project --script test_mynode.gd
```

Expect `test_mynode: ALL CHECKS PASSED` near the end.

### Run the Go test suite

```sh
task test
# or:
go test ./...
```

## Authoring a new extension

The framework's bindings are generated against the version of Godot
*you* target — not pinned by the framework. The full setup is in
[`docs/setup.md`](./docs/setup.md); the short version:

1. Create the bindings package once. Drop a `godot/` directory into
   your project, copy in your Godot's `extension_api.json` (produced
   by `godot --headless --dump-extension-api`), and add a setup
   file that triggers the bindgen:

   ```go
   // godot/generate_bindings.go
   package godot

   //go:generate go run github.com/legendary-code/godot-go/cmd/godot-go-bindgen -api ./extension_api.json
   ```

   `go generate ./godot/...` then stamps out the bindings into the
   same package — engine classes, builtins, enums, utility
   functions, plus a `godot/runtime` sub-package with
   `IsEditorHint`, `Print`, `RunOnMain`, etc.

2. Author your extension class in any other directory. Embed
   `godot.Node` (or any other engine class), add
   `//go:generate godot-go` for the user-class codegen, run
   `go generate ./...` to produce `<file>_bindings.go`.

3. Build with `go build -buildmode=c-shared -o yourext.dll ./yourpkg`
   and drop the resulting library + a `.gdextension` config file
   into your Godot project.

The `examples/smoke/` and `examples/locale_language/` directories
are working references — copy from there.

## Documentation

- [`docs/setup.md`](./docs/setup.md) — full walkthrough of setting
  up a new project: `godot/` directory layout, `extension_api.json`
  pinning, version drift behavior, multi-version targeting.
- [`docs/threading.md`](./docs/threading.md) — goroutines and
  Godot's main thread; `RunOnMain` / `DrainMain`.
- [`docs/lifecycle.md`](./docs/lifecycle.md) — how the framework
  handles refcounted resources (`Resource`, `Image`, `RegEx`, ...)
  via Go finalizers, and when you'd reach for explicit
  `Unreference`.
- [`docs/hot-reload.md`](./docs/hot-reload.md) — why hot-reload of
  Go extensions doesn't work, and what dev workflow does.

## How it works

Two code generators live under `cmd/`:

- **`cmd/godot-go-bindgen`** — run from your project's `godot/`
  setup file via `go generate`. Reads `extension_api.json` and
  emits the bindings package: engine classes, builtins, global
  enums, utility functions, native structures, plus a sibling
  `godot/runtime/` sub-package with `IsEditorHint` / `Print` /
  `RunOnMain` / etc.

- **`cmd/godot-go`** — the user-class codegen invoked by
  `//go:generate godot-go`. Reads doc-tagged Go source, validates
  it, and emits a `<file>_bindings.go` containing the cgo
  trampolines, `RegisterClass*` calls, and any synthesized
  accessor / emit methods.

The runtime layer `gdextension/` wraps the GDExtension C ABI; the
generated `godot/runtime/` provides user-level conveniences
(`Print` / `Printf`, `IsEditorHint`, `RunOnMain`, etc.).

## Build configuration

godot-go targets the **single-precision** Godot build by default
(`extension_api.json`'s `float_32` configuration). To target a
double-precision Godot build, add `-tags godot_double_precision` to
your `go build` invocation. Mixed-precision binaries are not
supported — the host Godot must match the build tag.

## Common commands

```sh
task                          # list all targets
task generate:bindings        # regenerate godot/ from extension_api.json
task build:example            # build the smoke c-shared library
task build:locale_language    # build the locale_language showcase
task test                     # run the Go test suite (regenerates bindings first)
```

## Comparison with other Go-Godot projects

[graphics.gd](https://github.com/quaadgras/graphics.gd) is the
sibling project. The architectural difference is who owns the
process:

- **godot-go**: Godot loads the extension as a c-shared DLL.
  Production ships a `.gdextension` package alongside your Godot
  project. Familiar deployment model; easy editor integration.
  Cost: no hot-reload (Go runtime can't be unloaded).
- **graphics.gd**: a Go binary loads `libgodot` itself. Production
  ships a Go executable that bundles the engine. Different
  deployment story; supports more flexible reload patterns.

If you're writing a tool or game where deploying as a `.gdextension`
fits naturally — particularly if your team's workflow centers on the
Godot editor — godot-go is the closer match. If you want Go to
own the process and use Godot as a renderer/scene library, look at
graphics.gd.

## Contributing

Open an issue or PR. Changes are easiest to review when scoped to
one feature or fix at a time.
