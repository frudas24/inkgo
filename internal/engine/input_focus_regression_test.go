package engine

import (
	"io"
	"strings"
	"testing"
)

func TestStopDuringInputSkipsRuntimeFollowup(t *testing.T) {
	t.Run("tab", func(t *testing.T) {
		first, second := Button(Style{}, nil), Button(Style{}, nil)
		rt := NewRuntime(Root(first, second), strings.NewReader(""), io.Discard, RenderOptions{})
		defer rt.Close()
		rt.Focus.Focus(first)
		first.Handlers.OnKeyDown = func(*KeyboardEvent) { rt.Stop() }
		rt.HandleInput([]byte("\t"))
		if rt.Focus.Focused() != first {
			t.Fatal("Tab moved focus after its handler stopped the runtime")
		}
	})
	t.Run("paste", func(t *testing.T) {
		root := Root()
		rt := NewRuntime(root, strings.NewReader(""), io.Discard, RenderOptions{})
		defer rt.Close()
		root.Handlers.OnPaste = func(*PasteEvent) { rt.Stop() }
		calls := 0
		rt.OnPaste = func(string) { calls++ }
		rt.HandleInput([]byte(PasteStart + "quit" + PasteEnd))
		if calls != 0 {
			t.Fatal("runtime paste callback ran after the node handler stopped it")
		}
	})
}

func TestInputDoesNotReachRemovedOrHiddenFocus(t *testing.T) {
	for _, hide := range []bool{false, true} {
		button := Button(Style{}, nil)
		parent := Box(Style{}, button)
		root := Root(parent)
		rt := NewRuntime(root, strings.NewReader(""), io.Discard, RenderOptions{})
		defer rt.Close()
		rt.Focus.Focus(button)
		invalidCalls, rootCalls := 0, 0
		button.Handlers.OnKeyDown = func(*KeyboardEvent) { invalidCalls++ }
		button.Handlers.OnPaste = func(*PasteEvent) { invalidCalls++ }
		root.Handlers.OnKeyDown = func(*KeyboardEvent) { rootCalls++ }
		root.Handlers.OnPaste = func(*PasteEvent) { rootCalls++ }
		if hide {
			parent.SetStyle(Style{Display: DisplayNone})
		} else {
			parent.Remove(button)
		}
		rt.HandleInput([]byte("a" + PasteStart + "text" + PasteEnd))
		if invalidCalls != 0 || rootCalls != 2 || button.ButtonState.Focused {
			t.Fatalf("hide=%v invalid=%d root=%d focused=%v", hide, invalidCalls, rootCalls, button.ButtonState.Focused)
		}
	}
}
