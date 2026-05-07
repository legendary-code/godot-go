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

# RefCounted lifecycle callback state — same module-level pattern as
# the MyNode signal callbacks. Used by _check_refcounted_stability's
# RefHolder.touched.connect cycle.
var _signal_callback_count: int = 0
func _on_refholder_touched(_count: int) -> void:
	_signal_callback_count += 1


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

	# Phase 6: property groups + export hints. ClassDB.class_get_property_list
	# exposes each property's PropertyHint enum value and hint string; we
	# look up the registered properties and assert the inspector metadata
	# matches what the @export_* doctags promised.
	#   PropertyHint enum values (extension_api.json#global_enums):
	#     RANGE=1, ENUM=2, FILE=13, DIR=14,
	#     MULTILINE_TEXT=18, PLACEHOLDER_TEXT=20.
	var props := {}
	for p in ClassDB.class_get_property_list("MyNode", true):
		props[p["name"]] = p

	_check("damage_range hint=RANGE",        props["damage_range"]["hint"],        1)
	_check("damage_range hint_string",       props["damage_range"]["hint_string"], "0,200,5")
	_check("mode hint=ENUM",                 props["mode"]["hint"],                2)
	_check("mode hint_string",               props["mode"]["hint_string"],         "Idle,Attack,Defend")
	_check("skin hint=FILE",                 props["skin"]["hint"],                13)
	_check("skin hint_string",               props["skin"]["hint_string"],         "*.png,*.jpg")
	_check("display_name hint=PLACEHOLDER",  props["display_name"]["hint"],        20)
	_check("display_name hint_string",       props["display_name"]["hint_string"], "e.g. Hero")
	_check("save_dir hint=DIR",              props["save_dir"]["hint"],            14)
	_check("notes hint=MULTILINE",           props["notes"]["hint"],               18)
	# Round-trip a value through one of the hinted properties to confirm
	# the property registration still bridges to the synthesized getter/
	# setter alongside the inspector metadata.
	n.damage_range = 175
	_check("hinted property still r/w (damage_range)", n.damage_range, 175)

	# PLAN3 Phase A: registered methods carry the source-level arg names
	# instead of arg0/arg1/... ClassDB.class_get_method_list returns an
	# array of method-info dicts; each "args" entry is a dict with
	# "name", "class_name", "type", etc. The assertions below introspect
	# the slots so CI catches a regression even though the editor's
	# autocomplete is the user-visible payoff.
	var method_args: Dictionary = {}
	for m: Dictionary in ClassDB.class_get_method_list("MyNode"):
		method_args[m["name"]] = m["args"]
	_check("add(a,b) arg[0].name == 'a'", method_args["add"][0]["name"], "a")
	_check("add(a,b) arg[1].name == 'b'", method_args["add"][1]["name"], "b")
	_check("greet(name) arg[0].name == 'name'", method_args["greet"][0]["name"], "name")
	# Synthesized field-form setter takes a single arg conventionally
	# named "value" so the editor's hover surfaces something readable.
	_check("set_health(value) arg[0].name == 'value'", method_args["set_health"][0]["name"], "value")
	# Virtual override: _process(delta) — name comes from the Go source.
	_check("_process(delta) arg[0].name == 'delta'", method_args["_process"][0]["name"], "delta")

	# Same introspection for signals — ClassDB.class_get_signal_list
	# returns dicts shaped like the method-info entries.
	var signal_args: Dictionary = {}
	for s: Dictionary in ClassDB.class_get_signal_list("MyNode"):
		signal_args[s["name"]] = s["args"]
	_check("damaged(amount) arg[0].name == 'amount'", signal_args["damaged"][0]["name"], "amount")
	_check("tagged(label) arg[0].name == 'label'", signal_args["tagged"][0]["name"], "label")

	# PLAN3 Phase B: typed enums (@enum / @bitfield). The Stance and
	# AbilityFlags Go types register as class-scoped enums; each value
	# becomes a class integer constant; method args / returns of the
	# typed type carry "MyNode.Stance" as their class_name so the editor
	# treats them as the typed enum. ClassDB.class_get_integer_constant
	# resolves the registered values; class_get_method_list args carry
	# class_name on each PropertyInfo entry.
	_check("Stance.NEUTRAL == 0",   ClassDB.class_get_integer_constant("MyNode", "NEUTRAL"),   0)
	_check("Stance.OFFENSIVE == 1", ClassDB.class_get_integer_constant("MyNode", "OFFENSIVE"), 1)
	_check("Stance.DEFENSIVE == 2", ClassDB.class_get_integer_constant("MyNode", "DEFENSIVE"), 2)
	_check("AbilityFlags.FLY == 1",   ClassDB.class_get_integer_constant("MyNode", "FLY"),   1)
	_check("AbilityFlags.SWIM == 2",  ClassDB.class_get_integer_constant("MyNode", "SWIM"),  2)
	_check("AbilityFlags.CLIMB == 4", ClassDB.class_get_integer_constant("MyNode", "CLIMB"), 4)

	# Typed-enum class_name surfacing on method args / returns.
	_check("set_stance(stance: MyNode.Stance) arg[0].class_name",
		method_args["set_stance"][0]["class_name"], &"MyNode.Stance")
	_check("current_stance() return.class_name",
		ClassDB.class_get_method_list("MyNode")
			.filter(func(m: Dictionary) -> bool: return m["name"] == "current_stance")[0]["return"]["class_name"],
		&"MyNode.Stance")

	# Phase 6a — *<MainClass> arg + return marshaling. Echo takes another
	# MyNode, returns it. We construct a second MyNode and verify object
	# identity round-trips through the engine ObjectPtr ↔ side-table
	# bridge.
	var other: MyNode = MyNode.new()
	var got_back: MyNode = n.echo(other)
	_check("echo: round-trip object identity", got_back, other)
	_check("echo: returned object is not self", got_back == n, false)
	# Method registration carries the bare class name as ArgClassNames /
	# ReturnClassName — no <MainClass>.<EnumName> qualification on
	# user-class boundaries.
	var echo_meta: Dictionary = ClassDB.class_get_method_list("MyNode") \
		.filter(func(m: Dictionary) -> bool: return m["name"] == "echo")[0]
	_check("echo: arg[0].class_name", echo_meta["args"][0]["class_name"], &"MyNode")
	_check("echo: return.class_name", echo_meta["return"]["class_name"], &"MyNode")

	# Phase 6b — []*<MainClass> arg + return marshaling. EchoMany takes a
	# slice of MyNode, returns the same slice. Wire form is
	# Array[MyNode] (TypedArray of OBJECT with class_name = "MyNode") —
	# the one shape Godot's set_typed actually accepts at runtime.
	var third: MyNode = MyNode.new()
	var input_array: Array[MyNode] = [other, third]
	var output_array: Array[MyNode] = n.echo_many(input_array)
	_check("echo_many: returned size", output_array.size(), 2)
	_check("echo_many: element 0 identity", output_array[0], other)
	_check("echo_many: element 1 identity", output_array[1], third)
	_check("echo_many: typed at runtime", output_array.is_typed(), true)
	_check("echo_many: typed-class identity", output_array.get_typed_class_name(), &"MyNode")

	# Phase 6c — *<bindings>.<EngineClass> arg + return marshaling.
	# EchoNode takes a *godot.Node (engine class), returns it. The
	# codegen wraps the engine ObjectPtr borrowed-view-style — no
	# refcount management. We pass a plain Node (not a MyNode) and
	# verify identity round-trips at the engine level.
	var plain_node: Node = Node.new()
	var got_node: Node = n.echo_node(plain_node)
	_check("echo_node: returned engine identity", got_node, plain_node)

	# MyNode IS-A Node, so echo_node should accept a MyNode just fine
	# (Godot upcasts the OBJECT slot at the variant boundary).
	var got_via_mynode: Node = n.echo_node(other)
	_check("echo_node: accepts MyNode upcast", got_via_mynode, other)

	# Phase 6c slice — EchoNodes takes []*godot.Node.
	var node_array: Array[Node] = [plain_node, other, third]
	var got_nodes: Array[Node] = n.echo_nodes(node_array)
	_check("echo_nodes: returned size", got_nodes.size(), 3)
	_check("echo_nodes: element 0 identity", got_nodes[0], plain_node)
	_check("echo_nodes: element 1 identity", got_nodes[1], other)
	_check("echo_nodes: element 2 identity", got_nodes[2], third)
	_check("echo_nodes: typed-class identity", got_nodes.get_typed_class_name(), &"Node")

	plain_node.free()
	third.free()
	other.free()

	n.free()

	# Refcount-stability test for user-extension RefCounted classes —
	# reproduces the qux DialogController null-out the user reported.
	# Without the framework's refcount-boundary fix, storing an
	# extension RefCounted in a typed Variant property and reading it
	# back zeroes the refcount within a few accesses.
	_check_refcounted_stability()

	# @abstract_methods + same-package @extends — Dog inherits from
	# Animal (an @abstract @class), Animal declares speak/move via
	# @abstract_methods, Dog provides the concrete impls. From
	# GDScript: Animal.new() should fail (abstract); Dog.new() works
	# and dog.speak() / dog.move(N) reach Dog's user impl.
	_check_abstract_methods()

	if _failed == 0:
		print("test_mynode: ALL CHECKS PASSED")
		quit(0)
	else:
		print("test_mynode: FAILED (", _failed, " check(s) failed)")
		quit(1)


