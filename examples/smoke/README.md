# Smoke extension

A trivial GDExtension that proves the load path and the Phase 1 interface
table are alive.

## Build

```sh
task build:example
```

This produces `godot_project/bin/godot_go_smoke.dll` (or `.so` / `.dylib`).

## Run

1. Open `examples/smoke/godot_project/` in Godot 4.6.
2. Watch the Output dock. The extension prints two warnings:
   - at SCENE init: `godot-go: hello godot-go (Godot vMAJOR.MINOR.PATCH — <full version string>)`,
   - at SCENE deinit (closing the project): `godot-go: SCENE deinit, goodbye`.

The version is fetched through the host's `get_godot_version2` interface,
so seeing a real version string also confirms the function-pointer table
loaded correctly.
