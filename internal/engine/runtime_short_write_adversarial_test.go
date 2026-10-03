package engine

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

type shortFirstWriteWriter struct {
	bytes.Buffer
	short bool
}

func (w *shortFirstWriteWriter) Write(p []byte) (int, error) {
	if !w.short {
		w.short = true
		n := len(p) / 2
		_, _ = w.Buffer.Write(p[:n])
		return n, nil
	}
	return w.Buffer.Write(p)
}

// WriteString routes the io.StringWriter fast path through Write. Without it the
// embedded bytes.Buffer promotes its own WriteString, io.WriteString stores the
// whole payload there, and the fixture would never produce a partial write at
// all - it would prove nothing about short-write handling.
func (w *shortFirstWriteWriter) WriteString(s string) (int, error) { return w.Write([]byte(s)) }

func TestRuntimeRenderDetectsShortWriteAndRetriesFrame(t *testing.T) {
	out := &shortFirstWriteWriter{}
	rt := NewRuntime(
		Root(Text("complete frame")),
		strings.NewReader(""),
		out,
		RenderOptions{Width: 24, Height: 3, Fullscreen: true},
	)

	first, firstErr := rt.Render()
	retry, retryErr := rt.Render()
	if retryErr != nil {
		t.Fatalf("retry render failed: %v", retryErr)
	}
	if !errors.Is(firstErr, io.ErrShortWrite) {
		t.Fatalf(
			"FINDING: partial terminal frame write was treated as success: error=%v, first patch bytes=%d, retry patch bytes=%d, emitted bytes=%d",
			firstErr, len(first.Patch), len(retry.Patch), out.Len(),
		)
	}
	if retry.Patch == "" || !strings.Contains(out.String(), "complete frame") {
		t.Fatalf("short write was not recovered by a complete frame retry: patch=%q output=%q", retry.Patch, out.String())
	}
}

func TestRendererWriteFrameDetectsShortWriteAndRetriesFrame(t *testing.T) {
	out := &shortFirstWriteWriter{}
	root := Root(Text("complete frame"))
	renderer := NewRenderer(RenderOptions{Width: 24, Height: 3, Fullscreen: true})

	first, firstErr := renderer.WriteFrame(out, root)
	retry, retryErr := renderer.WriteFrame(out, root)
	if retryErr != nil {
		t.Fatalf("retry render failed: %v", retryErr)
	}
	if !errors.Is(firstErr, io.ErrShortWrite) {
		t.Fatalf(
			"FINDING: partial Renderer.WriteFrame output was treated as success: error=%v, first patch bytes=%d, retry patch bytes=%d, emitted bytes=%d",
			firstErr, len(first.Patch), len(retry.Patch), out.Len(),
		)
	}
	if retry.Patch == "" || !strings.Contains(out.String(), "complete frame") {
		t.Fatalf("short write was not recovered by a complete frame retry: patch=%q output=%q", retry.Patch, out.String())
	}
}
