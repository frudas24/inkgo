package ptytest

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/frudas24/inkgo/test/pty/harness"
)

const (
	altScreenEnter = "\x1b[?1049h"
	altScreenExit  = "\x1b[?1049l"
)

var fixtureBinary string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "inkgo-pty-fixture-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "create fixture temp dir:", err)
		os.Exit(2)
	}
	defer os.RemoveAll(dir)
	fixtureBinary = filepath.Join(dir, "app")
	if runtime.GOOS == "windows" {
		fixtureBinary += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", fixtureBinary, "./fixtures/app")
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=local")
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build PTY fixture: %v\n%s", err, out)
		os.Exit(2)
	}
	os.Exit(m.Run())
}

func startFixture(t *testing.T, width, height int) *harness.Session {
	t.Helper()
	return harness.Start(t, fixtureBinary, width, height)
}

func TestPTYStartupInputPasteFocusAndGracefulExit(t *testing.T) {
	s := startFixture(t, 80, 24)
	out := s.WaitFor("READY:80x24")
	if !strings.Contains(out, altScreenEnter) {
		t.Fatalf("alternate screen was not entered; output=%q", out)
	}

	s.Write("a")
	s.WaitFor(`KEY:name=a,text="a",ctrl=false,alt=false,shift=false`)

	s.Write("\x1b[200~hello pty\x1b[201~")
	s.WaitFor("PASTE_LEN:9")
	s.WaitFor(`"hello pty"`)

	s.Write("q")
	if err := s.Wait(5 * time.Second); err != nil {
		t.Fatal(err)
	}
	out = s.Output()
	if !strings.Contains(out, altScreenExit) {
		t.Fatalf("alternate screen was not restored on graceful exit; output=%q", out)
	}
}

