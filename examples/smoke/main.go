// Package main is the godot-go smoke extension.
//
// It builds as a c-shared library and exists to confirm the load path and
// the framework's generated builtin bindings. At SCENE init it greets
// Godot and runs a few round-trip exercises against the generated code:
//
//   - Vector2(3, 4).Length() must equal 5 (float arg + float return ABI)
//   - Vector2 → Variant → Vector2 round-trips byte-equal
//   - String "hello" → Length() must equal 5 (int return ABI)
//   - String "hello".Find("ll", 0) must equal 2 (int arg + int return ABI)
//   - StringNumInt64(1<<32 + 7) → "4294967303" stresses the int arg path
//     across the 32-bit boundary (truncation would yield "7" or "1")
//   - String("4294967303").ToInt() → 1<<32 + 7 stresses the int return
//     path the same way
//   - Array.Append three Variants → Size() must agree (Variant arg path)
//   - Engine.Singleton().GetVersionInfo()["major"] must equal 4 (engine-class
//     ABI: classdb singleton lookup + method bind + Dictionary return)
//   - util.Lerpf(0, 10, 0.5) must equal 5 (utility-function ABI: a free
//     ptrcall via variant_get_ptr_utility_function with three float args)
//   - enums.SideRight must equal 2 and enums.Ok must equal 0 (typed
//     global enums import cleanly from downstream packages)
package main

import (
	"github.com/legendary-code/godot-go/core"
	"github.com/legendary-code/godot-go/enums"
	"github.com/legendary-code/godot-go/internal/gdextension"
	"github.com/legendary-code/godot-go/internal/runtime"
	"github.com/legendary-code/godot-go/util"
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
	passed, failed := 0, 0
	check := func(label string, ok bool, got, want any) {
		if ok {
			passed++
			runtime.Printf("godot-go: PASS  %s = %v", label, got)
			return
		}
		failed++
		runtime.Printerrf("godot-go: FAIL  %s = %v (expected %v)", label, got, want)
	}

	// Vector2(3, 4).Length() exercises the PtrCall float ABI on both sides:
	// Godot encodes "float" as 8-byte double regardless of the engine's
	// real_t precision, so the generated bindings widen to float64 buffers
	// at the boundary even though the user-facing type is float32.
	pos := variant.NewVector2XY(3, 4)
	check("Vector2(3,4).Length()", pos.Length() == 5, pos.Length(), float32(5))

	// Vector2 → Variant → Vector2.
	slot := pos.ToVariant()
	defer slot.Destroy()
	recovered := variant.Vector2FromVariant(slot)
	rx, ry := recovered.X(), recovered.Y()
	check("Vector2 → Variant → Vector2",
		rx == 3 && ry == 4,
		[2]float32{rx, ry}, [2]float32{3, 4})

	// Transparent string boundary: NewStringFromString takes a Go string,
	// String.Length() returns int64 — the user never sees the opaque type.
	// Find exercises the int arg path (the from offset).
	greeting := variant.NewStringFromString("hello")
	defer greeting.Destroy()
	gotLen := greeting.Length()
	check("String(\"hello\").Length()", gotLen == 5, gotLen, int64(5))
	gotFind := greeting.Find("ll", 0)
	check("String(\"hello\").Find(\"ll\", 0)", gotFind == 2, gotFind, int64(2))

	// Stress the int ABI across the 32-bit boundary. 1<<32 + 7 has bits in
	// both halves of an int64, so any truncation to int32 along the call
	// path is visible in the result (low half alone would give 7, high
	// half alone would give 1).
	const big int64 = (int64(1) << 32) + 7
	const bigStr = "4294967303"
	gotNum := variant.StringNumInt64(big, 10, false)
	check("StringNumInt64(1<<32+7, 10, false)", gotNum == bigStr, gotNum, bigStr)

	bigGo := variant.NewStringFromString(bigStr)
	defer bigGo.Destroy()
	gotInt := bigGo.ToInt()
	check("String(\"4294967303\").ToInt()", gotInt == big, gotInt, big)

	// Array.Append exercises the Variant arg path; Size() exercises an
	// int return on a stateful builtin. Each Append takes a Variant whose
	// underlying slot is freed when the Array is destroyed.
	arr := variant.NewArray()
	defer arr.Destroy()
	for _, v := range []variant.Vector2{
		variant.NewVector2XY(1, 0),
		variant.NewVector2XY(0, 1),
		variant.NewVector2XY(1, 1),
	} {
		slot := v.ToVariant()
		arr.Append(*slot)
		slot.Destroy()
	}
	gotSize := arr.Size()
	check("Array.Append x3 → Size()", gotSize == 3, gotSize, int64(3))

	// Engine-class ABI proof: Engine singleton → get_version_info →
	// Dictionary["major"]. Touches classdb_construct/global_get_singleton,
	// classdb_get_method_bind, object_method_bind_ptrcall, and the
	// Variant<->int converter all in one call chain.
	eng := core.EngineSingleton()
	if eng == nil || eng.IsNil() {
		check("Engine singleton resolved", false, eng, "non-nil")
	} else {
		info := eng.GetVersionInfo()
		defer info.Destroy()
		key := variant.NewVariantString("major")
		defer key.Destroy()
		def := variant.NewVariantInt(-1)
		defer def.Destroy()
		got := info.Get(key, def)
		defer got.Destroy()
		major := got.AsInt()
		check("Engine.GetVersionInfo()[\"major\"]", major == 4, major, int64(4))
	}

	// Utility-function ABI: a free ptrcall through variant_get_ptr_utility_function
	// rather than the builtin-method or method-bind dispatchers. Lerpf(0, 10, 0.5)
	// pushes three float args and pulls one float back, all widened to/from
	// 8-byte slots at the boundary.
	gotLerp := util.Lerpf(0, 10, 0.5)
	check("util.Lerpf(0, 10, 0.5)", gotLerp == 5, gotLerp, float32(5))

	// Typed global enums: pure constants, no ABI surface — the only thing
	// to confirm is that the package imports and the values are sane.
	check("enums.SideRight == 2", int64(enums.SideRight) == 2, int64(enums.SideRight), int64(2))
	check("enums.Ok == 0", int64(enums.Ok) == 0, int64(enums.Ok), int64(0))

	// runtime.IsEditorHint() — wraps Engine.is_editor_hint(). In a
	// headless game-mode run (no --editor), the engine reports false.
	// Just confirms the helper resolves the singleton and ptrcalls
	// through cleanly; the real `true` branch only fires when running
	// inside the editor.
	check("runtime.IsEditorHint() == false (headless)",
		runtime.IsEditorHint() == false,
		runtime.IsEditorHint(), false)

	if failed == 0 {
		runtime.Printf("godot-go: smoke checks OK (%d/%d passed)", passed, passed+failed)
	} else {
		runtime.Printerrf("godot-go: smoke checks FAILED (%d/%d passed, %d failed)",
			passed, passed+failed, failed)
	}
}

func main() {}
