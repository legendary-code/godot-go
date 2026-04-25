// Package variant holds the framework-generated bindings for Godot's
// builtin classes (Vector2, String, Variant, Packed*Array, …).
//
// Files matching *.gen.go are emitted by cmd/godot-go-bindgen from
// godot/extension_api.json — do not edit those by hand. This file holds
// the small hand-written runtime support the generated code calls into.
package variant

import (
	"sync"
	"unsafe"

	"github.com/legendary-code/godot-go/internal/gdextension"
)

// internStringName returns a stable StringNamePtr for s. The backing storage
// is a heap-allocated [stringNameSize]byte slot held forever — Godot interns
// StringNames in its own table, and the framework only needs the host-visible
// pointer to remain valid for the extension's lifetime.
//
// Phase 2b: hardcoded for the float_32 build config (StringName size = 4).
// Phase 2c will fan this out across the build_config matrix.
func internStringName(s string) gdextension.StringNamePtr {
	internMu.Lock()
	defer internMu.Unlock()
	if p, ok := internCache[s]; ok {
		return p
	}
	slot := new([stringNameSize]byte)
	p := gdextension.StringNamePtr(unsafe.Pointer(slot))
	gdextension.StringNameNewWithUtf8(p, s)
	if internCache == nil {
		internCache = map[string]gdextension.StringNamePtr{}
	}
	internCache[s] = p
	return p
}

const stringNameSize = 4

var (
	internMu    sync.Mutex
	internCache map[string]gdextension.StringNamePtr
)
