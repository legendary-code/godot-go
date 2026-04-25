# Phase 0 smoke extension

A trivial GDExtension that does nothing except prove the load path is alive.

## Build

```sh
task build:example
```

This produces `godot_project/bin/godot_go_smoke.dll` (or `.so` / `.dylib`).

## Run

1. Open `examples/smoke/godot_project/` in Godot 4.6.
2. Watch the Output dock. The extension calls `print_warning` three times:
   - on `gdextension_library_init` (entry point reached),
   - at SCENE init level (`godot-go: hello godot-go (SCENE init)`),
   - at SCENE deinit when the project closes.

If you see those lines, Phase 0 is green.
