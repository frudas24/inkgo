# Architecture

`github.com/frudas24/inkgo` is an embeddable Go TUI library. The architecture intentionally separates the stable public surface from the stateful implementation so applications do not depend on implementation file layout.

## Repository shape

```text
inkgo/
├── api.go                    generated root compatibility facade
├── version.go                source version constant
├── widgets/                  public node constructors
├── layout/                   public layout/style surface
├── text/                     public Unicode/text helpers
├── render/                   public screen/renderer surface
├── input/                    public normalized input parser surface
├── interaction/              public focus/hit/event surface
├── selection/                public selection/search surface
├── terminal/                 public terminal/runtime surface
├── scheduler/                public shared clock implementation
├── internal/
│   ├── engine/               stateful node/layout/render/runtime implementation
│   ├── core/                 stable geometry/style/key value types
│   ├── escscan/              streaming ANSI/CSI/OSC/DCS boundaries
│   ├── inputparser/          stateful input parser
│   ├── textutil/             grapheme/width/wrap/tab helpers
│   └── cmd/genapi/           reproducible root-facade generator
├── integration/              external-package composition tests
├── test/pty/                 separate Go module for PTY/ConPTY end-to-end tests
├── examples/                 standalone/embed/manual terminal smoke
├── scripts/                  release/CI policy checks
└── docs/                     architecture, migration, status, validation
```

The root is deliberately boring. `api.go` contains aliases/forwarders generated from `internal/engine`; implementation files no longer live beside `README.md` and `go.mod`.

## Dependency direction

```text
internal/core       internal/escscan
      │                  │
      ├──────┐      ┌────┘
      ▼      ▼      ▼
internal/textutil  internal/inputparser
          \          /
           \        /
            ▼      ▼
           internal/engine
                 │
                 ▼
          root facade (ink)
                 │
      public domain packages
```

`scheduler` owns its clock implementation directly and does not require the stateful engine.

Public domain packages intentionally alias canonical root/engine identities. A `*widgets.Node` is the same Go pointer type accepted by `render.Renderer`, `interaction.HitTest`, `selection`, and `terminal.Runtime`.

## Generated root facade

`api.go` is not hand-maintained:

```bash
go generate ./...
```

`internal/cmd/genapi` scans exported non-test declarations in `internal/engine`, classifies types/constants/variables/functions, and regenerates the facade deterministically. CI runs the generator and rejects drift.

This keeps the convenient flat root API without making the root the implementation owner.

## UI ownership

A node tree has one logical UI owner. The application should serialize node mutation and callbacks on its event/reactor goroutine. `Node` is intentionally not a concurrent mutable graph.

The engine separates layout invalidation from paint invalidation. Text-style, selection, hover, and scrolling can repaint without forcing flex geometry to be recomputed. Geometry-affecting changes invalidate layout up the ancestor path.

On a stable large vertical scroll layout, the engine certifies directly ordered/non-overlapping children and binary-searches the visible range. The layout also retains root-local indexes of scroll/button nodes so hot scroll frames do not repeatedly walk thousands of static descendants just to rediscover control nodes. The initial materialized tree remains O(n); subsequent scroll paint scales primarily with the viewport.

## Runtime concurrency

`Runtime` can own the loop with `Run()` or participate in a caller-owned loop through `Start`, `HandleInput`, `RenderSettled`, and `Close`.

`Run` reads input through a worker and dispatches UI work on its owning goroutine.
Timers and Unix signal handlers enqueue work instead of invoking application
callbacks or mutating nodes from background goroutines. An embedded loop must
select on `Runtime.Events()` and call `Runtime.ProcessEvents()` from the same UI
goroutine as `HandleInput`, rendering, and application state changes. This also
applies to delayed hyperlink callbacks. Canceled timeout events are ignored.

`Stop` wakes the runtime's select loop without closing caller-owned input. For a
generic `io.Reader`, an outstanding read cannot be canceled: its worker remains
until that read returns, potentially consuming one final chunk. The worker issues
no further reads after shutdown. Embedders needing control over cancellation or
input handoff should own the input loop, or unblock their source before reuse.
A failed `Start` restores acquired terminal state and invalidates render history
so a subsequent start emits a complete frame.

Concurrent subsystems protect their own mutable state where appropriate:

- terminal writes share one serialization gate, including async query traffic;
- the input parser synchronizes parser state; timers only enqueue UI work;
- terminal query bookkeeping is synchronized;
- renderer physical-screen history is synchronized;
- lifecycle transitions are synchronized/idempotent;
- `scheduler.Clock` synchronizes subscribers and timer state.

These protections do not make arbitrary concurrent node mutation safe; the node tree remains application/UI-owned.

## Renderer memory ownership

The renderer owns reusable front/back `Screen` storage plus a scratch framebuffer for fullscreen hardware-scroll simulation. Default `Frame.Screen` is cloned into a stable snapshot so a consumer may retain it after another render. `BorrowFrameScreen=true` explicitly borrows renderer storage to reduce bytes allocated in high-frequency loops; that screen must be consumed before the next render. The hardware-scroll scratch buffer is reused and never escapes the renderer.

Per-node text/wrap/ANSI caches retain only the latest key/value, rather than global/LRU histories. Renderer scroll history removes IDs for detached scroll nodes, preventing session-length accumulation as roots are replaced.

## Terminal portability

Linux and macOS use native termios/ioctl paths. Windows uses console mode APIs plus an exclusive `ReadConsoleInputW` pump in `Runtime.Run`: `KEY_EVENT`, `MOUSE_EVENT`, `FOCUS_EVENT` and `WINDOW_BUFFER_SIZE_EVENT` share one owner, and the pump blocks on console input plus a stop event through `WaitForMultipleObjects`. This removes periodic resize polling and prevents a resize watcher from racing key reads. All paths are CGO-free.

CI executes ordinary tests/builds on Linux, macOS, and Windows. `test/pty` is a separate Go module that spawns fixture binaries under a real Unix PTY or Windows ConPTY using `go-pty`, so process-level terminal lifecycle, resize, input and restoration are observed from outside the application without contaminating the published module dependency graph. The PTY module is test infrastructure only; the root `github.com/frudas24/inkgo` module remains dependency-free.

A headless PTY/ConPTY still cannot synthesize every physical host-console event deterministically (notably Windows focus/mouse transitions), so `examples/terminal-smoke` remains the final manual check on actual terminal hosts.

## Compatibility rule

Application code should import documented root/domain packages only. Anything under `internal/` is free to move without compatibility guarantees. See `STABILITY.md` for the v0.x policy.
