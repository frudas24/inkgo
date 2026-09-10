//go:build windows

package engine

import "time"

// Windows consoles have no SIGWINCH equivalent and the runtime reads input
// through a generic io.Reader, so it cannot observe console resize events
// directly. Poll the console size instead; queueResize coalesces and the
// renderer diffs, so an idle window stays quiet.
const windowsResizePollInterval = 60 * time.Millisecond

func installRuntimeSignalHandlers(rt *Runtime) func() {
	if rt == nil || rt.Terminal == nil {
		return func() {}
	}
	// Capture the terminal before the goroutine starts: the runtime owns the
	// loop and callers set Terminal before Run, so this is a single read.
	term := rt.Terminal
	sample := func() (Size, bool) {
		sz, err := term.Size()
		if err != nil || sz.Width <= 0 || sz.Height <= 0 {
			return Size{}, false
		}
		return sz, true
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		pollSizeChanges(windowsResizePollInterval, stop, sample, rt.queueResize)
	}()
	return func() {
		close(stop)
		<-done
	}
}

func suspendRuntime(_ *Runtime) bool { return false }
