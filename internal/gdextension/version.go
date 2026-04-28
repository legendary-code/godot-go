package gdextension

import (
	"fmt"
	"sync"
)

// Version drift check.
//
// The framework's bindings are generated against a specific Godot
// API version (recorded as APIMajor / APIMinor / APIPatch in
// version.gen.go). The host engine reports its own version at
// runtime via the get_godot_version2 interface call. If those
// disagree at the major or minor level, the binding-generated code
// may call methods with the wrong hash, miss new fields, or
// segfault on layout changes.
//
// The check fires once on the first engine→Go callback. Patch-level
// differences (4.6.1 vs 4.6.2) are tolerated; major/minor (4.6 vs
// 4.7) produces a warning to Godot's output. We don't refuse the
// load — Godot has already mapped the DLL and our cleanest path
// after a mismatch is to surface the diagnostic and let the user
// see what happened. Hard refusal would manifest as a confusing
// crash rather than an actionable message.

var versionCheckOnce sync.Once

// checkVersionDrift compares the bundled bindings' API version
// against the host's reported version. Warns on (major, minor)
// mismatch; silent otherwise. Idempotent — runs at most once per
// process load.
func checkVersionDrift() {
	versionCheckOnce.Do(func() {
		host := GetGodotVersion()
		if host.Major == APIMajor && host.Minor == APIMinor {
			return
		}
		msg := fmt.Sprintf(
			"godot-go: API version drift — bindings generated for v%d.%d.%d (%s), host reports v%d.%d.%d (%s); some method calls may misbehave",
			APIMajor, APIMinor, APIPatch, APIVersion,
			host.Major, host.Minor, host.Patch, host.String,
		)
		PrintWarning(msg, "checkVersionDrift",
			"internal/gdextension/version.go", 0, false)
	})
}
