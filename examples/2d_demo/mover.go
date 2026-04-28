package main

//go:generate godot-go

import (
	"github.com/legendary-code/godot-go/core"
	"github.com/legendary-code/godot-go/variant"
)

// Mover oscillates its position along the X axis between -Range and
// +Range relative to its starting point, moving at Speed pixels per
// second. At each bound it reverses direction and emits the
// `bounced` signal so listeners can react.
//
// The class extends Node2D so position lives directly on the node
// itself — drop a Mover into your scene, attach a Sprite2D (or any
// other CanvasItem) as a child for visual feedback, set Speed and
// Range in the inspector, and the framework's Process override drives
// the rest.
//
// @class
type Mover struct {
	// @extends
	core.Node2D

	// @group("Motion")
	// @export_range(0, 500, 10)
	// @property
	Speed float64

	// @group("Motion")
	// @export_range(0, 1000, 10)
	// @property
	Range float64

	// direction is the current sign of motion: +1 rightward, -1
	// leftward. Lowercase = invisible to Godot. Lazily initialized to
	// +1 on the first Process call so the user doesn't need a
	// constructor hook.
	direction float32

	// origin is the X position the mover should oscillate around — set
	// to whatever the node's X is at first Process. Lets the user place
	// the Mover wherever and have it oscillate around that location
	// rather than around the world origin.
	origin       float32
	originSeeded bool
}

// @signals declares the engine-visible signal contract. Codegen
// synthesizes a typed Go-side `Bounced(direction int64)` method on
// *Mover that the Process loop calls below; GDScript subscribers can
// `n.bounced.connect(callable)` to react.
//
// @signals
type Signals interface {
	// Bounced fires every time the mover hits a bound and reverses.
	// The argument is the new direction (+1 or -1).
	Bounced(direction int64)
}

// Process is the per-frame motion driver. Engine virtual override
// (registered as `_process`); Godot calls it on every live Mover
// once per frame with the elapsed delta.
//
// @override
func (m *Mover) Process(delta float64) {
	pos := m.GetPosition()
	if !m.originSeeded {
		m.origin = pos.X()
		m.direction = 1
		m.originSeeded = true
	}

	step := m.direction * float32(m.Speed*delta)
	newX := pos.X() + step

	// Clamp to bounds and flip direction at each. Use >= / <= so
	// landing exactly on the bound counts as a bounce — there's no
	// further travel possible in the current direction, and waiting
	// one extra tick to reverse leaves the mover sitting at the
	// bound for a frame. Two separate branches because a fast tick
	// could otherwise overshoot one bound and then ping-pong
	// incorrectly on the next frame.
	rng := float32(m.Range)
	switch {
	case newX >= m.origin+rng:
		newX = m.origin + rng
		m.direction = -1
		m.Bounced(int64(m.direction))
	case newX <= m.origin-rng:
		newX = m.origin - rng
		m.direction = 1
		m.Bounced(int64(m.direction))
	}

	pos.SetX(newX)
	m.SetPosition(pos)
}

// Reset returns the mover to its starting position and reseeds the
// origin to the node's current location. Exposed to GDScript as
// `reset()` — useful to call from a button or input handler.
func (m *Mover) Reset() {
	// GetPosition returns a Vector2 value; Vector2's accessor methods
	// are pointer-receiver (host ABI requires an addressable slot to
	// pass as `self`), so capture into a local before reading Y.
	cur := m.GetPosition()
	pos := variant.NewVector2XY(m.origin, cur.Y())
	m.SetPosition(pos)
	m.direction = 1
}
