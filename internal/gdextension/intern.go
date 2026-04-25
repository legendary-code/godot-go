package gdextension

import (
	"sync"
	"unsafe"
)

// Builtin payload sizes for the framework's float_64 build (single-precision
// floats with 64-bit pointers). The numbers come from extension_api.json#
// builtin_class_sizes; they're consumed both here (for the StringName intern
// pool) and by the variant package's transparent-string helpers.
const (
	StringSize     = 8
	StringNameSize = 8
)

// InternStringName returns a stable StringNamePtr for s. The backing storage
// is a heap-allocated [StringNameSize]byte slot held forever — Godot interns
// StringNames in its own table, and the framework only needs the host-visible
// pointer to remain valid for the extension's lifetime.
//
// Used by both the variant package (looking up builtin-class methods) and
// the core/editor packages (looking up engine-class names + method binds).
func InternStringName(s string) StringNamePtr {
	internMu.Lock()
	defer internMu.Unlock()
	if p, ok := internCache[s]; ok {
		return p
	}
	slot := new([StringNameSize]byte)
	p := StringNamePtr(unsafe.Pointer(slot))
	StringNameNewWithUtf8(p, s)
	if internCache == nil {
		internCache = map[string]StringNamePtr{}
	}
	internCache[s] = p
	return p
}

var (
	internMu    sync.Mutex
	internCache map[string]StringNamePtr
)
