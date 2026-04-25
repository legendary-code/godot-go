#include "shim.h"
#include "_cgo_export.h"

GDExtensionInterfaceFunctionPtr godot_go_resolve(GDExtensionInterfaceGetProcAddress p_get_proc_address,
                                                 const char *p_name) {
    if (p_get_proc_address == NULL) {
        return NULL;
    }
    return p_get_proc_address(p_name);
}

static void godot_go_initialize_trampoline(void *p_userdata, GDExtensionInitializationLevel p_level) {
    (void)p_userdata;
    godotGoOnInitialize((GDExtensionInt)p_level);
}

static void godot_go_deinitialize_trampoline(void *p_userdata, GDExtensionInitializationLevel p_level) {
    (void)p_userdata;
    godotGoOnDeinitialize((GDExtensionInt)p_level);
}

GDExtensionInitializeCallback godot_go_initialize_cb(void) {
    return godot_go_initialize_trampoline;
}

GDExtensionDeinitializeCallback godot_go_deinitialize_cb(void) {
    return godot_go_deinitialize_trampoline;
}

void godot_go_call_get_godot_version2(GDExtensionInterfaceGetGodotVersion2 fn,
                                      GDExtensionGodotVersion2 *r_version) {
    fn(r_version);
}

void godot_go_call_print_error(GDExtensionInterfacePrintError fn,
                               const char *p_description, const char *p_function,
                               const char *p_file, int32_t p_line, GDExtensionBool p_editor_notify) {
    fn(p_description, p_function, p_file, p_line, p_editor_notify);
}

void godot_go_call_print_warning(GDExtensionInterfacePrintWarning fn,
                                 const char *p_description, const char *p_function,
                                 const char *p_file, int32_t p_line, GDExtensionBool p_editor_notify) {
    fn(p_description, p_function, p_file, p_line, p_editor_notify);
}

void godot_go_call_print_script_error(GDExtensionInterfacePrintScriptError fn,
                                      const char *p_description, const char *p_function,
                                      const char *p_file, int32_t p_line, GDExtensionBool p_editor_notify) {
    fn(p_description, p_function, p_file, p_line, p_editor_notify);
}

void *godot_go_call_mem_alloc2(GDExtensionInterfaceMemAlloc2 fn, size_t p_bytes, GDExtensionBool p_pad_align) {
    return fn(p_bytes, p_pad_align);
}

void *godot_go_call_mem_realloc2(GDExtensionInterfaceMemRealloc2 fn, void *p_ptr, size_t p_bytes, GDExtensionBool p_pad_align) {
    return fn(p_ptr, p_bytes, p_pad_align);
}

void godot_go_call_mem_free2(GDExtensionInterfaceMemFree2 fn, void *p_ptr, GDExtensionBool p_pad_align) {
    fn(p_ptr, p_pad_align);
}

const char *godot_go_version2_status(const GDExtensionGodotVersion2 *v) { return v->status; }
const char *godot_go_version2_build (const GDExtensionGodotVersion2 *v) { return v->build;  }
const char *godot_go_version2_hash  (const GDExtensionGodotVersion2 *v) { return v->hash;   }
const char *godot_go_version2_string(const GDExtensionGodotVersion2 *v) { return v->string; }