func TestPTYFocusOut(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("ConPTY does not provide a deterministic way to synthesize a host focus transition from its input pipe")
	}
	s := startFixture(t, 80, 24)
	s.WaitFor("READY:80x24")
	s.Write("\x1b[O")
	s.WaitFor("FOCUS:out")
	s.Write("q")
	if err := s.Wait(5 * time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestPTYFocusIn(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("ConPTY does not provide a deterministic way to synthesize a host focus transition from its input pipe")
	}
	s := startFixture(t, 80, 24)
	s.WaitFor("READY:80x24")
	s.Write("\x1b[I")
	s.WaitFor("FOCUS:in")
	s.Write("q")
	if err := s.Wait(5 * time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestPTYUnicodeInput(t *testing.T) {
	s := startFixture(t, 80, 24)
	s.WaitFor("READY:80x24")
	s.Write("é")
	s.WaitFor(`KEY:name=é,text="é",ctrl=false,alt=false,shift=false`)
	s.Write("q")
	if err := s.Wait(5 * time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestPTYImmediateResizeDuringStartupIsNotLost(t *testing.T) {
	s := startFixture(t, 73, 21)
	// Resize immediately, before waiting for the first frame. The child may see
	// the new size as its initial geometry or as SIGWINCH/ConPTY resize, but it
	// must not remain stuck at the old size waiting for unrelated keyboard input.
	s.Resize(101, 33)
	s.WaitForTimeout("101x33", 6*time.Second)
	s.Write("q")
	if err := s.Wait(5 * time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestPTYLargeBracketedPasteAcrossReads(t *testing.T) {
	s := startFixture(t, 80, 24)
	s.WaitFor("READY:80x24")
	payloadBytes := 24 * 1024 // > Runtime's generic 8192-byte read buffer.
	if runtime.GOOS == "windows" {
		// Native Windows input arrives as INPUT_RECORD batches (64 records per
		// read), so 4 KiB already crosses many real pump reads without making
		// headless ConPTY CI process hundreds of render batches unnecessarily.
		payloadBytes = 4 * 1024
	}
	payload := strings.Repeat("abcdefgh", payloadBytes/8)
	s.Write("\x1b[200~" + payload + "\x1b[201~")
	s.WaitForTimeout(fmt.Sprintf("PASTE_LEN:%d", len(payload)), 8*time.Second)
	s.Write("q")
	if err := s.Wait(5 * time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestPTYCallbackPanicStillRestoresTerminal(t *testing.T) {
	s := harness.Start(t, fixtureBinary, 80, 24, "INKGO_PTY_PANIC=1")
	s.WaitFor("READY:80x24")
	s.Write("p")
	if err := s.Wait(5 * time.Second); err == nil {
		t.Fatal("expected fixture panic to exit non-zero")
	}
	out := s.Output()
	if !strings.Contains(out, altScreenExit) {
		t.Fatalf("panic path did not restore alternate screen; output=%q", out)
	}
	if !strings.Contains(out, "intentional PTY fixture panic") {
		t.Fatalf("panic output missing sentinel; output=%q", out)
	}
}

func TestPTYResizeIsObservedWithoutKeyboardInput(t *testing.T) {
	s := startFixture(t, 73, 21)
	s.WaitFor("READY:73x21")

	s.Resize(101, 33)
	// Unix delivers SIGWINCH; Windows ConPTY should surface a console resize
	// event to the ReadConsoleInputW pump. No key is sent to provoke a render.
	s.WaitForTimeout("RESIZE:101x33", 6*time.Second)

	s.Write("q")
	if err := s.Wait(5 * time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestPTYIsolatedEscapeFlushesAndStops(t *testing.T) {
	s := startFixture(t, 80, 24)
	s.WaitFor("READY:80x24")
	s.Write("\x1b")
	if err := s.Wait(3 * time.Second); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s.Output(), altScreenExit) {
		t.Fatalf("terminal was not restored after isolated Escape; output=%q", s.Output())
	}
}

func TestPTYCtrlCIsInputInRawModeAndStops(t *testing.T) {
	s := startFixture(t, 80, 24)
	s.WaitFor("READY:80x24")
	s.Write("\x03")
	if err := s.Wait(3 * time.Second); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s.Output(), altScreenExit) {
		t.Fatalf("terminal was not restored after Ctrl+C; output=%q", s.Output())
	}
}

func TestPTYMouseClick(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("headless ConPTY cannot reliably synthesize a physical mouse transition; native INPUT_RECORD translation is covered separately")
	}
	s := startFixture(t, 80, 24)
	s.WaitFor("READY:80x24")
	// Runtime enables SGR mouse reporting on entry. Coordinates are 1-based
	// on the wire and become zero-based ClickEvent coordinates.
	s.Write("\x1b[<0;10;10M")
	s.Write("\x1b[<0;10;10m")
	s.WaitFor("MOUSE:x=9,y=9,button=0")
	s.Write("q")
	if err := s.Wait(5 * time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestPTYGracefulExitRestoresAllRuntimeModes(t *testing.T) {
	s := startFixture(t, 80, 24)
	s.WaitFor("READY:80x24")
	s.Write("q")
	if err := s.Wait(5 * time.Second); err != nil {
		t.Fatal(err)
	}
	out := s.Output()
	for _, seq := range []string{
		"\x1b[?2004l", // bracketed paste
		"\x1b[?1004l", // focus events
		"\x1b[?1000l", // mouse button tracking
		"\x1b[?1002l", // mouse drag tracking
		"\x1b[?1006l", // SGR mouse mode
		"\x1b[?25h",   // cursor visible
		altScreenExit,
	} {
		if !strings.Contains(out, seq) {
			t.Fatalf("exit did not restore %q; output=%q", seq, out)
		}
	}
}

func TestPTYRepeatedSessionsRestoreCleanly(t *testing.T) {
	for i := 0; i < 8; i++ {
		s := harness.Start(t, fixtureBinary, 80+i, 24)
		s.WaitFor("READY:")
		s.Write("q")
		if err := s.Wait(5 * time.Second); err != nil {
			t.Fatalf("session %d: %v", i, err)
		}
		if !strings.Contains(s.Output(), altScreenExit) {
			t.Fatalf("session %d did not restore alternate screen", i)
		}
		s.Close()
	}
}

func TestPTYResizeBurstSettlesAtFinalSize(t *testing.T) {
	if runtime.GOOS == "windows" {
		// This is intentionally enabled on Windows too: it exercises ConPTY's
		// resize path and inkgo's native ReadConsoleInputW ownership.
	}
	s := startFixture(t, 80, 24)
	s.WaitFor("READY:80x24")
	for _, size := range [][2]int{{81, 25}, {96, 28}, {120, 35}, {91, 27}} {
		s.Resize(size[0], size[1])
	}
	s.WaitForTimeout("RESIZE:91x27", 6*time.Second)
	s.Write("q")
	if err := s.Wait(5 * time.Second); err != nil {
		t.Fatal(err)
	}
}