func _check_refcounted_stability() -> void:
	# Static factory mints a fresh RefHolder. Refcount should be ≥ 1
	# right after the call returns; the engine pointer must survive
	# subsequent variant operations (assignment, getter reads, method
	# calls).
	var rh: RefHolder = RefHolder.new_ref_holder_tagged("alpha")
	_check("refholder: factory return non-null", rh != null, true)
	if rh == null:
		return
	_check("refholder: tag round-trip", rh.tag(), "alpha")
	_check("refholder: rc-after-factory >= 1", rh.get_reference_count() >= 1, true)

	# Stash on a Node-typed property holder, then read back through
	# multiple getter accesses. This mirrors the user's
	# Character._controller pattern.
	var holder = Node.new()
	holder.set_meta("rh", rh)
	# At this point the local `rh`, the meta-storage variant, and any
	# pipeline temporaries hold references. The exact count depends on
	# Godot's internal copy/move semantics across versions, so we just
	# require: rc > 0 (alive) and reading the meta back yields the same
	# tagged instance with a working method dispatch.
	for i in range(5):
		var read_rh: RefHolder = holder.get_meta("rh")
		_check("refholder: get_meta non-null (iter %d)" % i, read_rh != null, true)
		if read_rh == null:
			break
		_check("refholder: tag stable (iter %d)" % i, read_rh.tag(), "alpha")
		_check("refholder: rc > 0 (iter %d)" % i, read_rh.get_reference_count() > 0, true)

	# Also drop the holder and ensure the local rh stays valid (engine
	# ptr should still be live since we haven't released our local ref).
	holder.free()
	_check("refholder: alive after holder freed", rh != null, true)
	if rh != null:
		_check("refholder: tag after holder freed", rh.tag(), "alpha")

	# Typed-Variant property on a GDScript class — mirrors the qux
	# Character.controller pattern exactly. RefHolderOwner has a
	# typed `holder: RefHolder` property and a `_holder` backing var.
	# Each getter access reads the underlying refcount; we want it to
	# stay > 0 across many reads.
	var owner_rh: RefHolder = RefHolder.new_ref_holder_tagged("typed")
	var owner = RefHolderOwner.new(owner_rh)
	for i in range(5):
		var read_h: RefHolder = owner.holder
		_check("refholder.typed: getter non-null (iter %d)" % i, read_h != null, true)
		if read_h == null:
			break
		_check("refholder.typed: tag stable (iter %d)" % i, read_h.tag(), "typed")
		_check("refholder.typed: rc > 0 (iter %d)" % i, read_h.get_reference_count() > 0, true)

	# First-emit refcount stability — the qux DialogController bug.
	# Without the New<X>() factory's InitRef() call, the first
	# emit_signal on an extension RefCounted instance trips Godot's
	# refcount_init self-balance asymmetrically and drains one ref.
	# With InitRef called post-set_instance, the variant boxing inside
	# emit_signal is symmetric (init_ref +1/-1 + dtor unreference -1
	# was the bug; now it's just init_ref +1 + dtor -1 = 0).
	var sig_rh: RefHolder = RefHolder.new_ref_holder_tagged("signaled")
	var rc_before_emit = sig_rh.get_reference_count()
	sig_rh.touch(1)
	var rc_after_first_emit = sig_rh.get_reference_count()
	_check("refholder.signal: rc stable across first emit",
			rc_after_first_emit, rc_before_emit)
	# A few more emits to ensure subsequent ones don't drift.
	sig_rh.touch(2)
	sig_rh.touch(3)
	sig_rh.touch(4)
	_check("refholder.signal: rc stable across multiple emits",
			sig_rh.get_reference_count(), rc_before_emit)

	# Connect + emit cycle — mirrors the exact qux flow:
	#   _mount(chr): var ctl = chr._controller
	#                ctl.signal.connect(callback)
	#                ctl.method_that_emits_signal()
	# A connected callable is the typical receiver shape; we want to
	# confirm rc stays stable across the connect+emit chain and the
	# callback actually fires.
	_signal_callback_count = 0
	var conn_rh: RefHolder = RefHolder.new_ref_holder_tagged("connected")
	var rc_before_connect = conn_rh.get_reference_count()
	conn_rh.touched.connect(_on_refholder_touched)
	_check("refholder.connect: rc stable across connect",
			conn_rh.get_reference_count(), rc_before_connect)
	conn_rh.touch(7)
	_check("refholder.connect: rc stable across emit",
			conn_rh.get_reference_count(), rc_before_connect)
	_check("refholder.connect: callback fired",
			_signal_callback_count, 1)
	conn_rh.touch(11)
	conn_rh.touch(13)
	_check("refholder.connect: rc stable after multiple emits",
			conn_rh.get_reference_count(), rc_before_connect)
	_check("refholder.connect: callback fired 3 times total",
			_signal_callback_count, 3)

	# Multi-level typed-Variant property storage — Node holds a custom
	# GDScript class which holds the extension RefCounted. Reads chain
	# through both property accessors. This is the full qux shape:
	# %CharacterDialog.character (Node) → character._controller (extension RefCounted).
	var deep_rh: RefHolder = RefHolder.new_ref_holder_tagged("deep")
	var deep_owner = RefHolderOwner.new(deep_rh)
	var outer = Node.new()
	outer.set_meta("inner_owner", deep_owner)
	for i in range(3):
		var got_owner: RefHolderOwner = outer.get_meta("inner_owner")
		_check("refholder.deep: outer.inner_owner non-null (iter %d)" % i,
				got_owner != null, true)
		if got_owner == null:
			break
		var got_rh: RefHolder = got_owner.holder
		_check("refholder.deep: inner.holder non-null (iter %d)" % i,
				got_rh != null, true)
		if got_rh == null:
			break
		_check("refholder.deep: tag stable (iter %d)" % i, got_rh.tag(), "deep")
		_check("refholder.deep: rc > 0 (iter %d)" % i,
				got_rh.get_reference_count() > 0, true)
		# Emit on the deeply-stored instance; rc must stay positive
		# (replicates _mount → ctl.reset() inside the qux dialog flow).
		got_rh.touch(i)
		_check("refholder.deep: rc > 0 after emit (iter %d)" % i,
				got_rh.get_reference_count() > 0, true)
	outer.free()

	# Lifecycle endpoint — when all references drop, the engine ptr is
	# freed and the framework's Free callback fires. We can't directly
	# observe Free from GDScript, but we can confirm the wrapper isn't
	# leaking the engine ptr indefinitely: get_reference_count() on a
	# fresh instance ought to come back rc=1 each time, regardless of
	# how many were created before. Stable refcount baseline = no leak.
	for i in range(3):
		var fresh: RefHolder = RefHolder.new_ref_holder_tagged("fresh-%d" % i)
		_check("refholder.lifecycle: fresh rc=1 (iter %d)" % i,
				fresh.get_reference_count(), 1)
		fresh.touch(i)
		_check("refholder.lifecycle: rc stable across emit (iter %d)" % i,
				fresh.get_reference_count(), 1)


