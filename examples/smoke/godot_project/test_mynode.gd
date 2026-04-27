extends SceneTree

# Phase 5d/5e verification: instantiate the Go-defined MyNode and exercise
# no-arg, primitive-arg, and string-arg dispatch (5d), plus static dispatch
# and virtual-override dispatch (5e).
#   godot --headless --path examples/smoke/godot_project --script test_mynode.gd

func _initialize() -> void:
	print("test_mynode: ClassDB has MyNode? ", ClassDB.class_exists("MyNode"))
	var n: MyNode = MyNode.new()
	print("test_mynode: MyNode.new() = ", n)
	n.hello()
	print("test_mynode: add(2, 3) = ", n.add(2, 3))
	print("test_mynode: greet('world') = ", n.greet("world"))
	# Phase 5e: static call uses ClassName.method() syntax.
	print("test_mynode: MyNode.origin() = ", MyNode.origin())
	# Phase 5e: virtual override — engine routes _process through
	# get_virtual_call_data_func + call_virtual_with_data_func into Go.
	n._process(0.5)
	n.free()
	print("test_mynode: done")
	quit()
