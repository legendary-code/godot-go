# godot-go — Execution Plan

This is the working execution plan derived from `PLAN.md`. `PLAN.md` is the
source of truth for goals/UX; this file is the implementation roadmap.
Update this file as decisions land. Do not edit `PLAN.md`.

## 1. Goals (recap)

- Provide a Go framework for authoring Godot 4.6 GDExtensions.
- Author experience: write idiomatic Go structs/methods → run
  `//go:generate godot-go` → all GDExtension registration code is generated
  into a sibling `_bindings.go` file.
- Doc-comment tags (`@abstract`, `@description`, `@deprecated`, `@innerclass`,
  `@name`) drive metadata that can't be expressed in Go syntax.
- Convention over configuration:
  - One file = one main Godot class (the lone struct without `@innerclass`).
  - Embedded Godot type = inheritance.
  - Named-receiver method = instance method; unnamed-receiver = static.
  - `int`-typedef + `const ( … )` block = Godot enum.
  - Lowercase first-letter method `process` → Godot's `_process` (private/
    virtual-style identifier mapping).
  - Default Go camelCase → Godot snake_case, overridable with `@name`.

## 2. Inputs already in tree

- `godot/extension_api.json` — full Godot 4.6.2 API spec (≈350k lines).
- `godot/gdextension_interface.h` — C interface for the host ↔ extension ABI.
- `godot/README.md` — instructions for refreshing those two files.

`extension_api.json` is structured as:
`header`, `builtin_class_sizes` (per `build_configuration`),
`builtin_class_member_offsets`, `global_constants`, `global_enums`,
`utility_functions`, `builtin_classes`, `classes` (engine classes,
each tagged `api_type: "core" | "editor"`), `singletons`,
`native_structures`.

## 3. Repository layout (target)

```
godot-go/
├── go.mod                       # module github.com/legendary-code/godot-go
├── godot/                       # top-level facade package — re-exports
│   │                            # the most common engine + builtin types
│   │                            # (e.g. godot.Node = core.Node)
│   ├── extension_api.json       # input spec (already present)
│   ├── gdextension_interface.h  # C interface (already present)
│   └── *_aliases.go             # generated re-exports
├── internal/
│   ├── gdextension/             # cgo wrapper around gdextension_interface.h
│   │   ├── interface.go         # function-pointer table loader
│   │   ├── types.go             # Go mirrors of GDExtension* C types
│   │   └── entrypoint.go        # //export GDExtensionInit (extension entry)
│   └── runtime/                 # registration + dispatch runtime used by
│                                # generated code (class registry, method
│                                # trampolines, virtual-call routing, etc.)
├── variant/                     # generated: builtin_classes (Vector2,
│                                # String, Variant, Packed*Array, …)
├── core/                        # generated: classes with api_type=="core"
├── editor/                      # generated: classes with api_type=="editor"
├── enums/                       # generated: global_enums + global_constants
├── util/                        # generated: utility_functions
├── native/                      # generated: native_structures
├── cmd/
│   ├── godot-go/                # codegen for *user* extensions
│   │                            # (the //go:generate target)
│   └── godot-go-bindgen/        # codegen for *framework* bindings
│                                # (run by maintainers when the json bumps)
├── examples/
│   └── locale_language/         # the PLAN.md example, kept buildable
└── PLAN.md / PLAN_EXECUTION.md / README.md
```

Decision: builtin_classes don't carry an `api_type`, so place them under
`variant/` rather than forcing an api_type-named package. The PLAN.md
"package named after api_type" rule applies to engine classes; builtin
classes get `variant/` + facade aliases on `godot/`.

## 4. Phases

Phases are roughly ordered by dependency. Each phase ends with a runnable
example or test gate so we never have a long unverified stretch.

### Phase 0 — Bootstrap & decisions

- [x] Pin Go toolchain version in `go.mod` (currently 1.26.1, sanity-check
      against the installed toolchain).
- [x] Add a top-level `Taskfile.yaml` script with targets:
      `generate:types` (regenerate framework code from json), `test`, `example`.
- [x] Decide cgo build strategy:
      - extension built as a Go `c-shared` `.dll`/`.so`/`.dylib`,
      - `gdextension_interface.h` consumed via cgo `#include`,
      - per-platform `cgo` flags captured in build tags.
- [x] Decide build configurations matrix (`float_32`, `float_64`,
      `double_32`, `double_64`) — single-precision `float_64` is the
      live target (matches the Steam build of Godot 4.6.2 we're testing
      against); the other three configs are deferred behind build tags.
- **Exit:** module builds an empty `c-shared` library on Windows that loads
  in Godot and prints from `GDExtensionInit`.

### Phase 1 — GDExtension C interface (host ABI)

