# Migration map: customized Ink → Go

The important architectural change is intentional: **do not port React application code literally**. Keep long-lived `*inkgo.Node` references and mutate them. A mutation marks the path dirty; the next `Render()` calculates layout and emits only terminal damage.

| TypeScript / React | Go |
|---|---|
| `<Box ...>` | `inkgo.Box(inkgo.Style{...}, children...)` |
| `<Text>` | `inkgo.Text(...)` / `inkgo.TextWithWrap(...)` |
| `<RawAnsi>` / `<Ansi>` | `inkgo.RawANSI(...)` |
| `<Link url>` | `inkgo.Link(url, child)` |
| `<Button onAction>` | `inkgo.Button(...)` |
| Button render prop | `inkgo.ButtonWithState(..., func(ButtonState) []*Node {...})` |
| `<ScrollBox>` | `inkgo.ScrollBox(style, sticky, children...)` |
| `<Spacer>` | `inkgo.Spacer()` |
| `<Newline count>` | `inkgo.Newline(count)` |
| `<NoSelect>` | `inkgo.NoSelectBox(...)` / `inkgo.NoSelectFromLeft(...)` |
| `<AlternateScreen>` | `inkgo.AlternateScreen(...)` |
| React reconciliation | direct `SetText`, `SetStyle`, `SetChildren`, `Append`, `Remove` |
| `useInput` | `EventHandlers.OnKeyDown` or `Runtime.HandleInput` |
| focus hooks | `FocusManager` / `Runtime.Focus` |
| `useSelection` | `Runtime.Selection`, `Selection.Text` |
| search highlight | `ScanPositions`, `ApplySearchHighlight` |
| `useDeclaredCursor` | `Renderer.DeclareCursor(node, line, column, active)` |
| terminal size hook | `Terminal.Size()` / `Runtime.RefreshSize()` |
| animation/interval hooks | ordinary Go ticker/timer + node mutation + `Runtime.Render()` |
| terminal title | `TerminalTitle(...)` |
| notifications | `NotifyITerm2`, `NotifyKitty`, `NotifyGhostty`, `Bell` |
| progress | `ProgressSequence(...)` |
| clipboard OSC52 | `ClipboardOSC52(...)` |

## Style conversion

Numeric TS dimensions become `inkgo.Cells(n)`. Percent strings become `inkgo.Percent(n)`:

```tsx
<Box width="100%" height={8} paddingX={1} flexDirection="column" />
```

becomes:

```go
inkgo.Box(inkgo.Style{
    Width:         inkgo.Percent(100),
    Height:        inkgo.Cells(8),
    PaddingX:      inkgo.I(1),
    FlexDirection: inkgo.Column,
})
```

Pointers such as `inkgo.I`, `inkgo.F`, and `inkgo.B` exist because the TS API distinguishes “unset” from explicit zero/false.

## Stateful text

Instead of a React state update:

```go
status := inkgo.Text("idle")
// later
status.SetText("working")
_, err := runtime.Render()
```

No component rebuild is required unless your own application wants one.

## ScrollBox

The outer viewport contains the same non-shrinking column used by the TS component. Existing child refs remain valid.

```go
scroll := inkgo.ScrollBox(
    inkgo.Style{Height: inkgo.Cells(12), Width: inkgo.Percent(100)},
    true,
    rows...,
)
scroll.ScrollBy(3)
scroll.ScrollTo(40)
scroll.ScrollToBottom()
scroll.ScrollToElement(row, -2)
```

Pending deltas drain across frames (`ScrollDrainPerFrame`, default 12), matching the fork rather than jumping an arbitrarily large wheel delta in one frame.

## Main screen vs alternate screen

`Renderer` uses two different terminal patch strategies. Main-screen patches are relative to the application's block so shell scrollback is safe. Fullscreen patches can use absolute rows and the `DECSTBM + SU/SD` scroll fast path.

Do not replace the main-screen path with raw `CSI row;col H` writes.
