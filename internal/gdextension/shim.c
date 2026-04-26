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

void godot_go_call_variant_new_copy(GDExtensionInterfaceVariantNewCopy fn,
                                    GDExtensionUninitializedVariantPtr r_dest,
                                    GDExtensionConstVariantPtr p_src) {
    fn(r_dest, p_src);
}

void godot_go_call_variant_destroy(GDExtensionInterfaceVariantDestroy fn,
                                   GDExtensionVariantPtr p_self) {
    fn(p_self);
}

GDExtensionVariantType godot_go_call_variant_get_type(GDExtensionInterfaceVariantGetType fn,
                                                      GDExtensionConstVariantPtr p_self) {
    return fn(p_self);
}

GDExtensionInt godot_go_call_string_new_with_utf8_chars_and_len2(GDExtensionInterfaceStringNewWithUtf8CharsAndLen2 fn,
                                                                 GDExtensionUninitializedStringPtr r_dest,
                                                                 const char *p_contents, GDExtensionInt p_size) {
    return fn(r_dest, p_contents, p_size);
}

GDExtensionInt godot_go_call_string_to_utf8_chars(GDExtensionInterfaceStringToUtf8Chars fn,
                                                  GDExtensionConstStringPtr p_self,
                                                  char *r_text, GDExtensionInt p_max_write_length) {
    return fn(p_self, r_text, p_max_write_length);
}

void godot_go_call_string_name_new_with_utf8_chars_and_len(GDExtensionInterfaceStringNameNewWithUtf8CharsAndLen fn,
                                                           GDExtensionUninitializedStringNamePtr r_dest,
                                                           const char *p_contents, GDExtensionInt p_size) {
    fn(r_dest, p_contents, p_size);
}

godot_go_anyfn godot_go_call_get_variant_from_type_constructor(GDExtensionInterfaceGetVariantFromTypeConstructor fn,
                                                               GDExtensionVariantType p_type) {
    return (godot_go_anyfn)fn(p_type);
}

godot_go_anyfn godot_go_call_get_variant_to_type_constructor(GDExtensionInterfaceGetVariantToTypeConstructor fn,
                                                             GDExtensionVariantType p_type) {
    return (godot_go_anyfn)fn(p_type);
}

godot_go_anyfn godot_go_call_variant_get_ptr_constructor(GDExtensionInterfaceVariantGetPtrConstructor fn,
                                                         GDExtensionVariantType p_type, int32_t p_constructor) {
    return (godot_go_anyfn)fn(p_type, p_constructor);
}

godot_go_anyfn godot_go_call_variant_get_ptr_destructor(GDExtensionInterfaceVariantGetPtrDestructor fn,
                                                        GDExtensionVariantType p_type) {
    return (godot_go_anyfn)fn(p_type);
}

godot_go_anyfn godot_go_call_variant_get_ptr_builtin_method(GDExtensionInterfaceVariantGetPtrBuiltinMethod fn,
                                                            GDExtensionVariantType p_type,
                                                            GDExtensionConstStringNamePtr p_method,
                                                            GDExtensionInt p_hash) {
    return (godot_go_anyfn)fn(p_type, p_method, p_hash);
}

godot_go_anyfn godot_go_call_variant_get_ptr_operator_evaluator(GDExtensionInterfaceVariantGetPtrOperatorEvaluator fn,
                                                                GDExtensionVariantOperator p_op,
                                                                GDExtensionVariantType p_type_a,
                                                                GDExtensionVariantType p_type_b) {
    return (godot_go_anyfn)fn(p_op, p_type_a, p_type_b);
}

godot_go_anyfn godot_go_call_variant_get_ptr_indexed_setter(GDExtensionInterfaceVariantGetPtrIndexedSetter fn,
                                                            GDExtensionVariantType p_type) {
    return (godot_go_anyfn)fn(p_type);
}

godot_go_anyfn godot_go_call_variant_get_ptr_indexed_getter(GDExtensionInterfaceVariantGetPtrIndexedGetter fn,
                                                            GDExtensionVariantType p_type) {
    return (godot_go_anyfn)fn(p_type);
}

