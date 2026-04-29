# godot-go

[![CI](https://github.com/legendary-code/godot-go/actions/workflows/ci.yml/badge.svg)](https://github.com/legendary-code/godot-go/actions/workflows/ci.yml)

Write [Godot 4](https://godotengine.org/) extensions in idiomatic Go.
Define your engine classes as plain structs, mark behaviour with
doc-comment tags, and a code generator turns them into the C ABI
Godot expects.

---

## Overview

godot-go is a framework for authoring [GDExtensions](https://docs.godotengine.org/en/stable/tutorials/scripting/gdextension/index.html)
in Go. Your project ships as a c-shared library that Godot loads
alongside your `.gdextension` config — the same deployment model
GDScript or godot-cpp users are used to.

The framework has two halves:

- **`gdextension/`** — the cgo bridge to Godot's GDExtension ABI.
  Hand-written, version-aware, the part most users never look at.
- **`cmd/godot-go-bindgen/` and `cmd/godot-go/`** — two code
  generators. The first produces the Godot bindings package
  (engine classes, builtins, enums, utility functions) from
  `extension_api.json`. The second turns your doc-tagged Go classes
  into the registration glue Godot's ClassDB needs.

The bindings aren't pre-generated. Each project picks its target
Godot version (4.4, 4.5, or 4.6), drops the matching
`extension_api.json` into a `godot/` directory, and runs
`go generate`. The framework's tests run against all three versions
on Linux, macOS, and Windows.

If your team's workflow centers on the Godot editor and you want to
deploy as a normal `.gdextension`, godot-go is the closer match to
GDScript. If you want a Go binary that owns the process and uses
Godot as a library, look at [graphics.gd](#comparison-with-graphicsgd)
instead.

## A taste

```go
package main

//go:generate godot-go

import (
    "github.com/yourname/yourproject/godot"
    "github.com/yourname/yourproject/godot/runtime"
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

Running `go generate` produces a sibling `<file>_bindings.go` that
registers the class, the property (with the slider hint Godot's
inspector shows), the signal, the `_process` virtual override, and
`take_damage` as a regular method. The source above is the entire
ergonomic surface — no boilerplate.

## Install

### Prerequisites

- **Go 1.26+** (the module's `go.mod` pins 1.26.1).
- **Godot 4.4, 4.5, or 4.6.** The framework's CI matrix exercises
  all three on Linux, macOS, and Windows.
- **A C toolchain reachable by cgo.** MinGW-w64 on Windows; the
  standard clang/gcc on macOS / Linux.
- **[`go-task`](https://taskfile.dev) v3** for the build orchestration
  (optional — `go build` works directly).

### Add the framework to your project

In a fresh project:

```sh
go mod init github.com/yourname/yourproject
go get github.com/legendary-code/godot-go
go install github.com/legendary-code/godot-go/cmd/godot-go
```

`go get` adds the framework to your `go.mod`. `go install`
installs the `godot-go` binary on `$PATH` so the
`//go:generate godot-go` directive in your extension source can
find it (the bindgen, by contrast, is invoked via `go run` from
the `godot/generate_bindings.go` trigger — no separate install).

## Generate the bindings

The framework ships no pre-generated bindings — your project pins
the Godot version it targets and the bindgen stamps out the
matching package.

Inside your project, create a `godot/` directory with two files:

```
godot/
├── extension_api.json     # the API dump for your target Godot version
└── generate_bindings.go   # //go:generate trigger for the bindgen
```

`generate_bindings.go`:

```go
package godot

//go:generate go run github.com/legendary-code/godot-go/cmd/godot-go-bindgen -api ./extension_api.json
```

For `extension_api.json`, either dump it from your local Godot
binary:

```sh
godot --headless --dump-extension-api
```

…or grab it from [godot-cpp's version branches](https://github.com/godotengine/godot-cpp/branches)
— they keep one JSON per Godot release at `gdextension/extension_api.json`
on the `4.4`, `4.5`, `4.6` branches.

Then generate:

```sh
go generate ./godot/...
```

This produces ~1100 `*.gen.go` files in `godot/` plus a tiny
`godot/runtime/` sub-package with user-facing helpers
(`IsEditorHint`, `Print`, `RunOnMain`, …). They're fully
reproducible from `extension_api.json`, so committing them or
gitignoring them is your call — neither breaks anything.

[`docs/setup.md`](./docs/setup.md) covers the full setup — multi-
version targeting, version-drift detection, build orchestration via
Taskfile.

## Start a project

Once the bindings are in place, your extension is plain Go. A
minimal example:

```go
// hello.go
package main

//go:generate godot-go

import (
    "github.com/yourname/yourproject/godot"
    "github.com/yourname/yourproject/godot/runtime"
)

// @class
type Hello struct {
    // @extends
    godot.Node
}

func (h *Hello) Greet() {
    runtime.Print("hello from Go")
}
```

Build:

```sh
go generate ./...
# Pick the extension your platform loads:
#   Linux   → hello.so
#   macOS   → hello.dylib
#   Windows → hello.dll
go build -buildmode=c-shared -o hello.so ./yourpkg
```

The framework's `gdextension/` package provides the
`gdextension_library_init` entry symbol Godot looks for —
importing it (directly or transitively, via your bindings package)
is enough. You never write C glue or a separate entrypoint.

Drop the resulting library into your Godot project alongside a
`.gdextension` manifest that points at it:

```ini
; godot_project/hello.gdextension
[configuration]
entry_symbol = "gdextension_library_init"
compatibility_minimum = "4.4"

[libraries]
linux.x86_64   = "res://bin/hello.so"
macos          = "res://bin/hello.dylib"
windows.x86_64 = "res://bin/hello.dll"
```

`compatibility_minimum` is the floor your project enforces —
Godot refuses to load the extension on older versions. The
framework supports `4.4` as the lowest. `entry_symbol` is the C
function the framework exports; the value is always
`gdextension_library_init`.

After both files are in place, from GDScript:

```gdscript
var h = Hello.new()
h.greet()  # prints "hello from Go" to the Output dock
```

[`examples/smoke/`](./examples/smoke/) and
[`examples/locale_language/`](./examples/locale_language/) are the
working references — `examples/2d_demo/` adds a Node2D subclass
with property + signal + virtual override that actually moves a
sprite around. Copy from any of them.

## Reference

[**`docs/reference.md`**](./docs/reference.md) is the comprehensive
reference for everything the framework exposes:

- [Doc tag reference](./docs/reference.md#doc-tags) — every
  recognized tag, organized by what kind of declaration it applies
  to (struct / field / method / interface) with a one-line
  semantics summary and a minimal example.
- [Feature reference](./docs/reference.md#features) — class
  registration, methods, properties, signals, virtuals, runtime
  helpers, refcounted lifecycle.
- [Type support](./docs/reference.md#supported-types) — what Go
  types are valid for method args / returns / properties / signal
  parameters.
- [Multi-version targeting](./docs/reference.md#multi-version-targeting)
  — which symbols differ between 4.4 / 4.5 / 4.6 and how the
  framework handles fallback.

## Documentation

- [`docs/setup.md`](./docs/setup.md) — full project setup: `godot/`
  layout, `extension_api.json` pinning, version drift, multi-
  version targeting, build orchestration patterns.
- [`docs/threading.md`](./docs/threading.md) — goroutines and
  Godot's main thread; `RunOnMain` / `DrainMain` semantics.
- [`docs/lifecycle.md`](./docs/lifecycle.md) — refcounted resources
  (`Resource`, `Image`, `RegEx`, …) and the Go-finalizer cleanup
  path; when explicit `Unreference` is needed.
- [`docs/hot-reload.md`](./docs/hot-reload.md) — why hot-reload of
  Go extensions doesn't work, and what a productive edit-build-run
  loop looks like.

## Comparison with graphics.gd

[graphics.gd](https://github.com/quaadgras/graphics.gd) is the
sibling project. The architectural difference is who owns the
process:

- **godot-go**: Godot loads the extension as a c-shared library.
  Production ships a `.gdextension` package alongside your Godot
  project. Familiar deployment model; easy editor integration.
  Cost: no hot-reload (Go runtime can't be cleanly unloaded).
- **graphics.gd**: a Go binary loads `libgodot` itself. Production
  ships a Go executable that bundles the engine. Different
  deployment story; supports more flexible reload patterns.

Pick godot-go if you want the GDScript-style workflow — author in
Godot's editor, drop a c-shared library next to your project,
deploy through Godot's export pipeline. Pick graphics.gd if Go
should drive the process and Godot is more of a renderer/scene
library.

## Known limitations

- **Hot-reload doesn't work.** Go's runtime can't be cleanly
  unloaded, so editing the extension requires restarting Godot.
  See [`docs/hot-reload.md`](./docs/hot-reload.md) for the full
  explanation and the recommended dev loop.
- **PropertyHint coverage is partial.** The seven most-used hint
  values are wired (`RANGE`, `ENUM`, `FILE`, `DIR`,
  `MULTILINE_TEXT`, `PLACEHOLDER_TEXT`, plus default `NONE`).
  `LAYERS_*`, `RESOURCE_TYPE`, `COLOR_NO_ALPHA`, `FLAGS`, etc.
  aren't reachable from doctags yet.
- **Method-form `@property` doesn't accept export hints.** Hints
  are field-form only in the current release.
- **Method args/returns are restricted to primitives + user int
  enums.** `Vector2`, `Array`, engine-class pointers, etc. work
  fine when calling INTO Godot from your extension code, but you
  can't yet declare them as parameter types on a `@class` method
  that GDScript calls. See the
  [type-support reference](./docs/reference.md#supported-types).

## Contributing

Issues and pull requests welcome. Changes are easiest to review
when scoped to one feature or fix per PR.

A few conventions that keep the tree predictable:

- **Generated files (`*.gen.go`) are not committed.** The bindgen
  reproduces them from `extension_api.json` on every CI run.
- **CI runs the full matrix on every push** — three OS × three
  Godot versions = nine cells. A red cell blocks the merge; the
  workflow file is at
  [`.github/workflows/ci.yml`](./.github/workflows/ci.yml).
- **Examples are integration tests too.** Each `examples/*/` ships
  a `test_*.gd` script that exercises the extension end-to-end;
  CI runs them headlessly. New features should add an assertion
  in whichever example surfaces them.
- **Doctag changes need to land in the reference.** If you add or
  rename a tag, update [`docs/reference.md`](./docs/reference.md)
  in the same PR — that's the user-facing source of truth.

`task --list` shows the available development tasks. The most
common are `task generate:bindings`, `task build:example`, and
`task test`. `task clean` wipes generated artifacts when you want
a fresh state.
