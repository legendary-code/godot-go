#ifndef GODOT_GO_SHIM_H
#define GODOT_GO_SHIM_H

#include "gdextension_interface.h"

/* Stash the proc-address resolver and library handle from the entry point. */
void godot_go_shim_set_globals(GDExtensionInterfaceGetProcAddress p_get_proc_address,
                               GDExtensionClassLibraryPtr p_library);

/* Resolve and cache print_warning so the runtime can log without re-resolving. */
void godot_go_shim_resolve_print_warning(void);

/* Log a NUL-terminated message via Godot's print_warning interface.
 * Falls back to fprintf(stderr,...) until the proc has been resolved. */
void godot_go_shim_print_warning(const char *p_message);

/* Function-pointer accessors so Go can fill GDExtensionInitialization
 * without having to express function-pointer casts directly. */
GDExtensionInitializeCallback   godot_go_shim_initialize_cb(void);
GDExtensionDeinitializeCallback godot_go_shim_deinitialize_cb(void);

#endif /* GODOT_GO_SHIM_H */
