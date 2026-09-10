package main

import (
	"fmt"
	"os"
	"strconv"

	ink "github.com/frudas24/inkgo"
)

func main() {
	term := ink.DefaultTerminal()
	sz, err := term.Size()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fixture terminal size: %v\n", err)
		os.Exit(2)
	}

	status := ink.Text(fmt.Sprintf("READY:%dx%d", sz.Width, sz.Height), ink.TextStyle{Bold: true})
	keyState := ink.Text("--------------------------------------------------------------------")
	pasteState := ink.Text("====================================================================")
	focusState := ink.Text("++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++")
	mouseState := ink.Text("....................................................................")
	resizeState := ink.Text(fmt.Sprintf("RESIZE:%dx%d", sz.Width, sz.Height))

	root := ink.Root(ink.AlternateScreen(
		ink.Box(ink.Style{
			FlexDirection: ink.Column,
			Width:         ink.Percent(100),
			Height:        ink.Percent(100),
			Padding:       ink.I(1),
		}, status, keyState, pasteState, focusState, mouseState, resizeState),
	))

	var rt *ink.Runtime
	root.SetHandlers(ink.EventHandlers{
		OnKeyDown: func(e *ink.KeyboardEvent) {
			k := e.Key
			if os.Getenv("INKGO_PTY_PANIC") == "1" && k.Name == "p" {
				panic("intentional PTY fixture panic")
			}
			keyState.SetText(fmt.Sprintf("KEY:name=%s,text=%s,ctrl=%t,alt=%t,shift=%t", k.Name, strconv.Quote(k.Text), k.Ctrl, k.Alt, k.Shift))
			if (k.Name == "q" && !k.Ctrl && !k.Alt) || (k.Name == "c" && k.Ctrl && !k.Alt) || k.Name == "escape" {
				rt.Stop()
			}
		},
		OnPaste: func(e *ink.PasteEvent) {
			if len(e.Text) <= 64 {
				pasteState.SetText(fmt.Sprintf("PASTE_LEN:%d,text=%s", len(e.Text), strconv.Quote(e.Text)))
			} else {
				pasteState.SetText(fmt.Sprintf("PASTE_LEN:%d", len(e.Text)))
			}
		},
		OnResize: func(e *ink.ResizeEvent) {
			resizeState.SetText(fmt.Sprintf("RESIZE:%dx%d", e.Columns, e.Rows))
		},
		OnClick: func(e *ink.ClickEvent) {
			mouseState.SetText(fmt.Sprintf("MOUSE:x=%d,y=%d,button=%d", e.X, e.Y, e.Button))
		},
	})

	rt = ink.NewRuntime(root, term.In, term.Out, ink.RenderOptions{
		Width: sz.Width, Height: sz.Height,
		Fullscreen: true, HideCursor: true,
	})
	rt.Terminal = &term
	rt.OnTerminalFocus = func(focused bool) {
		if focused {
			focusState.SetText("FOCUS:in")
		} else {
			focusState.SetText("FOCUS:out")
		}
	}
	if err := rt.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "fixture runtime: %v\n", err)
		os.Exit(3)
	}
}
