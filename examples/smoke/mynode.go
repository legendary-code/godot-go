package main

import (
	"fmt"

	"github.com/legendary-code/godot-go/godot"
	"github.com/legendary-code/godot-go/godot/runtime"
)

// Signals declares the engine-visible signals MyNode emits. Codegen
// produces typed Emit-style methods on *MyNode for each interface method
// (Damaged → snake_case "damaged"); GDScript subscribers can connect()
// to those signals by name.
//
// @signals
type Signals interface {
	// Damaged is emitted when the node takes damage. The amount is the
	// hit-point loss the receiver should react to.
	Damaged(amount int64)

	// LeveledUp is a no-arg signal — convenient because GDScript
	// connect() supports both shapes uniformly.
	LeveledUp()

	// Tagged carries a string payload, exercising the string-Variant
	// marshaling path through emit_signal's vararg dispatch.
	Tagged(label string)
}

// MyNode is a minimal Go-defined extension class — embeds godot.Node so the
// host treats instances as nodes, and exposes Hello/Add/Greet to GDScript.
//
// Properties exercise the four cells of the @property matrix:
//   - Health     — field form, read-write. Codegen synthesizes Get/Set.
//   - MaxHealth  — field form, read-only. @readonly drops the setter.
//   - Score      — method form, read-only. User wrote GetScore only;
//                  no SetScore means the property is read-only.
//   - Tag        — method form, read-write. User wrote both GetTag
//                  and SetTag; codegen wires both as a normal property.
//
// @class
// @brief Demo extension class wired up for the smoke tests.
// @tutorial("godot-go README", "https://github.com/legendary-code/godot-go")
// @since 0.1
type MyNode struct {
	// @extends
	godot.Node

	// @property
	Health int64

	// @readonly
	// @property
	MaxHealth int64

	// Combat group properties exercise the @group + export-hint path.
	// DamageRange uses @export_range to render as a slider in the
	// inspector; Mode uses @export_enum to render as a dropdown.

	// @group("Combat")
	// @export_range(0, 200, 5)
	// @property
	DamageRange int64

	// @group("Combat")
	// @export_enum("Idle", "Attack", "Defend")
	// @property
	Mode int64

	// Visuals group with a Texture subgroup. Each hint type lights up
	// a different inspector widget — file picker, dir picker, multi-
	// line text, single-line placeholder.

	// @group("Visuals")
	// @subgroup("Texture")
	// @export_file("*.png", "*.jpg")
	// @property
	Skin string

	// @group("Visuals")
	// @subgroup("Texture")
	// @export_placeholder("e.g. Hero")
	// @property
	DisplayName string

	// @group("Visuals")
	// @export_dir
	// @property
	SaveDir string

	// @group("Visuals")
	// @export_multiline
	// @property
	Notes string

	// Stash exercises the @var doctag. Registered with
	// PROPERTY_USAGE_STORAGE only — GDScript can read/write
	// `n.stash = 7`, but the field is hidden from the inspector.
	//
	// @var
	Stash int64

	// score / tag are private backings that the user-written getters
	// dispatch on. Lowercase = invisible to Godot; visible only to the
	// package itself.
	score int64
	tag   string
}

// Stance is a typed enum exposed to Godot via @enum. Values register
// as class-scoped integer constants (Stance.NEUTRAL / OFFENSIVE /
// DEFENSIVE), and any method/property that takes or returns Stance
// surfaces it in the editor's autocomplete with the typed-enum identity
// instead of plain int.
//
// @enum
type Stance int

const (
	// StanceNeutral is the default — neither attacking nor defending.
	StanceNeutral Stance = iota
	// StanceOffensive prioritizes damage output over survivability.
	StanceOffensive
	// StanceDefensive prioritizes survivability — extra armor, less damage.
	//
	// @deprecated Use StanceNeutral with a defensive item instead.
	StanceDefensive
)

// AbilityFlags is a typed bitfield exposed via @bitfield. Values
// compose with bitwise OR; the editor renders them as a flag-grid
// rather than an exclusive dropdown.
//
// @bitfield
type AbilityFlags int

const (
	AbilityFlagsFly  AbilityFlags = 1 << iota // 1
	AbilityFlagsSwim                          // 2
	AbilityFlagsClimb                         // 4
)

// Hello is the method GDScript reaches via `n.hello()`.
//
// @description Greet GDScript callers via Godot's Output dock.
// @see [method greet]
func (n *MyNode) Hello() {
	runtime.PrintInfo("godot-go: MyNode.Hello() reached from GDScript")
}

