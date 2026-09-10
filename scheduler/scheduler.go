package scheduler

import (
	"sync"
	"time"
)

// Clock consolidates animation/interval wake-ups into one timer, matching the
// fork's shared clock without React. Subscribers marked keepAlive drive the
// clock; passive subscribers receive synchronized ticks only while another
// subscriber keeps it alive.
type Clock struct {
	mu sync.Mutex

	baseInterval time.Duration
	interval     time.Duration
	start        time.Time
	tickTime     time.Duration
	timer        *time.Timer
	closed       bool
	nextID       uint64
	subscribers  map[uint64]clockSubscriber
}

type clockSubscriber struct {
	fn        func(time.Duration)
	keepAlive bool
}

// NewClock creates an idle shared clock. No timer is allocated until a
// keepAlive subscriber is registered.
func NewClock(interval time.Duration) *Clock {
	if interval <= 0 {
		interval = 16 * time.Millisecond
	}
	return &Clock{
		baseInterval: interval,
		interval:     interval,
		subscribers:  make(map[uint64]clockSubscriber),
	}
}

// Now returns elapsed time since first use. During an active tick all
// subscribers observe the same value.
func (c *Clock) Now() time.Duration {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureStartedLocked()
	if c.timer != nil && c.tickTime > 0 {
		return c.tickTime
	}
	return time.Since(c.start)
}

// Subscribe registers a synchronized tick callback and returns an idempotent
// unsubscribe function. keepAlive controls whether this subscription drives
// the underlying timer.
func (c *Clock) Subscribe(fn func(time.Duration), keepAlive bool) func() {
	if c == nil || fn == nil {
		return func() {}
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return func() {}
	}
	c.ensureStartedLocked()
	c.nextID++
	id := c.nextID
	c.subscribers[id] = clockSubscriber{fn: fn, keepAlive: keepAlive}
	c.updateTimerLocked()
	c.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			c.mu.Lock()
			delete(c.subscribers, id)
			c.updateTimerLocked()
			c.mu.Unlock()
		})
	}
}

// SetTickInterval changes the shared wake-up cadence.
func (c *Clock) SetTickInterval(interval time.Duration) {
	if c == nil || interval <= 0 {
		return
	}
	c.mu.Lock()
	if c.closed || c.interval == interval {
		c.mu.Unlock()
		return
	}
	c.interval = interval
	c.restartTimerLocked()
	c.mu.Unlock()
}

// SetFocused is the Go equivalent of ClockProvider's focus throttling: focused
// uses the base cadence and blurred uses twice that interval.
func (c *Clock) SetFocused(focused bool) {
	if c == nil {
		return
	}
	interval := c.baseInterval
	if !focused {
		interval *= 2
	}
	c.SetTickInterval(interval)
}

// Every subscribes at a coarser logical interval while still sharing the
// underlying clock. It is the Go equivalent of useInterval/useAnimationTimer.
func (c *Clock) Every(interval time.Duration, keepAlive bool, fn func(time.Duration)) func() {
	if c == nil || fn == nil || interval <= 0 {
		return func() {}
	}
	last := c.Now()
	return c.Subscribe(func(now time.Duration) {
		if now-last >= interval {
			last = now
			fn(now)
		}
	}, keepAlive)
}

// Close stops the clock and drops all subscribers.
func (c *Clock) Close() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.closed = true
	if c.timer != nil {
		c.timer.Stop()
		c.timer = nil
	}
	clear(c.subscribers)
	c.mu.Unlock()
}

func (c *Clock) ensureStartedLocked() {
	if c.start.IsZero() {
		c.start = time.Now()
	}
}

func (c *Clock) hasKeepAliveLocked() bool {
	for _, sub := range c.subscribers {
		if sub.keepAlive {
			return true
		}
	}
	return false
}

func (c *Clock) updateTimerLocked() {
	if c.closed || !c.hasKeepAliveLocked() {
		if c.timer != nil {
			c.timer.Stop()
			c.timer = nil
		}
		return
	}
	c.restartTimerLocked()
}

func (c *Clock) restartTimerLocked() {
	if c.closed || !c.hasKeepAliveLocked() {
		return
	}
	if c.timer != nil {
		c.timer.Stop()
	}
	interval := c.interval
	c.timer = time.AfterFunc(interval, c.tick)
}

func (c *Clock) tick() {
	c.mu.Lock()
	if c.closed || !c.hasKeepAliveLocked() {
		c.timer = nil
		c.mu.Unlock()
		return
	}
	c.ensureStartedLocked()
	c.tickTime = time.Since(c.start)
	now := c.tickTime
	callbacks := make([]func(time.Duration), 0, len(c.subscribers))
	for _, sub := range c.subscribers {
		callbacks = append(callbacks, sub.fn)
	}
	c.timer = time.AfterFunc(c.interval, c.tick)
	c.mu.Unlock()

	for _, fn := range callbacks {
		fn(now)
	}
}

// New is the canonical domain constructor.
func New(interval time.Duration) *Clock { return NewClock(interval) }
