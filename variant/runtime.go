// Package variant holds the framework-generated bindings for Godot's
// builtin classes (Vector2, String, Variant, Packed*Array, …).
//
// Files matching *.gen.go are emitted by cmd/godot-go-bindgen from
// godot/extension_api.json — do not edit those by hand. This file holds
// the small hand-written runtime support the generated code calls into.
package variant

import (
	"unsafe"

	"github.com/legendary-code/godot-go/internal/gdextension"
)

// internStringName is a thin facade over gdextension.InternStringName so the
// generated *.gen.go files keep their existing call shape. The size constants
// and the cache itself live in the gdextension package because the core/
// engine-class bindings need the same pool.
func internStringName(s string) gdextension.StringNamePtr {
	return gdextension.InternStringName(s)
}

// The transparent string boundary helpers below depend on resolved
// constructor/destructor pointers populated by string.gen.go / stringname.gen.go
// / nodepath.gen.go at CORE init level. Those generated files declare:
//
//   stringDtor    — String destructor
//   stringNameDtor — StringName destructor
//   nodePathDtor   — NodePath destructor
//   stringCtor2    — String <- StringName
//   stringCtor3    — String <- NodePath
//   nodepathCtor2  — NodePath <- String
//
// Init-callback ordering: every generated initX runs at InitLevelCore before
// any user code does, so the helpers below are always called against
// already-populated pointers.

// stringFromGo fills the uninitialized String slot dst with the contents of
// s, encoded as UTF-8. Caller must call stringDestroy(dst) once dst is no
// longer needed.
func stringFromGo(dst *String, s string) {
	gdextension.StringNewWithUtf8(gdextension.StringPtr(unsafe.Pointer(dst)), s)
}

// stringToGo extracts the contents of self as a Go string. Does not destroy
// self.
func stringToGo(self *String) string {
	return gdextension.StringToGo(gdextension.StringPtr(unsafe.Pointer(self)))
}

// stringDestroy runs String's destructor on self.
func stringDestroy(self *String) {
	gdextension.CallPtrDestructor(stringDtor, gdextension.TypePtr(unsafe.Pointer(self)))
}

// stringNameFromGo fills the uninitialized StringName slot dst from a Go
// string. Caller must call stringNameDestroy(dst) once dst is no longer
// needed.
func stringNameFromGo(dst *StringName, s string) {
	gdextension.StringNameNewWithUtf8(gdextension.StringNamePtr(unsafe.Pointer(dst)), s)
}

// stringNameToGo extracts a StringName as a Go string by routing through a
// temporary String (StringName has no direct UTF-8 extractor in the host
// interface).
func stringNameToGo(self *StringName) string {
	var tmp String
	args := [...]gdextension.TypePtr{gdextension.TypePtr(unsafe.Pointer(self))}
	gdextension.CallPtrConstructor(stringCtor2, gdextension.TypePtr(unsafe.Pointer(&tmp)), args[:])
	defer stringDestroy(&tmp)
	return stringToGo(&tmp)
}

// stringNameDestroy runs StringName's destructor on self.
func stringNameDestroy(self *StringName) {
	gdextension.CallPtrDestructor(stringNameDtor, gdextension.TypePtr(unsafe.Pointer(self)))
}

// nodePathFromGo fills the uninitialized NodePath slot dst from a Go string.
// Caller must call nodePathDestroy(dst) once dst is no longer needed.
func nodePathFromGo(dst *NodePath, s string) {
	var tmp String
	stringFromGo(&tmp, s)
	defer stringDestroy(&tmp)
	args := [...]gdextension.TypePtr{gdextension.TypePtr(unsafe.Pointer(&tmp))}
	gdextension.CallPtrConstructor(nodePathCtor2, gdextension.TypePtr(unsafe.Pointer(dst)), args[:])
}

// nodePathToGo extracts a NodePath as a Go string by routing through a
// temporary String.
func nodePathToGo(self *NodePath) string {
	var tmp String
	args := [...]gdextension.TypePtr{gdextension.TypePtr(unsafe.Pointer(self))}
	gdextension.CallPtrConstructor(stringCtor3, gdextension.TypePtr(unsafe.Pointer(&tmp)), args[:])
	defer stringDestroy(&tmp)
	return stringToGo(&tmp)
}

// nodePathDestroy runs NodePath's destructor on self.
func nodePathDestroy(self *NodePath) {
	gdextension.CallPtrDestructor(nodePathDtor, gdextension.TypePtr(unsafe.Pointer(self)))
}
