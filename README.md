# reopencode TUI / Ink — native Go port

This tree is a **native Go rewrite** of the customized `ink/` source supplied on 2026-09-09. It is not a Node wrapper and does not require React, Bun, Yoga bindings, `bidi-js`, Chalk, or any other runtime dependency.

The design keeps the behavior that matters to reopencode while using Go-native ownership: a concrete `*Node` tree replaces React Fiber, direct mutation + dirty propagation replaces reconciliation, and `Renderer` owns layout/screen diff state.

## What is implemented

- `Box`, `Text`, `RawANSI`, `Link`, `Button`, `ScrollBox`, `Spacer`, `Newline`, `NoSelect`, `AlternateScreen`
- row/column/reverse flex layout, wrapping, grow/shrink, percentages, min/max, gaps, margins/padding, absolute/relative positioning, borders and overflow
- ScrollBox inner-content semantics (`flexGrow:1`, `flexShrink:0`), sticky scroll, anchor, clamps and per-frame pending-wheel drain
- Unicode cell measurement, wide characters, combining marks, emoji/ZWJ clusters, 8-column terminal tab stops
- ANSI SGR parser, 16/256/RGB colors, OSC-8 hyperlinks and styled raw ANSI
- screen buffer, wide-cell spacer handling, damage bounds, incremental patches
- safe **relative** main-screen updates (does not address the shell viewport absolutely)
- alternate-screen absolute diff plus full-width `DECSTBM + SU/SD` hardware scrolling
- focus/tab order, capture+bubble keyboard/focus handlers, hit testing through nested scroll viewports
- SGR mouse, bracketed paste, CSI-u/Kitty keys, xterm `modifyOtherKeys`, legacy function/navigation keys, terminal responses
- fullscreen selection, `noSelect` / `from-left-edge`, soft-wrap-aware copy, search scanning/highlight
- Button state render callback (`focused`, `hovered`, `active`) and 100 ms active state expiry
- declared native cursor for IME/accessibility
- terminal capability helpers, OSC52 clipboard sequence, title/progress/notification/tab-status helpers
- Linux raw TTY support and Windows Console raw/VT input + VT output setup

There are **no external Go dependencies**.

## Quick start

```go
term := inkgo.DefaultTerminal()
root := inkgo.Root(
    inkgo.AlternateScreen(
        inkgo.Box(
            inkgo.Style{FlexDirection: inkgo.Column, Padding: inkgo.I(1)},
            inkgo.Text("hello from Go", inkgo.TextStyle{Bold: true}),
        ),
    ),
)

rt := inkgo.NewRuntime(root, term.In, term.Out, inkgo.RenderOptions{
    Fullscreen: true,
    SynchronizedOutput: inkgo.SupportsSynchronizedOutput(),
    HideCursor: true,
})
rt.Terminal = &term
err := rt.Run()
```

Run the included interactive example:

```bash
go run ./examples/fullscreen
```

## Development

```bash
gofmt -w .
go vet ./...
go test ./...
go test -race ./...
```

`MIGRATION.md` maps the TS/React API to Go. `PORT_STATUS.md` documents the exact parity boundary instead of pretending every implementation detail is identical.
