# PTY / ConPTY integration validation

## Purpose

Unit tests, fuzzing and statement coverage cannot prove every terminal lifecycle
property. `test/pty` is therefore a separate Go module that launches a compiled
inkgo fixture under a real Unix PTY or Windows ConPTY and observes terminal
bytes/input/resize from outside the process. The published root module remains
dependency-free.

The harness uses `github.com/aymanbagabas/go-pty` pinned to
`v0.2.3-0.20260305173209-bef704d47d97`, a Go-1.23-compatible upstream revision.

## Scenarios

The permanent suite covers:

- alternate-screen entry and graceful restoration;
- ordinary key input and Unicode text;
- bracketed paste, including payloads larger than Runtime's generic read buffer;
- focus and mouse VT input where a headless host can inject them deterministically;
- resize observed without a keyboard wakeup;
- resize immediately during startup;
- resize bursts settling on the final geometry;
- isolated Escape timeout and Ctrl+C as raw-mode input;
- callback panic unwinding with alternate-screen restoration;
- repeated sessions restoring cleanly.

On Windows, focus/mouse host transitions remain covered by the native
`INPUT_RECORD` unit/regression suite because headless ConPTY does not provide a
deterministic physical-host injection mechanism for those event classes.

## Startup resize window

The PTY work also closed a narrow Unix startup window. `Runtime.Run` formerly
called `Start()` (which enters terminal modes and performs the initial render)
before installing SIGWINCH/SIGCONT handlers, so a resize landing between the
first paint and loop setup could stay invisible until unrelated input caused
another render.

The fix installs runtime signal handlers before `Start()`. A resize that arrives
during terminal entry or the first paint is now queued and processed by the UI
owner.

Caveat, recorded deliberately: `TestPTYImmediateResizeDuringStartupIsNotLost` is
**not** a regression test for that reorder. The harness resizes the PTY before
the fixture reads its initial geometry, so the child usually starts with the new
size and no signal is needed; reverting only the reorder still passes the test
over 240 repetitions, and `TestPTYResizeIsObservedWithoutKeyboardInput` /
`TestPTYResizeBurstSettlesAtFinalSize` behave the same. The reorder is defensive
hardening whose benefit is not discriminated by this suite, and it should not be
cited as validated by it.

## Development validation

The optimized harness builds its fixture once per test binary so repetition
primarily stresses PTY/runtime lifecycle rather than the Go compiler. During the
implementation campaign the full PTY suite passed `-count=50 -shuffle=on`, and
the Linux PTY suite passed `-race -count=20 -shuffle=on`.

CI runs the test module on Ubuntu, macOS and Windows at the minimum supported Go
line, plus a Linux PTY race campaign. The root module's normal dependency-policy
gate remains unchanged and still requires zero external modules.

## Commands

```sh
cd test/pty
go mod download
go mod verify
GOFLAGS=-mod=readonly go test -count=5 -shuffle=on -timeout=120s ./...
GOFLAGS=-mod=readonly go vet ./...
```

Linux race campaign:

```sh
cd test/pty
GOFLAGS=-mod=readonly go test -race -count=5 -shuffle=on -timeout=120s ./...
```
