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
	firstStarted := make(chan struct{}, 1)
	releaseFirst := make(chan struct{})
	secondDone := make(chan struct{}, 1)

	unsub := c.Subscribe(func(time.Duration) {
		if active.Add(1) != 1 {
			overlap.Store(true)
		}
		call := calls.Add(1)
		if call == 1 {
			firstStarted <- struct{}{}
			<-releaseFirst
		}
		active.Add(-1)
		if call == 2 {
			secondDone <- struct{}{}
		}
	}, true)
	defer unsub()

	select {
	case <-firstStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("first clock callback did not start")
	}

	// Hold the first callback past several nominal tick intervals. A broken
	// re-entrant clock would invoke the subscriber again while it is blocked.
	time.Sleep(10 * time.Millisecond)
	if got := calls.Load(); got != 1 {
		t.Fatalf("slow subscriber was re-entered: calls=%d", got)
	}
	if overlap.Load() {
		t.Fatal("shared clock re-entered a slow subscriber")
	}

	close(releaseFirst)
	select {
	case <-secondDone:
	case <-time.After(2 * time.Second):
		t.Fatal("clock did not resume after slow subscriber completed")
	}
	if overlap.Load() {
		t.Fatal("shared clock re-entered a slow subscriber")
	}
}
