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
//   - godot.Lerpf(0, 10, 0.5) must equal 5 (utility-function ABI: a free
//     ptrcall via variant_get_ptr_utility_function with three float args)
//   - godot.SideRight must equal 2 and godot.Ok must equal 0 (typed
//     global enums import cleanly from downstream packages)
package main

import (
	"sync"
	goruntime "runtime"

	"github.com/legendary-code/godot-go/gdextension"
	"github.com/legendary-code/godot-go/godot"
	"github.com/legendary-code/godot-go/godot/runtime"
)

func init() {
	gdextension.RegisterInitCallback(gdextension.InitLevelScene, func() {
		v := gdextension.GetGodotVersion()
		runtime.PrintInfof("godot-go: hello godot-go (Godot %s — %s)", v.Short(), v.String)

		runSmokeChecks()
	})
	gdextension.RegisterDeinitCallback(gdextension.InitLevelScene, func() {
		runtime.PrintInfo("godot-go: SCENE deinit, goodbye")
	})
}

func runSmokeChecks() {
	passed, failed := 0, 0
	check := func(label string, ok bool, got, want any) {
		if ok {
			passed++
			runtime.PrintInfof("godot-go: PASS  %s = %v", label, got)
			return
		}
		failed++
		runtime.PrintErrorf("godot-go: FAIL  %s = %v (expected %v)", label, got, want)
	}

	// Vector2(3, 4).Length() exercises the PtrCall float ABI on both sides:
	// Godot encodes "float" as 8-byte double regardless of the engine's
	// real_t precision, so the generated bindings widen to float64 buffers
	// at the boundary even though the user-facing type is float32.
	pos := godot.NewVector2XY(3, 4)
	check("Vector2(3,4).Length()", pos.Length() == 5, pos.Length(), float32(5))

	// Vector2 → Variant → Vector2.
	slot := pos.ToVariant()
	defer slot.Destroy()
	recovered := godot.Vector2FromVariant(slot)
	rx, ry := recovered.X(), recovered.Y()
	check("Vector2 → Variant → Vector2",
		rx == 3 && ry == 4,
		[2]float32{rx, ry}, [2]float32{3, 4})

	// Transparent string boundary: NewStringFromString takes a Go string,
	// String.Length() returns int64 — the user never sees the opaque type.
	// Find exercises the int arg path (the from offset).
	greeting := godot.NewStringFromString("hello")
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
	gotNum := godot.StringNumInt64(big, 10, false)
	check("StringNumInt64(1<<32+7, 10, false)", gotNum == bigStr, gotNum, bigStr)

	bigGo := godot.NewStringFromString(bigStr)
	defer bigGo.Destroy()
	gotInt := bigGo.ToInt()
	check("String(\"4294967303\").ToInt()", gotInt == big, gotInt, big)

	// Array.Append exercises the Variant arg path; Size() exercises an
	// int return on a stateful builtin. Each Append takes a Variant whose
	// underlying slot is freed when the Array is destroyed.
	arr := godot.NewArray()
	defer arr.Destroy()
	for _, v := range []godot.Vector2{
		godot.NewVector2XY(1, 0),
		godot.NewVector2XY(0, 1),
		godot.NewVector2XY(1, 1),
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
	eng := godot.EngineSingleton()
	if eng == nil || eng.IsNil() {
		check("Engine singleton resolved", false, eng, "non-nil")
	} else {
		info := eng.GetVersionInfo()
		defer info.Destroy()
		key := godot.NewVariantString("major")
		defer key.Destroy()
		def := godot.NewVariantInt(-1)
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
	gotLerp := godot.Lerpf(0, 10, 0.5)
	check("godot.Lerpf(0, 10, 0.5)", gotLerp == 5, gotLerp, float32(5))

	// Typed global enums: pure constants, no ABI surface — the only thing
	// to confirm is that the package imports and the values are sane.
	check("godot.SideRight == 2", int64(godot.SideRight) == 2, int64(godot.SideRight), int64(2))
	check("godot.Ok == 0", int64(godot.Ok) == 0, int64(godot.Ok), int64(0))

	// runtime.IsEditorHint() — wraps Engine.is_editor_hint(). The
	// returned bool depends on run mode (true under --editor, false in
	// a deployed game build), and the smoke fires from SCENE init in
	// both, so the assertion can't pin a value. Confirm the helper
	// resolves the Engine singleton and ptrcalls through cleanly by
	// reaching it twice and matching against Engine.is_editor_hint()
	// straight from the bindings — same code path either way, so
	// agreement is the test.
	hint := runtime.IsEditorHint()
	hintViaEngine := godot.EngineSingleton().IsEditorHint()
	check("runtime.IsEditorHint() agrees with Engine.is_editor_hint()",
		hint == hintViaEngine, hint, hintViaEngine)

	// IsMainThread / RunOnMain / DrainMain — the goroutine ↔ main-
	// thread bridge. We're inside SCENE init, which Godot calls on the
	// engine main thread, so IsMainThread must be true here.
	check("runtime.IsMainThread() during SCENE init",
		runtime.IsMainThread(), runtime.IsMainThread(), true)

	// Inline-when-on-main: RunOnMain from main runs synchronously.
	// `inlineRan` flips before RunOnMain returns.
	inlineRan := false
	runtime.RunOnMain(func() { inlineRan = true })
	check("RunOnMain inlines when called from main", inlineRan, inlineRan, true)

	// Off-thread post: a goroutine queues a func; we wait via the
	// WaitGroup so the post completes before we drain. The goroutine's
	// IsMainThread must be false (worker thread), and the queued func
	// must run on this thread (the engine main thread) at DrainMain
	// time, observably setting `posted` to true.
	var posted bool
	var wg sync.WaitGroup
	var goroutineSawMain bool
	wg.Add(1)
	go func() {
		defer wg.Done()
		goroutineSawMain = runtime.IsMainThread()
		runtime.RunOnMain(func() { posted = true })
	}()
	wg.Wait()
	check("goroutine.IsMainThread() == false",
		goroutineSawMain == false, goroutineSawMain, false)
	check("posted == false before drain",
		posted == false, posted, false)
	runtime.DrainMain()
	check("posted == true after drain",
		posted == true, posted, true)

	// Refcounted lifecycle: RegEx is a RefCounted-derived class, so
	// the bindgen-emitted RegExFromPtr attached a Go finalizer that
	// will drop the engine reference once the GC marks the wrapper.
	// Exercise the explicit ref/unref path first, then the finalizer
	// path. We don't observe the final destruction (the wrapper is
	// gone by then), but we do confirm that draining queued
	// finalizers doesn't crash the process.
	r := godot.RegExCreateFromString("h.llo", false)
	check("RegEx refcount after construction == 1",
		r.GetReferenceCount() == 1, r.GetReferenceCount(), int64(1))
	r.Reference()
	check("RegEx refcount after explicit Reference() == 2",
		r.GetReferenceCount() == 2, r.GetReferenceCount(), int64(2))
	r.Unreference()
	check("RegEx refcount after explicit Unreference() == 1",
		r.GetReferenceCount() == 1, r.GetReferenceCount(), int64(1))

	// Leak smoke: allocate a bunch of RegEx wrappers, drop them,
	// then force GC + drain. Each finalizer queues an Unreference
	// onto the main-thread queue; after DrainMain they execute.
	// If finalizers were broken or the bounce didn't work, this
	// would either deadlock or leak; we'd see it as a stuck or
	// memory-bloated test.
	for i := 0; i < 100; i++ {
		_ = godot.RegExCreateFromString("loop", false)
	}
	goruntime.GC()
	goruntime.GC()
	runtime.DrainMain()
	check("refcount leak smoke (100 RegEx alloc/drop) — main loop survives", true, true, true)

	if failed == 0 {
		runtime.PrintInfof("godot-go: smoke checks OK (%d/%d passed)", passed, passed+failed)
	} else {
		runtime.PrintErrorf("godot-go: smoke checks FAILED (%d/%d passed, %d failed)",
			passed, passed+failed, failed)
	}
}

func main() {}
