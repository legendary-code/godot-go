package core

import (
	"sync"
	"unsafe"

	"github.com/legendary-code/godot-go/internal/gdextension"
	"github.com/legendary-code/godot-go/variant"
)

// Engine is a hand-bound subset of Godot's Engine singleton, sufficient for
// Phase 3a smoke verification. The full surface will be regenerated in 3b.
type Engine struct {
	Object
}

// engineClassName is the interned StringName "Engine"; resolved on first use
// so importing core/ doesn't force a host roundtrip during package init.
var engineClassName = sync.OnceValue(func() gdextension.StringNamePtr {
	return gdextension.InternStringName("Engine")
})

// engineSingletonInstance memoizes the host's Engine singleton pointer. The
// host owns its lifetime; we never destroy it.
var engineSingletonInstance = sync.OnceValue(func() *Engine {
	o := gdextension.GetSingleton(engineClassName())
	if o == nil {
		return nil
	}
	return &Engine{Object: Object{ptr: o}}
})

// EngineSingleton returns the global Engine singleton. Returns nil if the
// host has no Engine registered (shouldn't happen at runtime, but the host
// API technically permits it).
func EngineSingleton() *Engine {
	return engineSingletonInstance()
}

// engineMethodGetVersionInfo lazy-resolves the method bind for
// Engine.get_version_info. Sticking to sync.OnceValue here is the model the
// 3b bindgen sweep will copy: no init-callback needed, first method call
// pays for the lookup, every subsequent call is a plain pointer load.
var engineMethodGetVersionInfo = sync.OnceValue(func() gdextension.MethodBindPtr {
	return gdextension.GetMethodBind(
		engineClassName(),
		gdextension.InternStringName("get_version_info"),
		3102165223,
	)
})

// GetVersionInfo mirrors Engine.get_version_info. Returns a Dictionary with
// keys "major", "minor", "patch", "hex", "status", "build", "year", "string".
// Caller owns the returned Dictionary.
func (e *Engine) GetVersionInfo() variant.Dictionary {
	var ret variant.Dictionary
	gdextension.CallMethodBindPtrCall(
		engineMethodGetVersionInfo(),
		e.Ptr(),
		nil,
		gdextension.TypePtr(unsafe.Pointer(&ret)),
	)
	return ret
}
