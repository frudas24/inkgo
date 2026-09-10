# Architecture

`github.com/frudas24/inkgo` is designed as an embeddable Go library, not as a translated JavaScript runtime.

## Package boundaries

The root package (`package ink`) is the **canonical kernel and compatibility surface**. It owns the concrete `Node`, `Style`, `Renderer`, `Runtime`, event and screen types so those objects keep one type identity across the library. Large consumers do not need to import that whole conceptual surface directly: narrow domain packages expose stable views over it.

```text
github.com/frudas24/inkgo
├── widgets      declarative nodes: Box/Text/Button/ScrollBox/...
├── layout       style, geometry and flex layout
├── text         Unicode width, wrapping, ANSI and bidi
├── render       cell screen, damage and renderer
├── input        escape-sequence parser and key/mouse records
├── interaction  focus, hit testing and capture/bubble events
├── selection    text selection, URL lookup and search overlays
├── terminal     runtime, terminal modes, queries, clipboard, notifications
└── scheduler    shared animation/interval clock
```

Domain packages intentionally reuse the canonical root types through Go type aliases. That gives a larger application two useful properties at once:

- package boundaries communicate ownership and keep imports small;
- a `*widgets.Node` is exactly the same type as a `*layout.Node`, `*render.Node` or root `*inkgo.Node`, with no adapters, allocations or conversion layers.

Domain packages do not import each other, so there is no horizontal dependency mesh. They depend only on the canonical kernel. This leaves room to move implementation details into `internal/` later without changing consumer type identity or import paths.

## Ownership model

The application owns a long-lived `*Node` tree. Node mutation (`SetText`, `SetStyle`, `SetChildren`, scrolling, handlers) marks the relevant path dirty. `Renderer` owns physical-screen history and emits only terminal damage. `Runtime` owns terminal lifecycle and event orchestration when the application chooses the built-in loop.

There is no hidden React reconciler, virtual DOM, Node process or sidecar.

For a program that already has its own event loop, use `Runtime.HandleInput` plus `Runtime.Render`/`RenderSettled`. `Runtime.SetRoot` can replace the whole UI tree while preserving terminal state and valid focus. `SuspendTerminal`/`ResumeTerminal` bracket external editors or subprocesses.

## State and concurrency

UI tree mutation is intentionally explicit. As with most terminal UI frameworks, application-owned node mutation and callbacks should be serialized by the application's UI/event goroutine. Internally, terminal parsing/query bookkeeping, renderer screen state and the shared scheduler protect their own concurrent state.

The `scheduler.Clock` consolidates animation wake-ups: passive subscribers do not keep the timer alive, visible animations can opt into keep-alive, and terminal blur can switch the shared clock to a slower cadence.

## Rendering pipeline

```text
Node tree
  -> layout
  -> scroll/selection reconciliation
  -> paint to cell Screen
  -> selection/search overlays
  -> damage detection
  -> main-screen relative diff OR fullscreen absolute/hardware-scroll diff
  -> terminal patch
```

Main-screen and alternate-screen output deliberately remain separate. The main screen never assumes it owns absolute terminal rows; fullscreen can safely use absolute addressing and `DECSTBM + SU/SD` scrolling.

## Extension rules

When adding features:

1. Put behavioral state on the owning domain object, not in package globals.
2. Keep terminal escape generation separate from UI policy.
3. Preserve root type identity; expose new concepts through the narrowest domain package.
4. Prefer standard library implementations over vendoring JS behavior.
5. Add a parity test for behavior copied from the TypeScript source.
6. Do not introduce QA-style terminal special cases unless a capability/protocol actually requires them.