- [x] `internal/gdextension/types.go` — Go mirrors of the enums/structs in
      `gdextension_interface.h` (variant types, call errors, init levels,
      property info, method info, etc.). Hand-written; small surface.
- [x] `internal/gdextension/interface.go` — load function-pointer table:
      every `GDExtensionInterface*` getter is wrapped in a Go func that
      calls through the cached pointer.
- [x] `internal/gdextension/entrypoint.go` — `//export gdextension_library_init` that
      Godot calls; sets up the interface table, library pointer, init/
      deinit callbacks at each `GDExtensionInitializationLevel`.
- [x] `internal/runtime/log.go` — print/print_error helpers backed by
      the host's `print` interface (handy for all later phases).
- **Exit:** load extension, log "hello godot-go" at SCENE init level.

### Phase 2 — Variant + builtin types codegen

- [x] `cmd/godot-go-bindgen` reads `extension_api.json`, walks
      `builtin_classes`, emits Go types into `variant/`:
      - opaque value types sized per `builtin_class_sizes` for the chosen
        build config (use `[N]byte` backing arrays; expose typed accessors
        via the host's pointer constructors / member offsets);
      - constructors via `variant_get_ptr_constructor`,
      - methods via `variant_get_ptr_builtin_method`,
      - operators via `variant_get_ptr_operator_evaluator`,
      - to/from-Variant conversions.
- [x] Map primitive Godot types to Go primitives where idiomatic:
      `bool`→`bool`, `int`→`int64`, `float`→`float32` (single-precision
      build; Godot's PtrCall ABI widens to 8-byte `double` at the
      boundary regardless). `String`, `StringName`, and `NodePath` are
      **transparent**: the user-facing API always takes and returns Go
      `string`. Their opaque types exist only for Variant slot storage
      and engine ABI calls — users never see, import, or construct them.
- [x] Generate `godot/aliases.gen.go` re-exports for the high-traffic
      types (Vector2/3/4, Color, Rect2, Transform2D/3D, Variant, String,
      StringName, NodePath, Callable, Signal, Dictionary, Array, Packed*).
- **Exit (met):** the `examples/smoke` extension loads in Godot 4.6.2
  and reports 7/7 passing checks covering Vector2 → Variant → Vector2
  round-trip, the float PtrCall ABI on both call directions, the int
  PtrCall ABI across the 32-bit boundary, the transparent String
  boundary, and `Array.Append` × 3 → `Size() == 3`.

### Phase 3 — Engine class codegen

There are 944 `core` + 79 `editor` classes (1023 total) and 39
singletons. The phase is sliced into four steps to mirror Phase 2's
a/b/c rhythm and keep every commit landable.

#### 3a — ABI shims + hand-bound proof

- [x] Wrap `classdb_construct_object2`, `classdb_get_method_bind`,
      `object_method_bind_ptrcall`, `object_method_bind_call`,
      `object_destroy`, `global_get_singleton` in
      `internal/gdextension` (cgo trampolines + Go wrappers).
- [x] Hand-write the minimum `core.Object` (opaque pointer + Destroy)
      and `core.Engine` with a singleton accessor + `GetVersionInfo()`
      returning a `Dictionary`. This proves the engine-class ABI works
      before automating.
- **Exit:** smoke project calls `Engine.Singleton().GetVersionInfo()`,
  pulls "major" out of the returned `Dictionary`, asserts == 4.

#### 3b — bindgen sweep

- [x] Walk `classes`. For each, emit:
      - opaque struct with embedded base (per `inherits`),
      - method bindings via the 3a shims (vararg via `_call`, fixed via
        `_ptrcall`), using the new lazy-pointer pattern from the start
        (see 3d) so 1k classes don't pay tens of thousands of name
        lookups at load time,
      - enums (nested) → `type ClassNameEnumName int` + consts,
      - constants → untyped consts,
      - properties left as the underlying Get*/Set* method pairs.
      Vararg methods + native-struct pointer args (`const X*`) deferred
      to phases 4/5; virtual methods (user-overrideable) deferred to 5.
- [x] Reuse the Phase 2 argPrep/returnPrep machinery; engine classes
      pass as opaque `Object*` pointers.

#### 3c — facade aliases + cross-package edges

- [x] Resolve cross-package references. Default rule: `editor` may
      import `core`, never the reverse. A `core` method that references
      an `editor` type falls back to an interface or is skipped. Audit
      against extension_api.json: zero core methods reference editor
      types, so the resolveType skip is purely defensive — no fallback
      needed.
- [x] Re-export the root types (`Object`, `Node`, `Node2D`, `Node3D`,
      `Resource`, `RefCounted`, `Control`, `CanvasItem`, …) onto the
      `godot/` package via `godot/aliases.gen.go`. Singleton accessors
      (`EngineSingleton`, etc.) are also re-exported.

#### 3d — lazy method-pointer caching; retrofit Phase 2

- [x] Engine-class bindings already ship lazy from 3b — go back and
      retrofit the same pattern to the `variant/` package. Phase 2
      eagerly binds every constructor / method / operator pointer in
      `init()` because the surface was small (~34 classes); applying
      the same lazy resolver everywhere keeps load time near-zero and
      lets both packages share one strategy. Every cached resolver
      now goes out as `var <name> = sync.OnceValue(...)`; call sites
      use `<name>()`. The generated `init()` + `init<Class>()` pair
      and the `RegisterInitCallback(InitLevelCore, ...)` subscription
      are gone from `variant/*.gen.go`.

- **Phase 3 overall exit:** smoke reports 8/8 passing (Phase 2's seven
  checks plus the Engine version assertion) and load time stays flat
  as the class count grows. **[Met]**

### Phase 4 — Singletons, utility functions, native structures, globals **[Done]**

- [x] `singletons` lazy accessors. Already shipped during 3b/3c as
      `<ClassName>Singleton()` `sync.OnceValue` resolvers in `core/` and
      `editor/`, re-exported on `godot/` by the data-driven alias emitter.
      The PLAN.md form was `core.Engine()` returning `*Engine`; we landed
      on `core.EngineSingleton()` to reserve `core.Engine` for the type
      alias. Functionally equivalent.
- [x] `utility_functions` → `util/util.gen.go` (free functions), bound
      through `variant_get_ptr_utility_function`. 102/114 emitted
      (12 vararg deferred until the user-facing variadic Variant slice
      API). Each fn pointer is a `sync.OnceValue` resolved on first call.
      Smoke gains a `util.Lerpf(0, 10, 0.5) == 5` check for the
      multi-float ptrcall path.
- [x] `global_enums` + `global_constants` → `enums/enums.gen.go`. 22 typed
      `int64` enums (Side, Corner, Key, Error, PropertyHint, MouseButton,
      Variant.Type → VariantType, …) + 512 constants. Bitfields share the
      shape; users compose with `|`. Value names go SCREAMING_SNAKE →
      PascalCase verbatim — no prefix stripping (fragile against shapes
      like Error.OK or KeyModifierMask.KEY_*). `global_constants` is empty
      in 4.6.2 so nothing to emit there.
- [x] `native_structures` → `native/native.gen.go`. All 14 entries
      (AudioFrame, Glyph, ObjectID, PhysicsServer*Extension*, …) emitted
      as plain Go structs with C-layout-stable fields. Format strings
      parsed for primitives, builtins, `Object *` pointers,
      `TextServer::Direction` C++ enums (collapsed to int64), inline
      arrays (`Collisions [32]PhysicsServer3DExtensionMotionCollision`),
      and same-package struct refs (`ColliderId ObjectID`). Layout
      relies on Windows / Linux x86_64 conventions (C `int` → int32);
      noted in the package preamble.

### Phase 5 — User-class codegen tool (`cmd/godot-go`)

This is the headline feature. Steps:

0. **[Done] ABI shims + hand-bound proof (Phase 5a).** `internal/gdextension/classdb.go`
   exposes `RegisterClass` / `RegisterClassMethod` / `UnregisterClass`. The C
   side (`shim.h`/`shim.c`) hosts `godot_go_register_extension_class`,
   `godot_go_register_extension_class_method`, and the
   `create_instance_func` / `free_instance_func` / `get_virtual_func` /
   `method_call_func` / `method_ptrcall_func` trampolines that bridge into
   `//export`'d Go functions (`godotGoCreateInstance`,
   `godotGoFreeInstance`, `godotGoMethodCall`, `godotGoMethodPtrcall`).
   Class/method ids flow through the host's `class_userdata` / `method_userdata`
   void\* slots so Go pointers never sit inside C-visible structs (cgo's
   pointer-storage rule). Smoke now ships a hand-bound `MyNode` extending
   `core.Node`; `examples/smoke/godot_project/test_mynode.gd` runs
   `MyNode.new(); n.hello()` headlessly and confirms `MyNode.Hello()` reaches
   Go. Phase 5b–5f remain — they automate what 5a wired by hand.

