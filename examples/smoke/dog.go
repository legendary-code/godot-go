package main

// Dog is a concrete subclass of Animal. Implements both abstract
// methods declared on AnimalAbstract — these are registered as
// regular ClassDB methods on Dog. When code holds a `*Animal`
// reference (e.g. via the dispatcher synthesized on the parent),
// Godot's hierarchy lookup routes to Dog's `speak` / `move`.
//
// Demonstrates the same-package user-class extension pattern: bare
// `Animal` (no package qualifier) on the embedded @extends field.
// Discover-time accepts the bare ident; emit-time validates against
// the package's @class set.
//
// @class
type Dog struct {
	// @extends
	Animal

	// distanceTraveled accumulates Move() args so the GDScript
	// integration test can verify the dispatcher actually reached
	// Dog's implementation.
	distanceTraveled int64
}

// Speak overrides AnimalAbstract.Speak.
func (d *Dog) Speak() string {
	return "Woof"
}

// Move overrides AnimalAbstract.Move and tallies distance into the
// instance for inspection from GDScript.
func (d *Dog) Move(distance int64) {
	d.distanceTraveled += distance
}

// DistanceTraveled exposes the accumulated movement total to GDScript
// — without this, there's no way to confirm the Move dispatcher
// reached Dog's implementation vs hitting Animal's "method not found"
// error.
func (d *Dog) DistanceTraveled() int64 {
	return d.distanceTraveled
}
