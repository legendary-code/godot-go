extends SceneTree

# Phase 5f verification — exits Phase 5 when ClassDB reports the
# generated class with the right shape: registered with the right
# parent, has the @name-renamed instance method under
# do_something_alt_name (not do_something), has the static parse
# method, and has the _process virtual override. The static method's
# Go-side enum return round-trips through ClassDB.class_call_static.
#
# We talk to ClassDB rather than using `LocaleLanguage.parse(...)`
# script-level syntax: GDScript's parser only resolves non-abstract
# extension classes as type-name identifiers, so the literal
# ClassName.method() form errors at parse time on @abstract classes.
#
#   godot --headless --path examples/locale_language/godot_project --script test_locale_language.gd

func _initialize() -> void:
	var ok := true
	ok = _check("ClassDB.class_exists('LocaleLanguage')",
			ClassDB.class_exists("LocaleLanguage"), true) and ok

	ok = _check("ClassDB.is_class_enabled('LocaleLanguage')",
			ClassDB.is_class_enabled("LocaleLanguage"), true) and ok

	ok = _check("ClassDB.get_parent_class('LocaleLanguage') == 'Node'",
			ClassDB.get_parent_class("LocaleLanguage"), &"Node") and ok

	ok = _check("ClassDB.class_has_method('LocaleLanguage', 'parse')",
			ClassDB.class_has_method("LocaleLanguage", "parse"), true) and ok

	# @name override: the renamed binding is present, the source identifier
	# (do_something) is not.
	ok = _check("ClassDB.class_has_method('LocaleLanguage', 'do_something_alt_name')",
			ClassDB.class_has_method("LocaleLanguage", "do_something_alt_name"), true) and ok
	ok = _check("ClassDB.class_has_method('LocaleLanguage', 'do_something')",
			ClassDB.class_has_method("LocaleLanguage", "do_something"), false) and ok

	# Lowercase-first method becomes the engine virtual `_process`.
	ok = _check("ClassDB.class_has_method('LocaleLanguage', '_process', true)",
			ClassDB.class_has_method("LocaleLanguage", "_process", true), true) and ok

	# Static dispatch through ClassDB — bypasses the parser identifier
	# resolution that would block `LocaleLanguage.parse(...)` syntax on an
	# abstract class.
	var got_en: int = ClassDB.class_call_static("LocaleLanguage", "parse", "en")
	ok = _check("class_call_static('LocaleLanguage', 'parse', 'en') = ENGLISH(1)",
			got_en, 1) and ok
	var got_de: int = ClassDB.class_call_static("LocaleLanguage", "parse", "de")
	ok = _check("class_call_static('LocaleLanguage', 'parse', 'de') = GERMAN(2)",
			got_de, 2) and ok
	var got_unknown: int = ClassDB.class_call_static("LocaleLanguage", "parse", "??")
	ok = _check("class_call_static('LocaleLanguage', 'parse', '??') = UNKNOWN(0)",
			got_unknown, 0) and ok

	# Slice arg round-trip: PackedInt64Array → []int64 → int64 sum.
	# Phase 4 codegen marshals the typed array into a Go slice and back
	# through the int64 return path.
	var values := PackedInt64Array([1, 2, 3, 4, 5])
	var got_sum: int = ClassDB.class_call_static("LocaleLanguage", "sum", values)
	ok = _check("class_call_static('LocaleLanguage', 'sum', [1..5]) = 15",
			got_sum, 15) and ok

	# Slice return round-trip: []string → PackedStringArray. The Go-side
	# Names() returns three known entries; verify the array's length and
	# membership at the GDScript boundary.
	var got_names: PackedStringArray = ClassDB.class_call_static("LocaleLanguage", "names")
	ok = _check("class_call_static('LocaleLanguage', 'names').size() = 3",
			got_names.size(), 3) and ok
	ok = _check("class_call_static('LocaleLanguage', 'names').has('english')",
			got_names.has("english"), true) and ok
	ok = _check("class_call_static('LocaleLanguage', 'names').has('german')",
			got_names.has("german"), true) and ok
	ok = _check("class_call_static('LocaleLanguage', 'names').has('unknown')",
			got_names.has("unknown"), true) and ok

	# Phase 5 + 9 — user-enum slice return. Languages() returns
	# []Language on the Go side; wire form is untyped Variant::ARRAY
	# with PROPERTY_HINT_TYPE_STRING carrying the typed-element
	# identity ("2/2:UNKNOWN,ENGLISH,GERMAN"). The editor's autocomplete
	# / docs panel reads the hint and renders this as Array[Language]
	# rather than the bare PackedInt64Array we'd get with the
	# more-efficient wire form. Per-element marshal boxes each int
	# through a Variant.
	var got_langs: Array = ClassDB.class_call_static("LocaleLanguage", "languages")
	ok = _check("class_call_static('LocaleLanguage', 'languages').size() = 3",
			got_langs.size(), 3) and ok
	ok = _check("class_call_static('LocaleLanguage', 'languages')[0] = UNKNOWN(0)",
			got_langs[0], 0) and ok
	ok = _check("class_call_static('LocaleLanguage', 'languages')[1] = ENGLISH(1)",
			got_langs[1], 1) and ok
	ok = _check("class_call_static('LocaleLanguage', 'languages')[2] = GERMAN(2)",
			got_langs[2], 2) and ok

	# Phase 5 + 9 — user-enum slice arg + variadic. Wire form on both
	# sides is Variant::ARRAY now (the typed-element-identity path);
	# pass an Array of [0, 1, 2], expect [1, 2] back.
	var input_langs: Array = [0, 1, 2]
	var filtered: Array = ClassDB.class_call_static("LocaleLanguage", "filter_languages", input_langs)
	ok = _check("filter_languages([UNKNOWN, ENGLISH, GERMAN]).size() = 2",
			filtered.size(), 2) and ok
	ok = _check("filter_languages(...)[0] = ENGLISH(1)",
			filtered[0], 1) and ok
	ok = _check("filter_languages(...)[1] = GERMAN(2)",
			filtered[1], 2) and ok

	# Phase 5 — primitive variadic. ConcatNames takes ...string; Godot
	# sees a single PackedStringArray arg.
	var concat_input := PackedStringArray(["en", "de", "fr"])
	var concat: String = ClassDB.class_call_static("LocaleLanguage", "concat_names", concat_input)
	ok = _check("concat_names(['en','de','fr']) = 'en,de,fr'",
			concat, "en,de,fr") and ok

	# Cross-file enum reference: Greeter (declared in greeter.go) takes a
	# Language arg (declared in locale_language.go alongside the
	# LocaleLanguage @class). The codegen aggregates enums across files
	# in the package, so Greeter.greet_in resolves Language to the
	# owning class's qualifier — registration carries
	# "LocaleLanguage.Language" as the arg's class_name.
	ok = _check("ClassDB.class_exists('Greeter')",
			ClassDB.class_exists("Greeter"), true) and ok
	var greet_en: String = ClassDB.class_call_static("Greeter", "greet_in", 1)
	ok = _check("Greeter.greet_in(ENGLISH) = 'hello'",
			greet_en, "hello") and ok
	var greet_de: String = ClassDB.class_call_static("Greeter", "greet_in", 2)
	ok = _check("Greeter.greet_in(GERMAN) = 'hallo'",
			greet_de, "hallo") and ok
	# The arg's class_name on the registration carries the owning class
	# (LocaleLanguage), not the consuming class (Greeter).
	var greet_meta: Dictionary = ClassDB.class_get_method_list("Greeter") \
		.filter(func(m: Dictionary) -> bool: return m["name"] == "greet_in")[0]
	ok = _check("Greeter.greet_in arg[0].class_name = LocaleLanguage.Language",
			greet_meta["args"][0]["class_name"], &"LocaleLanguage.Language") and ok

	# Default-init constructor hook: Greeter defines an unexported
	# newGreeter() factory that seeds defaultLang to LanguageEnglish.
	# Codegen routes the engine-side Construct callback through it, so
	# Greeter.new() from GDScript picks up the same defaults Go-side
	# NewGreeter() would.
	var g: Greeter = ClassDB.instantiate("Greeter")
	ok = _check("Greeter.new() succeeded", g != null, true) and ok
	ok = _check("Greeter.new().hello() = 'hello' (newGreeter seeded ENGLISH)",
			g.hello(), "hello") and ok

	# map[K]V at the @class boundary: Greeter.count_letters returns
	# a Dictionary built from a Go-side map[string]int64. Letters count.
	var counts: Dictionary = ClassDB.class_call_static("Greeter", "count_letters", PackedStringArray(["abc", "ab"]))
	ok = _check("Greeter.count_letters(['abc','ab']).size() = 3",
			counts.size(), 3) and ok
	ok = _check("Greeter.count_letters(['abc','ab'])['a'] = 2",
			counts["a"], 2) and ok
	ok = _check("Greeter.count_letters(['abc','ab'])['b'] = 2",
			counts["b"], 2) and ok
	ok = _check("Greeter.count_letters(['abc','ab'])['c'] = 1",
			counts["c"], 1) and ok

	# Variant → Go map → Variant round-trip via Greeter.echo.
	var input_dict: Dictionary = {"x": 1, "y": 2}
	var echoed: Dictionary = ClassDB.class_call_static("Greeter", "echo", input_dict)
	ok = _check("Greeter.echo({x:1,y:2}).size() = 2", echoed.size(), 2) and ok
	ok = _check("Greeter.echo({x:1,y:2})['x'] = 1", echoed["x"], 1) and ok
	ok = _check("Greeter.echo({x:1,y:2})['y'] = 2", echoed["y"], 2) and ok

	# Phase 2a: user-enum value in a Dictionary. Wire wraps each Go
	# Language value as Variant::INT; GDScript sees the int constants
	# (LanguageEnglish=1, LanguageGerman=2 from locale_language.go's
	# const block).
	var lang_codes: Dictionary = ClassDB.class_call_static("Greeter", "lang_codes")
	ok = _check("Greeter.lang_codes().size() = 2", lang_codes.size(), 2) and ok
	ok = _check("Greeter.lang_codes()['en'] = 1 (ENGLISH)", lang_codes["en"], 1) and ok
	ok = _check("Greeter.lang_codes()['de'] = 2 (GERMAN)", lang_codes["de"], 2) and ok

	# Phase 2b: typed-Dictionary registration metadata. The lang_codes
	# return registration carries PROPERTY_HINT_DICTIONARY_TYPE (38)
	# with an encoded "K_section;V_section" hint string. Editor surface
	# varies across Godot 4.x minors; we just assert the hint flag is
	# set so the registration roundtrips through ClassDB cleanly.
	var lang_codes_meta: Dictionary = ClassDB.class_get_method_list("Greeter") \
		.filter(func(m: Dictionary) -> bool: return m["name"] == "lang_codes")[0]
	ok = _check("Greeter.lang_codes return.hint = HINT_DICTIONARY_TYPE",
			lang_codes_meta["return"]["hint"], 38) and ok

	# Phase 2a: slice value in a Dictionary. Each entry's value is
	# built as a fresh PackedStringArray inline by the codegen.
	var chars_by_lang: Dictionary = ClassDB.class_call_static("Greeter", "chars_by_lang")
	ok = _check("Greeter.chars_by_lang().size() = 2", chars_by_lang.size(), 2) and ok
	var chars_en: PackedStringArray = chars_by_lang["en"]
	ok = _check("Greeter.chars_by_lang()['en'].size() = 3", chars_en.size(), 3) and ok
	ok = _check("Greeter.chars_by_lang()['en'][0] = 'a'", chars_en[0], "a") and ok

	if ok:
		print("test_locale_language: ALL CHECKS PASSED")
		quit(0)
	else:
		print("test_locale_language: FAILED")
		quit(1)


func _check(label: String, got: Variant, want: Variant) -> bool:
	if got == want:
		print("test_locale_language: PASS  ", label, " = ", got)
		return true
	print("test_locale_language: FAIL  ", label, " = ", got, " (want ", want, ")")
	return false
