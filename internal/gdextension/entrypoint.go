package gdextension

/*
#cgo CFLAGS: -I${SRCDIR} -I${SRCDIR}/../../godot

#include "shim.h"
*/
import "C"

import "unsafe"

//export gdextension_library_init
func gdextension_library_init(
	pGetProcAddress C.GDExtensionInterfaceGetProcAddress,
	pLibrary C.GDExtensionClassLibraryPtr,
	rInit *C.GDExtensionInitialization,
) C.GDExtensionBool {
	C.godot_go_shim_set_globals(pGetProcAddress, pLibrary)
	C.godot_go_shim_resolve_print_warning()

	rInit.minimum_initialization_level = C.GDEXTENSION_INITIALIZATION_SCENE
	rInit.userdata = unsafe.Pointer(nil)
	rInit.initialize = C.godot_go_shim_initialize_cb()
	rInit.deinitialize = C.godot_go_shim_deinitialize_cb()

	C.godot_go_shim_print_warning(C.CString("godot-go: library_init complete"))
	return 1
}

//export godotGoOnInitialize
func godotGoOnInitialize(level C.GDExtensionInt) {
	if int32(level) == int32(C.GDEXTENSION_INITIALIZATION_SCENE) {
		C.godot_go_shim_print_warning(C.CString("godot-go: hello godot-go (SCENE init)"))
	}
}

//export godotGoOnDeinitialize
func godotGoOnDeinitialize(level C.GDExtensionInt) {
	if int32(level) == int32(C.GDEXTENSION_INITIALIZATION_SCENE) {
		C.godot_go_shim_print_warning(C.CString("godot-go: SCENE deinit"))
	}
}
