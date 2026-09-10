//go:build linux

package engine

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

func resizePTY(t *testing.T) (*os.File, func(Size)) {
	t.Helper()
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		t.Skipf("PTY unavailable: %v", err)
	}
	t.Cleanup(func() { master.Close() })
	var unlocked int32
	if err := ioctl(master.Fd(), syscall.TIOCSPTLCK, unsafe.Pointer(&unlocked)); err != nil {
		t.Fatal(err)
	}
	var number uint32
	if err := ioctl(master.Fd(), syscall.TIOCGPTN, unsafe.Pointer(&number)); err != nil {
		t.Fatal(err)
	}
	slave, err := os.OpenFile(fmt.Sprintf("/dev/pts/%d", number), os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { slave.Close() })
	return slave, func(size Size) {
		t.Helper()
		ws := winsize{Row: uint16(size.Height), Col: uint16(size.Width)}
		if err := ioctl(slave.Fd(), syscall.TIOCSWINSZ, unsafe.Pointer(&ws)); err != nil {
			t.Fatal(err)
		}
	}
}

func resizeRuntime(t *testing.T) (*Runtime, *bytes.Buffer, func(Size)) {
	t.Helper()
	slave, setSize := resizePTY(t)
	setSize(Size{Width: 80, Height: 24})
	label := Text("terminal 80x24")
	root := Root(AlternateScreen(label, Spacer(), Text("viewport-bottom")))
	root.SetHandlers(EventHandlers{OnResize: func(e *ResizeEvent) { label.SetText(fmt.Sprintf("terminal %dx%d", e.Columns, e.Rows)) }})
	out := &bytes.Buffer{}
	rt := NewRuntime(root, strings.NewReader(""), out, RenderOptions{Width: 80, Height: 24, Fullscreen: true, HideCursor: true})
	rt.Terminal = &Terminal{Out: slave}
	if err := rt.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { rt.Close() })
	out.Reset()
	return rt, out, setSize
}

func checkResizeScreen(t *testing.T, rt *Runtime, size Size) {
	t.Helper()
	if rt.Viewport() != size {
		t.Fatalf("viewport=%v want=%v", rt.Viewport(), size)
	}
	screen := rt.Renderer.Previous()
	rows := strings.Split(screen.PlainText(), "\n")
	if screen.Width != size.Width || screen.Height != size.Height || len(rows) < size.Height {
		t.Fatalf("screen dimensions=%dx%d rows=%d", screen.Width, screen.Height, len(rows))
	}
	if !strings.Contains(rows[0], fmt.Sprintf("terminal %dx%d", size.Width, size.Height)) {
		t.Fatalf("stale header: %q", rows[0])
	}
	if !strings.Contains(rows[size.Height-1], "viewport-bottom") {
		t.Fatalf("last row missing content at %v: %q", size, rows[size.Height-1])
	}
}

func TestResizePTYGrowthShrinkAndUnchangedSignal(t *testing.T) {
	rt, out, setSize := resizeRuntime(t)
	// Exercise the actual SIGWINCH handler, not only the resize helper.
	remove := installRuntimeSignalHandlers(rt)
	defer remove()
	if err := syscall.Kill(os.Getpid(), syscall.SIGWINCH); err != nil {
		t.Fatal(err)
	}
	awaitRuntimeEvent(t, rt)
	if err := rt.ProcessEvents(); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Fatalf("unchanged SIGWINCH wrote %q", out.String())
	}
	for _, size := range []Size{
		{Width: 100, Height: 30},
		{Width: 120, Height: 33},
		{Width: 146, Height: 35},
		{Width: 146, Height: 50},
		{Width: 80, Height: 24},
	} {
		setSize(size)
		for i := 0; i < 10; i++ {
			rt.queueResize()
		}
		rt.eventMu.Lock()
		pending := len(rt.pendingEvents)
		rt.eventMu.Unlock()
		if pending != 1 {
			t.Fatalf("resize burst queued %d events", pending)
		}
		if err := rt.ProcessEvents(); err != nil {
			t.Fatal(err)
		}
		checkResizeScreen(t, rt, size)
		if strings.Contains(out.String(), EnterAltScreen) || strings.Contains(out.String(), EraseScreen) {
			t.Fatalf("resize re-entered/cleared alternate screen at %v", size)
		}
		out.Reset()
		rt.queueResize()
		if err := rt.ProcessEvents(); err != nil {
			t.Fatal(err)
		}
		if out.Len() != 0 {
			t.Fatalf("unchanged resize emitted %d bytes", out.Len())
		}
	}
}

