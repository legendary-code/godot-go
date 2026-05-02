# Arrays & TypedArrays at the @class boundary

Plan for letting user-class methods take and return Go slices (`[]T`),
mapping to Godot's `Array` / `Packed<X>Array` / `Array[T]` (TypedArray)
at the GDExtension ABI.

Status: planning. No code shipped. Each phase below is a separate
batch — commit + green CI before starting the next.

---

## Goals

- Method args and returns can be slices: `[]int64`, `[]float64`,
  `[]string`, `[]bool`, `[]<UserEnum>`, `[]*<UserClass>`,
  `[]*<EngineClass>`.
- Element types reuse the existing primitive / enum / class
  resolution paths — slices are a wrapper around a single element
  type, not a new family.
- The Godot side sees the idiomatic Godot type: `PackedInt64Array`
  for primitive slices, `Array[T]` (TypedArray) where typed-element
  identity matters or no Packed variant exists.

## Non-goals (v1)

- Nested slices (`[][]T`).
- Untyped `Array` (heterogeneous Variant elements). Users who need
  this construct `godot.Array` directly.
- `map[K]V` → Godot Dictionary. Separate feature.
- Slices of `Vector2` / `Vector3` / `Color` / `Vector4` — those
  scalar types aren't supported at the @class boundary yet, so
  their slice forms come along whenever the scalars do.
- Variadic methods on the user-class boundary (today's blanket no).

---

## Type mapping

| Go type             | Godot type             | Element ABI                      |
|---------------------|------------------------|----------------------------------|
| `[]byte`            | `PackedByteArray`      | packed bytes                     |
| `[]int32`           | `PackedInt32Array`     | packed int32                     |
| `[]int` / `[]int64` | `PackedInt64Array`     | packed int64                     |
| `[]float32`         | `PackedFloat32Array`   | packed float32                   |
| `[]float64`         | `PackedFloat64Array`   | packed float64                   |
| `[]string`          | `PackedStringArray`    | packed String                    |
| `[]bool`            | `Array[bool]`          | TypedArray (no PackedBoolArray)  |
| `[]<UserEnum>` (tagged) | `Variant::ARRAY` | untyped Array + `PROPERTY_HINT_TYPE_STRING` payload `"2/2:VAL1,VAL2,..."`. Godot's autocomplete renders bare `Array` (the hint is property-only); we ship the hint anyway in case Godot's renderer evolves. |
| `[]<UntaggedUserInt>` | `PackedInt64Array` | no enum identity to preserve; faster runtime. |
| `[]*<UserClass>`    | `Array[<UserClass>]`   | TypedArray, Object elements, user class_name |
| `[]*<EngineClass>`  | `Array[<EngineClass>]` | TypedArray, Object elements, engine class_name |

Class element types are **pointer-only**. Reasoning: engine wrappers
and `@class` structs already use pointer semantics everywhere; pointer
elements give us nilability (TypedArray<Object> elements can be null)
which value semantics would lose.

---

## User-facing helpers

Generated, but appear in user code. Conventions:

- **Packed<X>Array.** `NewPacked<X>Array(values ...X) Packed<X>Array`
  and `(p Packed<X>Array) ToSlice() []X`. Variadic constructor lets
  callers spread a slice (`NewPackedInt64Array(s...)`) or pass
  literals.
- **Array (TypedArray).** `NewArray<X>(values ...X) Array` and
  `(a Array) To<X>Slice() []X`. One pair per element type. For
  primitive / engine-class element types these live in the bindings
  package (one per type, generated). For user classes / enums, the
  codegen synthesizes the matching pair in the user's `_bindings.go`
  since those types live in the user's package.

---

## Phases

Each phase is a separate landing — green CI before starting the next.
The "exit gate" describes what has to be true before the phase
counts as done.

### Phase 1 — Bindgen: Variant↔Array marshaling

**Scope.** Generate `VariantAs<X>Array` / `VariantSet<X>Array` and
`PtrCallArg<X>Array` / `PtrCallStore<X>Array` for `Array` and every
`Packed<X>Array`. Pure mechanical extension of the existing
runtime.gen.go pattern (the bool/int/string variants).

**Files.** `cmd/godot-go-bindgen/` — likely a new emitter that
walks the array-type list and produces the helpers. Output lands in
the user's bindings package as a generated file.

**Exit gate.** Bindings package compiles. New helpers are reachable
from user code; no codegen path uses them yet. Existing tests stay
green.

**Open.** None.

---

### Phase 2 — Bindgen: slice ↔ Packed<X>Array

**Scope.** Generate `NewPacked<X>Array(values ...X) Packed<X>Array`
and `(p Packed<X>Array) ToSlice() []X` for each Packed type.
Element pump uses the `Get` / `PushBack` / `Resize` builtin methods
the array generator already emits.

**Files.** Same emitter as Phase 1, additional output.

**Exit gate.** A hand-written test (cmd/godot-go-bindgen test, no
runtime needed) round-trips a Go slice → Packed<X>Array → Go slice
for each primitive type and asserts equality.

**Open.** Performance — `PushBack` per element is fine for small
arrays. If hot-path users hit scale issues, fast-path via
`Resize` + direct memory write is a follow-up; not blocking v1.

---

### Phase 3 — Bindgen: slice ↔ TypedArray construction

**Scope.** Build `NewArrayBool(values ...bool) Array` and the matching
`(a Array) ToBoolSlice() []bool` — uses the
`Array(base, type, class_name, script)` constructor (ctor 4 in
array.gen.go) to set the typed-element filter on construction. For
engine class elements, generate the same pair per engine class:
`NewArrayNode(values ...*Node) Array`, `(a Array) ToNodeSlice() []*Node`,
etc. Yes, that's ~1023 pairs — same scale we already emit for engine
classes.

**Files.** Bindgen emitter — pulls the engine-class list from
extension_api.json#classes (the same list that drives the
existing per-class generators).

