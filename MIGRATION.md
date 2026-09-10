# Migration map: customized Ink -> Go

Do not translate React application code literally. Keep long-lived Go node references and mutate them. The next render recalculates the affected UI and emits terminal damage.

| TypeScript / React | Go |
|---|---|
| `<Box ...>` | `widgets.Box(...)` / `tui.Box(...)` |
| `<Text>` | `widgets.Text(...)` |
| `<RawAnsi>` / `<Ansi>` | `widgets.RawANSI(...)` |
| `<Link url>` | `widgets.Link(...)` |
| `<Button onAction>` | `widgets.Button(...)` |
| Button render prop | `ButtonWithState(..., func(ButtonState) []*Node {...})` |
| `<ScrollBox>` | `widgets.ScrollBox(...)` |
| `<Spacer>` / `<Newline>` | `widgets.Spacer()` / `widgets.Newline(n)` |
| `<NoSelect>` | `NoSelectBox(...)` / `NoSelectFromLeft(...)` |
| `<AlternateScreen>` | `widgets.AlternateScreen(...)` |
| `<ErrorOverview>` | `widgets.ErrorOverview(err)` |
| React reconciliation | `SetText`, `SetStyle`, `SetChildren`, `Append`, `Remove` |
| replace application tree | `Runtime.SetRoot(root)` |
| `useInput` | `EventHandlers.OnKeyDown` or `Runtime.HandleInput` |
| focus hooks | `interaction.FocusManager` / `Runtime.Focus` |
| terminal focus hook | `Runtime.TerminalFocused()` / `TerminalFocusState()` |
| `useSelection` | `Runtime.Selection`, `Selection.Text`, `CopySelection` |
| search hook | `SetSearchHighlight`, `SetSearchPositions`, `ClearSearch` |
| `useDeclaredCursor` | `Renderer.DeclareCursor(...)` |
| `useTerminalViewport` | `Runtime.Viewport()` / `Runtime.IsVisible(node)` |
| `ClockProvider` | `scheduler.New(...)` |
| `useAnimationFrame` | `Clock.Every(interval, true, callback)` + `Runtime.IsVisible` |
| `useInterval` | `Clock.Every(interval, false, callback)` |
| terminal raw writer context | `Runtime.WriteRaw(...)` |
| clear terminal | `Runtime.ClearTerminal()` / `terminal.ClearSequence()` |
| terminal title | `Runtime.WriteRaw(TerminalTitle(...))` |
| notifications | `NotifyITerm2`, `NotifyKitty`, `NotifyGhostty`, `Bell` |
| progress | `ProgressSequence(...)` |
| clipboard | `Runtime.CopySelection` / `SetClipboard` |

## Style conversion

```tsx
<Box width="100%" height={8} paddingX={1} flexDirection="column" />
```

becomes:

```go
widgets.Box(layout.Style{
    Width:         layout.Percent(100),
    Height:        layout.Cells(8),
    PaddingX:      layout.I(1),
    FlexDirection: layout.Column,
})
```

Pointer helpers such as `I`, `F`, and `B` preserve the distinction between an unset property and an explicit zero/false value.

## Stateful UI

```go
status := widgets.Text("idle")

// later, on the UI/event goroutine
status.SetText("working")
_, err := rt.Render()
```

No component rebuild is required. If a state change starts a large wheel movement, `RenderSettled()` preserves the fork's multi-frame drain contract.

## ScrollBox

```go
scroll := widgets.ScrollBox(
    layout.Style{Height: layout.Cells(12), Width: layout.Percent(100)},
    true,
    rows...,
)
scroll.ScrollBy(3)
scroll.ScrollTo(40)
scroll.ScrollToBottom()
scroll.ScrollToElement(row, -2)
```

The outer viewport owns clipping/scroll state and contains a non-shrinking column, matching the source fork's important layout invariant.

## Main screen vs alternate screen

The renderer intentionally uses two output strategies. Main-screen patches are relative to the application's block so shell scrollback remains safe. Fullscreen patches can use absolute rows and `DECSTBM + SU/SD` hardware scrolling. Do not collapse those paths into unconditional absolute cursor addressing.

## Importing into a large Go program

Use domain packages when they make dependency ownership clearer:

```text
widgets -> application view construction
layout  -> application styling/layout config
input + interaction -> controller/event layer
selection -> editor/search feature layer
render  -> renderer tests/custom rendering
terminal -> executable/PTY integration layer
scheduler -> animation/timing layer
```

The domain packages share canonical types, so `*widgets.Node` can be passed directly to `render.Renderer`, `interaction.HitTest` and `terminal.Runtime`.
