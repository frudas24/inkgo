package render

import (
	"github.com/frudas24/inkgo/widgets"
	"strings"
	"testing"
)

func TestPublicRenderer(t *testing.T) {
	r := New(RenderOptions{Width: 8, Height: 3, Fullscreen: true})
	f := r.Render(widgets.AlternateScreen(widgets.Text("hello")))
	if f.Screen == nil || !strings.Contains(f.Screen.PlainText(), "hello") || f.Patch == "" {
		t.Fatalf("frame=%+v", f)
	}
}