func TestResizePTYRechecksDelayedSizeWithoutInput(t *testing.T) {
	rt, out, setSize := resizeRuntime(t)
	rt.queueResize()
	if err := rt.ProcessEvents(); err != nil {
		t.Fatal(err)
	}
	// Keep the old size through the first delayed read as well.
	awaitRuntimeEvent(t, rt)
	if err := rt.ProcessEvents(); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Fatalf("unchanged size caused output: %q", out.String())
	}
	// The bridge catches up without another signal or key press.
	size := Size{Width: 146, Height: 50}
	setSize(size)
	awaitRuntimeEvent(t, rt)
	if err := rt.ProcessEvents(); err != nil {
		t.Fatal(err)
	}
	checkResizeScreen(t, rt, size)
	out.Reset()
	for i := 2; i < len(resizeProbeDelays); i++ {
		awaitRuntimeEvent(t, rt)
		if err := rt.ProcessEvents(); err != nil {
			t.Fatal(err)
		}
	}
	rt.eventMu.Lock()
	timer := rt.resizeTimer
	rt.eventMu.Unlock()
	if timer != nil {
		t.Fatal("resize polling continued beyond bounded retries")
	}
	if out.Len() != 0 {
		t.Fatalf("stable retry repainted: %q", out.String())
	}
}

func TestCloseCancelsResizeRetries(t *testing.T) {
	rt, _, _ := resizeRuntime(t)
	rt.queueResize()
	if err := rt.ProcessEvents(); err != nil {
		t.Fatal(err)
	}
	rt.eventMu.Lock()
	generation := rt.resizeGeneration
	rt.eventMu.Unlock()
	rt.Close()
	rt.probeResize(generation, 1)
	rt.eventMu.Lock()
	defer rt.eventMu.Unlock()
	if rt.resizeTimer != nil || rt.resizeQueued || len(rt.pendingEvents) != 0 {
		t.Fatal("resize work survived Close")
	}
}

type blockFirstWrite struct {
	entered     chan struct{}
	release     chan struct{}
	firstOnce   sync.Once
	releaseOnce sync.Once
}

func newBlockFirstWrite() *blockFirstWrite {
	return &blockFirstWrite{entered: make(chan struct{}), release: make(chan struct{})}
}

func (w *blockFirstWrite) Write(p []byte) (int, error) {
	first := false
	w.firstOnce.Do(func() {
		first = true
		close(w.entered)
	})
	if first {
		<-w.release
	}
	return len(p), nil
}

func (w *blockFirstWrite) Release() { w.releaseOnce.Do(func() { close(w.release) }) }

func TestRunInstallsResizeHandlerBeforeTerminalEntry(t *testing.T) {
	// Block the very first terminal write made by Start. A SIGWINCH delivered
	// here is the exact window that the PTY black-box test cannot discriminate:
	// with the old Run order the signal handler did not exist yet and the signal
	// was lost; with the current order queueResize must run while Start is still
	// blocked in terminal entry.
	out := newBlockFirstWrite()
	rt := NewRuntime(Root(Text("startup")), strings.NewReader(""), out, RenderOptions{Width: 20, Height: 4, Fullscreen: true})
	done := make(chan error, 1)
	go func() { done <- rt.Run() }()
	defer func() {
		rt.Stop()
		out.Release()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Errorf("Run did not stop during cleanup")
		}
	}()

	select {
	case <-out.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not reach terminal entry write")
	}
	if err := syscall.Kill(os.Getpid(), syscall.SIGWINCH); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		rt.eventMu.Lock()
		queued := rt.resizeQueued || len(rt.pendingEvents) > 0
		rt.eventMu.Unlock()
		if queued {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("SIGWINCH was not queued while Start was blocked; signal handler was installed too late")
		}
		time.Sleep(time.Millisecond)
	}
}
