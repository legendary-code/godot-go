//go:build !godot_double_precision

package gdextension

// Precision is the float build configuration this binary was compiled
// against, matching the `precision` field in extension_api.json. Single
// precision is the Godot default and the godot-go MVP target.
//
// Build with `-tags godot_double_precision` to produce a binary compatible
// with a double-precision Godot build.
const Precision = "single"
