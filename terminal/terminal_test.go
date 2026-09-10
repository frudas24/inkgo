package terminal

import (
	"bytes"
	"github.com/frudas24/inkgo/widgets"
	"testing"
)

func TestRuntimeEmbeddingAndSequences(t *testing.T) {
	var out bytes.Buffer
	rt := NewRuntime(widgets.Root(widgets.Text("ok")), bytes.NewBuffer(nil), &out, RenderOptions{Width: 10, Height: 2})
	if _, err := rt.RenderSettled(); err != nil {
		t.Fatal(err)
	}
	if TerminalTitle("x") == "" || Bell() == "" || ClearSequence() == "" {
		t.Fatal("terminal sequences")
	}
	caps := DetectCapabilities("xterm-256color")
	_ = caps
}
