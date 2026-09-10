package engine

import "testing"

func TestReparentWholeChildList(t *testing.T) {
	for _, appendMode := range []bool{false, true} {
		a, b, c := Text("a"), Text("b"), Text("c")
		source := Box(Style{}, a, b, c)
		target := Box(Style{})
		if appendMode {
			target.Append(source.Children...)
		} else {
			target.SetChildren(source.Children...)
		}
		if len(target.Children) != 3 || len(source.Children) != 0 {
			t.Fatalf("append=%v moved=%d remaining=%d", appendMode, len(target.Children), len(source.Children))
		}
		for i, n := range []*Node{a, b, c} {
			if target.Children[i] != n || n.Parent != target {
				t.Fatal("reparent order or parent corrupted")
			}
		}
	}
}

func TestDuplicateChildrenCannotCorruptParent(t *testing.T) {
	child := Text("x")
	root := Root(child)
	root.Append(child, child)
	if len(root.Children) != 1 {
		t.Fatalf("duplicate children: %d", len(root.Children))
	}
	root.Remove(child)
	if len(root.Children) != 0 || child.Parent != nil {
		t.Fatal("Remove left a detached child in parent")
	}
}

func TestFocusCallbackRedirectKeepsOneFocusedButton(t *testing.T) {
	a, b := Button(Style{}, nil), Button(Style{}, nil)
	f := NewFocusManager(Root(a, b))
	a.Handlers.OnFocus = func(*FocusEvent) { f.Focus(b) }
	f.Focus(a)
	if f.Focused() != b || a.ButtonState.Focused || !b.ButtonState.Focused {
		t.Fatalf("redirect left inconsistent state: focused=%p a=%v b=%v", f.Focused(), a.ButtonState.Focused, b.ButtonState.Focused)
	}
}

func TestBlurCallbackRedirectWins(t *testing.T) {
	a, b, c := Button(Style{}, nil), Button(Style{}, nil), Button(Style{}, nil)
	f := NewFocusManager(Root(a, b, c))
	f.Focus(a)
	blurCount := 0
	a.Handlers.OnBlur = func(*FocusEvent) {
		blurCount++
		if blurCount == 1 {
			f.Focus(c)
		}
	}
	f.Focus(b)
	if blurCount != 1 || f.Focused() != c || a.ButtonState.Focused || b.ButtonState.Focused || !c.ButtonState.Focused {
		t.Fatalf("nested blur/redirect count=%d focused=%p want=%p", blurCount, f.Focused(), c)
	}
}

func TestTreeMutationsRejectCyclesAndDeduplicate(t *testing.T) {
	leaf := Text("leaf")
	child := Box(Style{}, leaf)
	root := Root(child)
	child.Append(root, child, leaf)
	if len(child.Children) != 1 || child.Children[0] != leaf || root.Parent != nil {
		t.Fatal("Append introduced cycle or duplicate")
	}
	child.SetChildren(root, child, leaf, leaf, nil)
	if len(child.Children) != 1 || child.Children[0] != leaf || root.Parent != nil || child.Parent != root {
		t.Fatal("SetChildren introduced cycle or duplicate")
	}
}

func TestFocusCallbacksCanDisableBlurAndChangeRoot(t *testing.T) {
	t.Run("disable-on-blur", func(t *testing.T) {
		a, b := Button(Style{}, nil), Button(Style{}, nil)
		f := NewFocusManager(Root(a, b))
		f.Focus(a)
		a.Handlers.OnBlur = func(*FocusEvent) { f.Disable() }
		if f.Focus(b) || f.Focused() != nil || a.ButtonState.Focused || b.ButtonState.Focused {
			t.Fatal("disabled manager restored focus")
		}
	})
	t.Run("blur-on-focus", func(t *testing.T) {
		a := Button(Style{}, nil)
		f := NewFocusManager(Root(a))
		a.Handlers.OnFocus = func(*FocusEvent) { f.Blur() }
		if f.Focus(a) || f.Focused() != nil || a.ButtonState.Focused {
			t.Fatal("callback blur overwritten")
		}
	})
	t.Run("change-root-on-blur", func(t *testing.T) {
		a, b, c := Button(Style{}, nil), Button(Style{}, nil), Button(Style{}, nil)
		f := NewFocusManager(Root(a, b))
		other := Root(c)
		f.Focus(a)
		a.Handlers.OnBlur = func(*FocusEvent) { f.SetRoot(other); f.Focus(c) }
		if f.Focus(b) || f.Focused() != c || a.ButtonState.Focused || b.ButtonState.Focused || !c.ButtonState.Focused {
			t.Fatal("root-change redirect overwritten")
		}
	})
}

func FuzzTreeMutationInvariants(f *testing.F) {
	f.Add([]byte{0, 0, 1, 0, 0, 2, 0, 0, 3, 1, 4, 0})
	f.Add([]byte{0, 0, 1, 0, 1, 0, 3, 1, 1, 3, 0, 1})
	f.Fuzz(func(t *testing.T, data []byte) {
		nodes := make([]*Node, 8)
		for i := range nodes {
			nodes[i] = Box(Style{})
		}
		for i := 0; i+2 < len(data); i += 3 {
			target := nodes[int(data[i+1])%len(nodes)]
			source := nodes[int(data[i+2])%len(nodes)]
			switch data[i] % 4 {
			case 0:
				target.Append(source)
			case 1:
				target.SetChildren(source.Children...)
			case 2:
				target.Remove(source)
			case 3:
				target.SetChildren(source, source, nil)
			}
			for _, node := range nodes {
				seen := map[*Node]bool{}
				for _, child := range node.Children {
					if child == nil || seen[child] || child.Parent != node {
						t.Fatal("broken child/parent ownership")
					}
					seen[child] = true
				}
				if node.Parent != nil {
					count := 0
					for _, child := range node.Parent.Children {
						if child == node {
							count++
						}
					}
					if count != 1 {
						t.Fatal("parent does not own child exactly once")
					}
				}
				hops := 0
				for ancestor := node; ancestor != nil; ancestor = ancestor.Parent {
					hops++
					if hops > len(nodes) {
						t.Fatal("ancestor cycle")
					}
				}
			}
		}
	})
}