godot_go_anyfn godot_go_call_variant_get_ptr_keyed_setter(GDExtensionInterfaceVariantGetPtrKeyedSetter fn,
                                                          GDExtensionVariantType p_type) {
    return (godot_go_anyfn)fn(p_type);
}

godot_go_anyfn godot_go_call_variant_get_ptr_keyed_getter(GDExtensionInterfaceVariantGetPtrKeyedGetter fn,
                                                          GDExtensionVariantType p_type) {
    return (godot_go_anyfn)fn(p_type);
}

godot_go_anyfn godot_go_call_variant_get_ptr_utility_function(GDExtensionInterfaceVariantGetPtrUtilityFunction fn,
                                                              GDExtensionConstStringNamePtr p_function,
                                                              GDExtensionInt p_hash) {
    return (godot_go_anyfn)fn(p_function, p_hash);
}

void godot_go_call_variant_from_type(godot_go_anyfn fn,
                                     GDExtensionUninitializedVariantPtr r_dest,
                                     GDExtensionTypePtr p_src) {
    ((GDExtensionVariantFromTypeConstructorFunc)fn)(r_dest, p_src);
}

void godot_go_call_type_from_variant(godot_go_anyfn fn,
                                     GDExtensionUninitializedTypePtr r_dest,
                                     GDExtensionVariantPtr p_src) {
    ((GDExtensionTypeFromVariantConstructorFunc)fn)(r_dest, p_src);
}

void godot_go_call_ptr_constructor(godot_go_anyfn fn,
                                   GDExtensionUninitializedTypePtr p_base,
                                   const GDExtensionConstTypePtr *p_args) {
    ((GDExtensionPtrConstructor)fn)(p_base, p_args);
}

void godot_go_call_ptr_destructor(godot_go_anyfn fn, GDExtensionTypePtr p_base) {
    ((GDExtensionPtrDestructor)fn)(p_base);
}

void godot_go_call_ptr_builtin_method(godot_go_anyfn fn,
                                      GDExtensionTypePtr p_base,
                                      const GDExtensionConstTypePtr *p_args,
                                      GDExtensionTypePtr r_return,
                                      int32_t p_argument_count) {
    ((GDExtensionPtrBuiltInMethod)fn)(p_base, p_args, r_return, p_argument_count);
}

void godot_go_call_ptr_operator_evaluator(godot_go_anyfn fn,
                                          GDExtensionConstTypePtr p_left,
                                          GDExtensionConstTypePtr p_right,
                                          GDExtensionTypePtr r_result) {
    ((GDExtensionPtrOperatorEvaluator)fn)(p_left, p_right, r_result);
}

void godot_go_call_ptr_indexed_setter(godot_go_anyfn fn,
                                      GDExtensionTypePtr p_base,
                                      GDExtensionInt p_index,
                                      GDExtensionConstTypePtr p_value) {
    ((GDExtensionPtrIndexedSetter)fn)(p_base, p_index, p_value);
}

void godot_go_call_ptr_indexed_getter(godot_go_anyfn fn,
                                      GDExtensionConstTypePtr p_base,
                                      GDExtensionInt p_index,
                                      GDExtensionTypePtr r_value) {
    ((GDExtensionPtrIndexedGetter)fn)(p_base, p_index, r_value);
}

void godot_go_call_ptr_keyed_setter(godot_go_anyfn fn,
                                    GDExtensionTypePtr p_base,
                                    GDExtensionConstTypePtr p_key,
                                    GDExtensionConstTypePtr p_value) {
    ((GDExtensionPtrKeyedSetter)fn)(p_base, p_key, p_value);
}

void godot_go_call_ptr_keyed_getter(godot_go_anyfn fn,
                                    GDExtensionConstTypePtr p_base,
                                    GDExtensionConstTypePtr p_key,
                                    GDExtensionTypePtr r_value) {
    ((GDExtensionPtrKeyedGetter)fn)(p_base, p_key, r_value);
}

