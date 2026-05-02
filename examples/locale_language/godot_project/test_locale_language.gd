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
