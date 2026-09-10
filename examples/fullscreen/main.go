package main

import (
	"fmt"
	"os"

	inkgo "github.com/frudas24/inkgo"
)

func main() {
	count := 0
	label := inkgo.Text("count: 0")
	button := inkgo.ButtonWithState(
		inkgo.Style{BorderStyle: &inkgo.BorderRound, PaddingX: inkgo.I(1)},
		func() {
			count++
			label.SetText(fmt.Sprintf("count: %d", count))
		},
		func(state inkgo.ButtonState) []*inkgo.Node {
			style := inkgo.TextStyle{}
			if state.Focused {
				style.Bold = true
			}
			return []*inkgo.Node{inkgo.Text(" increment ", style)}
		},
	)

	root := inkgo.Root(inkgo.AlternateScreen(
		inkgo.Box(inkgo.Style{FlexDirection: inkgo.Column, Gap: inkgo.I(1), Padding: inkgo.I(1)}, label, button),
	))

	term := inkgo.DefaultTerminal()
	rt := inkgo.NewRuntime(root, term.In, term.Out, inkgo.RenderOptions{
		Fullscreen:         true,
		SynchronizedOutput: inkgo.SupportsSynchronizedOutput(),
		HideCursor:         true,
	})
	rt.Terminal = &term
	root.SetHandlers(inkgo.EventHandlers{OnKeyDown: func(e *inkgo.KeyboardEvent) {
		if e.Key.Name == "q" || (e.Key.Ctrl && e.Key.Name == "c") {
			rt.Stop()
		}
	}})
	if err := rt.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
