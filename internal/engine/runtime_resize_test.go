package engine

import (
	"sync"
	"testing"
	"time"
)

// pollSizeChanges backs the Windows resize path, which has no signal to wait
// for. It must report a change exactly once and stay silent while the size is
// stable or unreadable.
func TestPollSizeChangesReportsOnlyChanges(t *testing.T) {
	var mu sync.Mutex
	current := Size{Width: 80, Height: 24}
	valid := true
	changed := make(chan struct{}, 8)
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		pollSizeChanges(2*time.Millisecond, stop, func() (Size, bool) {
			mu.Lock()
			defer mu.Unlock()
			return current, valid
		}, func() { changed <- struct{}{} })
	}()

	quiet := func(label string) {
		t.Helper()
		select {
		case <-changed:
			t.Fatalf("%s reported a change", label)
		case <-time.After(60 * time.Millisecond):
		}
	}

	quiet("stable size")
	mu.Lock()
	valid = false
	current = Size{Width: 120, Height: 30}
	mu.Unlock()
	quiet("unreadable size")

	mu.Lock()
	valid = true
	mu.Unlock()
	select {
	case <-changed:
	case <-time.After(2 * time.Second):
		t.Fatal("size change was never reported")
	}
	quiet("size after the change settled")

	close(stop)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("poller did not stop")
	}
}

func TestPollSizeChangesStopsWithoutTicking(t *testing.T) {
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		pollSizeChanges(time.Hour, stop, func() (Size, bool) { return Size{Width: 1, Height: 1}, true }, func() {})
	}()
	close(stop)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("poller did not stop before its first tick")
	}
}
