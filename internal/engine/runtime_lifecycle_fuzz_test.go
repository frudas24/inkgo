package engine

import (
	"bytes"
	"testing"
)

// FuzzRuntimeLifecycleInvariants exercises lifecycle transitions as a small
// state machine, including transient output failures. The goal is not to model
// terminal bytes perfectly; it protects ownership/cleanup postconditions that
// must hold regardless of transition order.
func FuzzRuntimeLifecycleInvariants(f *testing.F) {
	f.Add([]byte{0, 1})                // Start, Close
	f.Add([]byte{0, 4, 1, 4, 0, 1})    // failed Close, recovery, Start
	f.Add([]byte{0, 4, 2, 3, 4, 3, 1}) // failed Suspend/Resume, recovery
	f.Add([]byte{0, 5, 3, 1, 0})       // Stop is permanent
	f.Add([]byte{2, 3, 1, 3, 1, 3, 1}) // repeated embedded transitions

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 256 {
			data = data[:256]
		}
		w := &auditToggleWriter{}
		rt := NewRuntime(Root(AlternateScreen(Text("x"))), bytes.NewReader(nil), w, RenderOptions{Width: 12, Height: 3, Fullscreen: true})
		defer func() {
			w.fail = false
			_ = rt.Close()
		}()

		for step, raw := range data {
			switch raw % 7 {
			case 0: // Start
				wasStarted := rt.Started()
				err := rt.Start()
				if err == nil && !rt.Started() {
					t.Fatalf("step %d: successful Start did not publish ownership", step)
				}
				if err != nil && !wasStarted && rt.Started() {
					t.Fatalf("step %d: failed Start newly published ownership: %v", step, err)
				}
			case 1: // Close
				err := rt.Close()
				if err == nil {
					if rt.Started() {
						t.Fatalf("step %d: successful Close left runtime started", step)
					}
					if rt.terminalCleanupPending() {
						t.Fatalf("step %d: successful Close left cleanup pending", step)
					}
				}
			case 2: // Suspend
				rt.SuspendTerminal()
				if rt.Started() {
					t.Fatalf("step %d: SuspendTerminal left runtime started", step)
				}
			case 3: // Resume
				wasStopped := rt.Stopped()
				wasStarted := rt.Started()
				rt.ResumeTerminal()
				if wasStopped && !wasStarted && rt.Started() {
					t.Fatalf("step %d: ResumeTerminal resurrected stopped runtime", step)
				}
				if !wasStopped && !w.fail && !rt.Started() {
					t.Fatalf("step %d: healthy ResumeTerminal failed to enter", step)
				}
			case 4: // transient writer health
				w.fail = !w.fail
			case 5: // permanent termination
				rt.Stop()
			case 6: // root transition while embedded lifecycle changes around it
				rt.SetRoot(Root(AlternateScreen(Text("y"))))
			}
		}
	})
}
