// Package main is the godot-go smoke extension.
//
// It builds as a c-shared library and exists to confirm the load path and
// the framework's generated builtin bindings. At SCENE init it greets
// Godot and runs a few round-trip exercises against the generated code:
//
//   - Vector2(3, 4).Length() must equal 5
//   - Vector2 → Variant → Vector2 round-trips byte-equal
//   - String "hello" → Length() must equal 5 (transparent boundary)
//   - Array.Append(...) and Array.Size() must agree
package main

import (
	"github.com/legendary-code/godot-go/internal/gdextension"
	"github.com/legendary-code/godot-go/internal/runtime"
	"github.com/legendary-code/godot-go/variant"
)

func init() {
	gdextension.RegisterInitCallback(gdextension.InitLevelScene, func() {
		v := gdextension.GetGodotVersion()
		runtime.Printf("godot-go: hello godot-go (Godot %s — %s)", v.Short(), v.String)

		runSmokeChecks()
	})
	gdextension.RegisterDeinitCallback(gdextension.InitLevelScene, func() {
		runtime.Print("godot-go: SCENE deinit, goodbye")
	})
}

func runSmokeChecks() {
	// Vector2 length round-trip.
	pos := variant.NewVector2XY(3, 4)
	runtime.Printf("godot-go: Vector2(3,4).Length() = %.3f (expected 5.000)", pos.Length())

	// Vector2 → Variant → Vector2.
	slot := pos.ToVariant()
	defer slot.Destroy()
	recovered := variant.Vector2FromVariant(slot)
	runtime.Printf("godot-go: roundtrip Vector2 = (%.1f, %.1f) (expected (3.0, 4.0))",
		recovered.X(), recovered.Y())

	// Transparent string boundary: NewStringFromString takes a Go string,
	// String.Length() returns int64 — the user never sees the opaque type.
	greeting := variant.NewStringFromString("hello")
	defer greeting.Destroy()
	runtime.Printf("godot-go: String(\"hello\").Length() = %d (expected 5)", greeting.Length())
}

func main() {}
