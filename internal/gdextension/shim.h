#ifndef GODOT_GO_SHIM_H
#define GODOT_GO_SHIM_H

#include <stddef.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>

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

/* godot_go_anyfn is a generic function pointer used to shuttle resolved
 * GDExtensionPtr* function pointers through Go (as `unsafe.Pointer`) and
 * back into typed C trampolines. C permits casts between function pointer
 * types, so this round-trip is well-defined. It's the same shape as
 * GDExtensionInterfaceFunctionPtr. */
typedef void (*godot_go_anyfn)(void);

/* ---------- Typed call helpers — direct interface invocations. ----------- */

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

void                    godot_go_call_variant_new_copy (GDExtensionInterfaceVariantNewCopy fn, GDExtensionUninitializedVariantPtr r_dest, GDExtensionConstVariantPtr p_src);
void                    godot_go_call_variant_destroy  (GDExtensionInterfaceVariantDestroy fn, GDExtensionVariantPtr p_self);
GDExtensionVariantType  godot_go_call_variant_get_type (GDExtensionInterfaceVariantGetType  fn, GDExtensionConstVariantPtr p_self);

GDExtensionInt godot_go_call_string_new_with_utf8_chars_and_len2(GDExtensionInterfaceStringNewWithUtf8CharsAndLen2 fn,
                                                                 GDExtensionUninitializedStringPtr r_dest,
                                                                 const char *p_contents, GDExtensionInt p_size);
GDExtensionInt godot_go_call_string_to_utf8_chars(GDExtensionInterfaceStringToUtf8Chars fn,
                                                  GDExtensionConstStringPtr p_self,
                                                  char *r_text, GDExtensionInt p_max_write_length);
void           godot_go_call_string_name_new_with_utf8_chars_and_len(GDExtensionInterfaceStringNameNewWithUtf8CharsAndLen fn,
                                                                     GDExtensionUninitializedStringNamePtr r_dest,
                                                                     const char *p_contents, GDExtensionInt p_size);

/* ---------- Resolved-getter trampolines.  Each returns a function pointer
 * the caller routes back into the matching invocation trampoline below. -- */

godot_go_anyfn godot_go_call_get_variant_from_type_constructor(GDExtensionInterfaceGetVariantFromTypeConstructor fn, GDExtensionVariantType p_type);
godot_go_anyfn godot_go_call_get_variant_to_type_constructor  (GDExtensionInterfaceGetVariantToTypeConstructor   fn, GDExtensionVariantType p_type);
godot_go_anyfn godot_go_call_variant_get_ptr_constructor       (GDExtensionInterfaceVariantGetPtrConstructor       fn, GDExtensionVariantType p_type, int32_t p_constructor);
godot_go_anyfn godot_go_call_variant_get_ptr_destructor        (GDExtensionInterfaceVariantGetPtrDestructor        fn, GDExtensionVariantType p_type);
godot_go_anyfn godot_go_call_variant_get_ptr_builtin_method    (GDExtensionInterfaceVariantGetPtrBuiltinMethod     fn, GDExtensionVariantType p_type, GDExtensionConstStringNamePtr p_method, GDExtensionInt p_hash);
godot_go_anyfn godot_go_call_variant_get_ptr_operator_evaluator(GDExtensionInterfaceVariantGetPtrOperatorEvaluator fn, GDExtensionVariantOperator p_op, GDExtensionVariantType p_type_a, GDExtensionVariantType p_type_b);
godot_go_anyfn godot_go_call_variant_get_ptr_indexed_setter    (GDExtensionInterfaceVariantGetPtrIndexedSetter     fn, GDExtensionVariantType p_type);
godot_go_anyfn godot_go_call_variant_get_ptr_indexed_getter    (GDExtensionInterfaceVariantGetPtrIndexedGetter     fn, GDExtensionVariantType p_type);
godot_go_anyfn godot_go_call_variant_get_ptr_keyed_setter      (GDExtensionInterfaceVariantGetPtrKeyedSetter       fn, GDExtensionVariantType p_type);
godot_go_anyfn godot_go_call_variant_get_ptr_keyed_getter      (GDExtensionInterfaceVariantGetPtrKeyedGetter       fn, GDExtensionVariantType p_type);
godot_go_anyfn godot_go_call_variant_get_ptr_utility_function  (GDExtensionInterfaceVariantGetPtrUtilityFunction   fn, GDExtensionConstStringNamePtr p_function, GDExtensionInt p_hash);

/* ---------- Resolved-pointer invocation trampolines. ---------- */

