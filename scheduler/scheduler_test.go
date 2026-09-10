package scheduler

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestClockLifecycleAndEvery(t *testing.T) {
	c := New(2 * time.Millisecond)
	defer c.Close()
	var ticks atomic.Int32
	unsub := c.Subscribe(func(time.Duration) { ticks.Add(1) }, true)
	deadline := time.Now().Add(100 * time.Millisecond)
	for ticks.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if ticks.Load() == 0 {
		t.Fatal("clock did not tick")
	}
	unsub()
	unsub()
	before := ticks.Load()
	time.Sleep(8 * time.Millisecond)
	if ticks.Load() != before {
		t.Fatal("unsubscribed clock kept ticking")
	}

	var every atomic.Int32
	unsubEvery := c.Every(4*time.Millisecond, true, func(time.Duration) { every.Add(1) })
	deadline = time.Now().Add(100 * time.Millisecond)
	for every.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	unsubEvery()
	if every.Load() == 0 {
		t.Fatal("Every did not fire")
	}
	c.SetFocused(false)
	c.SetTickInterval(time.Millisecond)
	_ = c.Now()
}

func TestClockDoesNotOverlapSlowSubscriberTicks(t *testing.T) {
	c := New(time.Millisecond)
	defer c.Close()

	var active atomic.Int32
	var overlap atomic.Bool
	var calls atomic.Int32
	unsub := c.Subscribe(func(time.Duration) {
		if active.Add(1) != 1 {
			overlap.Store(true)
		}
		time.Sleep(4 * time.Millisecond)
		calls.Add(1)
		active.Add(-1)
	}, true)
	defer unsub()

	deadline := time.Now().Add(100 * time.Millisecond)
	for calls.Load() < 3 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if calls.Load() < 3 {
		t.Fatalf("only %d callbacks completed", calls.Load())
	}
	if overlap.Load() {
		t.Fatal("shared clock re-entered a slow subscriber")
	}
}
