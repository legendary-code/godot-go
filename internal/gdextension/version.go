package gdextension

import (
	"fmt"
	"sync"
)

// Version drift check.
//
// The user's bindings are generated against a specific Godot API
// version (recorded in version.gen.go inside the user's bindings
// package). That file's init() calls RegisterAPIVersion below to
// hand the constants over to the framework runtime — different
// users target different Godot versions, so the constants can't
// live here as compile-time literals.
//
// On the first engine→Go callback, checkVersionDrift compares the
// registered values against the host's runtime-reported version.
// Patch-level differences (4.6.1 vs 4.6.2) are tolerated; major or
// minor mismatch (4.6 vs 4.7) produces a warning to Godot's output.
// We don't refuse the load — Godot has already mapped the DLL and
// our cleanest path after a mismatch is to surface the diagnostic
// rather than crash with a confusing stack.

var (
	registeredAPIMajor   int
	registeredAPIMinor   int
	registeredAPIPatch   int
	registeredAPIVersion string
	apiRegistered        bool

	versionCheckOnce sync.Once
)

// RegisterAPIVersion records the bindings' compile-time view of the
// Godot API version. The user's generated version.gen.go calls this
// from its init(). Idempotent — if called more than once, the first
// registration wins (later calls are no-ops). Callers should pass
// the (Major, Minor, Patch, Full-version-string) quadruple straight
// from extension_api.json's header block.
func RegisterAPIVersion(major, minor, patch int, version string) {
	if apiRegistered {
		return
	}
	registeredAPIMajor = major
	registeredAPIMinor = minor
	registeredAPIPatch = patch
	registeredAPIVersion = version
	apiRegistered = true
}

// checkVersionDrift compares the registered bindings' API version
// against the host's reported version. Warns on (major, minor)
// mismatch; silent otherwise. Idempotent — runs at most once per
// process load. If the user's bindings package never called
// RegisterAPIVersion (i.e. version.gen.go is missing from the
// build), the check is skipped silently.
func checkVersionDrift() {
	versionCheckOnce.Do(func() {
		if !apiRegistered {
			return
		}
		host := GetGodotVersion()
		if int(host.Major) == registeredAPIMajor && int(host.Minor) == registeredAPIMinor {
			return
		}
		msg := fmt.Sprintf(
			"godot-go: API version drift — bindings generated for v%d.%d.%d (%s), host reports v%d.%d.%d (%s); some method calls may misbehave",
			registeredAPIMajor, registeredAPIMinor, registeredAPIPatch, registeredAPIVersion,
			host.Major, host.Minor, host.Patch, host.String,
		)
		PrintWarning(msg, "checkVersionDrift",
			"internal/gdextension/version.go", 0, false)
	})
}