// Add exercises Phase 5d primitive arg / return marshalling.
func (n *MyNode) Add(a, b int64) int64 {
	return a + b
}

// Greet exercises Phase 5d string marshalling in both directions.
func (n *MyNode) Greet(name string) string {
	return fmt.Sprintf("hello, %s!", name)
}

// Echo exercises Phase 6a *<MainClass> arg + return marshalling. Godot
// passes other as the engine ObjectPtr for the MyNode instance the
// caller constructed; the codegen looks it up in our parallel side
// table and hands back the *MyNode wrapper. Returning other lets the
// GDScript driver verify object identity round-tripped.
//
// Foreign-instance behavior (decision B from .claude/ARRAYS.md): if
// the caller passes a *MyNode instance not registered with this
// extension, the lookup returns nil and codegen returns
// CallErrorInvalidArgument. The user method body never sees a nil
// arg.
func (n *MyNode) Echo(other *MyNode) *MyNode {
	return other
}

// EchoMany exercises Phase 6b []*<MainClass> arg + return marshalling.
// Godot passes an Array[MyNode] (TypedArray of OBJECT with class_name
// = "MyNode"); the codegen unpacks it into a Go slice via per-element
// engine-pointer lookup, then re-packs the return value into a fresh
// Array[MyNode]. Per-element foreign-instance handling matches Echo's:
// any nil lookup short-circuits the call.
func (n *MyNode) EchoMany(others []*MyNode) []*MyNode {
	return others
}

// EchoNode exercises Phase 6c *<bindings>.<EngineClass> arg + return
// marshalling. The codegen wraps the borrowed engine ObjectPtr inline
// (`&godot.Node{}` + BindPtr); no refcount management. Returning the
// same pointer round-trips the underlying engine identity.
//
// Lifecycle caveat for RefCounted classes: the wrapper is a borrowed
// view. Callers retaining it past the method scope must call
// Reference() themselves. Plain Node isn't RefCounted, so this method
// is safe to use without lifecycle care.
func (n *MyNode) EchoNode(other *godot.Node) *godot.Node {
	return other
}

// EchoNodes exercises Phase 6c []*<bindings>.<EngineClass> arg + return
// marshalling. Wire form is Array[Node] — TypedArray of OBJECT with
// class_name = "Node".
func (n *MyNode) EchoNodes(others []*godot.Node) []*godot.Node {
	return others
}

// Origin is a class-level method — `@static` registers it with
// MethodFlagStatic so GDScript callers reach it as `MyNode.origin()`
// without an instance.
//
// @static
// @experimental Behavior may shift once the static-method ABI moves out of beta.
func (MyNode) Origin() int64 {
	return 42
}

// GetScore demonstrates the method form of @property, read-only branch:
// the user owns the getter, no SetScore exists, so codegen registers
// `score` with no setter. Read-only is inferred — there's no @readonly
// tag here because there's nothing to disambiguate.
//
// @property
func (n *MyNode) GetScore() int64 {
	// score is initialized lazily so test_mynode.gd's first read sees a
	// deterministic value without us needing a constructor hook.
	if n.score == 0 {
		n.score = 99
	}
	return n.score
}

// GetTag / SetTag demonstrate the method form of @property, read-write
// branch: both methods exist in source AND both carry @property. The
// rule is symmetric — codegen requires the tag on each side so the
// user's intent is explicit on both halves of the property.
//
// @property
func (n *MyNode) GetTag() string { return n.tag }

// @property
func (n *MyNode) SetTag(v string) { n.tag = v }

// SetStance exercises typed-enum args. Hover in the editor shows
// `set_stance(stance: MyNode.Stance)` rather than `(stance: int)`.
func (n *MyNode) SetStance(stance Stance) {
	_ = stance
}

// CurrentStance exercises typed-enum returns. The registration's
// return_class_name surfaces as `MyNode.Stance` in autocomplete.
func (n *MyNode) CurrentStance() Stance {
	return StanceNeutral
}

// Process is a Phase 5e/6 virtual override — `@override` opts into
// virtual binding so codegen routes through RegisterClassVirtual and
// adds the leading underscore Godot expects on engine virtuals
// (`Process` → `_process`). The engine calls this on every frame for
// live nodes; the smoke test just invokes it directly from GDScript to
// keep the run deterministic without spinning up a scene tree.
//
// @override
func (n *MyNode) Process(delta float64) {
	runtime.PrintInfo(fmt.Sprintf("godot-go: MyNode._process(%.2f) reached from GDScript", delta))
}
