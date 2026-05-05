package main

import (
	"github.com/legendary-code/godot-go/godot"
)

// RefHolder reproduces the typed-Variant property-storage scenario for
// user-extension RefCounted classes: a static factory mints a fresh
// instance and returns it through the @class boundary, GDScript stores
// it on a typed property of a Node, and later code reads the property
// repeatedly. Without the framework's refcount fix, the engine ptr
// would drop to refcount=0 and the property would null-out on the
// second read.
//
// @class
type RefHolder struct {
	// @extends
	godot.RefCounted

	tag string
}

// NewRefHolderTagged is a static factory mirroring the user-reported
// pattern (DialogController.LoadDialog) — constructs a fresh instance,
// stamps a tag on it, returns it through the @class boundary as
// `*RefHolder`. The boundary marshalling needs to keep the engine
// pointer alive through any number of subsequent variant operations
// (property assignment, getter reads, method calls).
//
// @static
func (*RefHolder) NewRefHolderTagged(tag string) *RefHolder {
	r := NewRefHolder()
	r.tag = tag
	return r
}

// Tag exposes the stamped value so GDScript can verify identity on
// each access — if the engine pointer was freed mid-flight, the
// method-bind dispatch would crash or return stale memory.
func (r *RefHolder) Tag() string {
	return r.tag
}

// CurrentRefCount surfaces the underlying RefCounted's get_reference_count()
// for tests to assert directly on Go-side reads (in addition to the
// GDScript-side get_reference_count() readings).
func (r *RefHolder) CurrentRefCount() int64 {
	return r.GetReferenceCount()
}
