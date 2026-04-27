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
	// VariantSize is the byte size of a Variant slot under float_64.
	// Sourced from extension_api.json#builtin_class_sizes; mirrors
	// `variantSize` in the user-facing variant package. Used by the
	// signal-emit helper to stack-allocate transient Variants.
	VariantSize = 24
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

// internedEmptyString returns a stable StringPtr to a default-constructed
// (empty) Godot String. Used by the class-registration shim as the
// hint_string field of GDExtensionPropertyInfo entries — Godot expects a
// non-NULL String there even when no hint is set, and a single shared
// empty instance is fine since the host treats it as immutable input.
func internedEmptyString() StringPtr {
	emptyStringOnce.Do(func() {
		emptyStringSlot = new([StringSize]byte)
		StringNewWithUtf8(StringPtr(unsafe.Pointer(emptyStringSlot)), "")
	})
	return StringPtr(unsafe.Pointer(emptyStringSlot))
}

var (
	emptyStringOnce sync.Once
	emptyStringSlot *[StringSize]byte
)

// InternString returns a stable StringPtr for s. Mirrors InternStringName
// but for the Godot `String` type — used by property registration to
// supply hint_string payloads (export_range "0,100", export_enum
// "A,B,C", export_file "*.png", etc.) without having the Go side
// allocate a fresh String per registration call.
//
// Godot uses Strings as immutable inputs at register time, so we hold
// each interned slot for the extension's lifetime; that's cheap because
// the call site is once-per-class-load, not per frame. The empty string
// is special-cased and goes through internedEmptyString() to share its
// slot.
func InternString(s string) StringPtr {
	if s == "" {
		return internedEmptyString()
	}
	stringInternMu.Lock()
	defer stringInternMu.Unlock()
	if p, ok := stringInternCache[s]; ok {
		return p
	}
	slot := new([StringSize]byte)
	p := StringPtr(unsafe.Pointer(slot))
	StringNewWithUtf8(p, s)
	if stringInternCache == nil {
		stringInternCache = map[string]StringPtr{}
	}
	stringInternCache[s] = p
	return p
}

var (
	stringInternMu    sync.Mutex
	stringInternCache map[string]StringPtr
)
