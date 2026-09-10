// Command terminal-smoke is a manual, interactive smoke test for the host
// terminal. It intentionally exercises the real console/PTY rather than a
// bytes.Buffer, making it useful on Windows Terminal, cmd/PowerShell hosts,
// Linux terminals, and macOS terminals before a release.
package main

import (
	"fmt"
	"io"
	"os"

	ink "github.com/frudas24/inkgo"
)

func main() {
	term := ink.DefaultTerminal()
	size, err := term.Size()
	if err != nil {
		fmt.Fprintf(os.Stderr, "terminal size: %v\n", err)
		os.Exit(1)
	}

	rt := newSmokeRuntime(term.In, term.Out, size)
	rt.Terminal = &term
	if err := rt.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "runtime: %v\n", err)
		os.Exit(1)
	}
}

// Keep the demo construction separate so input behavior is testable without
// changing the host terminal's modes.
func newSmokeRuntime(in io.Reader, out io.Writer, size ink.Size) *ink.Runtime {
	var rt *ink.Runtime
	statusText := func(width, height int) string {
		return fmt.Sprintf("terminal %dx%d · q / Ctrl+C / Escape to exit", width, height)
	}
	status := ink.Text(statusText(size.Width, size.Height), ink.TextStyle{Bold: true})
	footer := ink.Text(fmt.Sprintf("Viewport bottom · %dx%d", size.Width, size.Height))
	pasted := ink.Text("Paste: waiting for text")
	root := ink.Root(ink.AlternateScreen(
		ink.Box(ink.Style{FlexDirection: ink.Column, Width: ink.Percent(100), Height: ink.Percent(100), Padding: ink.I(1)},
			status,
			ink.Text("Move the mouse, resize the terminal, paste text, and press Tab/Shift+Tab."),
			ink.Button(ink.Style{}, func() {}, ink.Text("focusable button")),
			ink.Box(ink.Style{Height: ink.Cells(4), FlexShrink: ink.F(0), Overflow: ink.OverflowHidden}, pasted),
			ink.Spacer(),
			footer,
		),
	))
	root.SetHandlers(ink.EventHandlers{
		OnKeyDown: func(e *ink.KeyboardEvent) {
			key := e.Key
			if (key.Name == "q" && !key.Ctrl && !key.Alt && !key.Meta) ||
				(key.Name == "c" && key.Ctrl && !key.Alt && !key.Meta) ||
				(key.Name == "escape" && !key.Ctrl && !key.Alt && !key.Meta) {
				rt.Stop()
			}
		},
		OnResize: func(e *ink.ResizeEvent) {
			status.SetText(statusText(e.Columns, e.Rows))
			footer.SetText(fmt.Sprintf("Viewport bottom · %dx%d", e.Columns, e.Rows))
		},
		OnPaste: func(e *ink.PasteEvent) {
			pasted.SetText(fmt.Sprintf("Paste (%d bytes): %q", len(e.Text), e.Text))
		},
	})
	rt = ink.NewRuntime(root, in, out, ink.RenderOptions{
		Width: size.Width, Height: size.Height,
		Fullscreen: true, HideCursor: true,
		SynchronizedOutput: ink.SupportsSynchronizedOutput(),
	})
	return rt
}
