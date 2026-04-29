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
- [Features](#features)
  - [Class registration](#class-registration)
  - [Inheritance](#inheritance)
  - [Methods](#methods)
  - [Properties](#properties)
  - [Signals](#signals)
  - [Runtime helpers](#runtime-helpers)
  - [Refcounted lifecycle](#refcounted-lifecycle)
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
| `@class` | one per file | none | Marks this struct as the file's main extension class. The codegen registers it with Godot's ClassDB. Mutually exclusive with `@innerclass`. |
| `@innerclass` | optional | none | Marks the struct as a nested type the user wants to record but NOT register with Godot. Useful for grouping related types in one file. Inner classes can't be instantiated from GDScript. |
| `@abstract` | optional | none | Only valid on `@class` structs. Sets `is_abstract` on the registration so GDScript can't instantiate the class directly — only subclasses. Equivalent to GDScript's `class_name` + `abstract`. |
| `@editor` | optional | none | Only valid on `@class` structs. Registers the class at `INIT_LEVEL_EDITOR` instead of `INIT_LEVEL_SCENE`, so it's invisible to deployed game builds. Use for `EditorPlugin` subclasses, custom inspectors, gizmos, etc. |
| `@description` | optional | inline text | Only valid on `@class` structs. Overrides the auto-synthesized class description (which strips the type-name prefix from the leading doc-comment sentence). Used for the docs XML. |

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

// Helper is recorded but not registered with Godot.
//
// @innerclass
type Helper struct{}
```

### On an embedded field of a `@class` struct

| Tag | Required? | Argument | What it does |
|---|---|---|---|
| `@extends` | exactly one per `@class` struct | none | Marks the embedded type as the parent class. Single-inheritance only — Godot's model. The embedded type must be a class from your bindings package (e.g., `godot.Node`, `godot.Node2D`, `godot.RefCounted`). |

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
```

Method classification rules at a glance:

| Receiver | Tag | Outcome |
|---|---|---|
| `func (p *T) Foo()` | none | regular instance method (Godot name `foo`) |
| `func (p *T) Foo()` | `@override` | virtual override (Godot name `_foo`) |
| `func (T) Foo()` | none | static method |
| `func (p *T) foo()` | none | NOT registered — Go-private helper |
| `func (p *T) foo()` | `@override` | virtual override (Godot name `_foo`) |

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

---

## Features

### Class registration

Every file with extension code declares one main class via
`@class`. The codegen emits a `register<Class>()` function plus an
`init()` that hooks it into `INIT_LEVEL_SCENE` (or
`INIT_LEVEL_EDITOR` if `@editor` is set). Inner classes
(`@innerclass`) are recorded but not registered with Godot's
ClassDB.

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

Valid parents are any class from the generated bindings package —
about 1023 engine classes when generated against Godot 4.6's
`extension_api.json`. Common ones: `godot.Node`, `godot.Node2D`,
`godot.Node3D`, `godot.RefCounted`, `godot.Resource`,
`godot.EditorPlugin`.

Cross-module class inheritance (extending a class from another
extension) isn't supported.

### Methods

Three kinds, distinguished by receiver shape and tags:

- **Instance methods** — `func (p *Player) Foo()`. The most common
  shape. Registered with the snake-cased Go name.
- **Static methods** — `func (Player) Foo()` (unnamed receiver).
  Registered with `MethodFlagStatic` so GDScript can call
  `Player.foo()` without an instance.
- **Virtual overrides** — any method with `@override`. Registered
  via `RegisterClassVirtual` + `_`-prefixed Godot name. Used for
  Godot-defined virtuals like `_process`, `_ready`, `_input`.

Lowercase-first methods without `@override` are treated as
Go-private helpers and not registered at all — matching Go's own
export conventions.

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

## Supported types

The user-class boundary (method args/returns, signal arguments,
property types) currently accepts:

| Go type | Notes |
|---|---|
| `bool` | one-byte bool on the wire |
| `int` | int64 on the wire |
| `int32` | int64 on the wire (narrows on read, widens on write) |
| `int64` | int64 on the wire |
| `float32` | double on the wire (Godot's FLOAT is always double-width regardless of build config) |
| `float64` | double on the wire |
| `string` | transparent — internally crosses as Godot's `String`/`StringName`/`NodePath` opaque type, but the user never sees it |
| user-defined `int` enum types | e.g., `type Mode int; const ( Idle Mode = iota; ...)` declared in the same file. Wire type is int64; the user-facing arg/return uses the typed name for ergonomics. |

Outside this list — `Vector2`, `Array`, engine-class pointers,
`Variant`, packed arrays, etc. — the codegen rejects with a
file:line error. Calling INTO these types from your extension code
works fine (the bindings package provides them); you just can't
expose your own methods that take/return them yet.

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
