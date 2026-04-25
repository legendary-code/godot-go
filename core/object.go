// Package core holds the framework's bindings for Godot engine classes
// where extension_api.json#classes[*].api_type == "core". Phase 3a only
// hand-writes the bare minimum (Object, Engine) required to prove the
// engine-class ABI end-to-end against a live host. Phase 3b will replace
// this hand-written surface with bindgen output.
package core

import (
	"github.com/legendary-code/godot-go/internal/gdextension"
)

// Object is the root of Godot's engine-class hierarchy. It carries the
// host-allocated GDExtensionObjectPtr; subclass types (Node, Resource, …)
// embed Object by value so promoted methods reach the same pointer.
type Object struct {
	ptr gdextension.ObjectPtr
}

// Ptr exposes the underlying GDExtensionObjectPtr for ABI calls. Generated
// engine-class methods will read this through embedding-promoted access.
func (o *Object) Ptr() gdextension.ObjectPtr {
	if o == nil {
		return nil
	}
	return o.ptr
}

// IsNil reports whether the wrapped pointer is null. Useful for guarding
// singleton lookups that may legitimately return nil (e.g. an editor-only
// singleton at runtime).
func (o *Object) IsNil() bool {
	return o == nil || o.ptr == nil
}
