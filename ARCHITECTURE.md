# Architecture

`github.com/frudas24/inkgo` is an embeddable Go TUI library, not a translated JavaScript runtime.

## Public package boundaries

```text
github.com/frudas24/inkgo
├── widgets      declarative nodes: Box/Text/Button/ScrollBox/...
├── layout       style, geometry and flex-layout entry points
├── text         Unicode width, wrapping, ANSI and bidi
├── render       cell screen, damage and renderer
├── input        terminal input parser and normalized records
├── interaction  focus, hit testing and capture/bubble events
├── selection    text selection, URL lookup and search overlays
├── terminal     lifecycle, capabilities, queries, clipboard, notifications
└── scheduler    shared animation/interval clock
```

The root package remains a convenience/compatibility facade. New application code can prefer domain packages while old code can continue importing the root.

## Dependency direction

Round 3 begins the internal inversion away from a god package. Stable leaf concepts now own their implementations:

```text
internal/core
  geometry + Length/Style/Color/TextStyle + normalized Key

internal/escscan
  streaming escape-sequence boundary scanner

internal/inputparser
  key/mouse/paste/terminal-response parsing
  -> internal/core
  -> internal/escscan

internal/textutil
  graphemes + cell width + wrapping + truncation + tab expansion
  -> internal/core
  -> internal/escscan

scheduler
  shared clock implementation (no root-package dependency)

root facade / orchestration
  Node + layout engine + Screen/Renderer + Selection + Runtime
  -> leaf packages above
```

This is intentionally incremental. The stateful orchestration domains (`Node`, flex engine, renderer, selection and terminal runtime) still live in the root because they currently share object identity and mutable state heavily. Moving them mechanically would create adapters or cycles without improving behavior. Future extraction should happen behind existing public aliases only when a clean dependency cut exists.

## Type identity

Foundational types such as `Style`, `Color`, `Rect`, `Length` and `Key` have one canonical leaf identity. Root and public domain packages expose aliases, not conversion wrappers. Therefore a `layout.Style`, root `Style`, and any renderer field using that style are the exact same Go type.

The same principle applies to the long-lived `*Node`: domain packages currently alias the canonical node so callers never pay adapter allocations or lose pointer identity.

## Ownership model

The application owns a long-lived `*Node` tree. Node mutation (`SetText`, `SetStyle`, `SetChildren`, scrolling, handlers) marks the relevant path dirty. `Renderer` owns physical-screen history and emits only terminal damage. `Runtime` owns terminal lifecycle and event orchestration when requested.

There is no hidden React reconciler, virtual DOM, Node process or sidecar.

For a program with its own event loop:

```text
Runtime.Start()
  -> application read/select loop
       -> Runtime.HandleInput(bytes)
       -> mutate nodes/application state
       -> Runtime.RenderSettled()
Runtime.Close()
```

`Runtime.Run()` is only the convenience blocking loop and reuses the same lifecycle path.

## State and concurrency

Application-owned node mutation and callbacks should be serialized by the application's UI/event goroutine. Internally, parser state, query bookkeeping, renderer physical-screen state, lifecycle state and the shared scheduler protect their own concurrent state where needed.

`scheduler.Clock` consolidates animation wake-ups: passive subscribers do not keep the timer alive, visible animations can opt into keep-alive, and terminal blur can switch the shared clock to a slower cadence.

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

1. Put behavioral state on the owning domain object, not in package globals.
2. Keep terminal protocol scanning/generation separate from UI policy.
3. Preserve canonical type identity; expose aliases rather than adapters where practical.
4. New leaf domains must not import the root package.
5. Prefer standard-library implementations over vendoring JS behavior.
6. Add parity/regression coverage for behavior copied from the TypeScript source.
7. Fuzz parsers and byte-boundary logic independently from the runtime.
8. Do not introduce terminal-specific hacks unless a capability/protocol or failing fixture requires them.