void godot_go_call_variant_from_type     (godot_go_anyfn fn, GDExtensionUninitializedVariantPtr r_dest, GDExtensionTypePtr p_src);
void godot_go_call_type_from_variant     (godot_go_anyfn fn, GDExtensionUninitializedTypePtr r_dest, GDExtensionVariantPtr p_src);
void godot_go_call_ptr_constructor       (godot_go_anyfn fn, GDExtensionUninitializedTypePtr p_base, const GDExtensionConstTypePtr *p_args);
void godot_go_call_ptr_destructor        (godot_go_anyfn fn, GDExtensionTypePtr p_base);
void godot_go_call_ptr_builtin_method    (godot_go_anyfn fn, GDExtensionTypePtr p_base, const GDExtensionConstTypePtr *p_args, GDExtensionTypePtr r_return, int32_t p_argument_count);
void godot_go_call_ptr_operator_evaluator(godot_go_anyfn fn, GDExtensionConstTypePtr p_left, GDExtensionConstTypePtr p_right, GDExtensionTypePtr r_result);
void godot_go_call_ptr_indexed_setter    (godot_go_anyfn fn, GDExtensionTypePtr p_base, GDExtensionInt p_index, GDExtensionConstTypePtr p_value);
void godot_go_call_ptr_indexed_getter    (godot_go_anyfn fn, GDExtensionConstTypePtr p_base, GDExtensionInt p_index, GDExtensionTypePtr r_value);
void godot_go_call_ptr_keyed_setter      (godot_go_anyfn fn, GDExtensionTypePtr p_base, GDExtensionConstTypePtr p_key, GDExtensionConstTypePtr p_value);
void godot_go_call_ptr_keyed_getter      (godot_go_anyfn fn, GDExtensionConstTypePtr p_base, GDExtensionConstTypePtr p_key, GDExtensionTypePtr r_value);
void godot_go_call_ptr_utility_function  (godot_go_anyfn fn, GDExtensionTypePtr r_return, const GDExtensionConstTypePtr *p_args, int32_t p_argument_count);

/* ---------- Engine class (Object) ABI. ---------- */

GDExtensionObjectPtr godot_go_call_classdb_construct_object2(GDExtensionInterfaceClassdbConstructObject2 fn,
                                                              GDExtensionConstStringNamePtr p_classname);

GDExtensionMethodBindPtr godot_go_call_classdb_get_method_bind(GDExtensionInterfaceClassdbGetMethodBind fn,
                                                                GDExtensionConstStringNamePtr p_classname,
                                                                GDExtensionConstStringNamePtr p_methodname,
                                                                GDExtensionInt p_hash);

void godot_go_call_object_method_bind_ptrcall(GDExtensionInterfaceObjectMethodBindPtrcall fn,
                                              GDExtensionMethodBindPtr p_method_bind,
                                              GDExtensionObjectPtr p_instance,
                                              const GDExtensionConstTypePtr *p_args,
                                              GDExtensionTypePtr r_ret);

void godot_go_call_object_method_bind_call(GDExtensionInterfaceObjectMethodBindCall fn,
                                           GDExtensionMethodBindPtr p_method_bind,
                                           GDExtensionObjectPtr p_instance,
                                           const GDExtensionConstVariantPtr *p_args,
                                           GDExtensionInt p_arg_count,
                                           GDExtensionUninitializedVariantPtr r_ret,
                                           GDExtensionCallError *r_error);

void godot_go_call_object_destroy(GDExtensionInterfaceObjectDestroy fn, GDExtensionObjectPtr p_o);

GDExtensionObjectPtr godot_go_call_global_get_singleton(GDExtensionInterfaceGlobalGetSingleton fn,
                                                         GDExtensionConstStringNamePtr p_name);

/* Field accessors for GDExtensionGodotVersion2.
 * Avoids cgo's keyword-renaming around the `string` field. */
const char *godot_go_version2_status(const GDExtensionGodotVersion2 *v);
const char *godot_go_version2_build (const GDExtensionGodotVersion2 *v);
const char *godot_go_version2_hash  (const GDExtensionGodotVersion2 *v);
const char *godot_go_version2_string(const GDExtensionGodotVersion2 *v);

/* ---------- Class registration ABI (Phase 5: user-class registration). -- */

void godot_go_call_classdb_unregister_extension_class(
    GDExtensionInterfaceClassdbUnregisterExtensionClass fn,
    GDExtensionClassLibraryPtr p_library,
    GDExtensionConstStringNamePtr p_class_name);

void godot_go_call_object_set_instance(
    GDExtensionInterfaceObjectSetInstance fn,
    GDExtensionObjectPtr p_o,
    GDExtensionConstStringNamePtr p_classname,
    GDExtensionClassInstancePtr p_instance);

/* One-shot register helpers. The C-side builds the GDExtensionClassCreationInfo4
 * / GDExtensionClassMethodInfo struct on its own stack so the Go caller never
 * stores Go-allocated StringName pointers inside a Go struct that's then
 * passed to C — that combination would trip cgo's "Go pointer to unpinned
 * Go memory" check at call time. Function-pointer trampoline fields are
 * filled in by the C helper from internal symbols, so they never cross the
 * cgo boundary. */
void godot_go_register_extension_class(GDExtensionInterfaceClassdbRegisterExtensionClass5 fn,
                                       GDExtensionClassLibraryPtr p_library,
                                       GDExtensionConstStringNamePtr p_class_name,
                                       GDExtensionConstStringNamePtr p_parent_class_name,
                                       void *class_userdata,
                                       GDExtensionBool is_virtual,
                                       GDExtensionBool is_abstract,
                                       GDExtensionBool is_exposed);

