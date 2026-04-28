extends SceneTree

# Headless verification for the Mover extension class. Drives
# Process(delta) manually with known values and asserts the position
# evolves the way the math says it should. Also subscribes to the
# `bounced` signal and counts emissions to confirm the bound-detection
# branch is reached.
#
#   godot --headless --path examples/2d_demo/godot_project --script test_mover.gd

var _failed: int = 0
var _bounce_count: int = 0
var _last_direction: int = 0

func _on_bounced(direction: int) -> void:
	_bounce_count += 1
	_last_direction = direction


func _initialize() -> void:
	_check("ClassDB.class_exists('Mover')", ClassDB.class_exists("Mover"), true)

	var n: Mover = Mover.new()
	n.bounced.connect(_on_bounced)

	# Inspector-driven configuration. Speed / Range are real numbers
	# the user would tweak in the editor; here we set them in script.
	n.speed = 100.0
	n.range = 50.0

	# Sanity: position starts at (0, 0) for a fresh Node2D.
	_check("initial position.x", n.position.x, 0.0)

	# One frame at 0.1s should advance 10px to the right (Speed=100,
	# direction defaults to +1 on first Process). Process also seeds
	# the origin from current position.
	n._process(0.1)
	_check("after one 0.1s tick: x", n.position.x, 10.0)
	_check("no bounce yet", _bounce_count, 0)

	# Four more 0.1s ticks land us exactly at +Range=50, which trips
	# the >= bound check and reverses direction in the same tick.
	# Mover.Process emits Bounced exactly once per direction flip.
	for i in 4:
		n._process(0.1)
	_check("clamped at +Range", n.position.x, 50.0)
	_check("bounce_count == 1 after first reversal", _bounce_count, 1)
	_check("last_direction == -1 after rightward bounce", _last_direction, -1)

	# Travel left: from x=50 with direction=-1, ten 0.1s ticks moves
	# us 100px left → x=-50, the lower bound, triggering the second
	# bounce on the tenth tick.
	for i in 10:
		n._process(0.1)
	_check("clamped at -Range", n.position.x, -50.0)
	_check("bounce_count == 2 after second reversal", _bounce_count, 2)
	_check("last_direction == +1 after leftward bounce", _last_direction, 1)

	# Reset() returns to origin and re-seeds direction = +1. The next
	# tick should advance rightward again.
	n.reset()
	_check("after reset(): x back at origin", n.position.x, 0.0)
	n._process(0.1)
	_check("post-reset tick goes right", n.position.x, 10.0)

	# ClassDB shape — the inspector metadata for properties + signal
	# registration must surface, not just the dispatch path.
	_check("ClassDB.has speed", ClassDB.class_has_method("Mover", "get_speed"), true)
	_check("ClassDB.has set_speed", ClassDB.class_has_method("Mover", "set_speed"), true)
	_check("ClassDB.has range", ClassDB.class_has_method("Mover", "get_range"), true)
	_check("ClassDB.has signal bounced", ClassDB.class_has_signal("Mover", "bounced"), true)
	_check("ClassDB.has _process virtual", ClassDB.class_has_method("Mover", "_process", true), true)

	# Verify the export hint surfaces — Speed should carry RANGE with
	# the bounds we set on the @export_range tag.
	var props := {}
	for p in ClassDB.class_get_property_list("Mover", true):
		props[p["name"]] = p
	_check("speed hint=RANGE", props["speed"]["hint"], 1)
	_check("speed hint_string", props["speed"]["hint_string"], "0,500,10")
	_check("range hint=RANGE", props["range"]["hint"], 1)
	_check("range hint_string", props["range"]["hint_string"], "0,1000,10")

	n.free()

	if _failed == 0:
		print("test_mover: ALL CHECKS PASSED")
		quit(0)
	else:
		print("test_mover: FAILED (", _failed, " check(s) failed)")
		quit(1)


func _check(label: String, got: Variant, want: Variant) -> void:
	# Approximate comparison for floats so per-tick rounding doesn't
	# flake the test. Anything finer than 1e-3 wouldn't be visible
	# anyway given the 100 px/s speed and integer-ish bounds.
	var matches: bool = false
	if typeof(got) == TYPE_FLOAT or typeof(want) == TYPE_FLOAT:
		matches = absf(float(got) - float(want)) < 1e-3
	else:
		matches = got == want
	if matches:
		print("test_mover: PASS  ", label, " = ", got)
	else:
		_failed += 1
		print("test_mover: FAIL  ", label, " = ", got, " (want ", want, ")")
