# inkgo

`inkgo` is a native, dependency-free Go TUI engine derived from a customized Ink implementation. It is **not** a Node/React wrapper: there is no Node, Bun, React, Yoga binding, `bidi-js`, sidecar, CGO requirement, or runtime vendor tree.

The library provides concrete `*Node` trees, flex layout, Unicode-aware cell rendering, incremental terminal diffs, keyboard/mouse input, selection/search, scroll containers, terminal lifecycle management, and an embeddable runtime.

## Install

Requires **Go 1.23+**.

```bash
go get github.com/frudas24/inkgo@v0.1.4
```

The source version is `v0.1.4`. The Git tag is the authoritative published version; until that tag is published, consumers of a development checkout can use a local `replace` directive or the repository branch they intentionally pin.

**External Go dependencies: zero.** `go list -m all` contains only `github.com/frudas24/inkgo`.

## What it includes

- `Box`, `Text`, `RawANSI`, `Link`, `Button`, `ScrollBox`, `Spacer`, `Newline`, `NoSelect`, `AlternateScreen`, `ErrorOverview`
- row/column/reverse flex layout, wrap, grow/shrink, percentages, min/max, gaps, margin/padding, borders, absolute positioning and overflow
- 10k-row scroll containers with cached geometry and visible-range painting on stable layouts
- sticky/follow scrolling, anchors/clamps, smooth wheel drain, xterm.js adaptive policy and fullscreen hardware scroll
- Unicode cell measurement, combining marks, wide glyphs, emoji/ZWJ clusters, tab stops and mixed RTL/numeric software bidi fallback
- SGR/16/256/RGB ANSI, OSC-8 hyperlinks and raw styled ANSI
- double-buffered cell `Screen`, fuzz-hardened wide-cell atomicity, soft-wrap provenance, damage bounds and incremental patching
- relative main-screen updates and `DECSTBM + SU/SD` alternate-screen fast paths
- focus/tab order, capture+bubble keyboard/focus/paste/resize events and scroll-aware hit testing
- SGR + X10 mouse, click-on-release, drag suppression, hover and multi-click selection
- bracketed paste, CSI-u/Kitty keys, xterm `modifyOtherKeys`, legacy navigation/function keys and incomplete-sequence timeouts
- advanced selection, keyboard extension, no-select regions, scrolled-off row capture and search highlighting
- terminal focus, suspend/resume, SIGCONT/resize recovery, mode reassertion and extended-key negotiation
- asynchronous terminal queries (`DECRQM`, DA1/DA2, Kitty keyboard, cursor, OSC color, XTVERSION)
- OSC52, tmux and native clipboard paths (`pbcopy`, `wl-copy`, `xclip`, `xsel`, `clip.exe`)
- title, bell, notifications, version-gated progress and tab-status sequences
- shared animation/interval scheduler
- raw/VT terminal support for Linux, macOS and Windows
- serialized runtime output, bounded per-node caches and safe/borrowed renderer ownership modes

## Package layout

Small programs can use the root facade:

```go
import ink "github.com/frudas24/inkgo"

root := ink.Root(
    ink.Box(ink.Style{FlexDirection: ink.Column},
        ink.Text("hello from Go", ink.TextStyle{Bold: true}),
    ),
)
```

Larger programs can import by domain:

```go
import (
    layout "github.com/frudas24/inkgo/layout"
    render "github.com/frudas24/inkgo/render"
    widgets "github.com/frudas24/inkgo/widgets"
)

root := widgets.Root(
    widgets.Box(layout.Style{Width: layout.Percent(100)},
        widgets.Text("hello"),
    ),
)

r := render.New(render.RenderOptions{Width: 80, Height: 24})
frame := r.Render(root)
_ = frame.Patch
```

Domain aliases preserve Go type identity, so nodes/styles do not need adapters or conversion allocations.

The physical implementation lives under `internal/engine`; the root package is a generated compatibility facade. Run `go generate ./...` after changing exported engine declarations.

