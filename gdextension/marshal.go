package gdextension

import "unsafe"

// PtrCallArg returns the typed-value pointer for the idx-th argument in a
// PtrCall arg array. Godot's PtrCall ABI is `const void * const * p_args`
// — a pointer to an array of pointers, each pointing to the typed slot
// for one argument. The framework's generated bindings use this helper
// to keep per-arg reads to a one-line expression:
//
//	x := *(*int64)(gdextension.PtrCallArg(args, 0))
//
// idx is not bounds-checked here; the host always passes argument_count
// matching what the method registered, and the generated code only reads
// indices in that range.
func PtrCallArg(args unsafe.Pointer, idx int) unsafe.Pointer {
	// 1<<20 is the largest power-of-two array shape that's safe to cast
	// without overflowing — 32 args (the framework cap) is well within it.
	arr := (*[1 << 20]unsafe.Pointer)(args)
	return arr[idx]
}
