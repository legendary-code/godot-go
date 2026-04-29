# Project setup

This walkthrough covers setting up a new godot-go project from
scratch — picking a Godot version, generating the bindings package,
and wiring the build so contributors get a working tree from a
clean checkout.

## Prerequisites

- **Go 1.26+** (matches `go.mod`).
- **Godot 4.4 or newer** to load the resulting extension.
- A **C toolchain reachable by cgo**: MinGW-w64 on Windows, the
  standard clang/gcc on macOS / Linux.

## The bindings package

The framework doesn't ship pre-generated bindings. Each project
generates its own from the Godot version it targets — pick a
release, dump its API JSON, and run the bindgen against it. The
generator emits one flat package (engine classes, builtins,
enums, utility functions, native structures) plus a
sibling `runtime/` sub-package for user-facing conveniences.

### One-time setup

Inside your project, create a `godot/` directory with two files:

```
godot/
├── extension_api.json     # the API dump for your target Godot version
└── generate_bindings.go   # //go:generate trigger for the bindgen
```

`generate_bindings.go`:

```go
// Package godot holds the Godot bindings generated from
// extension_api.json. The //go:generate line below stamps out the
// *.gen.go files alongside this one, plus a runtime/ sub-package
// next door.
package godot

//go:generate go run github.com/legendary-code/godot-go/cmd/godot-go-bindgen -api ./extension_api.json
```

The package name is whatever you put on the `package` line — the
bindgen reads it from `$GOPACKAGE` when run via `go generate`. Most
projects use `godot`; pick something else if you have a strong
reason to.

### Producing extension_api.json

Run your target Godot binary once to dump the JSON:

```sh
godot --headless --dump-extension-api --dump-gdextension-interface
```

That writes `extension_api.json` (and `gdextension_interface.h`,
which the framework ignores — it ships a pinned 4.6 header
internally). Copy the JSON into your project's `godot/` dir.

If you don't have older Godot binaries handy, the
[godot-cpp](https://github.com/godotengine/godot-cpp) repo keeps a
`gdextension/extension_api.json` on each version branch — `4.4`,
`4.5`, `4.6`, etc. The framework's own `godot/extension_api_*.json`
files came from those branches.

For multi-version targeting, name the files and pick at generate
time:

```
godot/
├── extension_api_4_4.json
├── extension_api_4_5.json
├── extension_api_4_6.json
└── generate_bindings.go   #   -api ./extension_api_4_6.json
```

The bindgen accepts any path via the `-api` flag, so swapping
between versions is a single argument change. The framework's CI
runs the full matrix — three OS × three Godot versions — by
invoking the bindgen directly per cell rather than going through
`go generate`.

### Generating

```sh
go generate ./godot/...
```

This produces ~1100 `*.gen.go` files in `godot/` and a few in
`godot/runtime/`. They're fully reproducible from
`extension_api.json`, so it's up to you whether to commit them or
gitignore them — neither breaks anything. The trade-off is the
usual one for generated code: committing keeps fresh clones
buildable without a `go generate` step (and makes the diff visible
in code review), while gitignoring keeps the repo small. If you
gitignore, the patterns are:

```
godot/**/*.gen.go
godot/runtime/
```

Either way, commit the inputs: `extension_api.json` (the version
pin) and `generate_bindings.go` (the trigger). The framework's own
repo gitignores the generated files; the example projects in
`examples/*/` commit their `_bindings.go` (different generator,
different convention).

### Version drift

The bindgen records the Godot version it generated against in a
`version.gen.go` inside your bindings package. The framework's
runtime checks this against the live Godot instance on first
extension callback and warns to the engine's Output dock if the
major.minor doesn't match. Patch differences (4.6.1 vs 4.6.2) are
tolerated — Godot's extension ABI is patch-stable.

To bump versions: drop in a newer `extension_api.json`, re-run
`go generate ./godot/...`, rebuild. No framework changes required.

## Authoring an extension

Once the bindings are in place, your extension class is plain Go:

```go
package main

//go:generate godot-go

import (
    "github.com/yourname/yourproject/godot"
    "github.com/yourname/yourproject/godot/runtime"
)

// @class
type Player struct {
    // @extends
    godot.Node

    // @property
    // @export_range(0, 100)
    Health int64
}

func (p *Player) Hello() {
    runtime.Print("hello from Player")
}
```

`go generate ./...` runs the user-class codegen, which produces
`<file>_bindings.go` with the cgo trampolines, ClassDB
registrations, and synthesized accessors / emit methods.

```sh
go install github.com/legendary-code/godot-go/cmd/godot-go
go generate ./...
go build -buildmode=c-shared -o yourext.dll ./yourpkg
```

Drop the resulting `.dll` / `.so` / `.dylib` plus a `.gdextension`
config into your Godot project's filesystem and the engine picks
it up on next load.

## Build orchestration

A typical Taskfile or Makefile target chains the steps so
`task build` does the right thing from a clean checkout:

```yaml
# Taskfile.yaml
tasks:
  generate:bindings:
    cmds:
      - go generate ./godot/...
    sources:
      - godot/extension_api.json
    generates:
      - godot/*.gen.go
      - godot/runtime/runtime.gen.go

  build:
    deps: [generate:bindings]
    cmds:
      - go install github.com/legendary-code/godot-go/cmd/godot-go
      - go generate ./yourpkg/...
      - go build -buildmode=c-shared -o bin/yourext.dll ./yourpkg
```

The framework's own Taskfile follows this pattern — see the
top-level `Taskfile.yaml` for a working example.
