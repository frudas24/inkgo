//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package engine

import (
	"os"
	"os/signal"
	"syscall"
)

func installRuntimeSignalHandlers(rt *Runtime) func() {
	ch := make(chan os.Signal, 4)
	signal.Notify(ch, syscall.SIGWINCH, syscall.SIGCONT)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case sig := <-ch:
				switch sig {
				case syscall.SIGWINCH:
					rt.queueResize()
				case syscall.SIGCONT:
					rt.enqueueEvent(rt.recoverAfterResume)
				}
			case <-done:
				return
			}
		}
	}()
	return func() {
		signal.Stop(ch)
		close(done)
	}
}

func suspendRuntime(rt *Runtime) bool {
	if rt == nil {
		return false
	}
	rt.SuspendTerminal()
	_ = syscall.Kill(os.Getpid(), syscall.SIGTSTP)
	// Execution resumes here after SIGCONT. The signal handler also calls
	// ResumeTerminal; this direct call makes the path robust if SIGCONT was
	// swallowed by an embedding host.
	rt.ResumeTerminal()
	return true
}
