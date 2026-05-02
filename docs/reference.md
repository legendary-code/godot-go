# Reference

Comprehensive documentation for godot-go's user-facing surface.
Skim the [doc tags](#doc-tags) for the syntax, the
[features](#features) for the semantics, the
[supported types](#supported-types) for what Go types you can
exchange with Godot, and [multi-version targeting](#multi-version-targeting)
for the floor and ceiling.

For project setup (creating the `godot/` directory, dumping
`extension_api.json`, wiring `go generate`), see
[`setup.md`](./setup.md). For deeper dives on threading, refcount
lifecycle, and hot-reload, see the dedicated docs in this folder.

## Table of contents

- [Doc tags](#doc-tags)
  - [On a struct](#on-a-struct)
  - [On an embedded field of a `@class` struct](#on-an-embedded-field-of-a-class-struct)
  - [On an exported field](#on-an-exported-field)
  - [On a method](#on-a-method)
  - [On an interface](#on-an-interface)
  - [On a typed-int declaration](#on-a-typed-int-declaration)
  - [Documentation tags](#documentation-tags)
- [Features](#features)
  - [Class registration](#class-registration)
  - [Inheritance](#inheritance)
  - [Methods](#methods)
  - [Properties](#properties)
  - [Signals](#signals)
  - [Runtime helpers](#runtime-helpers)
  - [Refcounted lifecycle](#refcounted-lifecycle)
- [GDExtension manifest](#gdextension-manifest)
- [Supported types](#supported-types)
- [Multi-version targeting](#multi-version-targeting)

---

## Doc tags

godot-go's user-class codegen reads tags from Go doc comments
(`//` lines preceding a declaration). Tags start with `@` and may
take parenthesized arguments. The set is closed — unknown tags are
silently ignored, matching Go's general "extra comments are fine"
convention.

A tag's effect depends on what it's attached to. The sub-sections
below organize the tags by attachment site.

### On a struct

| Tag | Required? | Argument | What it does |
|---|---|---|---|
| `@class` | exactly one per file | none | Marks this struct as the file's extension class. The codegen registers it with Godot's ClassDB. Each file declares at most one Godot class; additional Go structs in the same file are plain Go types and aren't registered. |
| `@abstract` | optional | none | Only valid on `@class` structs. Sets `is_abstract` on the registration so GDScript can't instantiate the class directly — only subclasses. Equivalent to GDScript's `class_name` + `abstract`. |
| `@editor` | optional | none | Only valid on `@class` structs. Registers the class at `INIT_LEVEL_EDITOR` instead of `INIT_LEVEL_SCENE`, so it's invisible to deployed game builds. Use for `EditorPlugin` subclasses, custom inspectors, gizmos, etc. |

Documentation tags below land in the generated docs XML the editor
reads via `editor_help_load_xml_from_utf8_chars`. Each is also valid
on methods, properties, signals, enum types, and enum values where
Godot's class.xsd schema supports the corresponding attribute or
element — see [Documentation tags](#documentation-tags) for the full
table and renderings.

```go
// MyNode is a regular Godot Node subclass.
//
// @class
type MyNode struct {
    // @extends
    godot.Node
}

// AbstractBase can't be instantiated from GDScript.
//
// @class
// @abstract
// @description This is a custom description that overrides the auto-synthesized one.
type AbstractBase struct {
    // @extends
    godot.Node
}

// MyEditorTool only loads when Godot is running in editor mode.
//
// @class
// @editor
type MyEditorTool struct {
    // @extends
    godot.EditorPlugin
}
```

### On an embedded field of a `@class` struct

| Tag | Required? | Argument | What it does |
|---|---|---|---|
| `@extends` | exactly one per `@class` struct | none | Marks the embedded type as the parent class. Single-inheritance only — Godot's model. The embedded type must be a package-qualified type like `godot.Node` — works for your bindings package (`godot.Node`, `godot.RefCounted`), the user-class package of another GDExtension, or any third-party module that re-exports a registered class. The runtime registration succeeds when the parent class is in Godot's ClassDB by the time this class loads, so cross-extension inheritance requires the providing extension to register first. |

```go
// @class
type MyNode struct {
    // @extends — required, exactly one, must sit on an embedded field.
    godot.Node
}
```

`@extends` on a *named* (non-embedded) field is rejected — Go's
embedding is what gives the wrapper its inherited methods, so the
parent must come through embedding.

`@extends` on a struct that isn't tagged `@class` is also rejected
— only `@class` structs are registered with Godot's ClassDB, so
finding `@extends` elsewhere is almost always a missing-tag mistake
worth catching loudly.

Cross-extension inheritance (extending a class from another
GDExtension) is supported via the standard package-qualified form.
The user just imports the providing extension's user-class package
and embeds the parent type. Load order matters at runtime: Godot
registers extensions in dependency-free order, so an extension that
relies on another extension's classes needs to ensure that one
loads first.

### On an exported field

The field tags split into two groups: property declaration and
inspector polish.

#### Property declaration

| Tag | Required? | Argument | What it does |
|---|---|---|---|
| `@property` | yes (to be exposed) | none | Exposes the field to GDScript as a property of the class. Codegen synthesizes `Get<Name>` and `Set<Name>` Go methods that read/write the field; both register through the standard method-binding path so GDScript sees them as a property pair. The field must be exported (capitalized). |
| `@readonly` | optional | none | Only valid alongside `@property`. Drops the synthesized setter so the property is read-only from GDScript. The Go side can still write the field directly. |

```go
// @class
type Player struct {
    // @extends
    godot.Node

    // @property
    Health int64

    // @property
    // @readonly
    MaxHealth int64
}
```

#### Inspector polish

These tags are valid only on field-form `@property` declarations
(not method-form — see the [method tag table](#on-a-method)).

| Tag | Argument | Effect |
|---|---|---|
| `@group("Name")` | string | Inserts a `RegisterClassPropertyGroup("Name")` before this property. Subsequent tagged properties in the same group must be contiguous in source — going back to a previously-left group is rejected at codegen time. |
| `@subgroup("Name")` | string | Inserts a `RegisterClassPropertySubgroup("Name")` nested under the current `@group`. Requires an active `@group`. Same contiguity rule. |
| `@export_range(min, max[, step])` | 2–3 numbers | Renders the property as a slider in the inspector. Valid on numeric types. |
| `@export_enum("v1", "v2", ...)` | 1+ strings | Renders the property as a dropdown. Valid on int / int32 / int64; the user code receives the index as an int. |
| `@export_file([filter, ...])` | 0+ strings | File-picker widget. Filters are like `"*.png"` or `"*.png,*.jpg"`. No filter argument opens an unfiltered picker. Valid on `string`. |
| `@export_dir` | none | Directory-picker widget. Valid on `string`. |
| `@export_multiline` | none | Multi-line text editor. Valid on `string`. |
| `@export_placeholder("text")` | string | Single-line text editor with the supplied placeholder. Valid on `string`. |

At most one `@export_*` tag per field — multiple is rejected.

```go
// @class
type Combatant struct {
    // @extends
    godot.Node

    // @group("Combat")
    // @export_range(0, 200, 5)
    // @property
    DamageRange int64

    // @group("Combat")
    // @export_enum("Idle", "Attack", "Defend")
    // @property
    Mode int64

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
}
```

### On a method

| Tag | Argument | Where valid | What it does |
|---|---|---|---|
| `@property` | none | on `Get<Name>` and `Set<Name>` methods | Registers the pair as a property (method form). Both halves must carry the tag — tagging only `Get<X>` makes the property read-only; tagging `Set<X>` without a matching tagged `Get<X>` is rejected as an orphan setter. The user owns the backing storage. Method form does NOT accept export hints. |
| `@static` | none | on a method | Registers the method with `MethodFlagStatic` so GDScript callers reach it as `ClassName.method()` without an instance. The receiver may be unnamed (`func (T) Foo()` — clearer about not using the instance) or named (`func (t *T) Foo()` — fine, the argument is just unused). Mutually exclusive with `@override`. |
| `@override` | none | on a method | Registers the method as a Godot virtual (e.g., `_process`, `_ready`, `_input`). Codegen routes through `RegisterClassVirtual` and prepends a leading underscore to the Godot name unless the user supplied one with `@name`. |
| `@name` | string | on a method | Overrides the Godot-side name for the method. Default is `snake_case(GoName)`; with `@name`, the literal value is used. |

```go
// @class
type Player struct {
    // @extends
    godot.Node

    score int64
    tag   string
}

// GetScore is method-form @property, read-only (no SetScore exists).
//
// @property
func (p *Player) GetScore() int64 {
    return p.score
}

// GetTag / SetTag is method-form @property, read-write.
//
// @property
func (p *Player) GetTag() string { return p.tag }

// @property
func (p *Player) SetTag(v string) { p.tag = v }

// Process is registered as the virtual `_process`.
//
// @override
func (p *Player) Process(delta float64) {
    // per-frame logic
}

// Damage is exposed to GDScript as `take_damage` (not `damage`).
//
// @name take_damage
func (p *Player) Damage(amount int64) {
    p.score -= amount
}

// Origin is a class-level method — GDScript callers write
// `Player.origin()` without an instance.
//
// @static
func (Player) Origin() int64 { return 42 }
```

Method classification rules at a glance:

| Receiver | Tag | Outcome |
|---|---|---|
| `func (p *T) Foo()` | none | regular instance method (Godot name `foo`) |
| `func (p *T) Foo()` | `@override` | virtual override (Godot name `_foo`) |
| `func (p *T) Foo()` | `@static` | static method (Godot name `foo`, registered with `MethodFlagStatic`) |
| `func (T) Foo()` | none | **rejected** — unnamed receiver requires `@static` |
| `func (T) Foo()` | `@static` | static method (same as above; the unnamed-receiver form documents the intent) |
| `func (p *T) foo()` | none | NOT registered — Go-private helper |
| `func (p *T) foo()` | `@override` | virtual override (Godot name `_foo`) |
| `func (p *T) foo()` | `@static` | static method (Godot name `foo`) |

### On an interface

| Tag | Required? | Argument | What it does |
|---|---|---|---|
| `@signals` | yes (per signal interface) | none | Marks the interface as a signal contract. Each interface method becomes a Godot signal on the file's `@class`; the codegen synthesizes a typed `func (*Class) Name(args...)` emit method on the class. The interface itself isn't implemented by anything — the methods declare shape only. |

Constraints:

- Signal methods must have no return value.
- Embedded interfaces aren't supported — only direct method declarations become signals.
- Signal names must be unique across all `@signals` interfaces in the file.
- Signal names must not collide with regular methods on the `@class` struct.

```go
// @class
type Player struct {
    // @extends
    godot.Node
}

// @signals
type Signals interface {
    // Damaged fires when the player takes a hit.
    Damaged(amount int64)

    // LeveledUp fires on a level transition; no payload.
    LeveledUp()

    // Tagged carries a string label.
    Tagged(label string)
}

// Inside Player methods, fire signals via the synthesized emitters:
//
//   p.Damaged(75)
//   p.LeveledUp()
//   p.Tagged("hero")
//
// GDScript subscribers connect via the standard Signal API:
//
//   p.damaged.connect(_on_damaged)
```

The synthesized emit methods are Go-only — they construct typed
Variants and dispatch through `Object::emit_signal`. GDScript can't
call them as methods (signals aren't callable from outside the
class anyway); GDScript uses the standard `n.damaged.emit(75)` /
`n.emit_signal("damaged", 75)` syntax.

### On a typed-int declaration

| Tag | Argument | What it does |
|---|---|---|
| `@enum` | none | Marks a `type X int / int32 / int64` as a class-scoped enum exposed to Godot. Each value of the const block declaring its members registers as an integer constant under the enum, and any method / property / signal that takes or returns the type populates the registration's `class_name` field with `<MainClass>.<EnumName>` so the editor renders typed-enum autocomplete. Mutually exclusive with `@bitfield`. |
| `@bitfield` | none | Same registration path as `@enum`, but with `is_bitfield = true`. Values compose with bitwise OR; the editor renders the constants as a flag grid rather than a mutually-exclusive dropdown. |

Without `@enum` / `@bitfield`, the type is Go-only — values aren't
registered, and signatures using the type collapse to plain `int` at
the Godot boundary.

The constant names registered with Godot are the SCREAMING_SNAKE form
of the Go identifier with the type prefix stripped:
`StanceOffensive` under `type Stance int` registers as `OFFENSIVE`
(GDScript callers write `Stance.OFFENSIVE`, not the redundant
`Stance.STANCE_OFFENSIVE`). Constants without the prefix
(`Idle` under `type Mode int`) become `IDLE`.

Supported value expressions: integer literals, `iota`, and basic
arithmetic on them (`+`, `-`, `*`, `/`, `%`, `<<`, `>>`, `&`, `|`,
`^`, `&^`, unary `-`/`+`/`^`). References to other constants or
function calls aren't supported and surface as a file:line error.

```go
// @enum
type Stance int

const (
    StanceNeutral   Stance = iota // → Stance.NEUTRAL = 0
    StanceOffensive               // → Stance.OFFENSIVE = 1
    StanceDefensive               // → Stance.DEFENSIVE = 2
)

// @bitfield
type AbilityFlags int

const (
    AbilityFlagsFly  AbilityFlags = 1 << iota // → AbilityFlags.FLY = 1
    AbilityFlagsSwim                          // → AbilityFlags.SWIM = 2
    AbilityFlagsClimb                         // → AbilityFlags.CLIMB = 4
)

// @class
type Player struct {
    // @extends
    godot.Node
}

// SetStance shows up in the editor as `set_stance(stance: Player.Stance)`.
func (p *Player) SetStance(stance Stance) { /* ... */ }

// CurrentStance's return surfaces as Player.Stance, not int.
func (p *Player) CurrentStance() Stance { return StanceNeutral }
```

### Documentation tags

These tags populate the class XML the editor's docs page reads. They
apply to every declaration site Godot's `class.xsd` schema supports —
classes, methods, properties, signals, enum types, and enum values.

| Tag | Argument | Where valid | What it does |
|---|---|---|---|
| `@description` | inline text | class, method, property, signal, enum type, enum value | Overrides the auto-extracted description for the declaration. Without the tag, the description defaults to the doc-comment prose body (with `@`-prefixed lines stripped and the leading "TypeName " prefix removed). |
| `@brief` | inline text | class only | Overrides the one-line summary that fills `<brief_description>`. Default: the first sentence of the class doc comment. |
| `@tutorial("title", "url")` | two strings | class only | Adds one `<link>` under `<tutorials>`. Repeatable — every occurrence appends one entry. |
| `@deprecated [reason]` | optional inline text | class, method, property, signal, enum type, enum value | Sets the `deprecated="reason"` attribute on the corresponding XML element. Without an argument, the attribute is set to an empty string (Godot still flags the entry as deprecated). |
| `@experimental [note]` | optional inline text | class, method, property, signal, enum type, enum value | Sets the `experimental="note"` attribute. New in Godot 4.3+; entries flagged experimental render with a warning badge in the editor's docs panel. |
| `@since "version"` | inline text | class, method, property, signal, enum type, enum value | Appends `[i]Since: version[/i]` to the description. Godot's schema has no native field, so this renders as a description suffix in BBCode. |
| `@see "ref"` | inline text | class, method, property, signal, enum type, enum value | Appends a "See also: ref" line to the description. `ref` passes through verbatim — Godot's BBCode parser resolves cross-references like `[ClassName]`, `[method foo]`, `[member bar]`. |

The class XML for a registered class is baked into the generated
`<file>_bindings.go` as a `const` string and pushed to Godot at
SCENE init via `editor_help_load_xml_from_utf8_chars_and_len` (4.3+,
editor-only — game-mode runtimes resolve the symbol to nil and the
load is a no-op). No separate XML files in the user's project tree.

```go
// Player is the smoke example's hero class.
//
// @class
// @brief Smoke example class used for the test fixture.
// @tutorial("godot-go README", "https://github.com/legendary-code/godot-go")
// @since 0.1
type Player struct {
    // @extends
    godot.Node

    // @property
    // @description Hit points; reaches 0 → emit `died` and free the node.
    // @see [signal died]
    Health int64
}

// TakeDamage subtracts damage from health and fires died if it falls to 0.
//
// @description Standard hit-points decrement. Negative `amount` heals.
// @see [method heal]
func (p *Player) TakeDamage(amount int64) { /* ... */ }

// LegacyAttack is the pre-1.0 attack mode kept around for save-game
// compatibility.
//
// @deprecated Use TakeDamage with a negative offset instead.
// @experimental Behavior may change once damage types land.
func (p *Player) LegacyAttack() { /* ... */ }
```

---

## Features

### Class registration

Every file with extension code declares one class via `@class`.
The codegen emits a `register<Class>()` function plus an `init()`
that hooks it into `INIT_LEVEL_SCENE` (or `INIT_LEVEL_EDITOR` if
`@editor` is set). One file = one registered class; additional Go
structs in the same file are plain Go types.

The synthesized `<file>_bindings.go` contains:

- A per-class side table (`map[uintptr]*Class`) — Godot hands back
  a small integer handle as `void*`, not a Go pointer (cgo rules
  forbid that), so the framework keeps an id↔instance lookup.
- Cgo trampolines for each registered method, virtual, and signal
  emit.
- `gdextension.RegisterClass` / `RegisterClassMethod` /
  `RegisterClassVirtual` / `RegisterClassProperty` /
  `RegisterClassSignal` calls.
- Field-form property accessors (`Get<Name>` / `Set<Name>`) when
  field-form `@property` is used.

### Inheritance

Single inheritance via `@extends` on an embedded field. The
embedded type provides the parent's methods through Go's struct
embedding; the cgo bridge separately registers the parent name
with Godot's ClassDB at registration time.

Valid parents are:

- Any class from the generated bindings package — about 1023
  engine classes when generated against Godot 4.6's
  `extension_api.json`. Common ones: `godot.Node`, `godot.Node2D`,
  `godot.Node3D`, `godot.RefCounted`, `godot.Resource`,
  `godot.EditorPlugin`.
- Any class from a third-party Go module — another GDExtension's
  user-class package, a shared library, etc. Imported the same way
  any other Go package is. Runtime registration succeeds when the
  parent class is in Godot's ClassDB by the time this class
  registers.

### Methods

Three kinds, distinguished by tags:

- **Instance methods** — no special tag. `func (p *Player) Foo()`
  is the common shape; registered with the snake-cased Go name.
- **Static methods** — `@static` on the doc comment. The receiver
  may be unnamed (`func (Player) Foo()`) or named — Godot sees
  the method as static either way, registered with
  `MethodFlagStatic` so GDScript callers reach it as
  `Player.foo()` without an instance.
- **Virtual overrides** — `@override` on the doc comment.
  Registered via `RegisterClassVirtual` + `_`-prefixed Godot name.
  Used for Godot-defined virtuals like `_process`, `_ready`,
  `_input`.

`@static` and `@override` are mutually exclusive — statics aren't
dispatched virtually.

Lowercase-first methods without `@override` or `@static` are
treated as Go-private helpers and not registered at all —
matching Go's own export conventions.

Methods with an unnamed receiver (`func (T) Foo()`) require
`@static` explicitly. The codegen used to silently classify them
as static; the explicit tag avoids surprising readers and keeps
intent visible in the source.

Method args and returns must be from the
[supported types list](#supported-types). At most one return value
(Godot has a single return slot). Vararg methods aren't supported
on the user-class boundary (the framework's bindings package has
vararg engine methods, but you can't expose your own).

### Properties

Two surface forms, equivalent at the GDScript layer:

- **Field form**: an exported field tagged `@property` (and
  optionally `@readonly`). The codegen synthesizes `Get<Name>` /
  `Set<Name>` methods that read/write the field, then registers
  them. `@readonly` drops the synthesized setter. Field form is
  the only one that accepts inspector hints (`@group`,
  `@subgroup`, `@export_*`).
- **Method form**: the user writes `Get<Name>` (and optionally
  `Set<Name>`), tagging both halves with `@property`. The user
  owns the storage — useful when the property is computed,
  validated on write, or backed by something other than a struct
  field. Method form doesn't accept export hints.

A property name appearing in both forms (an exported field
`Foo` AND a `GetFoo` method) is rejected at codegen time.

### Signals

`@signals`-tagged interfaces are signal contracts. Each interface
method becomes a Godot signal on the file's `@class`, and the
codegen synthesizes a typed `func (*Class) Name(args...)` emit
method on the class. Use those emitters from inside class methods
when state changes; GDScript subscribers use Godot's standard
Signal API to connect.

Signal arg types are resolved through the same supported-types
table as method args, so adding a new supported arg type lights it
up for signals too.

### Runtime helpers

`godot/runtime/` (the sub-package the bindgen emits) provides the
user-facing convenience layer:

| Function | Purpose |
|---|---|
| `runtime.IsEditorHint() bool` | Mirrors GDScript's `Engine.is_editor_hint()`. True when running inside the Godot editor's edit-time context, false in deployed builds and in play-mode runs. Cheap — Engine singleton resolved once and cached. |
| `runtime.Print(args...)` | Logs to Godot's Output dock as a warning. Captures caller info (function/file/line) automatically. |
| `runtime.Printf(format, args...)` | Same as `Print` but with a format string. |
| `runtime.Printerr(args...)` / `runtime.Printerrf(...)` | Logs as an error. |
| `runtime.IsMainThread() bool` | Whether the calling goroutine is on Godot's main thread. |
| `runtime.RunOnMain(f func())` | Schedules `f` to run on the engine's main thread. Inline if already on main; queued otherwise. |
| `runtime.DrainMain()` | Executes everything queued via `RunOnMain`. Must be called from the main thread — typically from a `_process` override. |

[`docs/threading.md`](./threading.md) covers the threading model
in depth.

### Refcounted lifecycle

Engine classes that derive from `RefCounted` (`Resource`, `Image`,
`RegEx`, …) carry a Go finalizer the bindgen attaches to the
wrapper. When the GC marks the wrapper unreachable, the finalizer
calls `Unreference()` on the engine main thread (via
`RunOnMain`), so the engine reference drops without user code
needing to track it.

If you want deterministic cleanup (e.g., releasing a large
`Image`), call `Unreference()` directly — the finalizer becomes
harmless once the refcount hits zero.

[`docs/lifecycle.md`](./lifecycle.md) has the full ownership
model and the cases where explicit `Unreference` matters.

---

## GDExtension manifest

Godot loads your built library through a `.gdextension` config
file dropped into the project's filesystem. The framework's
`gdextension/` package exports the entry symbol Godot looks for
(`gdextension_library_init`), so importing the package — directly
or transitively, via your bindings package — is all that's needed
on the Go side. No C glue, no manually-defined `init` function.

A minimal `.gdextension` looks like:

```ini
[configuration]
entry_symbol = "gdextension_library_init"
compatibility_minimum = "4.4"

[libraries]
linux.x86_64   = "res://bin/myextension.so"
macos          = "res://bin/myextension.dylib"
windows.x86_64 = "res://bin/myextension.dll"
```

The fields:

| Key | Purpose |
|---|---|
| `entry_symbol` | The C function Godot calls to initialize the extension. Always `gdextension_library_init` for godot-go projects — the framework provides it. |
| `compatibility_minimum` | Lowest Godot version your build supports. Godot refuses to load the extension on older runtimes. The framework's floor is `"4.4"`; set this to whatever your `extension_api.json` and tested versions actually support. |
| `compatibility_maximum` | Optional upper bound. Useful when you've validated only specific versions and want Godot to refuse newer ones until you bump it. The framework's own examples leave this off (no upper bound). |
| `[libraries]` | Per-platform paths to the built c-shared library. Keys are `<os>.<arch>` (e.g., `linux.x86_64`, `windows.x86_64`); macOS uses just `macos` for universal binaries. Paths use Godot's `res://` URI scheme — relative to the project root. |

The `.gdextension` file must live somewhere under your Godot
project's filesystem (not under `.godot/`); the editor scans for
them on import. Naming convention is `<extension_name>.gdextension`,
matching the binary's basename.

For working examples that load and verify end-to-end in CI, see
[`examples/smoke/godot_project/godot_go_smoke.gdextension`](../examples/smoke/godot_project/godot_go_smoke.gdextension)
and the matching files in
[`examples/locale_language/`](../examples/locale_language/) and
[`examples/2d_demo/`](../examples/2d_demo/).

## Supported types

The user-class boundary (method args/returns, signal arguments,
property types) currently accepts:

### Scalar types

| Go type | Wire form | Notes |
|---|---|---|
| `bool` | `Variant::BOOL` | one-byte bool on the wire |
| `int` | `Variant::INT` | int64 on the wire |
| `int32` | `Variant::INT` | int64 on the wire (narrows on read, widens on write) |
| `int64` | `Variant::INT` | int64 on the wire |
| `float32` | `Variant::FLOAT` | always double-width on the Variant wire regardless of build config |
| `float64` | `Variant::FLOAT` | as above |
| `string` | `Variant::STRING` | transparent — internally crosses as Godot's `String`/`StringName`/`NodePath` opaque type, but the user never sees it |
| user-defined `int` enum types | `Variant::INT` | e.g., `type Mode int` declared in the same file. Wire type is int64; the user-facing arg/return uses the typed name for ergonomics. Tagged with `@enum` / `@bitfield`, the enum's class identity flows through `ArgClassNames` / `ReturnClassName` so the editor docs panel shows the typed-element identity. |
| `*<MainClass>` | `Variant::OBJECT` | self-reference to the file's `@class` struct. Codegen looks up the engine `ObjectPtr` in a per-class side table; instances not constructed by this extension fail with `CallErrorInvalidArgument` (see [Foreign-instance handling](#foreign-instance-handling)). |
| `*<bindings>.<EngineClass>` | `Variant::OBJECT` | borrowed view of any engine class from the bindings package (`*godot.Node`, `*godot.Resource`, etc.). Wrapper construction is inline (`&<bindings>.<Class>{}` + `BindPtr`); no refcount management. RefCounted-derived classes work but lifecycle is the caller's responsibility — see [RefCounted borrowed views](#refcounted-borrowed-views). |

### Slice and variadic types

Go slices (and the variadic `...T` shorthand for them) cross the
boundary as either a `Packed<X>Array` (efficient, contiguous storage
for primitive elements) or `Array[T]` (TypedArray, when no Packed
form exists or when element identity needs to flow):

| Go type | Wire form | Notes |
|---|---|---|
| `[]bool` | `Array[bool]` | no `PackedBoolArray` exists in Godot |
| `[]byte` | `PackedByteArray` | wire elements are int64; codegen narrows on read, widens on write |
| `[]int32` | `PackedInt32Array` | wire elements are int64; narrowing/widening cast at the boundary |
| `[]int` / `[]int64` | `PackedInt64Array` | direct |
| `[]float32` | `PackedFloat32Array` | direct under single-precision builds |
| `[]string` | `PackedStringArray` | direct |
| `[]<UserEnum>` / `[]<UserInt>` | `PackedInt64Array` | Godot has no enum-typed Array at runtime — `Array[<EnumName>]` in GDScript is compile-time sugar over `Array[int]`, and `set_typed(TYPE_INT, class_name, ...)` is rejected. The enum class identity still flows through `ArgClassNames` / `ReturnClassName` and the XML `enum=` attribute, so the editor docs panel renders the typed-element identity correctly. |
| `[]*<MainClass>` | `Array[<MainClass>]` | TypedArray of `OBJECT` with class_name set — the one filtered Array shape Godot supports natively at runtime. Per-element foreign-instance check applies. |
| `[]*<bindings>.<EngineClass>` | `Array[<EngineClass>]` | TypedArray of `OBJECT` with class_name set. Borrowed-view per-element wrapping. |
| `...T` (variadic, any supported `T`) | same as `[]T` | identical at the wire boundary. The Go method body sees `[]T`; the dispatch site spreads with `argN...` so the variadic shape works at the call site. |

#### Foreign-instance handling

When a method takes `*<MainClass>` or `[]*<MainClass>`, the codegen
recovers the Go wrapper from the engine `ObjectPtr` Godot passes
through the variant slot. Lookup happens in a per-class side table
the framework maintains (populated by the `Construct` hook).
Instances Godot got from another extension or constructed without
going through that hook fail the lookup. The codegen's response:

- **Call path**: returns `CallErrorInvalidArgument`. The user
  method body never runs.
- **PtrCall path**: bails without invoking the user method (writes
  a zero return slot if the method has a return type).

If you need to accept arbitrary `MyNode`-typed instances (rare —
typically only relevant when multiple extensions co-declare a
class), reach for `*<bindings>.<EngineClass>` instead, which uses
borrowed-view semantics.

#### RefCounted borrowed views

For `*<bindings>.<EngineClass>` arg/return types where the class
derives from `RefCounted` (`Resource`, `Image`, `RegEx`, `JSON`,
…), the wrapper is a borrowed view: the codegen constructs it
inline via `BindPtr` and does **not** bump the refcount. The
caller (Godot, GDScript, another extension) owns the underlying
engine pointer's lifecycle.

If your method body retains the wrapper past the method scope —
storing it on the `@class` struct, capturing it in a goroutine —
call `Reference()` on the wrapper yourself to extend the lifetime,
and `Unreference()` when you're done. The framework handles this
automatically only for wrappers constructed via the bindings'
`New<Class>()` factories, not for borrowed views.

For non-RefCounted classes (`Node`, `Node2D`, most of the engine),
the engine pointer's lifecycle is owned by the scene tree; borrowed
views just work without lifecycle care.

### Currently unsupported

- `[]float64` — under single-precision bindings (the framework
  default) the float Packed types narrow at the boundary; bridging
  cleanly needs a `RealT` typedef in the bindings.
- Bare cross-package types like `godot.Variant`, `godot.Vector2`,
  `godot.Array` — only the pointer form `*<bindings>.<EngineClass>`
  is recognized for cross-package types today.
- Cross-file user classes — `*<OtherUserClass>` only works for the
  file's own `@class`; you can't cross-reference a class from a
  different file in the same package.
- Nested slices `[][]T`, maps, channels, function types, untyped
  interfaces — the codegen rejects each with a clear `file:line`
  message.

For `@extends`, any engine class from the generated bindings
package is valid as the parent type — the bindgen emits all 1023+
classes from `extension_api.json#classes`.

---

## Multi-version targeting

The framework supports Godot **4.4, 4.5, and 4.6**. CI runs the
full matrix (3 OS × 3 Godot versions = 9 cells) on every push.

Pick the version your project targets by dropping the matching
`extension_api.json` into your `godot/` directory. The framework's
own tree commits all three (`extension_api_4_4.json`,
`extension_api_4_5.json`, `extension_api_4_6.json`) sourced from
[godot-cpp's version branches](https://github.com/godotengine/godot-cpp/branches).

The `gdextension/` cgo bridge ships pinned to Godot 4.6's
`gdextension_interface.h` but resolves a few 4.5+ / 4.6+ symbols
with fallbacks to older variants:

| Symbol | Since | Fallback | What's lost on the fallback |
|---|---|---|---|
| `get_godot_version2` | 4.5 | `get_godot_version` (4.1) | `version_full_name` build label, hex-packed version, status/build/hash/timestamp metadata. Major/Minor/Patch/String are present in both. |
| `classdb_register_extension_class5` | 4.5 | `classdb_register_extension_class4` (4.4) | Nothing — the 4.5 entry point takes a struct that's a typedef of the 4.4 struct; the function-pointer rename is a Godot-internal semantic flag, not a payload change. |
| `mem_alloc2` / `mem_realloc2` / `mem_free2` | 4.6 | (none) | These wrappers exist for completeness but the framework doesn't call them in any code path; the host logs a probe-time warning on 4.4/4.5 hosts but no crash follows. |

Version drift between bindings and host is checked on the first
engine→Go callback: if the host's reported (Major, Minor) doesn't
match the bindings' compile-time view, a warning lands in the
Output dock. Patch differences (4.6.1 vs 4.6.2) are tolerated —
Godot's GDExtension ABI is patch-stable.

`compatibility_minimum` in your example's `.gdextension` config
sets the Godot floor at load time. The framework's own examples
ship with `"4.4"`; lower it further at your own risk and bring it
up to your actual minimum supported version.

### Float precision

godot-go targets Godot's **single-precision** build by default
(`extension_api.json`'s `float_32` configuration — `real_t` is
`float`). To build for a double-precision Godot, regenerate the
bindings with `-build-config float_64` (already the default —
single precision uses `float_64` configuration on 64-bit
pointers) or `-build-config double_64`, and pass
`-tags godot_double_precision` to your `go build`:

```sh
go run github.com/legendary-code/godot-go/cmd/godot-go-bindgen \
  -api ./godot/extension_api_4_6.json -build-config double_64
go build -tags godot_double_precision -buildmode=c-shared ...
```

Mixed-precision binaries aren't supported — the host Godot's float
build must match what the bindings + the build tag say.
