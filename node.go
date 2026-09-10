package inkgo

import (
	"fmt"
	"sync/atomic"
	"time"
)

type NodeKind uint8

const (
	NodeRoot NodeKind = iota
	NodeBox
	NodeText
	NodeRawANSI
	NodeLink
	NodeButton
	NodeAlternateScreen
)

type ButtonState struct {
	Focused bool
	Hovered bool
	Active  bool
}

type EventHandlers struct {
	OnClick          func(*ClickEvent)
	OnFocus          func(*FocusEvent)
	OnFocusCapture   func(*FocusEvent)
	OnBlur           func(*FocusEvent)
	OnBlurCapture    func(*FocusEvent)
	OnKeyDown        func(*KeyboardEvent)
	OnKeyDownCapture func(*KeyboardEvent)
	OnMouseEnter     func()
	OnMouseLeave     func()
}

var nodeCounter uint64

// Node is the Go-native host node replacing React's Fiber host tree.
// It is intentionally concrete and inspectable: callers can keep refs and
// mutate text/style/scroll state without a reconciliation layer.
type Node struct {
	ID        string
	Kind      NodeKind
	Style     Style
	TextStyle TextStyle
	Text      string
	URL       string

	Children []*Node
	Parent   *Node

	Handlers  EventHandlers
	TabIndex  int // -2 = not focusable; -1 = programmatic only; >=0 = tab order
	AutoFocus bool

	Rect        Rect
	ContentRect Rect

	ScrollTop            int
	PendingScrollDelta   int
	ScrollHeight         int
	ScrollViewportHeight int
	ScrollViewportTop    int
	ScrollClampMin       *int
	ScrollClampMax       *int
	StickyScroll         bool
	ScrollAnchor         *ScrollAnchor
	ScrollDrainPerFrame  int

	ButtonState  ButtonState
	OnAction     func()
	ButtonRender func(ButtonState) []*Node
	ActiveUntil  time.Time

	MouseTracking bool

	dirty      bool
	generation uint64
}

type ScrollAnchor struct {
	Node   *Node
	Offset int
}

func nextNodeID() string {
	return fmt.Sprintf("n%d", atomic.AddUint64(&nodeCounter, 1))
}

func newNode(kind NodeKind, style Style) *Node {
	style.defaults()
	return &Node{
		ID:                  nextNodeID(),
		Kind:                kind,
		Style:               style,
		TabIndex:            -2,
		dirty:               true,
		ScrollDrainPerFrame: 12,
	}
}

func Root(children ...*Node) *Node {
	n := newNode(NodeRoot, Style{FlexDirection: Column, Width: Percent(100)})
	n.setChildren(children)
	return n
}

func Box(style Style, children ...*Node) *Node {
	n := newNode(NodeBox, style)
	n.setChildren(children)
	return n
}

func Text(text string, style ...TextStyle) *Node {
	n := newNode(NodeText, Style{FlexDirection: Row, FlexGrow: F(0), FlexShrink: F(1), TextWrap: TextWrapWrap})
	n.Text = ExpandTabs(text)
	if len(style) > 0 {
		n.TextStyle = style[0]
	}
	return n
}

func TextWithWrap(text string, wrap TextWrap, style TextStyle) *Node {
	n := Text(text, style)
	n.Style.TextWrap = wrap
	return n
}

func RawANSI(text string) *Node {
	n := newNode(NodeRawANSI, Style{FlexDirection: Row, FlexGrow: F(0), FlexShrink: F(1), TextWrap: TextWrapWrap})
	n.Text = ExpandTabs(text)
	return n
}

func Link(url string, child *Node) *Node {
	if child == nil {
		child = Text(url)
	}
	n := newNode(NodeLink, Style{FlexDirection: Row})
	n.URL = url
	n.setChildren([]*Node{child})
	return n
}

func Spacer() *Node {
	return Box(Style{FlexGrow: F(1), FlexShrink: F(1)})
}

func Newline(count ...int) *Node {
	n := 1
	if len(count) > 0 && count[0] > 0 {
		n = count[0]
	}
	s := ""
	for i := 0; i < n; i++ {
		s += "\n"
	}
	return Text(s)
}

func NoSelectBox(style Style, children ...*Node) *Node {
	style.NoSelect = NoSelect
	return Box(style, children...)
}

func NoSelectFromLeft(style Style, children ...*Node) *Node {
	style.NoSelect = NoSelectFromLeftEdge
	return Box(style, children...)
}

func ScrollBox(style Style, sticky bool, children ...*Node) *Node {
	style.OverflowX = OverflowScroll
	style.OverflowY = OverflowScroll
	// The TS component keeps the outer viewport's normal Box direction (row
	// unless overridden) and puts content in a non-shrinking column. Without
	// this wrapper Yoga/flex would shrink tall content to the viewport and
	// scrollHeight would never exceed clientHeight.
	n := newNode(NodeBox, style)
	n.StickyScroll = sticky
	inner := Box(Style{FlexDirection: Column, FlexGrow: F(1), FlexShrink: F(0), Width: Percent(100)}, children...)
	n.setChildren([]*Node{inner})
	return n
}

// ScrollContent exposes the implementation-equivalent inner content node.
// It is useful for virtualized lists and measurements; ordinary callers can
// keep refs to their child nodes instead.
func (n *Node) ScrollContent() *Node {
	if n == nil || (n.Style.OverflowY != OverflowScroll && n.Style.Overflow != OverflowScroll) || len(n.Children) != 1 {
		return nil
	}
	return n.Children[0]
}

