package ink

// HitTest returns the deepest painted node at a screen coordinate, including
// clipping and the visual offset introduced by nested ScrollBoxes.
func HitTest(root *Node, p Point) *Node {
	return hitNode(root, p, 0, 0, Rect{X: -1 << 20, Y: -1 << 20, Width: 1 << 21, Height: 1 << 21})
}
func hitNode(n *Node, p Point, dx, dy int, clip Rect) *Node {
	if n == nil || n.Style.Display == DisplayNone {
		return nil
	}
	r := shiftedRect(n.Rect, dx, dy)
	visible := r.Intersect(clip)
	if n.Kind != NodeRoot && !visible.Contains(p) {
		return nil
	}
	childClip := clip
	cr := shiftedRect(n.ContentRect, dx, dy)
	ox := n.Style.OverflowX == OverflowHidden || n.Style.OverflowX == OverflowScroll || n.Style.Overflow == OverflowHidden || n.Style.Overflow == OverflowScroll
	oy := n.Style.OverflowY == OverflowHidden || n.Style.OverflowY == OverflowScroll || n.Style.Overflow == OverflowHidden || n.Style.Overflow == OverflowScroll
	if ox || oy {
		childClip = clip
		if ox {
			childClip = childClip.Intersect(Rect{X: cr.X, Y: childClip.Y, Width: cr.Width, Height: childClip.Height})
		}
		if oy {
			childClip = childClip.Intersect(Rect{X: childClip.X, Y: cr.Y, Width: childClip.Width, Height: cr.Height})
		}
	}
	if oy && n.ScrollTop != 0 {
		dy -= n.ScrollTop
	}
	for i := len(n.Children) - 1; i >= 0; i-- {
		if h := hitNode(n.Children[i], p, dx, dy, childClip); h != nil {
			return h
		}
	}
	if visible.Contains(p) {
		return n
	}
	return nil
}

func DispatchClick(root *Node, x, y, button int) *Node {
	target := HitTest(root, Point{X: x, Y: y})
	if target == nil {
		return nil
	}
	e := &ClickEvent{Event: Event{Target: target}, X: x, Y: y, Button: button}
	for n := target; n != nil; n = n.Parent {
		e.CurrentTarget = n
		if n.Handlers.OnClick != nil {
			n.Handlers.OnClick(e)
		}
		if n.Kind == NodeButton && button == 0 && n.OnAction != nil && !e.DefaultPrevented() {
			n.activateButton()
			n.OnAction()
		}
		if e.PropagationStopped() {
			break
		}
	}
	return target
}
func DispatchKey(target *Node, key Key) *KeyboardEvent {
	if target == nil {
		return nil
	}
	e := &KeyboardEvent{Event: Event{Target: target}, Key: key}
	path := nodePath(target)
	for i := len(path) - 1; i >= 0; i-- {
		e.CurrentTarget = path[i]
		if h := path[i].Handlers.OnKeyDownCapture; h != nil {
			h(e)
		}
		if e.PropagationStopped() {
			return e
		}
	}
	for _, n := range path {
		e.CurrentTarget = n
		if h := n.Handlers.OnKeyDown; h != nil {
			h(e)
		}
		if n.Kind == NodeButton && !e.DefaultPrevented() && (key.Name == "return" || key.Text == " ") && !key.Release {
			if n.OnAction != nil {
				n.activateButton()
				n.OnAction()
			}
		}
		if e.PropagationStopped() {
			return e
		}
	}
	return e
}
func DispatchHover(root *Node, x, y int, previous map[*Node]struct{}) map[*Node]struct{} {
	current := map[*Node]struct{}{}
	target := HitTest(root, Point{X: x, Y: y})
	for n := target; n != nil; n = n.Parent {
		current[n] = struct{}{}
	}
	for n := range previous {
		if _, ok := current[n]; !ok {
			if n.Handlers.OnMouseLeave != nil {
				n.Handlers.OnMouseLeave()
			}
			if n.Kind == NodeButton {
				next := n.ButtonState
				next.Hovered = false
				n.setButtonState(next)
			}
		}
	}
	for n := range current {
		if _, ok := previous[n]; !ok {
			if n.Handlers.OnMouseEnter != nil {
				n.Handlers.OnMouseEnter()
			}
			if n.Kind == NodeButton {
				next := n.ButtonState
				next.Hovered = true
				n.setButtonState(next)
			}
		}
	}
	return current
}
