#ifndef GODOT_GO_SHIM_H
#define GODOT_GO_SHIM_H

#include <stddef.h>
#include <stdint.h>
#include <stdlib.h>

#include "gdextension_interface.h"

/* Resolve a function pointer through the host's get_proc_address.
 * cgo can't call C function pointers directly, so this wraps the call. */
GDExtensionInterfaceFunctionPtr godot_go_resolve(GDExtensionInterfaceGetProcAddress p_get_proc_address,
                                                 const char *p_name);

/* Function-pointer accessors for the GDExtensionInitialization callback fields.
 * They forward into the Go-exported godotGoOnInitialize / godotGoOnDeinitialize
 * declarations in _cgo_export.h. */
GDExtensionInitializeCallback   godot_go_initialize_cb(void);
GDExtensionDeinitializeCallback godot_go_deinitialize_cb(void);

/* Typed call helpers — one per interface-fn signature we currently invoke from Go.
 * The remaining ~145 GDExtensionInterface* functions will be generated alongside
 * their typed Go wrappers by the bindgen tool in a later phase. */

void godot_go_call_get_godot_version2(GDExtensionInterfaceGetGodotVersion2 fn,
                                      GDExtensionGodotVersion2 *r_version);

void godot_go_call_print_error(GDExtensionInterfacePrintError fn,
                               const char *p_description, const char *p_function,
                               const char *p_file, int32_t p_line, GDExtensionBool p_editor_notify);

void godot_go_call_print_warning(GDExtensionInterfacePrintWarning fn,
                                 const char *p_description, const char *p_function,
                                 const char *p_file, int32_t p_line, GDExtensionBool p_editor_notify);

void godot_go_call_print_script_error(GDExtensionInterfacePrintScriptError fn,
                                      const char *p_description, const char *p_function,
                                      const char *p_file, int32_t p_line, GDExtensionBool p_editor_notify);

void *godot_go_call_mem_alloc2  (GDExtensionInterfaceMemAlloc2   fn, size_t p_bytes, GDExtensionBool p_pad_align);
void *godot_go_call_mem_realloc2(GDExtensionInterfaceMemRealloc2 fn, void *p_ptr, size_t p_bytes, GDExtensionBool p_pad_align);
void  godot_go_call_mem_free2   (GDExtensionInterfaceMemFree2    fn, void *p_ptr, GDExtensionBool p_pad_align);

/* Field accessors for GDExtensionGodotVersion2.
 * Avoids cgo's keyword-renaming around the `string` field. */
const char *godot_go_version2_status(const GDExtensionGodotVersion2 *v);
const char *godot_go_version2_build (const GDExtensionGodotVersion2 *v);
const char *godot_go_version2_hash  (const GDExtensionGodotVersion2 *v);
const char *godot_go_version2_string(const GDExtensionGodotVersion2 *v);

#endif /* GODOT_GO_SHIM_H */
