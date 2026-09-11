package engine

import (
	"errors"
	"io"
	"strings"
	"testing"
)

func TestFrameWriterRetriesFailedOutput(t *testing.T) {
	for _, runtimeAPI := range []bool{false, true} {
		name := "Renderer.WriteFrame"
		if runtimeAPI {
			name = "Runtime.Render"
		}
		t.Run(name, func(t *testing.T) {
			out := &regressionWriter{err: io.ErrClosedPipe}
			node := Text("first")
			root := Root(node)
			opts := RenderOptions{Width: 20, Height: 4, Fullscreen: true}
			r := NewRenderer(opts)
			write := func() (Frame, error) { return r.WriteFrame(out, root) }
			if runtimeAPI {
				rt := NewRuntime(root, strings.NewReader(""), out, opts)
				defer rt.Close()
				write = rt.Render
			}
			for _, value := range []string{"first", "updated"} {
				node.SetText(value)
				out.fail = func(string) bool { return true }
				if _, err := write(); !errors.Is(err, io.ErrClosedPipe) {
					t.Fatalf("write error = %v", err)
				}
				out.fail = nil
				out.Reset()
				if _, err := write(); err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(out.String(), value) {
					t.Fatalf("retry omitted %q: %q", value, out.String())
				}
				out.Reset()
				if _, err := write(); err != nil {
					t.Fatal(err)
				}
				if out.Len() != 0 {
					t.Fatalf("successful frame was unnecessarily resent: %q", out.String())
				}
			}
		})
	}
}
