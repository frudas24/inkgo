# inkgo — native Go terminal UI (Ink-style)

A native, dependency-free Go rewrite of the customized Ink TUI supplied on 2026-09-09. It is **not** a Node/React wrapper: no Node, Bun, React, Yoga binding, `bidi-js`, sidecar or runtime vendor tree is required.

The port preserves the behavior useful to applications while using Go-native ownership: concrete `*Node` trees, explicit state mutation, flex layout, cell rendering, incremental terminal diffs and an embeddable runtime.

## Current functionality

- `Box`, `Text`, `RawANSI`, `Link`, `Button`, `ScrollBox`, `Spacer`, `Newline`, `NoSelect`, `AlternateScreen`, `ErrorOverview`
- row/column/reverse flex layout, wrap, grow/shrink, percentages, min/max, gaps, margin/padding, borders, absolute positioning and overflow
- ScrollBox sticky/follow behavior, anchors/clamps, smooth pending-wheel drain, xterm.js adaptive drain and fullscreen hardware scroll
- Unicode cell measurement, combining marks, wide glyphs, emoji/ZWJ clusters, tab stops and contextual mixed RTL/numeric software bidi fallback
- SGR/16/256/RGB ANSI, OSC-8 hyperlinks and raw styled ANSI
- cell `Screen`, wide-cell spacer correctness, damage bounds and incremental patching
- safe relative updates on the main screen; absolute diff plus `DECSTBM + SU/SD` in alternate screen
- focus/tab order, capture+bubble keyboard/focus/paste/resize events and scroll-aware hit testing
- SGR + X10 mouse, click-on-release, drag suppression, hover, multi-click word/line selection and drag-edge scrolling
- bracketed paste, CSI-u/Kitty keys, xterm `modifyOtherKeys`, legacy navigation/function keys and incomplete-sequence timeouts
- advanced selection including keyboard extension, soft wraps, no-select regions, scrolled-off row capture and sticky-follow reconciliation
- visible and positioned search highlighting for virtualized content
- declared physical cursor for IME/accessibility
- terminal focus state, suspend/resume, SIGCONT/resize recovery, mode reassertion and extended-key negotiation
- asynchronous terminal queries with DA1 barrier (`DECRQM`, DA1/DA2, Kitty keyboard, cursor, OSC color, XTVERSION)
- OSC52, tmux and native clipboard paths (`pbcopy`, `wl-copy`, `xclip`, `xsel`, `clip.exe`)
- title, bell, notifications, version-gated progress and tab-status control sequences
- shared `scheduler.Clock` for synchronized/visibility-aware application animations
- raw/VT terminal support for Linux, macOS and Windows

**External Go dependencies: zero.**

## Import styles

Small programs can use the root package directly:

```go
import tui "github.com/frudas24/inkgo"

root := tui.Root(
    tui.Box(tui.Style{FlexDirection: tui.Column},
        tui.Text("hello from Go", tui.TextStyle{Bold: true}),
    ),
)
```

Larger applications can import by domain:

```go
import (
    layout "github.com/frudas24/inkgo/layout"
    render "github.com/frudas24/inkgo/render"
    terminal "github.com/frudas24/inkgo/terminal"
    widgets "github.com/frudas24/inkgo/widgets"
)

root := widgets.Root(
    widgets.Box(layout.Style{Width: layout.Percent(100)},
        widgets.Text("hello"),
    ),
)

r := render.New(render.RenderOptions{Width: 80, Height: 24})
_ = r.Render(root)
_ = terminal.ClearSequence()
```

All domain `Node` aliases have identical Go type identity, so no adapters are required.

## Full runtime

```go
term := tui.DefaultTerminal()
root := tui.Root(tui.AlternateScreen(
    tui.Box(
        tui.Style{FlexDirection: tui.Column, Padding: tui.I(1)},
        tui.Text("hello from Go", tui.TextStyle{Bold: true}),
    ),
))

rt := tui.NewRuntime(root, term.In, term.Out, tui.RenderOptions{
    Fullscreen: true,
    SynchronizedOutput: tui.SupportsSynchronizedOutput(),
    HideCursor: true,
})
rt.Terminal = &term
if err := rt.Run(); err != nil {
    panic(err)
}
```

For an existing application event loop, use the explicit lifecycle:

```go
if err := rt.Start(); err != nil {
    panic(err)
}
defer rt.Close()

// inside your own reactor/select loop:
rt.HandleInput(chunk)
// mutate application/node state
_, err := rt.RenderSettled()
```

You do not have to give the library ownership of the process loop.

## Examples and development

```bash
go run ./examples/fullscreen
go run ./examples/embed

gofmt -w .
go vet ./...
go test ./...
go test -race ./...
go test ./internal/inputparser -run='^$' -fuzz=FuzzParserNeverPanics -fuzztime=3s
```

See `ARCHITECTURE.md` for package boundaries, `MIGRATION.md` for TS/React-to-Go mappings, and `PORT_STATUS.md` for the remaining parity boundary.

## License

MIT — see [`LICENSE`](LICENSE). This library is a native Go port of Ink
(MIT, Vadim Demedes) via a customized Ink fork; see
[`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md) for the full lineage and
the outstanding clarification about the fork's undeclared customizations.
