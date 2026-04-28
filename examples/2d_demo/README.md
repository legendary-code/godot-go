# 2d_demo

A small Node2D extension that oscillates back and forth along the X
axis. Driven by `Process(delta)`, configurable through inspector
properties, and emits a signal at each bound.

This is the canonical "see how a real game-like extension comes
together" reference — it exercises the engine virtual `_process`
path under load (60 calls per second), demonstrates inspector
integration via `@export_range` on the configurable knobs, and
shows how user code emits framework signals.

## What's in `mover.go`

- **`Mover`** — extends `core.Node2D`, has two inspector-tweakable
  properties (`Speed`, `Range`) under a "Motion" group, plus
  internal state for the current direction and origin point.
- **`Signals`** interface tagged `@signals` with one signal,
  `Bounced(direction int64)`. Codegen synthesizes a typed
  `func (m *Mover) Bounced(direction int64)` that emits the signal.
- **`Process(delta)`** — `@override` that drives motion: moves the
  node's `Position`, clamps at the bounds, flips direction and emits
  `bounced` when reversing.
- **`Reset()`** — a regular ClassDB method exposed to GDScript as
  `reset()`. Returns the node to its starting position.

## Build

```sh
task build:2d_demo
```

Produces `godot_project/bin/godot_go_2d_demo.dll` (or `.so`/`.dylib`).

## First-time setup

The first time you build, import the project so Godot picks up the
`.gdextension`:

```sh
godot --headless --editor --path godot_project --quit-after 5
```

## Headless verification

```sh
godot --headless --path godot_project --script test_mover.gd
```

The driver instantiates `Mover.new()`, drives `_process(delta)`
manually with known deltas, and asserts the position evolves the
way the math says it should — including bounces emitting the
`bounced` signal. Expect `test_mover: ALL CHECKS PASSED`.

## Visual demo (opening in the editor)

The headless test verifies behavior without a scene. To see the
mover actually move on screen:

1. Open `godot_project/` in the Godot 4.6 editor.
2. Create a new scene with a `Mover` root (the type appears in the
   "Add Node" dialog under Node2D).
3. Add a `Sprite2D` child and assign any texture (the Godot icon at
   `res://icon.svg` works if you drop one in).
4. Set the Mover's `Speed` and `Range` in the inspector under the
   "Motion" group.
5. Press F6 (Play This Scene) — the sprite oscillates back and
   forth and the Output dock shows `bounced` signals if you
   connect to them.