/* The arg / return type info travels as parallel scalar arrays so the Go
 * caller never assembles a Go-allocated struct that contains pointers
 * (cgo's "Go pointer to unpinned Go memory" rule rejects that pattern).
 * The C side walks the arrays, builds GDExtensionPropertyInfo entries on
 * the C stack, and fills the GDExtensionClassMethodInfo before calling fn.
 *
 * empty_string_name is reused as PropertyInfo.name + class_name for every
 * argument and the return value. empty_string is reused as hint_string.
 * Both come pre-constructed from the Go side (one-time interning) and are
 * passed through as direct C arguments (cgo permits passing one Go pointer
 * per arg slot).
 *
 * arg_count==0 with arg_types==NULL && arg_metadata==NULL is valid — it
 * registers a no-arg method. has_return==0 means return_type / return_metadata
 * are ignored. */
void godot_go_register_extension_class_method(GDExtensionInterfaceClassdbRegisterExtensionClassMethod fn,
                                              GDExtensionClassLibraryPtr p_library,
                                              GDExtensionConstStringNamePtr p_class_name,
                                              GDExtensionStringNamePtr p_method_name,
                                              void *method_userdata,
                                              uint32_t method_flags,
                                              GDExtensionConstStringNamePtr empty_string_name,
                                              GDExtensionConstStringPtr empty_string,
                                              GDExtensionBool has_return,
                                              uint32_t return_type,
                                              uint32_t return_metadata,
                                              uint32_t arg_count,
                                              const uint32_t *arg_types,
                                              const uint32_t *arg_metadata);

/* Property registration. Mirrors classdb_register_extension_class_property —
 * the engine wires (class, property_name) → (setter_method, getter_method)
 * already registered through godot_go_register_extension_class_method. The
 * setter / getter are looked up by StringName at register time, so the
 * methods MUST be registered first. PropertyInfo is built C-side so the Go
 * caller only passes scalars + the interned StringName/String pointers.
 *
 * `property_hint` is a PropertyHint enum value (NONE=0 by default);
 * `hint_string` is its payload (e.g. "0,100" for RANGE, comma-separated
 * names for ENUM, file filter for FILE). Both default to NONE/empty when
 * the user hasn't set an export hint. The hint_string is interned by the
 * Go side (or the empty StringPtr is passed) so the C trampoline never
 * has to copy. */
void godot_go_register_extension_class_property(GDExtensionInterfaceClassdbRegisterExtensionClassProperty fn,
                                                GDExtensionClassLibraryPtr p_library,
                                                GDExtensionConstStringNamePtr p_class_name,
                                                GDExtensionStringNamePtr p_property_name,
                                                GDExtensionConstStringNamePtr p_setter,
                                                GDExtensionConstStringNamePtr p_getter,
                                                GDExtensionConstStringPtr hint_string,
                                                uint32_t property_type,
                                                uint32_t property_hint);

/* Property group / subgroup registration. Calls must come BEFORE the
 * properties they apply to — Godot's inspector treats subsequent
 * properties as belonging to the most recent group/subgroup. The
 * `prefix` parameter is currently always passed empty from the codegen
 * (Phase 6 punt); a future iteration may wire it through. */
void godot_go_register_extension_class_property_group(GDExtensionInterfaceClassdbRegisterExtensionClassPropertyGroup fn,
                                                       GDExtensionClassLibraryPtr p_library,
                                                       GDExtensionConstStringNamePtr p_class_name,
                                                       GDExtensionConstStringPtr p_group_name,
                                                       GDExtensionConstStringPtr p_prefix);

void godot_go_register_extension_class_property_subgroup(GDExtensionInterfaceClassdbRegisterExtensionClassPropertySubgroup fn,
                                                          GDExtensionClassLibraryPtr p_library,
                                                          GDExtensionConstStringNamePtr p_class_name,
                                                          GDExtensionConstStringPtr p_subgroup_name,
                                                          GDExtensionConstStringPtr p_prefix);

/* Signal registration. Mirrors classdb_register_extension_class_signal —
 * arg type/metadata travels as parallel scalar arrays the same way method
 * arg info does, so the Go caller never assembles a Go-allocated struct
 * with pointers in it. arg_count==0 with NULL arrays is a no-arg signal. */
void godot_go_register_extension_class_signal(GDExtensionInterfaceClassdbRegisterExtensionClassSignal fn,
                                              GDExtensionClassLibraryPtr p_library,
                                              GDExtensionConstStringNamePtr p_class_name,
                                              GDExtensionConstStringNamePtr p_signal_name,
                                              GDExtensionConstStringNamePtr empty_string_name,
                                              GDExtensionConstStringPtr empty_string,
                                              uint32_t arg_count,
                                              const uint32_t *arg_types,
                                              const uint32_t *arg_metadata);

#endif /* GODOT_GO_SHIM_H */
