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

const char *godot_go_version2_status(const GDExtensionGodotVersion2 *v) { return v->status; }
const char *godot_go_version2_build (const GDExtensionGodotVersion2 *v) { return v->build;  }
const char *godot_go_version2_hash  (const GDExtensionGodotVersion2 *v) { return v->hash;   }
const char *godot_go_version2_string(const GDExtensionGodotVersion2 *v) { return v->string; }
