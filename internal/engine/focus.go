package engine

import "sort"

type FocusManager struct {
	root    *Node
	focused *Node
	enabled bool
}

func NewFocusManager(root *Node) *FocusManager {
	return &FocusManager{root: root, enabled: true}
}

func (f *FocusManager) SetRoot(root *Node) { f.root = root }

func (f *FocusManager) Focused() *Node { return f.focused }

func (f *FocusManager) Enable() { f.enabled = true }

func (f *FocusManager) Disable() {
	f.enabled = false
	f.Blur()
}

func (f *FocusManager) Focus(node *Node) bool {
	if !f.enabled || node == nil || node.TabIndex < -1 || node.Style.Display == DisplayNone {
		return false
	}
	if f.focused == node {
		return true
	}
	old := f.focused
	if old != nil {
		e := &FocusEvent{Event: Event{Target: old}, RelatedTarget: node}
		dispatchFocusEvent(old, e, false)
		if old.Kind == NodeButton {
			next := old.ButtonState
			next.Focused = false
			old.setButtonState(next)
		}
	}
	f.focused = node
	e := &FocusEvent{Event: Event{Target: node}, RelatedTarget: old}
	dispatchFocusEvent(node, e, true)
	if node.Kind == NodeButton {
		next := node.ButtonState
		next.Focused = true
		node.setButtonState(next)
	}
	return true
}

func (f *FocusManager) Blur() {
	if f.focused == nil {
		return
	}
	old := f.focused
	f.focused = nil
	e := &FocusEvent{Event: Event{Target: old}}
	dispatchFocusEvent(old, e, false)
	if old.Kind == NodeButton {
		next := old.ButtonState
		next.Focused = false
		old.setButtonState(next)
	}
}

type focusableNode struct {
	n     *Node
	order int
}

func (f *FocusManager) focusables() []*Node {
	if f.root == nil {
		return nil
	}
	items := make([]focusableNode, 0, 16)
	order := 0
	f.root.Walk(func(n *Node) bool {
		if n.Style.Display == DisplayNone {
			return false
		}
		if n.TabIndex >= 0 {
			items = append(items, focusableNode{n, order})
		}
		order++
		return true
	})
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].n.TabIndex == items[j].n.TabIndex {
			return items[i].order < items[j].order
		}
		return items[i].n.TabIndex < items[j].n.TabIndex
	})
	out := make([]*Node, len(items))
	for i := range items {
		out[i] = items[i].n
	}
	return out
}

func (f *FocusManager) FocusNext() *Node {
	if f == nil || !f.enabled {
		return nil
	}
	nodes := f.focusables()
	if len(nodes) == 0 {
		return nil
	}
	idx := -1
	for i, n := range nodes {
		if n == f.focused {
			idx = i
			break
		}
	}
	next := nodes[(idx+1)%len(nodes)]
	f.Focus(next)
	return next
}

func (f *FocusManager) FocusPrevious() *Node {
	if f == nil || !f.enabled {
		return nil
	}
	nodes := f.focusables()
	if len(nodes) == 0 {
		return nil
	}
	idx := 0
	for i, n := range nodes {
		if n == f.focused {
			idx = i
			break
		}
	}
	idx--
	if idx < 0 {
		idx = len(nodes) - 1
	}
	prev := nodes[idx]
	f.Focus(prev)
	return prev
}

func (f *FocusManager) AutoFocus() *Node {
	if f == nil || !f.enabled {
		return nil
	}
	var found *Node
	if f.root == nil {
		return nil
	}
	f.root.Walk(func(n *Node) bool {
		if n.AutoFocus && n.TabIndex >= -1 {
			found = n
			return false
		}
		return true
	})
	if found != nil {
		f.Focus(found)
	}
	return found
}

func dispatchFocusEvent(target *Node, e *FocusEvent, focused bool) {
	path := nodePath(target)
	for i := len(path) - 1; i >= 0; i-- {
		e.CurrentTarget = path[i]
		if focused {
			if h := path[i].Handlers.OnFocusCapture; h != nil {
				h(e)
			}
		} else {
			if h := path[i].Handlers.OnBlurCapture; h != nil {
				h(e)
			}
		}
		if e.PropagationStopped() {
			return
		}
	}
	for _, n := range path {
		e.CurrentTarget = n
		if focused {
			if h := n.Handlers.OnFocus; h != nil {
				h(e)
			}
		} else {
			if h := n.Handlers.OnBlur; h != nil {
				h(e)
			}
		}
		if e.PropagationStopped() {
			return
		}
	}
}

func nodePath(target *Node) []*Node {
	var path []*Node
	for n := target; n != nil; n = n.Parent {
		path = append(path, n)
	}
	return path // target -> root
}