1. **Discover input:** when invoked via `go generate`, env vars
   `GOFILE` / `GOPACKAGE` / `GOLINE` identify the trigger file. Parse with
   `go/parser` + `go/ast` + `go/types`.
2. **Identify the main class:** the unique top-level struct with no
   `@innerclass` tag in its doc. Error if zero or >1.
3. **Resolve base class:** the embedded field whose type resolves to a
   generated Godot class (in `godot/`, `core/`, or `editor/`). Error if
   missing or if more than one such embed exists (Godot is single-
   inheritance).
4. **Collect inner classes:** other top-level structs with `@innerclass`.
5. **Collect enums:** `type X int` + `const ( … X = iota … )` blocks where
   the type is `int` (or alias of). Strip the type-name prefix from each
   constant, screaming-snake-case the rest.
6. **Collect methods:** all methods on the main type and inner types.
   Classification:
   - unnamed receiver (`func (LocaleLanguage) Foo()`) → `is_static: true`;
   - lowercase first-letter exported-arity method (`process`) → bind as
     `_process` (Godot virtual-style); detect by checking for an override
     of a known virtual method on the base class.
   - else instance method, name = snake_case unless `@name` overrides.
7. **Doc-tag parser:** dedicated package `internal/doctag` that walks
   `*ast.CommentGroup`s and yields `[]Tag{Name, Value}`. Supported on
   classes (`@abstract`, `@description`, `@deprecated`, `@innerclass`),
   methods (`@name`, `@deprecated`), and enum values (`@deprecated`).