func _check_abstract_methods() -> void:
	# Animal is @abstract — ClassDB.can_instantiate should report
	# false, and ClassDB.instantiate should refuse. The class is
	# still registered (so subclasses can target it as a parent).
	_check("animal.classdb_exists",
			ClassDB.class_exists("Animal"), true)
	_check("animal.cannot_instantiate",
			ClassDB.can_instantiate("Animal"), false)

	# Dog is concrete — instantiates fine and inherits from Animal
	# in ClassDB's hierarchy.
	_check("dog.classdb_exists",
			ClassDB.class_exists("Dog"), true)
	_check("dog.can_instantiate",
			ClassDB.can_instantiate("Dog"), true)
	_check("dog.parent_is_animal",
			ClassDB.get_parent_class("Dog"), &"Animal")
	_check("animal.parent_is_refcounted",
			ClassDB.get_parent_class("Animal"), &"RefCounted")

	# Construct a Dog. Calling speak() / move() routes through Dog's
	# concrete registrations; the @abstract_methods dispatcher on
	# *Animal exists for Go-side polymorphism but isn't visible here
	# (GDScript dispatches by name on the receiver's class).
	var d: Dog = Dog.new()
	_check("dog.speak returns Woof", d.speak(), "Woof")
	d.move(7)
	d.move(11)
	_check("dog.distance_traveled accumulated 18",
			d.distance_traveled(), 18)


func _check(label: String, got: Variant, want: Variant) -> void:
	if got == want:
		print("test_mynode: PASS  ", label, " = ", got)
	else:
		_failed += 1
		print("test_mynode: FAIL  ", label, " = ", got, " (want ", want, ")")