**Exit gate.** Round-trip test for `Array[bool]`. Round-trip test for
`Array[Node]` (or any engine class) — verify that constructing,
filling, reading back yields the same `*Node` pointers.

**Open.**
- Type id for the `Array(base, type, class_name, script)` ctor: I
  think it's `Variant.Type` (an int corresponding to BOOL / INT /
  OBJECT / etc.) — needs verification by reading the constructor's
  hash signature in extension_api.json.
- Double-check: does `Array[T]` need the script param non-null? My
  read is no for engine/user classes — script is for GDScript-defined
  types only.

---

### Phase 4 — Codegen: scalar slice support at the @class boundary

**Scope.** Extend `cmd/godot-go/types.go`:

- `resolveType` recognizes `*ast.ArrayType` (Go slice). Recursively
  resolves element type via the existing path.
- For each supported element type, build a slice `typeInfo` whose
  fragments call into the Phase 1–3 helpers:
  - `CallReadArg`: `arg<i> := <bindings>.<helper>(args[<i>]).ToSlice()`
  - `CallWriteReturn`: `<bindings>.VariantSet<X>Array(ret, <bindings>.NewPacked<X>Array(<expr>...))`
  - PtrCall variants similar.

Update `cmd/godot-go/emit.go::godotXMLType` so docs render
`PackedInt64Array` etc. for the `<return type=...>` and
`<param type=...>` slots. For TypedArray, `<return type="Array">` with
extra metadata? — TBD via Phase 3 verification.

**Files.** `cmd/godot-go/types.go`, `cmd/godot-go/emit.go`,
`cmd/godot-go/discover_test.go` for new test coverage.

**Exit gate.** A test method on the locale_language example that
takes `[]int64` and returns `[]string`, dispatched via
`class_call_static` from `test_locale_language.gd`, round-trips.

**Open.**
- XML representation for TypedArray. Godot's class.xsd uses a
  `type="Array"` with optional attributes for typed elements; need
  to mirror exactly so the editor renders the typed-element form.

---

### Phase 5 — Codegen: enum slice support

**Scope.** `[]<UserEnum>` element types. Underlying ABI is
`PackedInt64Array`-or-`Array[int]` — actually, since enums need their
identity preserved for the editor, **TypedArray with int64 elements
and the enum's class_name set**. Not Packed — Packed loses the enum
identity.

The codegen synthesizes per-class `NewArray<EnumName>(values ...EnumName) <bindings>.Array`
and `Array<EnumName>ToSlice(a <bindings>.Array) []EnumName` helpers in
the user's `_bindings.go` (free functions, not methods on Array — Go
doesn't allow methods on third-package types).

**Files.** `cmd/godot-go/emit.go` (helper synthesis),
`cmd/godot-go/types.go` (slice typeInfo for user-enum elements).

**Exit gate.** A static method on `LocaleLanguage` returns
`[]Language`; GDScript-side check via `class_call_static` confirms
each element comes back as a typed `Language` value with the right
class identity.

**Open.**
- Free-function naming: `Array<X>ToSlice(a)` reads OK but isn't
  symmetric with `(a Array) ToBoolSlice()`. Alternative:
  `<X>SliceFromArray(a)`. Defer; settle when writing.

---

### Phase 6 — Codegen: class slice support

**Scope.** `[]*<UserClass>` and `[]*<EngineClass>`.

**Engine class elements.** Phase 3 already emits the
`NewArray<EngineClass>(values ...*<EngineClass>) Array` helpers per
class. Codegen wires `[]*<EngineClass>` to those.

**User class elements.** Codegen synthesizes the per-class
`NewArray<UserClass>(values ...*<UserClass>) Array` and the matching
ToSlice in the user's `_bindings.go`. Marshaling rules:
- *UserClass → ObjectPtr: existing `n.Ptr()` access (already used by
  embedded engine wrappers).
- ObjectPtr → *UserClass: side-table lookup
  `lookup<UserClass>Instance(ptr)` (already exists for instance
  resolution in trampolines).

