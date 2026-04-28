package runtime

import (
	"sync"
	"testing"
)

// The IsMainThread / RunOnMain / DrainMain semantics around "am I
// already on main" depend on the gdextension package's captured
// thread ID, which only gets recorded when Godot calls into us. In a
// pure `go test` run there's no engine, so MainThreadID stays zero
// and IsMainThread always reports false — which is exactly the
// behavior we need to exercise the queue path.
//
// These tests cover the queue's behavior directly; the inline-when-
// on-main fast path and the IsMainThread-true case get verified
// end-to-end via the smoke harness, where Godot really does call
// into us on the engine main thread.

func TestDrainMainOnEmptyIsNoop(t *testing.T) {
	// Calling drain when nothing's queued should not panic, deadlock,
	// or otherwise do anything observable. Run it twice for good
	// measure.
	DrainMain()
	DrainMain()
}

func TestRunOnMainQueuesFromOffThread(t *testing.T) {
	// In a `go test` context the main-thread ID is unset, so RunOnMain
	// always queues. Post several funcs; drain; verify they ran in
	// FIFO order.
	var order []int
	for i := 0; i < 5; i++ {
		i := i // capture
		RunOnMain(func() { order = append(order, i) })
	}
	DrainMain()
	want := []int{0, 1, 2, 3, 4}
	if len(order) != len(want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Errorf("order[%d] = %d, want %d", i, order[i], want[i])
		}
	}
}

func TestDrainSinglePass(t *testing.T) {
	// A func queued INSIDE a drained func must run on the NEXT drain,
	// not the current one. This protects the main thread from
	// pathological loops where each drained func enqueues another.
	var ran []string
	RunOnMain(func() {
		ran = append(ran, "outer")
		RunOnMain(func() { ran = append(ran, "inner") })
	})
	DrainMain()
	if len(ran) != 1 || ran[0] != "outer" {
		t.Fatalf("after first drain: ran = %v, want [outer]", ran)
	}
	DrainMain()
	if len(ran) != 2 || ran[1] != "inner" {
		t.Fatalf("after second drain: ran = %v, want [outer inner]", ran)
	}
}

func TestRunOnMainNilSkipped(t *testing.T) {
	// Posting nil should silently do nothing — easier for callers
	// than enforcing a panic at the post site.
	RunOnMain(nil)
	DrainMain() // must not panic
}

func TestRunOnMainConcurrent(t *testing.T) {
	// Many goroutines posting concurrently; drain once on the test
	// goroutine. Verify count matches and there's no race / lost
	// post.
	const total = 1000
	var wg sync.WaitGroup
	wg.Add(total)
	for i := 0; i < total; i++ {
		go func() {
			defer wg.Done()
			RunOnMain(func() {})
		}()
	}
	wg.Wait()
	// At this point every goroutine has finished its RunOnMain call.
	// Drain and count by setting up a counter the funcs increment.
	count := 0
	mainQueueMu.Lock()
	queued := len(mainQueue)
	mainQueueMu.Unlock()
	if queued != total {
		t.Errorf("queued = %d, want %d", queued, total)
	}
	// Now post `total` counter-incrementing funcs and drain.
	mainQueueMu.Lock()
	mainQueue = mainQueue[:0]
	mainQueueMu.Unlock()
	wg.Add(total)
	for i := 0; i < total; i++ {
		go func() {
			defer wg.Done()
			RunOnMain(func() { count++ })
		}()
	}
	wg.Wait()
	DrainMain()
	if count != total {
		t.Errorf("count = %d, want %d", count, total)
	}
}
