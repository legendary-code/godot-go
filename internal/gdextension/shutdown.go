package gdextension

import (
	"os"
	"runtime"
)

// Process exit hook for the c-shared DLL load model — Windows only.
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
// clean runs on Windows.
//
// **Why not on Linux/macOS.** Earlier versions of this hook fired
// on every OS and the doc claimed the call was "benign" elsewhere.
// CI evidence in 2026-04-28 disproved that for macOS specifically:
// extensions instantiating Node2D-derived classes hit the engine's
// static-destructor mutex during teardown, and skipping it via
// os.Exit raises an uncaught `system_error: mutex lock failed`
// that SIGABRTs the process AFTER our test code has already passed.
// Linux tolerates either path. Scoping the workaround to Windows
// keeps the race fix where it's needed and lets POSIX runners
// complete the engine's normal teardown — no observable race on
// either platform, and no post-test abort on macOS.
//
// If a future macOS or Linux scenario exposes a goroutine-thread
// race we're not currently seeing, this is the place to widen the
// scope back out (or add a per-OS variant).
func init() {
	if runtime.GOOS != "windows" {
		return
	}
	RegisterDeinitCallback(InitLevelCore, func() {
		os.Exit(0)
	})
}
