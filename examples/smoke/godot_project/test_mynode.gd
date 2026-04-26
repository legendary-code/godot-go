extends SceneTree

# Phase 5a verification: instantiate the Go-defined MyNode class and call
# its hello() method. Run with:
#   godot --headless --path examples/smoke/godot_project --script test_mynode.gd

func _initialize() -> void:
	print("test_mynode: ClassDB has MyNode? ", ClassDB.class_exists("MyNode"))
	var n: MyNode = MyNode.new()
	print("test_mynode: MyNode.new() = ", n)
	n.hello()
	n.free()
	print("test_mynode: done")
	quit()