void godot_go_call_ptr_utility_function(godot_go_anyfn fn,
                                        GDExtensionTypePtr r_return,
                                        const GDExtensionConstTypePtr *p_args,
                                        int32_t p_argument_count) {
    ((GDExtensionPtrUtilityFunction)fn)(r_return, p_args, p_argument_count);
}

GDExtensionObjectPtr godot_go_call_classdb_construct_object2(GDExtensionInterfaceClassdbConstructObject2 fn,
                                                              GDExtensionConstStringNamePtr p_classname) {
    return fn(p_classname);
}

GDExtensionMethodBindPtr godot_go_call_classdb_get_method_bind(GDExtensionInterfaceClassdbGetMethodBind fn,
                                                                GDExtensionConstStringNamePtr p_classname,
                                                                GDExtensionConstStringNamePtr p_methodname,
                                                                GDExtensionInt p_hash) {
    return fn(p_classname, p_methodname, p_hash);
}

void godot_go_call_object_method_bind_ptrcall(GDExtensionInterfaceObjectMethodBindPtrcall fn,
                                              GDExtensionMethodBindPtr p_method_bind,
                                              GDExtensionObjectPtr p_instance,
                                              const GDExtensionConstTypePtr *p_args,
                                              GDExtensionTypePtr r_ret) {
    fn(p_method_bind, p_instance, p_args, r_ret);
}

void godot_go_call_object_method_bind_call(GDExtensionInterfaceObjectMethodBindCall fn,
                                           GDExtensionMethodBindPtr p_method_bind,
                                           GDExtensionObjectPtr p_instance,
                                           const GDExtensionConstVariantPtr *p_args,
                                           GDExtensionInt p_arg_count,
                                           GDExtensionUninitializedVariantPtr r_ret,
                                           GDExtensionCallError *r_error) {
    fn(p_method_bind, p_instance, p_args, p_arg_count, r_ret, r_error);
}

void godot_go_call_object_destroy(GDExtensionInterfaceObjectDestroy fn, GDExtensionObjectPtr p_o) {
    fn(p_o);
}

GDExtensionObjectPtr godot_go_call_global_get_singleton(GDExtensionInterfaceGlobalGetSingleton fn,
                                                         GDExtensionConstStringNamePtr p_name) {
    return fn(p_name);
}

const char *godot_go_version2_status(const GDExtensionGodotVersion2 *v) { return v->status; }
const char *godot_go_version2_build (const GDExtensionGodotVersion2 *v) { return v->build;  }
const char *godot_go_version2_hash  (const GDExtensionGodotVersion2 *v) { return v->hash;   }
const char *godot_go_version2_string(const GDExtensionGodotVersion2 *v) { return v->string; }

/* ---------- Class registration ABI. ---------- */

void godot_go_call_classdb_unregister_extension_class(
    GDExtensionInterfaceClassdbUnregisterExtensionClass fn,
    GDExtensionClassLibraryPtr p_library,
    GDExtensionConstStringNamePtr p_class_name) {
    fn(p_library, p_class_name);
}

void godot_go_call_object_set_instance(
    GDExtensionInterfaceObjectSetInstance fn,
    GDExtensionObjectPtr p_o,
    GDExtensionConstStringNamePtr p_classname,
    GDExtensionClassInstancePtr p_instance) {
    fn(p_o, p_classname, p_instance);
}

/* Trampolines installed in the GDExtensionClassCreationInfo4 / MethodInfo
 * structs by godot_go_class_creation_info_init and friends. Each one forwards
 * into a //export'd Go function; the cgo-generated _cgo_export.h declares
 * those Go functions with C-compatible signatures.
 *
 * The userdata pointers (class_userdata, method_userdata) are *not* Go
 * pointers — they're small integer ids the Go side uses to look up the
 * registered class/method in a side table. Rationale: Go's cgo rules
 * forbid storing Go pointers in C memory, so we go through ids. */

static GDExtensionObjectPtr godot_go_create_instance_trampoline(
    void *p_class_userdata, GDExtensionBool p_notify_postinitialize) {
    return godotGoCreateInstance(p_class_userdata, p_notify_postinitialize);
}