**Cross-extension user classes.** A `[]*<OtherExtensionClass>`
element type works the same way as cross-extension `@extends`: the
type comes from an imported package, and at runtime the parent
extension must have registered first.

**Files.** `cmd/godot-go/types.go`, `cmd/godot-go/emit.go`.

**Exit gate.** Smoke or 2d_demo example exposes a method that
returns `[]*core.Node` (engine elements) and another that returns
`[]*<some user class>` (user elements); GDScript driver verifies
both.

**Open.**
- ObjectPtr ↔ *UserClass round-tripping when the array contains
  instances NOT created by us (e.g., GDScript constructs a
  TypedArray of our user class and passes it in). The side-table
  lookup returns nil for those — what's the right behavior? Probably
  return a wrapper that has the engine ptr but no Go-side state;
  needs design.

---

### Phase 7 — Validation + errors

**Scope.** Reject unsupported combinations with `file:line` errors:
- `[][]T` — nested
- `[]<map>`, `[]<func>`, `[]<channel>` — nonsense at the boundary
- `[]<unsupported scalar>` — element type not in the table
- `[]Variant` — deferred (point at the workaround of using `Array`
  directly)

**Files.** `cmd/godot-go/types.go::resolveType`, error paths in
`cmd/godot-go/discover.go` if any catch-all is needed.

**Exit gate.** New tests in `discover_test.go` for each rejection
shape.

**Open.** None.

---

### Phase 8 — Docs + example coverage

**Scope.** Update `docs/reference.md` supported-types table with
the array forms and a short worked example. Add slice-arg / slice-
return surface to `examples/locale_language` (the showcase) and a
GDScript driver assertion.

**Files.** `docs/reference.md`, `examples/locale_language/...`

**Exit gate.** docs build, locale_language driver passes
end-to-end, full CI matrix green.

---

## Phase 4 deferral: `[]float64`

Phase 4 ships seven slice element types: `bool`, `byte`, `int32`,
`int`, `int64`, `float32`, `string`. `[]float64` is deferred because
it requires a precision-aware codegen path:

- Under single-precision bindings (the framework default),
  `Packed<X>Array.PushBack` takes `float32` for both float Packed types.
  A `[]float64` boundary would require a lossy narrowing cast in the
  generated code (`PushBack(float32(v))`).
- Under double-precision bindings, the same hardcoded narrow would
  produce a type mismatch — the cast direction depends on the build
  config.

Cleanest fix is to expose a `RealT` type alias from the bindings
(set to `float32` under single-precision, `float64` under double) and
have the codegen use `bindings.RealT(v)` for the float boundary cast.
Tracked as a follow-up; current Phase 4 code rejects `[]float64` with
an "unsupported slice element" error pointing the user at the
limitation.

## Build-config / precision notes

Godot's GDExtension API has four build configurations distinguished by
`<precision>_<arch_int_size>`: `float_32`, `float_64`, `double_32`,
`double_64`. The `<precision>` axis controls Godot's `real_t` typedef
— `float32` for `float_*` builds, `float64` for `double_*` builds. The
framework defaults to `float_64` (single-precision real_t on 64-bit
pointer architecture).

The api.json `"float"` type at the ABI boundary is `real_t`. So:

- **Under `float_*` builds**: `"float"` is float32, methods narrow at the
  boundary. PackedFloat64Array's storage is double internally but the
  ABI surfaces float32 elements — **this is intentional Godot ABI
  behavior**, not a bindgen bug. Both Packed float types end up with
  float32 element types.
- **Under `double_*` builds**: `"float"` is float64. PackedFloat32Array's
  storage is float32 but its ABI signature widens to float64; both Packed
  float types end up with float64 element types.

The bindgen used to hardcode `"float" → float32` regardless of build
config. Fixed during the Phase 2 work via a precision-aware typemap
(setBuildPrecision in typemap.go). Phase 2's float Packed entries pull
their element type from the same source, so they auto-track.

Caveat: only `float_*` builds are exercised by CI today. The
double-precision path is built and emits the correct types but isn't
yet tested end-to-end against a double-precision Godot engine.

## Decisions still open across phases

- **PtrCall vs Call for Array/Packed types.** Existing codegen emits
  both. For arrays, PtrCall passes a `void*` to the typed builtin
  opaque. Simplest is to read it via `*(*Packed<X>Array)(args[i])` —
  works, but refcount semantics on copy need to be checked. Phase 1
  exit gate validates this empirically.
- **Refcount on returned arrays.** When we construct a Packed<X>Array
  in PtrCallWriteReturn and write it into ret, the host expects to
  take ownership. Need to verify by inspecting how the existing
  bindings package's array methods (e.g., `(*Array).Get`) handle
  return ownership.
- **TypedArray XML schema.** Phase 4 open question — verify that
  Godot's class.xsd renders typed-element identity in the docs panel
  the way we'd hope.

---

## Suggested first move

Start with Phase 1. It's pure bindgen, no semantics decisions, and
lights up the marshaling primitives the rest of the plan depends on.
Land it; verify CI; come back here to refine Phase 2.
