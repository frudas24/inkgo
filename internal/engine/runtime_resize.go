package engine

import "time"

// ConPTY/WSL can report the old PTY size when SIGWINCH arrives. Sample again
// over a bounded 750ms window, including after an unchanged first reading.
var resizeProbeDelays = [...]time.Duration{50 * time.Millisecond, 100 * time.Millisecond, 200 * time.Millisecond, 400 * time.Millisecond}

// queueResize is safe for the signal goroutine. Coalesce a burst into one UI
// event, cancel the old retry chain, and never mutate the tree in a timer.
func (rt *Runtime) queueResize() {
	rt.eventMu.Lock()
	defer rt.eventMu.Unlock()
	if rt.eventsClosed || rt.Stopped() {
		return
	}
	rt.resizeGeneration++
	if rt.resizeTimer != nil {
		rt.resizeTimer.Stop()
		rt.resizeTimer = nil
	}
	if rt.resizeQueued {
		return
	}
	rt.resizeQueued = true
	rt.enqueueEventLocked(func() {
		rt.eventMu.Lock()
		generation := rt.resizeGeneration
		rt.resizeQueued = false
		rt.eventMu.Unlock()
		rt.probeResize(generation, 0)
	})
}

// probeResize runs only on the UI owner. ProcessEvents renders after this
// callback; a changed viewport naturally forces a full-sized screen diff.
// Ordinary resize must not push another alternate-screen entry or clear it.
func (rt *Runtime) probeResize(generation uint64, attempt int) {
	rt.eventMu.Lock()
	if rt.eventsClosed || rt.Stopped() || generation != rt.resizeGeneration {
		rt.eventMu.Unlock()
		return
	}
	rt.resizeTimer = nil
	if attempt < len(resizeProbeDelays) {
		rt.resizeTimer = time.AfterFunc(resizeProbeDelays[attempt], func() {
			rt.enqueueEvent(func() { rt.probeResize(generation, attempt+1) })
		})
	}
	rt.eventMu.Unlock()
	rt.RefreshSize()
}

// Caller holds eventMu. Invalidate even callbacks whose timer already fired.
func (rt *Runtime) cancelResizeLocked() {
	rt.resizeGeneration++
	rt.resizeQueued = false
	if rt.resizeTimer != nil {
		rt.resizeTimer.Stop()
		rt.resizeTimer = nil
	}
}