static void godot_go_free_instance_trampoline(
    void *p_class_userdata, GDExtensionClassInstancePtr p_instance) {
    godotGoFreeInstance(p_class_userdata, p_instance);
}

static GDExtensionClassCallVirtual godot_go_get_virtual_trampoline(
    void *p_class_userdata, GDExtensionConstStringNamePtr p_name, uint32_t p_hash) {
    /* No virtual overrides in Phase 5a — return NULL means "not overridden",
     * the host will use the parent class's implementation. Phase 5e will
     * route this through Go to look up generated virtual binders. */
    (void)p_class_userdata; (void)p_name; (void)p_hash;
    return NULL;
}

static void godot_go_method_call_trampoline(
    void *method_userdata,
    GDExtensionClassInstancePtr p_instance,
    const GDExtensionConstVariantPtr *p_args,
    GDExtensionInt p_argument_count,
    GDExtensionVariantPtr r_return,
    GDExtensionCallError *r_error) {
    /* The host array is `const GDExtensionConstVariantPtr *` (pointer-to-
     * const-pointer); cgo's exported signature drops the inner const. The
     * cast is purely a type-system handshake — Go never writes through it. */
    godotGoMethodCall(method_userdata, p_instance,
                      (GDExtensionConstVariantPtr *)p_args,
                      p_argument_count, r_return, r_error);
}

static void godot_go_method_ptrcall_trampoline(
    void *method_userdata,
    GDExtensionClassInstancePtr p_instance,
    const GDExtensionConstTypePtr *p_args,
    GDExtensionTypePtr r_ret) {
    godotGoMethodPtrcall(method_userdata, p_instance,
                         (GDExtensionConstTypePtr *)p_args, r_ret);
}

void godot_go_register_extension_class(GDExtensionInterfaceClassdbRegisterExtensionClass5 fn,
                                       GDExtensionClassLibraryPtr p_library,
                                       GDExtensionConstStringNamePtr p_class_name,
                                       GDExtensionConstStringNamePtr p_parent_class_name,
                                       void *class_userdata,
                                       GDExtensionBool is_virtual,
                                       GDExtensionBool is_abstract,
                                       GDExtensionBool is_exposed) {
    /* Build the struct on the C-side stack so the Go caller never stores
     * the Go-allocated StringName pointers (or our Go-side class id, used
     * as a void*) in a Go-allocated outer struct passed to C — a pattern
     * cgo's runtime checker rejects.
     *
     * memset zeroes every optional field; set/get/property/notification/
     * to_string/reference/unreference are all NULL in Phase 5a. */
    GDExtensionClassCreationInfo4 info;
    memset(&info, 0, sizeof(info));
    info.is_virtual            = is_virtual;
    info.is_abstract           = is_abstract;
    info.is_exposed            = is_exposed;
    info.is_runtime            = 0;
    info.create_instance_func  = godot_go_create_instance_trampoline;
    info.free_instance_func    = godot_go_free_instance_trampoline;
    info.get_virtual_func      = godot_go_get_virtual_trampoline;
    info.class_userdata        = class_userdata;
    fn(p_library, p_class_name, p_parent_class_name, &info);
}

void godot_go_register_extension_class_method(GDExtensionInterfaceClassdbRegisterExtensionClassMethod fn,
                                              GDExtensionClassLibraryPtr p_library,
                                              GDExtensionConstStringNamePtr p_class_name,
                                              GDExtensionStringNamePtr p_method_name,
                                              void *method_userdata,
                                              uint32_t method_flags) {
    GDExtensionClassMethodInfo info;
    memset(&info, 0, sizeof(info));
    info.name             = p_method_name;
    info.method_userdata  = method_userdata;
    info.call_func        = godot_go_method_call_trampoline;
    info.ptrcall_func     = godot_go_method_ptrcall_trampoline;
    info.method_flags     = method_flags;
    /* return_value_info / arguments_info are left NULL — the Go side passes
     * no arg/return metadata for nullary void methods in Phase 5a. Phase 5d
     * wires up arg/return info from the user's Go method signature. */
    fn(p_library, p_class_name, &info);
}
