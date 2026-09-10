package engine

import (
	"testing"
	"time"
)

func TestExpiredButtonCallbackCanQueryRenderer(t *testing.T) {
	button := Button(Style{}, nil, Text("before"))
	root := Root(button)
	r := NewRenderer(RenderOptions{Width: 20, Height: 4, Fullscreen: true})
	r.Render(root)
	button.ButtonState.Active = true
	button.ActiveUntil = time.Now().Add(-time.Second)
	button.ButtonRender = func(ButtonState) []*Node {
		size := r.Viewport()
		r.SetSize(size.Width+10, size.Height+2)
		return []*Node{Text("after")}
	}
	done := make(chan Frame, 1)
	go func() { done <- r.Render(root) }()
	select {
	case frame := <-done:
		if frame.Screen.Width != 30 || frame.Screen.Height != 6 {
			t.Fatalf("callback size lost: %dx%d", frame.Screen.Width, frame.Screen.Height)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("renderer lock held across application callback")
	}
}
