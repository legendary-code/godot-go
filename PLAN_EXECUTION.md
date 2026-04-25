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

- [ ] Pin Go toolchain version in `go.mod` (currently 1.26.1, sanity-check
      against the installed toolchain).
- [ ] Add a top-level `Taskfile.yaml` script with targets:
      `generate:types` (regenerate framework code from json), `test`, `example`.
- [ ] Decide cgo build strategy:
      - extension built as a Go `c-shared` `.dll`/`.so`/`.dylib`,
      - `gdextension_interface.h` consumed via cgo `#include`,
      - per-platform `cgo` flags captured in build tags.
- [ ] Decide build configurations matrix (`float_32`, `float_64`,
      `double_32`, `double_64`) — pick `float_32` as MVP, gate the others
      behind build tags.
- **Exit:** module builds an empty `c-shared` library on Windows that loads
  in Godot and prints from `GDExtensionInit`.

### Phase 1 — GDExtension C interface (host ABI)

- [ ] `internal/gdextension/types.go` — Go mirrors of the enums/structs in
      `gdextension_interface.h` (variant types, call errors, init levels,
      property info, method info, etc.). Hand-written; small surface.
- [ ] `internal/gdextension/interface.go` — load function-pointer table:
      every `GDExtensionInterface*` getter is wrapped in a Go func that
      calls through the cached pointer.
- [ ] `internal/gdextension/entrypoint.go` — `//export gdextension_library_init` that
      Godot calls; sets up the interface table, library pointer, init/
      deinit callbacks at each `GDExtensionInitializationLevel`.
- [ ] `internal/runtime/log.go` — print/print_error helpers backed by
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

- [ ] Walk `classes`. For each entry, emit into `core/` or `editor/` based
      on `api_type`. Generated content per class:
      - opaque struct with embedded base (per `inherits`),
      - method bindings via `classdb_get_method_bind` + `object_method_bind_ptrcall`
        (or `_call` for vararg),
      - enums (nested) emitted as `type ClassNameEnumName int` + consts,
      - constants emitted as untyped consts,
      - properties emitted as Get*/Set* method pairs (Godot already exposes
        them as methods; skip synthesizing fields).
- [ ] Resolve cross-package references (a `core` class returning an
      `editor` type — handle via interface or by importing). Default rule:
      `editor` may import `core`, never the reverse.
- [ ] Re-export root types (`Object`, `Node`, `Node2D`, `Node3D`,
      `Resource`, `RefCounted`, `Control`, `CanvasItem`, …) as aliases on
      the `godot/` package.
- **Exit:** call `Engine.singleton().get_version_info()` from Go and assert
  the major version is 4.

### Phase 4 — Singletons, utility functions, native structures, globals

- [ ] `singletons` → `core/singletons.go` lazy accessors (e.g.
      `core.Engine()` returns the cached `*Engine`).
- [ ] `utility_functions` → `util/util.go` (free functions), bound through
      `variant_get_ptr_utility_function`.
- [ ] `global_enums` + `global_constants` → `enums/` (e.g. `enums.Error`,
      `enums.Key`).
- [ ] `native_structures` → `native/` plain Go structs (used in a handful
      of low-level callbacks; layout strings parsed from the json).

### Phase 5 — User-class codegen tool (`cmd/godot-go`)

This is the headline feature. Steps:

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
