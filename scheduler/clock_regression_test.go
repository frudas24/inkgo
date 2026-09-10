package scheduler

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestExpiredTimerDoesNotRunAfterReplacement(t *testing.T) {
	c := New(time.Hour)
	defer c.Close()
	var calls atomic.Int32
	ticked := make(chan struct{}, 1)
	c.Subscribe(func(time.Duration) {
		calls.Add(1)
		select {
		case ticked <- struct{}{}:
		default:
		}
	}, true)
	fired := make(chan struct{})
	finished := make(chan struct{})
	c.mu.Lock()
	c.timer.Stop()
	generation := c.timerGeneration
	// Hold the mutex until the old timer's callback has started. Replacing it
	// now must invalidate that callback even though Stop can no longer stop it.
	c.timer = time.AfterFunc(0, func() { close(fired); c.tick(generation); close(finished) })
	<-fired
	c.interval = 2 * time.Hour
	c.restartTimerLocked()
	replacement := c.timer
	c.mu.Unlock()
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("expired callback blocked")
	}
	if calls.Load() != 0 {
		t.Fatal("expired timer dispatched a subscriber")
	}
	c.mu.Lock()
	preserved := c.timer == replacement
	c.mu.Unlock()
	if !preserved {
		t.Fatal("expired callback overwrote replacement timer")
	}
	c.SetTickInterval(time.Millisecond)
	select {
	case <-ticked:
	case <-time.After(2 * time.Second):
		t.Fatal("replacement clock stopped ticking")
	}
}

func TestStaleTickCannotOverlapActiveCallback(t *testing.T) {
	c := New(time.Hour)
	release := make(chan struct{})
	entered := make(chan struct{}, 2)
	c.Subscribe(func(time.Duration) { entered <- struct{}{}; <-release }, true)
	defer func() { c.Close(); close(release) }()
	c.mu.Lock()
	stale := c.timerGeneration
	c.mu.Unlock()
	c.SetTickInterval(time.Millisecond)
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("callback did not start")
	}
	// A previous callback that was delayed by scheduling must return without
	// entering the subscriber while the current callback is still running.
	returned := make(chan struct{})
	go func() { c.tick(stale); close(returned) }()
	select {
	case <-returned:
	case <-time.After(2 * time.Second):
		t.Fatal("stale tick entered blocked subscriber")
	}
	select {
	case <-entered:
		t.Fatal("subscriber called concurrently")
	default:
	}
}

func TestNowUsesTickTimestampOnlyDuringCallbacks(t *testing.T) {
	c := New(time.Hour)
	defer c.Close()
	var observed [3]time.Duration
	c.Subscribe(func(at time.Duration) { observed = [3]time.Duration{at, c.Now(), c.Now()} }, true)
	c.mu.Lock()
	generation := c.timerGeneration
	c.timer.Stop()
	c.mu.Unlock()
	c.tick(generation)
	if observed[0] != observed[1] || observed[0] != observed[2] {
		t.Fatalf("inconsistent callback times: %v", observed)
	}
	if now := c.Now(); now <= observed[0] {
		t.Fatalf("Now remained frozen between ticks: %v <= %v", now, observed[0])
	}
}
