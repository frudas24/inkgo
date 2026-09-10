// Command terminal-smoke is a manual, interactive smoke test for the host
// terminal. It intentionally exercises the real console/PTY rather than a
// bytes.Buffer, making it useful on Windows Terminal, cmd/PowerShell hosts,
// Linux terminals, and macOS terminals before a release.
package main

import (
	"fmt"
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

	var rt *ink.Runtime
	status := ink.Text(fmt.Sprintf("terminal %dx%d · press q to exit", size.Width, size.Height), ink.TextStyle{Bold: true})
	root := ink.Root(ink.AlternateScreen(
		ink.Box(ink.Style{FlexDirection: ink.Column, Padding: ink.I(1)},
			status,
			ink.Text("Move the mouse, resize the terminal, paste text, and press Tab/Shift+Tab."),
			ink.Button(ink.Style{}, func() {}, ink.Text("focusable button")),
		),
	))
	root.SetHandlers(ink.EventHandlers{
		OnKeyDown: func(e *ink.KeyboardEvent) {
			if e.Key.Name == "q" && !e.Key.Ctrl && !e.Key.Alt && !e.Key.Meta {
				rt.Stop()
			}
		},
		OnResize: func(e *ink.ResizeEvent) {
			status.SetText(fmt.Sprintf("terminal %dx%d · press q to exit", e.Columns, e.Rows))
		},
	})

	rt = ink.NewRuntime(root, term.In, term.Out, ink.RenderOptions{
		Width:              size.Width,
		Height:             size.Height,
		Fullscreen:         true,
		HideCursor:         true,
		SynchronizedOutput: ink.SupportsSynchronizedOutput(),
	})
	rt.Terminal = &term
	if err := rt.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "runtime: %v\n", err)
		os.Exit(1)
	}
}
