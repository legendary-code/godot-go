package gdextension

import "os"

// Process exit hook for the c-shared DLL load model.
//
// Godot loads our extension as a c-shared DLL; the host process is
// godot.exe, and Go's runtime — sysmon, per-P scheduler workers, the
// concurrent GC — runs inside that process for the lifetime of the
// extension. After Godot's per-level deinitialize callbacks return,
// the engine continues winding down (XR teardown, classdb teardown,
// singleton release, then DLL unload via DLL_PROCESS_DETACH) while
// Go's auxiliary OS threads are still active. On Windows the two
// teardowns race, and a Go thread eventually touches memory the OS
// just unmapped — segfault.
//
// `runtime.GOMAXPROCS(1) + debug.SetGCPercent(-1)` at Scene deinit
// reduces the race window enough to pass casually but still flakes
// at ~2–4% across 50-run batches; the underlying Go runtime is
// fundamentally not designed to be cleanly torn down by an external
// host. graphics.gd avoids the problem architecturally — its
// production deployment runs Go as the host process and embeds
// libgodot.dll, sidestepping DLL_PROCESS_DETACH entirely. We're
// committed to the load-by-Godot model, so the only fully reliable
// option within it is to take ownership of process exit ourselves.
//
// Calling os.Exit(0) from Core-level deinit (the very last callback
// the engine fires) terminates the process via Windows ExitProcess
// after every user-visible cleanup has run. We skip Godot's final
// engine teardown — XR cleanup, static C++ destructors, atexit
// handlers — but at this point the scene tree is gone, all servers
// are stopped, and every extension class is unregistered. Anything
// observable to the user has already happened. Verified 100/100
// clean runs.
//
// On Linux and macOS the same call is benign: those platforms don't
// race the unload path, but skipping the last microseconds of engine
// teardown costs nothing the user can see.
func init() {
	RegisterDeinitCallback(InitLevelCore, func() {
		os.Exit(0)
	})
}
