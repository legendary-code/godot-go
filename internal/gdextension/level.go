package gdextension

import "sync"

// Level dispatch.
//
// Godot calls the extension's initialize/deinitialize callbacks once per
// InitializationLevel as the engine boots and shuts down. Code in later
// phases (e.g. class registration) wants to hook specific levels — class
// registry runs at SCENE, editor-only types at EDITOR, etc. — so we keep a
// per-level callback list and fan out from the C trampolines.
//
// Callbacks are invoked on Godot's main thread, in registration order on
// init and reverse-registration order on deinit (LIFO).

var (
	callbackMu      sync.Mutex
	initCallbacks   = map[InitializationLevel][]func(){}
	deinitCallbacks = map[InitializationLevel][]func(){}
)

// RegisterInitCallback adds fn to the list of callbacks invoked when Godot
// enters the given init level. Safe to call from a Go init() function.
func RegisterInitCallback(level InitializationLevel, fn func()) {
	callbackMu.Lock()
	defer callbackMu.Unlock()
	initCallbacks[level] = append(initCallbacks[level], fn)
}

// RegisterDeinitCallback adds fn to the list of callbacks invoked when Godot
// leaves the given init level. Deinit callbacks run in reverse order from
// the order they were registered.
func RegisterDeinitCallback(level InitializationLevel, fn func()) {
	callbackMu.Lock()
	defer callbackMu.Unlock()
	deinitCallbacks[level] = append(deinitCallbacks[level], fn)
}

func runInitCallbacks(level InitializationLevel) {
	callbackMu.Lock()
	cbs := initCallbacks[level]
	callbackMu.Unlock()
	for _, fn := range cbs {
		fn()
	}
}

func runDeinitCallbacks(level InitializationLevel) {
	callbackMu.Lock()
	cbs := deinitCallbacks[level]
	callbackMu.Unlock()
	for i := len(cbs) - 1; i >= 0; i-- {
		cbs[i]()
	}
}
