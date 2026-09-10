package engine

import "sort"

type FocusManager struct {
	root     *Node
	focused  *Node
	enabled  bool
	revision uint64
}

func NewFocusManager(root *Node) *FocusManager {
	return &FocusManager{root: root, enabled: true}
}

func (f *FocusManager) SetRoot(root *Node) {
	f.revision++
	f.root = root
	if f.focused != nil && !f.canFocus(f.focused) {
		f.Blur()
	}
}

func (f *FocusManager) Focused() *Node { return f.focused }

func (f *FocusManager) Enable() { f.enabled = true }

func (f *FocusManager) Disable() {
	f.enabled = false
	f.Blur()
}

func (f *FocusManager) canFocus(node *Node) bool {
	if !f.enabled || node == nil || node.TabIndex < -1 {
		return false
	}
	for ancestor := node; ancestor != nil; ancestor = ancestor.Parent {
		if ancestor.Style.Display == DisplayNone {
			return false
		}
		if ancestor == f.root {
			return true
		}
	}
	return false
}

func setButtonFocused(node *Node, focused bool) {
	if node != nil && node.Kind == NodeButton {
		next := node.ButtonState
		next.Focused = focused
		node.setButtonState(next)
	}
}

func (f *FocusManager) Focus(node *Node) bool {
	if !f.canFocus(node) {
		return false
	}
	if f.focused == node {
		return true
	}
	f.revision++
	revision := f.revision
	old := f.focused
	// Publish state before calling application code. A nested focus operation
	// supersedes this transition and must not be overwritten when it returns.
	f.focused = nil
	if old != nil {
		setButtonFocused(old, false)
		if f.revision != revision {
			return f.focused == node
		}
		dispatchFocusEvent(old, &FocusEvent{Event: Event{Target: old}, RelatedTarget: node}, false)
		if f.revision != revision {
			return f.focused == node
		}
	}
	if !f.canFocus(node) {
		return false
	}
	f.focused = node
	setButtonFocused(node, true)
	if f.revision != revision {
		return f.focused == node
	}
	if !f.canFocus(node) {
		f.Blur()
		return false
	}
	dispatchFocusEvent(node, &FocusEvent{Event: Event{Target: node}, RelatedTarget: old}, true)
	if f.revision == revision && !f.canFocus(node) {
		f.Blur()
	}
	return f.focused == node
}

func (f *FocusManager) Blur() {
	f.revision++
	revision := f.revision
	old := f.focused
	f.focused = nil
	if old == nil {
		return
	}
	setButtonFocused(old, false)
	if f.revision != revision {
		return
	}
	dispatchFocusEvent(old, &FocusEvent{Event: Event{Target: old}}, false)
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
	return f.Focused()
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
	return f.Focused()
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
		if found != nil || n.Style.Display == DisplayNone {
			return false
		}
		if n.AutoFocus && n.TabIndex >= -1 {
			found = n
			return false
		}
		return true
	})
	if found != nil {
		f.Focus(found)
		return f.Focused()
	}
	return nil
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
