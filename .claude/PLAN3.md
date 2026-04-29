# PLAN3 — Editor introspection: arg names, typed enums, docs

The framework currently registers classes, methods, properties, and
signals in shapes Godot can dispatch but can't explain to a user.
Hover a method in the editor and you see `take_damage(arg0: int)`
instead of `take_damage(amount: int)`; F1 a class and you get the
default empty docs page; declare a method that takes a `Mode` enum
and the editor sees a plain `int`.

This plan rebuilds the user-class-codegen → registration path so
each surface carries every piece of metadata Godot's docs and
autocomplete actually want.

## Source of asks

`.claude/FOLLOWUPS.md`. Three concrete items:

1. Generate documentation for GDExtension classes, modeled on
   Godot's existing `<class>` XML schema. Default the description
   to the Go doc comment (minus doctag lines) and let `@description`
   override.
2. Method args should expose the same names as in the Go source.
3. Enum-typed args/returns/properties should surface as the typed
   enum name in the editor's autocomplete and docs.

## Locked-in decisions

| # | Decision | Why |
|---|---|---|
| 1 | XML docs are loaded at runtime via `editor_help_load_xml_from_utf8_chars` (since 4.3, available on the framework's 4.4 floor). | Self-contained — no `[docs]` section in `.gdextension`, no static files in the user's project tree. |
| 2 | Enum exposure is opt-in via a new `@enum` (or `@bitfield`) doctag on the `type X int` declaration. Only marked types are registered as class-scoped enums + cross-referenced from arg/property/return signatures. | Keeps unrelated Go-internal int aliases out of Godot's namespace. Matches the rest of the codegen's "explicit > implicit" convention. |
| 3 | Doctag surface is broad — `@description`, `@brief`, `@tutorial`, `@deprecated`, `@experimental`, `@since`, `@see` — each available everywhere it's semantically valid. | "Don't skim, do them all." |
| 4 | Phasing is A → B → C in a fixed order. | A's `arguments_info` plumbing is a prerequisite for B's `class_name` field. |
| 5 | Doc tags apply to **every declaration site they're meaningful at**: classes, methods, properties, signals, enum types, enum values. No "class-only" carve-outs unless Godot's schema doesn't support it. | Same rationale as #3 — uniform surface. |

## Doctag surface (final)

### New tags introduced by this plan

| Tag | Argument | Where valid | Effect |
|---|---|---|---|
| `@description` | inline text or block | class, method, property, signal, enum type, enum value | Overrides the auto-extracted description for that declaration. Block form: omit the value and let the surrounding doc comment paragraphs become the description. Inline form: `@description One-line replacement.` |
| `@brief` | inline text | class only (Godot's schema has `<brief_description>` only on classes) | Overrides the synthesized one-line summary. Default: first sentence of the class doc comment. |
| `@tutorial("title", "url")` | two strings | class only | Adds a `<link>` to `<tutorials>`. Repeatable. |
| `@deprecated [reason]` | optional string | class, method, property, signal, enum type, enum value | Sets the `deprecated="reason"` attribute on the XML element. Already parsed for classes today (via the broader doctag.Parse) but never emitted; this plan wires it through. |
| `@experimental [note]` | optional string | class, method, property, signal, enum type, enum value | Sets `experimental="note"` on the XML element. New attribute Godot 4.3+. |
| `@since "version"` | string | class, method, property, signal, enum type, enum value | Appends "Since: <version>" to the description block. Godot's schema has no native `since` field, so this renders as a description suffix in BBCode. |
| `@see "ref"` | string | class, method, property, signal, enum type, enum value | Appends a "See also: [ref]" line to the description. Repeatable. `ref` can be a Godot BBCode reference like `[ClassName]`, `[method method_name]`, or `[member property_name]` — passes through verbatim. |
| `@enum` | none | a `type X int / int32 / int64` declaration | Marks the type for class-scoped enum registration. The const block declaring the values gets emitted as `RegisterClassIntegerConstant` calls under the enum's name. References to the type from method signatures populate `class_name` so editor autocomplete shows the enum. |
| `@bitfield` | none | a `type X int / int32 / int64` declaration | Same as `@enum` but registers with `is_bitfield = true`. The constants compose with bitwise OR. Mutually exclusive with `@enum`. |

### Existing tags affected by this plan

| Tag | Change |
|---|---|
| `@description` | Was captured on classes only and not emitted. Now captured on every applicable site and emitted into the XML. |

Everything else (`@class`, `@innerclass`, `@abstract`, `@editor`,
`@extends`, `@property`, `@readonly`, `@override`, `@name`,
`@signals`, `@group`, `@subgroup`, `@export_*`) is unchanged.

## Phase A — Method/signal argument names

**Goal:** the registered method's arg names match the Go source.
After this lands, hovering a method in the Godot editor shows
`take_damage(amount: int)` instead of `take_damage(arg0: int)`.

### Steps

1. **`gdextension/classdb.go`:** add `ArgNames []string` to
   `ClassMethodDef`, `ClassVirtualDef`, `ClassSignalDef`. Same
   length as `ArgTypes` (validated in the registration call).
   Empty entries fall back to `arg<i>` so unmodified callers keep
   working.
2. **`gdextension/shim.h` / `shim.c`:** extend
   `godot_go_register_extension_class_method` (and the signal /
   virtual variants) to take a `const GDExtensionStringNamePtr*
   p_arg_names` array. Populate `arguments_info[i].name` with each
   slot.
3. **`cmd/godot-go/emit.go`:** pre-intern the StringName for each
   arg name, store the resulting `StringNamePtr` slice in a
   `var` block at file scope, pass to the registration call. Use
   `safeIdent()` to rename Go-keyword args (`type` → `typ`, etc.)
   before exposing.
4. **`cmd/godot-go/emit.go` — signals:** existing signal emit code
   already extracts source names; just thread them through to
   `RegisterClassSignal` the same way.
5. **Verification:** smoke + locale_language regenerate; CI matrix
   stays green; manual editor check that hovering a registered
   method shows the typed names.

**Estimated effort:** 1–2 hours.

## Phase B — Typed enums

**Goal:** an enum-typed parameter or property surfaces as the
typed enum name in the editor's docs and autocomplete. Depends on
Phase A's `arguments_info` plumbing.

### Steps

1. **`internal/doctag` (no change needed):** `@enum` and
   `@bitfield` parse through the existing tag parser unchanged —
   no special-casing.
2. **`cmd/godot-go/discover.go`:** record the `@enum` /
   `@bitfield` flag on `enumInfo`. The const-block walker that
   already collects values runs unchanged. Reject `@enum` AND
   `@bitfield` on the same declaration.
3. **`gdextension/classdb.go`:** add `RegisterClassIntegerConstant`
   wrapping the host's
   `classdb_register_extension_class_integer_constant`. Takes
   class, enum, name, value, isBitfield. Emit per-value calls at
   SCENE init.
4. **`gdextension/classdb.go`:** add `ArgClassNames []string` to
   `ClassMethodDef`, `ClassVirtualDef`, `ClassSignalDef`,
   `ClassPropertyDef`. Parallel to `ArgTypes` (or singular for
   `ClassPropertyDef.ClassName`). When emitted, populates
   `GDExtensionPropertyInfo.class_name` with `<MainClass>.<EnumName>`.
5. **Same for return values:** `ReturnClassName string` on the
   method/virtual defs.
6. **`cmd/godot-go/emit.go`:** when `resolveType` returns a user
   enum and that enum is `@enum`-tagged, populate the matching
   `ArgClassNames` / `ReturnClassName` slot. Untagged user int
   types continue to register as plain `int` with no class_name.
7. **`cmd/godot-go/types.go`:** the generated dispatch shim for
   enum-typed args is unchanged from today (read int64, cast to
   the named type) — this phase only adds metadata.
8. **Verification:** add a `@bitfield`-tagged enum to one of the
   examples; verify the generated bindings register the constants
   and the editor displays the typed enum.

**Estimated effort:** 3–4 hours.

## Phase C — Documentation XML

**Goal:** Godot's editor F1 + autocomplete show full descriptions
for classes, methods, properties, signals, and enum values, sourced
from the Go doc comments.

### Steps

#### Discovery side

1. **`cmd/godot-go/discover.go`:** lift `synthDescription` /
   doc-comment capture to also run on each method, property,
   signal, enum type, and enum value. Each `methodInfo`,
   `propertyInfo`, `signalInfo`, `enumInfo`, `enumValue` gains a
   `Description` field (and `Deprecated`, `Experimental`,
   `Since`, `See []string`, etc.).
2. **`cmd/godot-go/discover.go`:** add `Brief` to `classInfo`
   (defaulted from the first sentence of the class doc comment;
   overridable with `@brief`).
3. **`cmd/godot-go/discover.go`:** parse `@tutorial("title", "url")`
   on classes (repeatable, accumulate). The tag's parenthesized
   form is already supported by the doctag parser.
4. **Validation:** `@brief` only on classes; `@tutorial` only on
   classes; `@deprecated` on a class can already be parsed but
   must now feed the XML emit. All errors carry file:line.

#### Description extraction rules

The "default description" for any declaration is its leading doc
comment, with these transformations:

- Lines starting with `@` are dropped (those are doctags).
- Trailing/leading blank lines are collapsed.
- For classes specifically, the first sentence (up to `.` or
  blank line) is the default `<brief_description>`.
- The full multi-paragraph remainder is the default
  `<description>`.
- BBCode (`[b]`, `[code]`, `[param x]`, `[ClassName]`, `[method
  name]`, `[member name]`) passes through unchanged — Godot's
  XML parser handles it.
- `@since "x.y"` appends `[br][i]Since: x.y[/i]` to the
  description.
- `@see "ref"` (each occurrence) appends `[br]See also: <ref>`.

#### XML emit

1. **`cmd/godot-go/xml.go`** (new file): build the
   `<class name="..." inherits="..." version="..." ...>`
   document from the discovered metadata. Use `encoding/xml` —
   define typed structs for every Godot schema element and let
   the encoder produce well-formed output.
2. **Embed in bindings:** the XML string lands as a `const
   classDocXML = "..."` literal in the generated
   `_bindings.go`. UTF-8, escapes done by `encoding/xml`.
3. **`gdextension/`:** add `LoadEditorDocXML(string)` wrapping
   `editor_help_load_xml_from_utf8_chars`. Resolve the symbol
   lazily (it's editor-only — game-mode runtimes return nil from
   `get_proc_address`); a nil pointer is a no-op.
4. **`cmd/godot-go/emit.go`:** at the end of the per-class
   `register<Class>()`, call `LoadEditorDocXML(classDocXML)`. The
   call is cheap and idempotent in editor builds; no-op in game
   builds.

#### Schema coverage

XML elements/attributes the emitter produces:

| Element | Attributes | Source |
|---|---|---|
| `<class>` | `name`, `inherits`, `version`, `deprecated`, `experimental` | `classInfo.Name`, `Parent`, hardcoded `4.4`, `Deprecated`, `Experimental` |
| `<brief_description>` | (text body) | `classInfo.Brief` (or first sentence of doc comment) |
| `<description>` | (text body) | `classInfo.Description` (or full doc comment minus tags, plus `@since`/`@see` suffixes) |
| `<tutorials>` → `<link>` | `title` | `classInfo.Tutorials[]` |
| `<methods>` → `<method>` | `name`, `qualifiers` (static/const), `deprecated`, `experimental` | per-method |
| `<method>` → `<return>` | `type`, `enum` (when typed enum) | per-method return |
| `<method>` → `<param>` | `index`, `name`, `type`, `enum`, `default` | per-method args; `default` only when literally extractable from a Go default arg expression — punt for now if non-trivial |
| `<method>` → `<description>` | (text body) | per-method doc comment |
| `<members>` → `<member>` | `name`, `type`, `setter`, `getter`, `default`, `enum`, `deprecated`, `experimental` | per-property |
| `<signals>` → `<signal>` | `name`, `deprecated`, `experimental` | per-signal |
| `<signal>` → `<param>` | `index`, `name`, `type`, `enum` | per-signal arg |
| `<signal>` → `<description>` | (text body) | per-signal doc comment |
| `<constants>` → `<constant>` | `name`, `value`, `enum`, `is_bitfield`, `deprecated`, `experimental` | per-enum-value (only `@enum`/`@bitfield`-tagged enums) |
| `<constant>` → (text body) | (text body) | per-value doc comment |

`default` on `<param>` and `<member>` is left empty until we
have a clean Go-AST-extractable default. The codegen doesn't
currently support default arg values either — picking that up
is a separate followup.

#### Verification

1. Add multi-paragraph doc comments (with `@deprecated`,
   `@experimental`, `@since`, `@see`, `@tutorial`) to the
   smoke example's `MyNode`. Regenerate.
2. Open the smoke project in Godot's editor (not headless), F1
   on `MyNode`, confirm the docs page renders.
3. Hover a method, confirm description tooltip appears.
4. CI exercises only the load path (no editor running) — the XML
   load call should be a no-op via the nil-resolved
   `editor_help_load_xml_from_utf8_chars` pointer.

**Estimated effort:** 6–10 hours.

## Out of scope (deferred)

- **Default argument values.** Godot's `<param default="..."`>
  is supported but the framework doesn't yet plumb default args
  through the registration ABI. Whole separate workstream.
- **`<theme_items>`.** Only relevant for `Control` subclasses with
  custom theme entries — no current example needs them.
- **`<members>` `default` attribute.** Same default-extraction
  problem as above; the field's Go zero value isn't always what
  Godot considers the "default" (the user might init in
  `_ready`).
- **Localization.** Godot's docs system supports translated
  XML; users who want localized docs can post-process the
  generated XML before shipping. The framework only generates
  the source-language version.
- **Doc inheritance.** Godot's `inherits` tag pulls in
  documentation from the parent class. Our generated XML doesn't
  inject inherited docs — Godot's editor handles that on its
  own when the parent is an engine class.

## Sequencing

A → B → C, one PR (or one commit chain) per phase. Each phase
keeps the matrix green and adds nothing visible to users until
its verification step. C lands the user-visible payoff.

After all three phases close, FOLLOWUPS.md gets archived (the
items are in CI as integration tests on the smoke example, and
the reference doc gets the new tags added in the same PRs).
