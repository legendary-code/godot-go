# locale_language extension

The Phase 5f showcase — the verbatim `LocaleLanguage` example from
[PLAN.md](../../PLAN.md), driven entirely by `//go:generate godot-go`.
Compared to the simpler `examples/smoke` extension, this one stresses
the codegen surface that landed across Phase 5b–5f:

| Source feature                                    | Where it shows up                |
| ------------------------------------------------- | -------------------------------- |
| `@abstract` on the main class                     | `IsAbstract: true` on RegisterClass |
| `type Language int` + `iota` const block          | recognized as a user enum, accepted as an `int64`-backed return type |
| `@static` on `Parse`                              | bound as a Godot static method (`MethodFlagStatic`) — callable as `LocaleLanguage.parse(...)` from GDScript |
| `@name do_something_alt_name` on `DoSomething`    | binding name overridden — `do_something` is *not* registered |
| `@override` on `Process`                          | bound as the engine virtual `_process` |

## Build

```sh
task build:locale_language
```

The task installs `godot-go`, runs `go generate` to produce
`locale_language_bindings.go`, and emits the c-shared DLL into
`godot_project/bin/`.

## First-time setup

Godot only auto-loads `.gdextension` files after the project has been
imported once. From this directory:

```sh
godot --headless --editor --path godot_project --quit-after 5
```

(One-shot. Creates `godot_project/.godot/`. Subsequent runs don't need
this.)

## Verify

```sh
godot --headless --path godot_project --script test_locale_language.gd
```

The driver pokes `ClassDB` rather than using `LocaleLanguage.parse(...)`
syntax — GDScript's parser only resolves *non-abstract* extension
classes as type-name identifiers, so the literal `ClassName.method()`
form errors at parse time on `@abstract` classes. `ClassDB.class_exists`,
`class_has_method`, and `class_call_static` all work regardless.

Expected output ends with:

```
test_locale_language: ALL CHECKS PASSED
```