8. **Description synthesis:** from the leading doc comment, drop the
   identifier prefix, uppercase the first surviving word — overridden by
   explicit `@description`.
9. **Emit `<file>_bindings.go`:** a Go file in the same package containing:
   - an `init()` that calls into `internal/runtime` to register the class
     at the right init level,
   - exported cgo-friendly trampolines for each method (signatures derived
     from `go/types` info, marshalling via `variant`/`core` helpers),
   - a class-info table (name, parent, abstract flag, virtual overrides,
     enums, methods, properties).
10. **Type marshalling:** map Go types in user signatures to Godot types
    — primitives, `string` ↔ Godot `String`, generated engine types
    pass-through, slices for `Packed*Array`. Reject unsupported types
    with a clear error pointing at file:line.
- **Exit:** the `examples/locale_language` package builds, exposes
  `LocaleLanguage` to Godot, and `Engine` reports it via `ClassDB`.

### Phase 6 — Lifecycle & runtime polish

- [ ] Property/group/signal annotations (likely `@property`, `@signal`,
      `@group`, `@export` doc tags) — design before implementing.
- [ ] Editor-only behavior (tool scripts) and run-mode gating.
- [ ] Hot-reload story (Godot 4.2+ supports it; verify Go's c-shared can
      participate or document the limitation).
- [ ] Goroutine + Godot main-thread interaction guidance — most engine
      calls must happen on the main thread; codify with a `runtime.Lock`
      helper or doc section.

### Phase 7 — Examples, docs, CI

- [ ] `examples/locale_language` (verbatim from PLAN.md).
- [ ] `examples/2d_demo` — minimal Node2D extension that moves a sprite,
      proves the virtual-method (`_process`) path end-to-end inside Godot.
- [ ] CI matrix: Windows / macOS / Linux × Godot 4.6 headless smoke test
      that loads the extension and exits 0.
- [ ] User-facing README with quickstart.

## 5. Cross-cutting concerns

- **Naming:** centralize Go↔Godot name conversion in one package
  (`internal/naming`) and unit-test the edge cases (acronyms, single-letter
  identifiers, leading underscore for virtuals).
- **Error reporting from codegen:** every diagnostic should carry a
  `token.Position` so editors can jump to the offending line.
- **Determinism:** generated files must be byte-stable (sorted maps, no
  timestamps) so `go generate` is idempotent and diffs stay reviewable.
- **Versioning:** capture `extension_api.json#header.version_full_name`
  in a generated `var GodotAPIVersion = "..."` constant; refuse to load
  if the host reports a mismatched major.minor.

## 6. Open questions

1. Builtin-class storage strategy — opaque `[N]byte` + host pointer ops
   (godot-cpp style) vs. Go-side mirrored layout. Opaque is safer across
   build configs; mirrored is faster. **Tentative: opaque for MVP.**
2. `String` mapping — **Resolved: transparent.** User-facing API uses Go
   `string`; the `variant.String` opaque type exists only for Variant slot
   storage and engine internals. Same for `StringName` and `NodePath`.
3. Virtual-method detection — does the framework need the user to tag
   overrides explicitly, or can it match against the base class's known
   virtual list? **Tentative: implicit match; tag with `@virtual` only
   when the framework can't resolve.**
4. Refcounted ownership — who decrements on Go-side GC? Need a finalizer
   strategy that doesn't deadlock the engine.
5. Build-config selection — single binary per config, or runtime-detected?
   Likely build-time (cgo flags differ).

## 7. Immediate next step

Phase 0: get a `c-shared` build of an empty extension loading in Godot
4.6 with a print-from-init smoke test. Everything downstream depends on
that loop being closed.

## Source Control
After each Phase step, create a commit for those changes.  If we make any
adjustments in a phase, update that commit that was created for that phase
with those changes.
