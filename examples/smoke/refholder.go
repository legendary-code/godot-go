package main

import (
	"github.com/legendary-code/godot-go/godot"
)

// RefHolderSignals exercises the qux-reproduced first-emit refcount
// drop. Godot's per-Object signal machinery does some lazy
// initialization on the first emit_signal call against an extension
// RefCounted instance — that initialization costs one refcount.
// Without the framework warmup in Construct, the first user-driven
// emit would drain the engine refcount and free the object out from
// under any Variant property storing it.
//
// @signals
type RefHolderSignals interface {
	Touched(count int64)
}

// RefHolder reproduces the typed-Variant property-storage scenario
// for user-extension RefCounted classes: a static factory mints a
// fresh instance and returns it through the @class boundary, GDScript
// stores it on a typed property of a Node, and later user code emits
// signals on the held instance. With the Construct-hook signal
// warmup, the first user-emitted signal is rc-stable; without it,
// the property would null-out a few accesses later.
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
// (property assignment, getter reads, method calls, signal emits).
//
// @static
func NewRefHolderTagged(tag string) *RefHolder {
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

// CurrentRefCount surfaces the underlying RefCounted's
// get_reference_count() for tests to assert directly on Go-side reads
// (in addition to the GDScript-side get_reference_count() readings).
func (r *RefHolder) CurrentRefCount() int64 {
	return r.GetReferenceCount()
}

// Touch fires the Touched signal so the GDScript test can verify the
// first emit doesn't drain the engine refcount. Without the framework's
// Construct-hook warmup, this first emit would drop one ref from the
// underlying RefCounted (Godot's per-instance signal lazy init) and
// the property storage would shortly null-out.
func (r *RefHolder) Touch(n int64) {
	r.Touched(n)
}
