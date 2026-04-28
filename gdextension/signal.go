package gdextension

import (
	"fmt"
	"sync"
	"unsafe"
)

// Object::emit_signal is vararg, so the bindgen skips it (the typed-PtrCall
// path doesn't apply). We manually resolve the MethodBind here once and
// dispatch through the variant Call path.
//
// Hash 4047867050 from extension_api.json#classes[Object].methods[emit_signal].hash;
// stable across Godot 4.x patch versions, but the host validates it on
// resolve and warns if the binding's hash doesn't match the running engine.
var (
	objectMethodEmitSignal = sync.OnceValue(func() MethodBindPtr {
		return GetMethodBind(
			InternStringName("Object"),
			InternStringName("emit_signal"),
			4047867050)
	})
	variantFromStringName = sync.OnceValue(func() VariantFromTypeFunc {
		return GetVariantFromTypeConstructor(VariantTypeStringName)
	})
)

// EmitSignal dispatches Object::emit_signal(name, args...) on instance.
// `signalName` is an interned StringName (use InternStringName to obtain
// one); `args` carries the per-signal-arg Variants the caller already
// constructed via variant.NewVariantX helpers. The caller still owns
// each arg Variant — destroy after this returns.
//
// `instance` is the host ObjectPtr the extension class wraps (typically
// `*MyNode.Ptr()` from the embedded Object). The return value of
// emit_signal (an Error enum) is consumed and discarded; signal emission
// is fire-and-forget from the framework's perspective.
func EmitSignal(instance ObjectPtr, signalName StringNamePtr, args []VariantPtr) {
	bind := objectMethodEmitSignal()
	if bind == nil {
		// Diagnostic — should not happen unless the engine doesn't expose
		// emit_signal under the cached hash. Bail early so we don't hand
		// a nil MethodBind to the host.
		return
	}
	// Wrap the signal name in a transient Variant. The host's first arg
	// to emit_signal is a Variant carrying a StringName; the
	// VariantFromTypeConstructor for STRING_NAME copies our interned
	// StringName into the slot.
	var nameSlot [VariantSize]byte
	nameVar := VariantPtr(unsafe.Pointer(&nameSlot))
	CallVariantFromType(variantFromStringName(), nameVar, TypePtr(unsafe.Pointer(signalName)))
	defer VariantDestroy(nameVar)

	allArgs := make([]VariantPtr, 0, len(args)+1)
	allArgs = append(allArgs, nameVar)
	allArgs = append(allArgs, args...)

	var retSlot [VariantSize]byte
	retVar := VariantPtr(unsafe.Pointer(&retSlot))
	cerr := CallMethodBindCall(bind, instance, allArgs, retVar)
	VariantDestroy(retVar)
	if cerr.Type != CallErrorOK {
		// emit_signal reported a CallError — typically an arg-count or
		// type mismatch. Surface to Godot's output so the user can
		// investigate; the call was already issued and won't retry.
		PrintError(
			fmt.Sprintf("EmitSignal(%q): CallError type=%d arg=%d expected=%d",
				StringNameToGo(signalName), cerr.Type, cerr.Argument, cerr.Expected),
			"EmitSignal", "gdextension/signal.go", 0, false)
	}
}
