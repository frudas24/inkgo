package main

import (
	"fmt"
	"os"

	ink "github.com/frudas24/inkgo"
)

func main() {
	count := 0
	label := ink.Text("count: 0")
	button := ink.ButtonWithState(
		ink.Style{BorderStyle: &ink.BorderRound, PaddingX: ink.I(1)},
		func() {
			count++
			label.SetText(fmt.Sprintf("count: %d", count))
		},
		func(state ink.ButtonState) []*ink.Node {
			style := ink.TextStyle{}
			if state.Focused {
				style.Bold = true
			}
			return []*ink.Node{ink.Text(" increment ", style)}
		},
	)

	root := ink.Root(ink.AlternateScreen(
		ink.Box(ink.Style{FlexDirection: ink.Column, Gap: ink.I(1), Padding: ink.I(1)}, label, button),
	))

	term := ink.DefaultTerminal()
	rt := ink.NewRuntime(root, term.In, term.Out, ink.RenderOptions{
		Fullscreen:         true,
		SynchronizedOutput: ink.SupportsSynchronizedOutput(),
		HideCursor:         true,
	})
	rt.Terminal = &term
	root.SetHandlers(ink.EventHandlers{OnKeyDown: func(e *ink.KeyboardEvent) {
		if e.Key.Name == "q" || (e.Key.Ctrl && e.Key.Name == "c") {
			rt.Stop()
		}
	}})
	if err := rt.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
