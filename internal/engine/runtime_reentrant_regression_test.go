package engine

import (
	"io"
	"strings"
	"testing"
)

func TestRuntimeSetRootReentrantBlurSupersedesOuter(t *testing.T) {
	a := Button(Style{}, nil, Text("a"))
	rootA := Root(a)
	b := Button(Style{}, nil, Text("b"))
	rootB := Root(b)
	c := Button(Style{}, nil, Text("c"))
	rootC := Root(c)
	rt := NewRuntime(rootA, strings.NewReader(""), io.Discard, RenderOptions{})
	rt.Focus.Focus(a)
	a.Handlers.OnBlur = func(*FocusEvent) {
		rt.SetRoot(rootC)
		rt.Focus.Focus(c)
	}
	rt.SetRoot(rootB)
	if rt.Root != rootC || rt.Focus.root != rootC || rt.Focus.Focused() != c {
		t.Fatalf("nested SetRoot was overwritten: runtimeRoot=%p focusRoot=%p focused=%p; want rootC=%p c=%p", rt.Root, rt.Focus.root, rt.Focus.Focused(), rootC, c)
	}
}
