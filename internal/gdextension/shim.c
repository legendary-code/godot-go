#include "shim.h"
#include "_cgo_export.h"

#include <stdio.h>

static GDExtensionInterfaceGetProcAddress g_get_proc_address = NULL;
static GDExtensionClassLibraryPtr         g_library          = NULL;
static GDExtensionInterfacePrintWarning   g_print_warning    = NULL;

void godot_go_shim_set_globals(GDExtensionInterfaceGetProcAddress p_get_proc_address,
                               GDExtensionClassLibraryPtr p_library) {
    g_get_proc_address = p_get_proc_address;
    g_library          = p_library;
}

void godot_go_shim_resolve_print_warning(void) {
    if (g_get_proc_address == NULL) {
        return;
    }
    g_print_warning = (GDExtensionInterfacePrintWarning)
        (void *)g_get_proc_address("print_warning");
}

void godot_go_shim_print_warning(const char *p_message) {
    if (g_print_warning != NULL) {
        g_print_warning(p_message, "godot-go", __FILE__, __LINE__, 0);
    } else {
        fprintf(stderr, "godot-go: %s\n", p_message);
    }
}

/* C-side trampolines that Godot will invoke at each init/deinit level.
 * They forward into Go via the cgo-generated _cgo_export.h declarations. */
static void godot_go_initialize_trampoline(void *p_userdata, GDExtensionInitializationLevel p_level) {
    (void)p_userdata;
    godotGoOnInitialize((GDExtensionInt)p_level);
}

static void godot_go_deinitialize_trampoline(void *p_userdata, GDExtensionInitializationLevel p_level) {
    (void)p_userdata;
    godotGoOnDeinitialize((GDExtensionInt)p_level);
}

GDExtensionInitializeCallback godot_go_shim_initialize_cb(void) {
    return godot_go_initialize_trampoline;
}

GDExtensionDeinitializeCallback godot_go_shim_deinitialize_cb(void) {
    return godot_go_deinitialize_trampoline;
}
