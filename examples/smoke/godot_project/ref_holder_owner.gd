class_name RefHolderOwner
extends Node

# Reproduces the qux Character pattern: a GDScript Node with a typed
# Variant property of an extension RefCounted class. Without the
# framework's refcount-boundary fix, storing a fresh extension
# RefCounted on `_holder` and reading it back drains the underlying
# refcount to 0 within a few accesses and the property nulls out.

var _holder: RefHolder
var holder: RefHolder:
	get():
		return _holder

func _init(holder: RefHolder) -> void:
	_holder = holder
