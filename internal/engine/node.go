package engine

import (
	"fmt"
	core "github.com/frudas24/inkgo/internal/core"
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
	OnPaste          func(*PasteEvent)
	OnPasteCapture   func(*PasteEvent)
	OnResize         func(*ResizeEvent)
	OnMouseEnter     func()
	OnMouseLeave     func()
}

var nodeCounter uint64

type nodeMeasureCache struct {
	valid          bool
	text           string
	style          Style
	availW, availH int
	result         measured
}

type nodeWrapCache struct {
	valid     bool
	text      string
	width     int
	mode      TextWrap
	lines     []string
	soft      []bool
	graphemes [][]Grapheme
}

type nodeANSICache struct {
	valid bool
	text  string
	base  TextStyle
	items []StyledGrapheme
}

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
	// ScrollDrainPerFrame > 0 forces a fixed drain step. Zero uses the
	// terminal-aware proportional/adaptive policy.
	ScrollDrainPerFrame int
	ScrollAdaptive      bool

	ButtonState  ButtonState
	OnAction     func()
	ButtonRender func(ButtonState) []*Node
	ActiveUntil  time.Time

	MouseTracking bool

	measureCache nodeMeasureCache
	wrapCache    nodeWrapCache
	ansiCache    nodeANSICache

	dirty       bool
	layoutDirty bool
	generation  uint64

	// Root-level layout cache metadata. Keeping it on Node avoids renderer-owned
	// geometry state and lets ComputeLayout be reused by standalone callers.
	layoutCached     bool
	layoutViewport   Size
	layoutFullscreen bool

	// Set by the layout pass when direct children are vertically ordered and
	// non-overlapping. The renderer can then binary-search the visible range
	// instead of visiting thousands of off-screen list rows on every scroll.
	childrenLinearY bool

	// Root-level index rebuilt with geometry. Scroll-only renders reuse this
	// list instead of repeatedly walking large static trees to rediscover the
	// same ScrollBox nodes.
	layoutScrollNodes []*Node
	layoutButtonNodes []*Node
}

type ScrollAnchor struct {
	Node   *Node
	Offset int
}

func nextNodeID() string {
	return fmt.Sprintf("n%d", atomic.AddUint64(&nodeCounter, 1))
}

func newNode(kind NodeKind, style Style) *Node {
	core.ApplyDefaults(&style)
	return &Node{
		ID:          nextNodeID(),
		Kind:        kind,
		Style:       style,
		TabIndex:    -2,
		dirty:       true,
		layoutDirty: true,
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

// acceptsChild prevents cycles through the supported tree mutation methods.
func (n *Node) acceptsChild(child *Node) bool {
	if child == nil {
		return false
	}
	for p := n; p != nil; p = p.Parent {
		if p == child {
			return false
		}
	}
	return true
}

func (n *Node) setChildren(children []*Node) {
	// Snapshot before detaching: children may alias this node or a source parent.
	next := make([]*Node, 0, len(children))
	seen := make(map[*Node]struct{}, len(children))
	for _, child := range children {
		if !n.acceptsChild(child) {
			continue
		}
		if _, duplicate := seen[child]; duplicate {
			continue
		}
		seen[child] = struct{}{}
		next = append(next, child)
	}
	for _, old := range n.Children {
		if old != nil && old.Parent == n {
			old.Parent = nil
		}
	}
	n.Children = next
	for _, child := range next {
		if child.Parent != nil && child.Parent != n {
			child.Parent.removeChild(child)
		}
		child.Parent = n
	}
	n.MarkDirty()
}

// SetChildren replaces the child list, preserving order and ignoring nil,
// repeated nodes, and entries that would create an ancestor cycle.
func (n *Node) SetChildren(children ...*Node) { n.setChildren(children) }

// Append adds new children in order. Existing children retain their position;
// nil, repeated nodes and entries that would create a cycle are ignored.
func (n *Node) Append(children ...*Node) {
	children = append([]*Node(nil), children...)
	changed := false
	for _, child := range children {
		if !n.acceptsChild(child) || child.Parent == n {
			continue
		}
		if child.Parent != nil {
			child.Parent.removeChild(child)
		}
		child.Parent = n
		n.Children = append(n.Children, child)
		changed = true
	}
	if changed {
		n.MarkDirty()
	}
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

func (n *Node) invalidateTextCaches() {
	if n == nil {
		return
	}
	n.measureCache = nodeMeasureCache{}
	n.wrapCache = nodeWrapCache{}
	n.ansiCache = nodeANSICache{}
}

func (n *Node) SetText(text string) {
	expanded := ExpandTabs(text)
	if n.Text == expanded {
		return
	}
	n.Text = expanded
	n.invalidateTextCaches()
	n.MarkDirty()
}

func (n *Node) SetStyle(style Style) {
	core.ApplyDefaults(&style)
	n.Style = style
	n.invalidateTextCaches()
	n.MarkDirty()
}

func (n *Node) SetTextStyle(style TextStyle) {
	if n.TextStyle == style {
		return
	}
	n.TextStyle = style
	n.markPaintDirty()
}

func (n *Node) SetHandlers(h EventHandlers) {
	n.Handlers = h
}

func (n *Node) MarkDirty() {
	for cur := n; cur != nil; cur = cur.Parent {
		if cur == n || !cur.dirty {
			cur.generation++
		}
		cur.dirty = true
		cur.layoutDirty = true
	}
}

// markPaintDirty invalidates rendering while preserving cached geometry. It is
// used for scrolling and purely visual style changes. Keeping this separate
// from MarkDirty is what makes large ScrollBoxes scale with the viewport rather
// than with total history length.
func (n *Node) markPaintDirty() {
	for cur := n; cur != nil; cur = cur.Parent {
		if cur == n || !cur.dirty {
			cur.generation++
		}
		cur.dirty = true
	}
}

func (n *Node) Dirty() bool { return n.dirty }

func (n *Node) clearDirtyRecursive() {
	if n == nil || !n.dirty {
		return
	}
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
	n.markPaintDirty()
}

func (n *Node) ScrollBy(dy int) {
	n.StickyScroll = false
	n.ScrollAnchor = nil
	n.PendingScrollDelta += dy
	n.markPaintDirty()
}

func (n *Node) ScrollToBottom() {
	n.PendingScrollDelta = 0
	n.StickyScroll = true
	n.markPaintDirty()
}

func (n *Node) ScrollToElement(el *Node, offset int) {
	if el == nil {
		return
	}
	n.StickyScroll = false
	n.PendingScrollDelta = 0
	n.ScrollAnchor = &ScrollAnchor{Node: el, Offset: offset}
	n.markPaintDirty()
}

func (n *Node) SetScrollClamp(minY, maxY *int) {
	n.ScrollClampMin = minY
	n.ScrollClampMax = maxY
	n.markPaintDirty()
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
