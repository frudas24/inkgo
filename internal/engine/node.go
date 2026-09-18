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

// nodeFlowMeasure is one memoised intrinsic measurement of a container node.
//
// measureNode reads only the node kind, its own style (with the defaults
// applied) and its children subtree for a Box/ScrollBox/Root, so for a given
// available space the result is fixed for as long as nothing inside the subtree
// (or the style of the node) changed. The entry therefore stores no style of its
// own: the style every entry was measured under is kept once in nodeFlowCache,
// because a style change can change the measurement under any available space
// and must drop them all.
type nodeFlowMeasure struct {
	// epoch is Node.measureEpoch at the time the entry was written. A mutation
	// anywhere in the subtree bumps it, which is what makes an entry written
	// before that mutation unusable.
	epoch          uint64
	availW, availH int
	result         measured
}

// nodeFlowMeasureSlots bounds how many available spaces one container memoises.
//
// The flex path measures a child at most twice per pass - once as a flex item
// under the space its parent offers and once again under the main size it was
// actually allocated - and once per ancestor level above that, so the number of
// distinct keys a node sees grows with how deeply it is nested. Measured with
// TestFlowMeasureCacheKeySetIsBounded: 2 keys per container in the transcript
// shapes of the PERF-001 benchmarks and in a deep transcript shape, 5 in a deeply
// nested auto-sized tree. The table is sized above that with room for deeper
// trees, and going over it stays correct either way: the lookup compares
// (epoch, availW, availH) exactly, so a missed key costs a recompute, never a
// wrong result. Eviction is oldest-first, which keeps the keys a pass uses.
const nodeFlowMeasureSlots = 8

// nodeFlowCache memoises the intrinsic measurements of one container node.
//
// An entry is only usable while none of the node's inputs changed since it was
// written: measureNode checks the node's measureEpoch, which MarkDirty bumps on
// the mutated node and on every ancestor, and the style/kind snapshot below.
// The cache is allocated lazily and at most once per node: text nodes and
// containers that are never measured as containers never pay for it.
type nodeFlowCache struct {
	kind  NodeKind
	style Style
	// next is the slot a full cache overwrites; entries are replaced oldest
	// first, which is enough for the one or two keys a pass uses.
	next  uint8
	count uint8
	// entries is a fixed array, not a slice: the cache lives inside Node and a
	// slice header per slot would allocate on every miss.
	entries [nodeFlowMeasureSlots]nodeFlowMeasure
}

// lookup returns the memoised measurement of one available space for one measure
// epoch. Only an exact match of all three is served. c must be non-nil (the
// caller allocates the cache before looking anything up).
func (c *nodeFlowCache) lookup(epoch uint64, availW, availH int) (measured, bool) {
	for i := 0; i < int(c.count); i++ {
		e := &c.entries[i]
		if e.epoch == epoch && e.availW == availW && e.availH == availH {
			return e.result, true
		}
	}
	return measured{}, false
}

// store records one measurement, evicting the oldest entry once the cache is
// full.
func (c *nodeFlowCache) store(epoch uint64, availW, availH int, result measured) {
	entry := nodeFlowMeasure{epoch: epoch, availW: availW, availH: availH, result: result}
	if int(c.count) < len(c.entries) {
		c.entries[c.count] = entry
		c.count++
		return
	}
	c.entries[c.next] = entry
	c.next = (c.next + 1) % uint8(len(c.entries))
}

// rebind points the cache at a different kind or style, dropping every entry:
// they were all measured under inputs the node no longer has.
func (c *nodeFlowCache) rebind(kind NodeKind, style Style) {
	c.kind, c.style, c.count, c.next = kind, style, 0, 0
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
	width int
	mode  TextWrap
	base  TextStyle
	lines []string
	rows  [][]StyledGrapheme
	soft  []bool
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
	// flowCache memoises the intrinsic measurements of a container node. It is
	// nil until the node is measured as a container; its entries are invalidated
	// by measureEpoch (see nodeFlowCache).
	flowCache *nodeFlowCache

	dirty       bool
	layoutDirty bool
	generation  uint64

	// subtreeGeomDirty means this sub-root (itself included) contains a change
	// that can still move geometry and has not been visited by a layout pass
	// yet. MarkDirty sets it on the mutated node and on every ancestor, so a
	// clean subtree can be pruned without re-measuring it. markPaintDirty
	// deliberately leaves it alone: paint-only changes never move geometry.
	subtreeGeomDirty bool
	// measureEpoch counts the geometry mutations this node or its subtree has
	// seen. MarkDirty bumps it on the mutated node and on every ancestor, so an
	// intrinsic measurement memoised at an older epoch can never be served: the
	// measurement is a pure function of (kind, style, subtree content), and this
	// counter is the cheapest sound witness that none of the three changed.
	//
	// It is deliberately not Node.generation: that counter only bumps on an
	// ancestor that was clean, which is enough for the renderer but not for a
	// cache that has to notice a second mutation while the first is still
	// unvisited.
	measureEpoch uint64
	// layoutStamp is the layout pass that last visited this node. ComputeLayout
	// uses it to find the nodes a pruned subtree skipped (see drainScroll).
	layoutStamp uint64

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
		// A fresh node carries no geometry, so it is dirty for layout until a
		// pass visits it.
		subtreeGeomDirty: true,
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

// invalidateTextCaches drops every measurement cache the node owns. It is called
// by the setters that change what this node measures: its text and its style.
// The caches of the ancestors are not touched here on purpose - MarkDirty, which
// those setters also call, bumps the measure epoch of the whole ancestor chain,
// and measureNode serves a container entry only while that epoch still matches.
func (n *Node) invalidateTextCaches() {
	if n == nil {
		return
	}
	n.measureCache = nodeMeasureCache{}
	n.wrapCache = nodeWrapCache{}
	n.ansiCache = nodeANSICache{}
	n.flowCache = nil
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

// MarkDirty invalidates geometry. Callers use it for every mutation that can
// move a rect (text, style, children, button state), so it also marks the whole
// ancestor chain as holding a geometrically dirty subtree, which is what lets
// ComputeLayout prune unchanged siblings and what lets measureNode drop the
// intrinsic measurements an ancestor memoised before this mutation.
func (n *Node) MarkDirty() {
	for cur := n; cur != nil; cur = cur.Parent {
		if cur == n || !cur.dirty {
			cur.generation++
		}
		cur.dirty = true
		cur.layoutDirty = true
		cur.subtreeGeomDirty = true
		cur.measureEpoch++
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
