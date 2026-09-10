package harness

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	pty "github.com/aymanbagabas/go-pty"
)

const defaultTimeout = 5 * time.Second

// BuildFixture builds one test program in the PTY integration module and
// returns its absolute executable path. Tests intentionally execute a real
// child process rather than calling inkgo APIs in-process.
func BuildFixture(t testing.TB, pkg string) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "fixture")
	if runtime.GOOS == "windows" {
		out += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", out, pkg)
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=local")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build fixture %s: %v\n%s", pkg, err, b)
	}
	abs, err := filepath.Abs(out)
	if err != nil {
		t.Fatalf("fixture path: %v", err)
	}
	return abs
}

type Session struct {
	t   testing.TB
	Pty pty.Pty
	Cmd *pty.Cmd

	cancel   context.CancelFunc
	mu       sync.Mutex
	out      bytes.Buffer
	wake     chan struct{}
	readErr  error
	waitCh   chan error
	readDone chan struct{}
	closed   sync.Once
}

func Start(t testing.TB, executable string, width, height int, env ...string) *Session {
	t.Helper()
	p, err := pty.New()
	if err != nil {
		t.Fatalf("pty.New: %v", err)
	}
	if err := p.Resize(width, height); err != nil {
		_ = p.Close()
		t.Fatalf("pty resize %dx%d: %v", width, height, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cmd := p.CommandContext(ctx, executable)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "COLORTERM=truecolor")
	cmd.Env = append(cmd.Env, env...)
	if err := cmd.Start(); err != nil {
		cancel()
		_ = p.Close()
		t.Fatalf("pty command start: %v", err)
	}
	s := &Session{
		t: t, Pty: p, Cmd: cmd, cancel: cancel,
		wake: make(chan struct{}, 1), waitCh: make(chan error, 1),
		readDone: make(chan struct{}),
	}
	go s.readLoop()
	go func() { s.waitCh <- cmd.Wait() }()
	t.Cleanup(func() { s.Close() })
	return s
}

func (s *Session) readLoop() {
	defer close(s.readDone)
	buf := make([]byte, 16*1024)
	for {
		n, err := s.Pty.Read(buf)
		s.mu.Lock()
		if n > 0 {
			_, _ = s.out.Write(buf[:n])
		}
		if err != nil {
			s.readErr = err
		}
		s.mu.Unlock()
		select {
		case s.wake <- struct{}{}:
		default:
		}
		if err != nil {
			return
		}
	}
}

func (s *Session) Output() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.out.String()
}

func (s *Session) Write(data string) {
	s.t.Helper()
	if _, err := s.Pty.Write([]byte(data)); err != nil {
		s.t.Fatalf("pty write %q: %v\noutput:\n%s", data, err, s.Output())
	}
}

func (s *Session) Resize(width, height int) {
	s.t.Helper()
	if err := s.Pty.Resize(width, height); err != nil {
		s.t.Fatalf("pty resize %dx%d: %v", width, height, err)
	}
}

func (s *Session) WaitFor(substr string) string {
	s.t.Helper()
	return s.WaitForTimeout(substr, defaultTimeout)
}

func (s *Session) WaitForTimeout(substr string, timeout time.Duration) string {
	s.t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		out := s.Output()
		if strings.Contains(out, substr) {
			return out
		}
		select {
		case <-s.wake:
		case err := <-s.waitCh:
			// Put the result back so Wait can still observe it.
			select {
			case s.waitCh <- err:
			default:
			}
			s.t.Fatalf("process exited before output %q: %v\noutput:\n%s", substr, err, out)
		case <-deadline.C:
			s.mu.Lock()
			readErr := s.readErr
			s.mu.Unlock()
			s.t.Fatalf("timeout waiting for %q (readErr=%v)\noutput:\n%s", substr, readErr, out)
		}
	}
}

func (s *Session) Wait(timeout time.Duration) error {
	s.t.Helper()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case err := <-s.waitCh:
		// Process exit and PTY EOF are independent observations. In particular
		// under the race detector the waiter can win while readLoop still has
		// the runtime's final restoration sequences buffered. Give the reader a
		// bounded chance to consume those bytes before callers inspect Output.
		s.waitForReadDrain()
		return normalizeWaitError(s.Cmd, err)
	case <-timer.C:
		return fmt.Errorf("timeout waiting for process exit; output:\n%s", s.Output())
	}
}

func (s *Session) waitForReadDrain() {
	// The PTY master may intentionally stay open after the child exits, so EOF
	// is not a reliable drain signal on every backend. Instead wait until the
	// reader has been quiet for a short interval, with a hard cap so a noisy or
	// unusual ConPTY implementation can never stall the suite.
	const (
		quiet    = 20 * time.Millisecond
		maxDrain = 250 * time.Millisecond
	)
	quietTimer := time.NewTimer(quiet)
	capTimer := time.NewTimer(maxDrain)
	defer quietTimer.Stop()
	defer capTimer.Stop()
	for {
		select {
		case <-s.readDone:
			return
		case <-s.wake:
			if !quietTimer.Stop() {
				select {
				case <-quietTimer.C:
				default:
				}
			}
			quietTimer.Reset(quiet)
		case <-quietTimer.C:
			return
		case <-capTimer.C:
			return
		}
	}
}

// normalizeWaitError keeps the harness compatible with the last Go-1.23
// friendly go-pty commit and newer go-pty, which differ only in whether a
// non-zero Windows exit is materialized as *exec.ExitError.
func normalizeWaitError(cmd *pty.Cmd, err error) error {
	if err != nil {
		return err
	}
	if cmd != nil && cmd.ProcessState != nil && !cmd.ProcessState.Success() {
		return fmt.Errorf("process exited unsuccessfully: %s", cmd.ProcessState.String())
	}
	return nil
}

func (s *Session) Close() {
	s.closed.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}
		if s.Pty != nil {
			_ = s.Pty.Close()
		}
	})
}

func IsClosedReadError(err error) bool {
	return err == nil || errors.Is(err, os.ErrClosed)
}
