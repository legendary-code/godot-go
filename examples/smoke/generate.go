// Package-level codegen trigger. `go generate` runs godot-go once per
// package — it walks every *.go file looking for @class structs and
// emits a single `bindings.gen.go` at the package root.

package main

//go:generate godot-go