func Button(style Style, onAction func(), children ...*Node) *Node {
	n := newNode(NodeButton, style)
	n.TabIndex = 0
	n.OnAction = onAction
	n.setChildren(children)
	return n
}

// ButtonWithState is the Go equivalent of Button's React render-prop form.
// render is called whenever focus/hover/active changes.
func ButtonWithState(style Style, onAction func(), render func(ButtonState) []*Node) *Node {
	n := newNode(NodeButton, style)
	n.TabIndex = 0
	n.OnAction = onAction
	n.ButtonRender = render
	n.refreshButtonView()
	return n
}

func (n *Node) refreshButtonView() {
	if n == nil || n.Kind != NodeButton || n.ButtonRender == nil {
		return
	}
	n.setChildren(n.ButtonRender(n.ButtonState))
}

func (n *Node) setButtonState(next ButtonState) {
	if n == nil || n.ButtonState == next {
		return
	}
	n.ButtonState = next
	n.refreshButtonView()
	n.MarkDirty()
}

func (n *Node) activateButton() {
	if n == nil || n.Kind != NodeButton {
		return
	}
	next := n.ButtonState
	next.Active = true
	n.ActiveUntil = time.Now().Add(100 * time.Millisecond)
	n.setButtonState(next)
}

func AlternateScreen(children ...*Node) *Node {
	n := newNode(NodeAlternateScreen, Style{
		FlexDirection: Column,
		FlexShrink:    F(0),
		Width:         Percent(100),
		Height:        Percent(100),
	})
	n.MouseTracking = true
	n.setChildren(children)
	return n
}

func (n *Node) setChildren(children []*Node) {
	for _, old := range n.Children {
		if old != nil && old.Parent == n {
			old.Parent = nil
		}
	}
	n.Children = n.Children[:0]
	for _, c := range children {
		if c == nil {
			continue
		}
		if c.Parent != nil && c.Parent != n {
			c.Parent.removeChild(c)
		}
		c.Parent = n
		n.Children = append(n.Children, c)
	}
	n.MarkDirty()
}

func (n *Node) SetChildren(children ...*Node) { n.setChildren(children) }

func (n *Node) Append(children ...*Node) {
	for _, c := range children {
		if c == nil {
			continue
		}
		if c.Parent != nil && c.Parent != n {
			c.Parent.removeChild(c)
		}
		c.Parent = n
		n.Children = append(n.Children, c)
	}
	n.MarkDirty()
}

func (n *Node) removeChild(child *Node) {
	for i, c := range n.Children {
		if c == child {
			copy(n.Children[i:], n.Children[i+1:])
			n.Children[len(n.Children)-1] = nil
			n.Children = n.Children[:len(n.Children)-1]
			child.Parent = nil
			n.MarkDirty()
			return
		}
	}
}

func (n *Node) Remove(child *Node) { n.removeChild(child) }

func (n *Node) SetText(text string) {
	if n.Text == text {
		return
	}
	n.Text = ExpandTabs(text)
	n.MarkDirty()
}

func (n *Node) SetStyle(style Style) {
	style.defaults()
	n.Style = style
	n.MarkDirty()
}

func (n *Node) SetTextStyle(style TextStyle) {
	if n.TextStyle == style {
		return
	}
	n.TextStyle = style
	n.MarkDirty()
}

func (n *Node) SetHandlers(h EventHandlers) {
	n.Handlers = h
}

func (n *Node) MarkDirty() {
	for cur := n; cur != nil; cur = cur.Parent {
		if cur.dirty {
			// Parent may already be dirty, but update generation on the source.
			if cur == n {
				cur.generation++
			}
			continue
		}
		cur.dirty = true
		cur.generation++
	}
}

func (n *Node) Dirty() bool { return n.dirty }

func (n *Node) clearDirtyRecursive() {
	n.dirty = false
	for _, c := range n.Children {
		c.clearDirtyRecursive()
	}
}

func (n *Node) ScrollTo(y int) {
	n.StickyScroll = false
	n.PendingScrollDelta = 0
	n.ScrollAnchor = nil
	n.ScrollTop = max(0, y)
	n.MarkDirty()
}

func (n *Node) ScrollBy(dy int) {
	n.StickyScroll = false
	n.ScrollAnchor = nil
	n.PendingScrollDelta += dy
	n.MarkDirty()
}

func (n *Node) ScrollToBottom() {
	n.PendingScrollDelta = 0
	n.StickyScroll = true
	n.MarkDirty()
}

func (n *Node) ScrollToElement(el *Node, offset int) {
	if el == nil {
		return
	}
	n.StickyScroll = false
	n.PendingScrollDelta = 0
	n.ScrollAnchor = &ScrollAnchor{Node: el, Offset: offset}
	n.MarkDirty()
}

func (n *Node) SetScrollClamp(minY, maxY *int) {
	n.ScrollClampMin = minY
	n.ScrollClampMax = maxY
	n.MarkDirty()
}

func (n *Node) Walk(fn func(*Node) bool) {
	if n == nil || !fn(n) {
		return
	}
	for _, c := range n.Children {
		c.Walk(fn)
	}
}

func (n *Node) IsDescendantOf(parent *Node) bool {
	for p := n.Parent; p != nil; p = p.Parent {
		if p == parent {
			return true
		}
	}
	return false
}