## Embedding

For a caller-owned event loop:

```go
term := ink.DefaultTerminal()
rt := ink.NewRuntime(root, term.In, term.Out, ink.RenderOptions{
    Fullscreen: true,
    HideCursor: true,
    SynchronizedOutput: ink.SupportsSynchronizedOutput(),
})
rt.Terminal = &term

if err := rt.Start(); err != nil {
    panic(err)
}
defer rt.Close()

// In your reactor/select loop, on the same UI goroutine:
var err error
select {
case chunk := <-inputChunks: // channel supplied by the embedding application
    rt.HandleInput(chunk)
    // mutate nodes/application state
    _, err = rt.RenderSettled()
case <-rt.Events():
    err = rt.ProcessEvents() // timeout callbacks and redraw
}
if err != nil {
    panic(err)
}
```

`Runtime.Run()` is the convenience blocking loop and uses the same lifecycle.
It dispatches input, timer callbacks, and terminal signals on its UI goroutine.
Embedded loops must service `Events()` with `ProcessEvents()` to deliver Escape,
partial-paste, and delayed hyperlink callbacks. Failed starts restore terminal
state before returning and can be retried.

`Stop()` wakes `Run()` even while input is blocked, without closing caller-owned
input. A generic `io.Reader` cannot cancel an outstanding read: the input worker
may remain blocked until that read returns, and may consume that next chunk.
Close or otherwise unblock the source before reusing it elsewhere; applications
that need full control over input cancellation should own the input loop.

## Renderer ownership

`Frame.Screen` is a stable snapshot by default. Hot loops that consume a frame immediately can opt into renderer-owned storage:

```go
r := render.New(render.RenderOptions{
    Width: 80, Height: 24,
    BorrowFrameScreen: true,
})
frame := r.Render(root)
// frame.Screen must be consumed before the next Render call.
```

For a materialized 10,000-row `ScrollBox`, the Round-5 validation host measured the final warm scroll path at about **0.114 ms/frame and 6.7 KB/frame** after the initial layout. The first frame remained about **130 ms** because creating/measuring all 10,000 concrete nodes is intentionally O(n). See the validation report for the exact benchmark command and allocation counts.

## Development

```bash
gofmt -w .
go generate ./...
go test ./...
go vet ./...
go test -race ./...
./scripts/check-coverage.sh 74.0 coverage.out
./scripts/check-no-external-deps.sh
./scripts/check-version.sh
```

Fuzz targets:

```bash
go test ./internal/inputparser -run='^$' -fuzz=FuzzParserNeverPanics -fuzztime=5s
go test ./internal/engine -run='^$' -fuzz=FuzzScreenWideCellInvariants -fuzztime=5s
go test ./internal/engine -run='^$' -fuzz=FuzzLayoutAndRenderInvariants -fuzztime=5s
```

Examples:

```bash
go run ./examples/fullscreen
go run ./examples/embed
go run ./examples/terminal-smoke
```

`terminal-smoke` is intentionally interactive and exercises the host console/PTY, including the Windows raw/VT path when run from Windows Terminal or PowerShell.

## Documentation

- [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) — ownership and package boundaries
- [`docs/MIGRATION.md`](docs/MIGRATION.md) — mapping from the TypeScript/React model
- [`docs/STABILITY.md`](docs/STABILITY.md) — public API/version policy
- [`docs/RELEASE.md`](docs/RELEASE.md) — `v0.1.4` publication checklist and repository metadata
- [`docs/PORT_STATUS.md`](docs/PORT_STATUS.md) — remaining parity boundary
- [`docs/validation/ROUND5.md`](docs/validation/ROUND5.md) — production-hardening evidence

## License

MIT — see [`LICENSE`](LICENSE). This library is a native Go port of Ink
(MIT, Vadim Demedes) via a customized Ink fork; see
[`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md) for the lineage and the
outstanding clarification about the fork's undeclared customizations.
