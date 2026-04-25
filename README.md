# godot-go

An intuitive API for implementing GDExtensions in go.

See [`PLAN.md`](./PLAN.md) for the design intent and
[`PLAN_EXECUTION.md`](./PLAN_EXECUTION.md) for the implementation roadmap.

## Status

Phase 0 — bootstrap. The repository builds an empty c-shared library that
Godot can load and that prints from `gdextension_library_init`. See
[`examples/smoke/`](./examples/smoke/) for the smoke test.

## Prerequisites

- Go 1.26.1
- A C toolchain reachable by cgo (MinGW-W64 on Windows; clang/gcc elsewhere)
- [`go-task`](https://taskfile.dev) v3 for the build orchestration
- Godot 4.6 to load the resulting extension

## Common commands

```sh
task                # list all targets
task build:example  # build the Phase 0 smoke c-shared library
task test           # run the Go test suite
```

## Build configuration

godot-go targets the **single-precision** Godot build by default
(`extension_api.json`'s `float_32` configuration). To target a
double-precision Godot build, add `-tags godot_double_precision` to your
`go build` invocation. Mixed-precision binaries are not supported — the
host Godot must match the build tag.
