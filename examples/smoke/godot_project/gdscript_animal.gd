class_name GDScriptAnimal extends Animal

# A pure-GDScript subclass of an @abstract Go class. Two things have to
# work here for `@abstract_methods` to be useful in practice:
#
#   1. Godot must accept `class_name X extends Y` — i.e. the Animal
#      registration must NOT pass is_abstract=true (which would null
#      creation_func and trip "Can't inherit from a virtual class").
#   2. Overriding speak() / move() must NOT trigger the parser's
#      NATIVE_METHOD_OVERRIDE warning. That's why the framework
#      registers @abstract_methods through
#      classdb_register_extension_class_virtual_method instead of
#      classdb_register_extension_class_method — virtual_methods_map
#      entries aren't visible to ClassDB::get_method, so the parser
#      treats these as fresh virtuals to override.

func speak() -> String:
	return "gdscript-speak"

func move(_distance: int) -> void:
	# No-op for the smoke check; existence + correct signature is the
	# whole point. Underscore prefix on the param name suppresses the
	# unused-arg warning.
	pass
