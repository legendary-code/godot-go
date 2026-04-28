package gdextension

/*
#include "shim.h"
*/
import "C"

import "unsafe"

// gdextension_library_init is the entry symbol declared in the .gdextension
// manifest. Godot calls this once when loading the library:
//
//   1. We populate the function-pointer table from the host's resolver.
//   2. We tell the host the minimum init level we need and hand back the
//      initialize/deinitialize trampolines that fan out to per-level
//      callbacks registered via Register{Init,Deinit}Callback.
//
// Returning 1 signals success; returning 0 aborts the load.
//
//export gdextension_library_init
func gdextension_library_init(
	pGetProcAddress C.GDExtensionInterfaceGetProcAddress,
	pLibrary C.GDExtensionClassLibraryPtr,
	rInit *C.GDExtensionInitialization,
) C.GDExtensionBool {
	loadInterface(pGetProcAddress, pLibrary)

	rInit.minimum_initialization_level = C.GDEXTENSION_INITIALIZATION_SCENE
	rInit.userdata = unsafe.Pointer(nil)
	rInit.initialize = C.godot_go_initialize_cb()
	rInit.deinitialize = C.godot_go_deinitialize_cb()

	return 1
}

//export godotGoOnInitialize
func godotGoOnInitialize(level C.GDExtensionInt) {
	// Godot's contract: per-level init fires on the engine's main
	// thread. CaptureMainThread is idempotent, so calling it on every
	// init level is fine — only the first call records the ID, and
	// each level gives us another chance to capture if (somehow) the
	// engine ever called us off-thread before.
	CaptureMainThread()
	runInitCallbacks(InitializationLevel(level))
}

//export godotGoOnDeinitialize
func godotGoOnDeinitialize(level C.GDExtensionInt) {
	runDeinitCallbacks(InitializationLevel(level))
}
