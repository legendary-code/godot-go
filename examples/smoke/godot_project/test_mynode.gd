extends SceneTree

# Phase 5d/5e/6 verification: instantiate the Go-defined MyNode and exercise
# no-arg, primitive-arg, and string-arg dispatch (5d), plus static dispatch
# and virtual-override dispatch (5e), plus property registration in both
# the field and method forms (6).
#   godot --headless --path examples/smoke/godot_project --script test_mynode.gd

var _failed: int = 0

# Captured-by-callback state for signal verification. Module-level so the
# Callable's mutation outlives any per-step closure scope quirks GDScript
# might have around lambda capture of locals.
var _damaged_amount: int = -1
var _leveled_up_count: int = 0
var _tagged_label: String = ""

func _on_damaged(amount: int) -> void: _damaged_amount = amount
func _on_leveled_up() -> void: _leveled_up_count += 1
func _on_tagged(label: String) -> void: _tagged_label = label


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

	# ClassDB shape — each property must have its getter; setters appear
	# only for non-readonly cases. Verifies registration regardless of
	# whether the dispatch path itself works.
	_check("ClassDB.has get_health",  ClassDB.class_has_method("MyNode", "get_health"),  true)
	_check("ClassDB.has set_health",  ClassDB.class_has_method("MyNode", "set_health"),  true)
	_check("ClassDB.has get_max_health", ClassDB.class_has_method("MyNode", "get_max_health"), true)
	_check("ClassDB.no set_max_health",  ClassDB.class_has_method("MyNode", "set_max_health"), false)
	_check("ClassDB.has get_score",   ClassDB.class_has_method("MyNode", "get_score"),   true)
	_check("ClassDB.no set_score",    ClassDB.class_has_method("MyNode", "set_score"),   false)
	_check("ClassDB.has get_tag",     ClassDB.class_has_method("MyNode", "get_tag"),     true)
	_check("ClassDB.has set_tag",     ClassDB.class_has_method("MyNode", "set_tag"),     true)

	# Field-form r/w (Health). Each axis exercises one direction of the
	# property↔method bridge — assigning via property must surface through
	# the synthesized getter, and vice versa.
	n.health = 75
	_check("field rw: n.health after `n.health = 75`",      n.health,         75)
	_check("field rw: get_health() after property write",   n.get_health(),   75)
	n.set_health(120)
	_check("field rw: n.health after set_health(120)",      n.health,         120)
	_check("field rw: get_health() after set_health(120)",  n.get_health(),   120)

	# Field-form readonly (MaxHealth). Read paths must agree; there is no
	# setter to exercise.
	_check("field ro: n.max_health (default)",      n.max_health,         0)
	_check("field ro: get_max_health() (default)",  n.get_max_health(),   0)

	# Method-form readonly (Score). User-written GetScore handles the lazy
	# default; both access paths must hit the same Go method and agree.
	_check("method ro: n.score (lazy default)",     n.score,        99)
	_check("method ro: get_score() (lazy default)", n.get_score(),  99)

	# Method-form r/w (Tag). Same r/w bridge test as Health, but via
	# user-written GetTag/SetTag — the property registration must wire
	# the property's setter to SetTag and getter to GetTag.
	n.tag = "alpha"
	_check("method rw: n.tag after `n.tag = 'alpha'`",      n.tag,        "alpha")
	_check("method rw: get_tag() after property write",     n.get_tag(),  "alpha")
	n.set_tag("beta")
	_check("method rw: n.tag after set_tag('beta')",        n.tag,        "beta")
	_check("method rw: get_tag() after set_tag('beta')",    n.get_tag(),  "beta")

	# Phase 6 signals — connect a Callable to each registered signal,
	# trigger emission from Go via a regular ClassDB method, and verify
	# the callback received the args we expect. Each closure mutates a
	# captured variable so the assertion runs after the connect/emit
	# round trip completes.
	_check("ClassDB.has signal damaged",
			ClassDB.class_has_signal("MyNode", "damaged"), true)
	_check("ClassDB.has signal leveled_up",
			ClassDB.class_has_signal("MyNode", "leveled_up"), true)
	_check("ClassDB.has signal tagged",
			ClassDB.class_has_signal("MyNode", "tagged"), true)

	n.damaged.connect(_on_damaged)
	n.leveled_up.connect(_on_leveled_up)
	n.tagged.connect(_on_tagged)

	# Trigger emission from GDScript via the Godot 4 Signal API. Each
	# registered signal surfaces as a property of type Signal on the
	# instance; `.emit(args...)` is the typed entry point. Equivalent to
	# `n.emit_signal("damaged", 75)` but reads naturally from the call
	# site. The synthesized Go-side methods on *MyNode are for the
	# class's own logic to fire signals from inside.
	n.damaged.emit(75)
	_check("signal damaged delivered amount=75", _damaged_amount, 75)

	n.leveled_up.emit()
	n.leveled_up.emit()
	_check("signal leveled_up fired twice", _leveled_up_count, 2)

	n.tagged.emit("hello-signals")
	_check("signal tagged delivered string payload", _tagged_label, "hello-signals")

	n.free()

	if _failed == 0:
		print("test_mynode: ALL CHECKS PASSED")
		quit(0)
	else:
		print("test_mynode: FAILED (", _failed, " check(s) failed)")
		quit(1)


func _check(label: String, got: Variant, want: Variant) -> void:
	if got == want:
		print("test_mynode: PASS  ", label, " = ", got)
	else:
		_failed += 1
		print("test_mynode: FAIL  ", label, " = ", got, " (want ", want, ")")
