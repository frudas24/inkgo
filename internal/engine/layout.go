package engine

import (
	core "github.com/frudas24/inkgo/internal/core"
	"math"
	"sort"
	"sync/atomic"
)

type layoutCtx struct {
	viewport Size
	// stamp identifies this pass. Every node the pass reaches records it, so
	// nodes that a pruned subtree skipped can be found afterwards.
	stamp uint64
	// visited counts the nodes the pass actually walked (diagnostics/tests).
	visited int
}

// layoutPassStamp issues a unique stamp per pass. Atomic because independent
// trees may be laid out from different goroutines under -race.
var layoutPassStamp uint64

type measured struct {
	w, h int
}

func resolveLength(l Length, parent int) (int, bool) {
	if parent < 0 {
		if l.Unit == LengthCells {
			return max(0, roundCell(l.Value)), true
		}
		return 0, false
	}
	if v, ok := l.Resolve(float64(parent)); ok {
		return max(0, roundCell(v)), true
	}
	return 0, false
}

func applyMinMax(v int, minL, maxL Length, parent int) int {
	if lo, ok := resolveLength(minL, parent); ok && v < lo {
		v = lo
	}
	if hi, ok := resolveLength(maxL, parent); ok && v > hi {
		v = hi
	}
	return max(0, v)
}

// measureNodeProbe, when non-nil, observes every node measureNode walks, with
// the available space it was measured under. It exists for the tests that have
// to prove a memoised container does not re-walk its subtree; production leaves
// it nil, so the call is a predictable branch on a package variable.
var measureNodeProbe func(n *Node, availW, availH int)

func nodeText(n *Node) string {
	if n.Kind == NodeRawANSI {
		return StripANSI(n.Text)
	}
	return n.Text
}

func measureNode(n *Node, availW, availH int) measured {
	if measureNodeProbe != nil {
		measureNodeProbe(n, availW, availH)
	}
	if n == nil || n.Style.Display == DisplayNone {
		return measured{}
	}
	s := n.Style
	core.ApplyDefaults(&s)
	pad := core.PaddingEdges(s)
	border := core.BorderEdges(s)
	extraW := pad.Left + pad.Right + border.Left + border.Right
	extraH := pad.Top + pad.Bottom + border.Top + border.Bottom

	explicitW, hasW := resolveLength(s.Width, availW)
	explicitH, hasH := resolveLength(s.Height, availH)
	innerAvailW := availW
	innerAvailH := availH
	if hasW {
		innerAvailW = max(0, explicitW-extraW)
	} else if innerAvailW >= 0 {
		innerAvailW = max(0, innerAvailW-extraW)
	}
	if hasH {
		innerAvailH = max(0, explicitH-extraH)
	} else if innerAvailH >= 0 {
		innerAvailH = max(0, innerAvailH-extraH)
	}

	if n.Kind == NodeText || n.Kind == NodeRawANSI {
		if c := &n.measureCache; c.valid && c.text == n.Text && c.style == s && c.availW == availW && c.availH == availH {
			return c.result
		}
		txt := nodeText(n)
		widthForWrap := innerAvailW
		if widthForWrap < 0 {
			widthForWrap = max(1, WidestLine(txt))
		}
		ms := MeasureText(txt, widthForWrap, s.TextWrap)
		w, h := ms.Width+extraW, ms.Height+extraH
		if hasW {
			w = explicitW
		}
		if hasH {
			h = explicitH
		}
		w = applyMinMax(w, s.MinWidth, s.MaxWidth, availW)
		h = applyMinMax(h, s.MinHeight, s.MaxHeight, availH)
		result := measured{w, h}
		n.measureCache = nodeMeasureCache{valid: true, text: n.Text, style: s, availW: availW, availH: availH, result: result}
		return result
	}

	// Container measurement memoisation.
	//
	// Everything below (the children filter, measureFlowContent, measureItem and
	// the box model arithmetic) reads only n.Kind, the style defaults applied
	// above and the children subtree; the node's own Rect, ContentRect and scroll
	// state are never read by this function nor by anything it calls, so for a
	// fixed available space the result is a pure function of (kind, style,
	// subtree content). Without this, a single leaf text update re-measured every
	// ancestor container through its whole subtree, because the parent's flex
	// pass measures all of its children before the incremental prune in
	// layoutNode can skip any of them.
	//
	// The entries are keyed on the exact available space plus the measure epoch
	// MarkDirty bumps on the mutated node and every ancestor, so an entry is
	// served only while no input of the measurement changed since it was written.
	// Gating on Node.subtreeGeomDirty instead would be unsound: a layout pass
	// clears that flag whenever it visits the node, and the root is not measured
	// in fullscreen mode, so a mutation followed by a fullscreen pass would clear
	// the flag and leave a stale root entry behind for the next natural-height
	// pass (reproduced by TestFlowMeasureCacheRootMeasuredOnlyOutsideFullscreen).
	// Paint-only mutations (SetTextStyle, scrolling) bump neither, and cannot
	// change a measurement.
	c := n.flowCache
	if c == nil {
		c = &nodeFlowCache{}
		n.flowCache = c
	}
	if c.kind != n.Kind || c.style != s {
		c.rebind(n.Kind, s)
	} else if result, ok := c.lookup(n.measureEpoch, availW, availH); ok {
		return result
	}

	flow := make([]*Node, 0, len(n.Children))
	for _, c := range n.Children {
		if c != nil && c.Style.Display != DisplayNone && c.Style.Position != PositionAbsolute {
			flow = append(flow, c)
		}
	}

	dir := s.FlexDirection
	if dir == "" {
		dir = Row
	}
	contentW, contentH := measureFlowContent(flow, s, dir, innerAvailW, innerAvailH)

	w := contentW + extraW
	h := contentH + extraH
	if hasW {
		w = explicitW
	}
	if hasH {
		h = explicitH
	}
	w = applyMinMax(w, s.MinWidth, s.MaxWidth, availW)
	h = applyMinMax(h, s.MinHeight, s.MaxHeight, availH)
	result := measured{w, h}
	c.store(n.measureEpoch, availW, availH, result)
	return result
}

// measureFlowContent performs the intrinsic measurement pass for flex
// children. In particular, wrapped containers must account for every flex
// line in the cross axis; measuring them as one unwrapped line underestimates
// auto height/width and can clip content before the layout pass even runs.
func measureFlowContent(flow []*Node, s Style, dir FlexDirection, availW, availH int) (int, int) {
	if len(flow) == 0 {
		return 0, 0
	}
	mainGap := max(0, core.GapMain(s, dir))
	crossGap := max(0, core.GapCross(s, dir))
	mainAvail := availW
	if dir == Column || dir == ColumnReverse {
		mainAvail = availH
	}
	wrap := s.FlexWrap != "" && s.FlexWrap != NoWrap && mainAvail >= 0

	type measuredItem struct{ main, cross int }
	items := make([]measuredItem, 0, len(flow))
	for _, c := range flow {
		cm := measureNode(c, availW, availH)
		m := core.MarginEdges(c.Style)
		if dir == Row || dir == RowReverse {
			items = append(items, measuredItem{main: cm.w + m.Left + m.Right, cross: cm.h + m.Top + m.Bottom})
		} else {
			items = append(items, measuredItem{main: cm.h + m.Top + m.Bottom, cross: cm.w + m.Left + m.Right})
		}
	}

	lineMain, lineCross := 0, 0
	maxMain, totalCross, lines := 0, 0, 0
	flush := func() {
		if lineMain == 0 && lineCross == 0 {
			return
		}
		if lines > 0 {
			totalCross += crossGap
		}
		totalCross += lineCross
		maxMain = max(maxMain, lineMain)
		lineMain, lineCross = 0, 0
		lines++
	}
	for _, it := range items {
		need := it.main
		if lineMain > 0 {
			need += mainGap
		}
		if wrap && lineMain > 0 && lineMain+need > mainAvail {
			flush()
		}
		if lineMain > 0 {
			lineMain += mainGap
		}
		lineMain += it.main
		lineCross = max(lineCross, it.cross)
	}
	flush()

	if dir == Row || dir == RowReverse {
		return maxMain, totalCross
	}
	return totalCross, maxMain
}

// ComputeLayout calculates node rectangles. In normal mode the root has a
// terminal-width constraint and natural height, matching Ink scrollback mode.
// In fullscreen mode root height is constrained to viewport.Height.
func ComputeLayout(root *Node, viewport Size, fullscreen bool) Size {
	sz, _ := computeLayout(root, viewport, fullscreen)
	return sz
}

// computeLayout is ComputeLayout plus the number of nodes the pass walked. The
// prune in layoutNode is only observably useful if whole subtrees are skipped,
// so tests assert on the count.
func computeLayout(root *Node, viewport Size, fullscreen bool) (Size, int) {
	if root == nil {
		return Size{}, 0
	}
	if root.layoutCached && !root.layoutDirty && root.layoutViewport == viewport && root.layoutFullscreen == fullscreen {
		refreshScrollState(root)
		return Size{Width: root.Rect.Width, Height: root.Rect.Height}, 0
	}
	// Snapshot the scroll nodes found by the previous pass: a pruned subtree is
	// not walked, so its ScrollBoxes are not reached by this pass either.
	prevScrolls := append([]*Node(nil), root.layoutScrollNodes...)
	ctx := layoutCtx{viewport: viewport, stamp: atomic.AddUint64(&layoutPassStamp, 1)}
	h := viewport.Height
	if !fullscreen {
		m := measureNode(root, viewport.Width, -1)
		h = max(0, m.h)
	}
	layoutNode(&ctx, root, Rect{X: 0, Y: 0, Width: max(0, viewport.Width), Height: max(0, h)}, true, true)
	root.layoutCached = true
	root.layoutViewport = viewport
	root.layoutFullscreen = fullscreen
	rebuildLayoutIndexes(root)
	drainSkippedScroll(prevScrolls, ctx.stamp, root)
	root.layoutDirty = false
	return Size{Width: root.Rect.Width, Height: root.Rect.Height}, ctx.visited
}

// drainSkippedScroll applies the scroll bookkeeping that layoutNode normally
// performs at the end of its body to the ScrollBoxes this pass did not visit,
// i.e. the ones sitting inside a pruned (geometrically unchanged) subtree.
// Running it once, after the pass, yields the same result as the in-pass call:
// the content rect and every child rect of a pruned subtree are unchanged, and
// a scroll node whose descendants moved can never be pruned.
//
// Nodes the pass did reach are skipped by stamp, which is also what keeps the
// call count at exactly one updateScrollState per scroll node and per pass.
func drainSkippedScroll(prevScrolls []*Node, stamp uint64, root *Node) {
	for _, n := range prevScrolls {
		if n == nil || n.layoutStamp == stamp {
			continue
		}
		// Only nodes the pass could have updated itself are drained. A node
		// that left the tree, sits under a hidden node, or is text content was
		// never updated by the in-pass call (layoutNode returns before its
		// scroll bookkeeping for those), so draining it would make a pruned
		// pass behave differently from the full pass it stands in for.
		if !layoutPassReaches(n, root) {
			continue
		}
		updateScrollState(n, n.ContentRect)
	}
}

// layoutPassReaches reports whether layoutNode would have applied
// updateScrollState to n during this pass. The pass never reaches a node that
// left the tree or that sits under a hidden node, and it returns before its
// scroll bookkeeping for text content (NodeText/NodeRawANSI render no
// children, so nothing there can move). Such a node keeps the scroll state the
// pre-prune code left on it: a pass that skips it must not touch it either.
func layoutPassReaches(n, root *Node) bool {
	if n == nil {
		return false
	}
	if n.Kind == NodeText || n.Kind == NodeRawANSI {
		return false
	}
	for cur := n; cur != nil; cur = cur.Parent {
		if cur.Style.Display == DisplayNone {
			return false
		}
		if cur == root {
			return true
		}
	}
	return false
}

func rebuildLayoutIndexes(root *Node) {
	if root == nil {
		return
	}
	scrolls := root.layoutScrollNodes[:0]
	buttons := root.layoutButtonNodes[:0]
	root.Walk(func(n *Node) bool {
		if n.Style.OverflowY == OverflowScroll || n.Style.Overflow == OverflowScroll {
			scrolls = append(scrolls, n)
		}
		if n.Kind == NodeButton {
			buttons = append(buttons, n)
		}
		return true
	})
	root.layoutScrollNodes = scrolls
	root.layoutButtonNodes = buttons
}

func discoverScrollNodes(root *Node) []*Node {
	if root == nil {
		return nil
	}
	var out []*Node
	root.Walk(func(n *Node) bool {
		if n.Style.OverflowY == OverflowScroll || n.Style.Overflow == OverflowScroll {
			out = append(out, n)
		}
		return true
	})
	return out
}

func layoutScrollNodes(root *Node) []*Node {
	if root == nil {
		return nil
	}
	if root.layoutCached && !root.layoutDirty {
		return root.layoutScrollNodes
	}
	return discoverScrollNodes(root)
}

func layoutButtonNodes(root *Node) []*Node {
	if root == nil {
		return nil
	}
	if root.layoutCached && !root.layoutDirty {
		return root.layoutButtonNodes
	}
	var out []*Node
	root.Walk(func(n *Node) bool {
		if n.Kind == NodeButton {
			out = append(out, n)
		}
		return true
	})
	return out
}

// refreshScrollState applies scroll-only mutations to an already valid layout.
// Child geometry remains in content coordinates; only the viewport translation
// changes, so a full flex pass would be wasted work.
func refreshScrollState(root *Node) {
	if root == nil {
		return
	}
	for _, n := range layoutScrollNodes(root) {
		updateScrollState(n, n.ContentRect)
	}
}

func LayoutNatural(root *Node, width int) Size {
	return ComputeLayout(root, Size{Width: width}, false)
}

type flexItem struct {
	node          *Node
	margin        Edges
	baseMain      int
	main          int
	cross         int
	explicitCross bool
	grow, shrink  float64
	minMain       int
	maxMain       int
	hasMinMain    bool
	hasMaxMain    bool
}

type flexLine struct {
	items    []*flexItem
	mainUsed int
	cross    int
}

func measureItem(n *Node, dir FlexDirection, mainAvail, crossAvail int) *flexItem {
	m := core.MarginEdges(n.Style)
	var availW, availH int
	if dir == Row || dir == RowReverse {
		availW, availH = mainAvail, crossAvail
	} else {
		availW, availH = crossAvail, mainAvail
	}
	ms := measureNode(n, availW, availH)
	it := &flexItem{node: n, margin: m}
	if n.Style.FlexGrow != nil {
		it.grow = *n.Style.FlexGrow
	}
	if n.Style.FlexShrink != nil {
		it.shrink = *n.Style.FlexShrink
	} else {
		it.shrink = 1
	}

	if dir == Row || dir == RowReverse {
		it.baseMain = ms.w
		if v, ok := resolveLength(n.Style.FlexBasis, mainAvail); ok {
			it.baseMain = v
		}
		it.cross = ms.h
		_, it.explicitCross = resolveLength(n.Style.Height, crossAvail)
		it.minMain, it.hasMinMain = resolveLength(n.Style.MinWidth, mainAvail)
		it.maxMain, it.hasMaxMain = resolveLength(n.Style.MaxWidth, mainAvail)
	} else {
		it.baseMain = ms.h
		if v, ok := resolveLength(n.Style.FlexBasis, mainAvail); ok {
			it.baseMain = v
		}
		it.cross = ms.w
		_, it.explicitCross = resolveLength(n.Style.Width, crossAvail)
		it.minMain, it.hasMinMain = resolveLength(n.Style.MinHeight, mainAvail)
		it.maxMain, it.hasMaxMain = resolveLength(n.Style.MaxHeight, mainAvail)
	}
	it.main = max(0, it.baseMain)
	if it.hasMinMain {
		it.main = max(it.main, it.minMain)
	}
	if it.hasMaxMain {
		it.main = min(it.main, it.maxMain)
	}
	return it
}

func makeFlexLines(items []*flexItem, dir FlexDirection, mainAvail, gap int, wrap FlexWrap) []*flexLine {
	if len(items) == 0 {
		return nil
	}
	if wrap == "" || wrap == NoWrap || mainAvail <= 0 {
		line := &flexLine{items: append([]*flexItem(nil), items...)}
		for i, it := range line.items {
			if i > 0 {
				line.mainUsed += gap
			}
			line.mainUsed += it.main + flexMainMargins(it, dir)
		}
		return []*flexLine{line}
	}
	var lines []*flexLine
	cur := &flexLine{}
	for _, it := range items {
		need := it.main + flexMainMargins(it, dir)
		if len(cur.items) > 0 {
			need += gap
		}
		if len(cur.items) > 0 && cur.mainUsed+need > mainAvail {
			lines = append(lines, cur)
			cur = &flexLine{}
		}
		if len(cur.items) > 0 {
			cur.mainUsed += gap
		}
		cur.items = append(cur.items, it)
		cur.mainUsed += it.main + flexMainMargins(it, dir)
	}
	if len(cur.items) > 0 {
		lines = append(lines, cur)
	}
	if wrap == WrapReverse {
		for i, j := 0, len(lines)-1; i < j; i, j = i+1, j-1 {
			lines[i], lines[j] = lines[j], lines[i]
		}
	}
	return lines
}

func mainMargins(it *flexItem, vertical bool) int {
	if vertical {
		return it.margin.Top + it.margin.Bottom
	}
	return it.margin.Left + it.margin.Right
}

func flexMainMargins(it *flexItem, dir FlexDirection) int {
	return mainMargins(it, dir == Column || dir == ColumnReverse)
}

func flexCrossMargins(it *flexItem, dir FlexDirection) int {
	return mainMargins(it, dir == Row || dir == RowReverse)
}

func distributeMain(line *flexLine, dir FlexDirection, mainAvail, gap int) {
	used := 0
	totalGrow := 0.0
	totalShrinkWeight := 0.0
	for i, it := range line.items {
		if i > 0 {
			used += gap
		}
		used += it.main + flexMainMargins(it, dir)
		totalGrow += maxFloat(0, it.grow)
		totalShrinkWeight += maxFloat(0, it.shrink) * float64(max(1, it.baseMain))
	}
	free := mainAvail - used
	if free > 0 && totalGrow > 0 {
		distributeGrow(line.items, free, totalGrow)
	} else if free < 0 && totalShrinkWeight > 0 {
		distributeShrink(line.items, -free, totalShrinkWeight)
	}
	line.mainUsed = 0
	for i, it := range line.items {
		if i > 0 {
			line.mainUsed += gap
		}
		line.mainUsed += it.main + flexMainMargins(it, dir)
	}
}

// distributeGrow apportions integer terminal cells only to items that actually
// participate in flex-grow. Keeping the remainder among eligible items avoids
// a subtle parity bug where an ineligible last sibling accidentally absorbed
// all rounding remainder.
func growCapacity(it *flexItem) int {
	if it == nil || it.grow <= 0 {
		return 0
	}
	if !it.hasMaxMain {
		return int(^uint(0) >> 2)
	}
	return max(0, it.maxMain-it.main)
}

func shrinkCapacity(it *flexItem) int {
	if it == nil || it.shrink <= 0 {
		return 0
	}
	lo := 0
	if it.hasMinMain {
		lo = it.minMain
	}
	return max(0, it.main-lo)
}

// distributeGrow allocates free cells among positive flex-grow siblings while
// honoring max-main constraints. If one item freezes at maxWidth/maxHeight,
// its unused share is redistributed to the remaining flexible siblings.
func distributeGrow(items []*flexItem, free int, _ float64) {
	remaining := free
	for remaining > 0 {
		active := make([]*flexItem, 0, len(items))
		total := 0.0
		for _, it := range items {
			if growCapacity(it) > 0 {
				active = append(active, it)
				total += it.grow
			}
		}
		if len(active) == 0 || total <= 0 {
			return
		}

		before := remaining
		budget := remaining
		for _, it := range active {
			share := int(math.Floor(float64(budget) * it.grow / total))
			if share <= 0 {
				continue
			}
			add := min(share, growCapacity(it), remaining)
			it.main += add
			remaining -= add
		}
		// Integer shares can all round to zero. Spend remainder one cell at a
		// time only among still-eligible items; this is bounded by active count
		// per round, not by the full free-space magnitude in the common case.
		for _, it := range active {
			if remaining == 0 {
				break
			}
			if growCapacity(it) <= 0 {
				continue
			}
			it.main++
			remaining--
		}
		if remaining == before {
			return
		}
	}
}

// distributeShrink mirrors Yoga's scaled shrink weighting and honors
// minWidth/minHeight freezes, redistributing any constrained share.
func distributeShrink(items []*flexItem, need int, _ float64) {
	remaining := need
	for remaining > 0 {
		active := make([]*flexItem, 0, len(items))
		total := 0.0
		for _, it := range items {
			if shrinkCapacity(it) <= 0 {
				continue
			}
			weight := it.shrink * float64(max(1, it.baseMain))
			if weight <= 0 {
				continue
			}
			active = append(active, it)
			total += weight
		}
		if len(active) == 0 || total <= 0 {
			return
		}

		before := remaining
		budget := remaining
		for _, it := range active {
			weight := it.shrink * float64(max(1, it.baseMain))
			share := int(math.Floor(float64(budget) * weight / total))
			if share <= 0 {
				continue
			}
			cut := min(share, shrinkCapacity(it), remaining)
			it.main -= cut
			remaining -= cut
		}
		for _, it := range active {
			if remaining == 0 {
				break
			}
			if shrinkCapacity(it) <= 0 {
				continue
			}
			it.main--
			remaining--
		}
		if remaining == before {
			return
		}
	}
}

func justifyStartGap(j Justify, free, count, baseGap int) (start float64, gap float64) {
	if free < 0 {
		free = 0
	}
	switch j {
	case JustifyCenter:
		return float64(free) / 2, float64(baseGap)
	case JustifyFlexEnd:
		return float64(free), float64(baseGap)
	case JustifySpaceBetween:
		if count > 1 {
			return 0, float64(baseGap) + float64(free)/float64(count-1)
		}
	case JustifySpaceAround:
		if count > 0 {
			extra := float64(free) / float64(count)
			return extra / 2, float64(baseGap) + extra
		}
	case JustifySpaceEvenly:
		if count > 0 {
			extra := float64(free) / float64(count+1)
			return extra, float64(baseGap) + extra
		}
	}
	return 0, float64(baseGap)
}

// layoutNode assigns rects to n and its subtree. A subtree whose assigned rect
// and cached rect agree and that holds no geometrically dirty node is left
// untouched: its rects (and its scroll bookkeeping, handled by
// drainSkippedScroll) are still valid.
func layoutNode(ctx *layoutCtx, n *Node, assigned Rect, forceW, forceH bool) {
	if n == nil {
		return
	}
	ctx.visited++
	s := n.Style
	core.ApplyDefaults(&s)
	n.Style = s
	if s.Display == DisplayNone {
		n.Rect = Rect{}
		n.ContentRect = Rect{}
		n.subtreeGeomDirty = false
		// Hidden: nothing to lay out and, matching the historical early return,
		// nothing for updateScrollState to do either.
		n.layoutStamp = ctx.stamp
		return
	}

	w, h := assigned.Width, assigned.Height
	if !forceW {
		if v, ok := resolveLength(s.Width, assigned.Width); ok {
			w = v
		} else {
			w = measureNode(n, assigned.Width, assigned.Height).w
		}
	}
	if !forceH {
		if v, ok := resolveLength(s.Height, assigned.Height); ok {
			h = v
		} else {
			h = measureNode(n, assigned.Width, assigned.Height).h
		}
	}
	w = applyMinMax(w, s.MinWidth, s.MaxWidth, assigned.Width)
	h = applyMinMax(h, s.MinHeight, s.MaxHeight, assigned.Height)

	next := Rect{X: assigned.X, Y: assigned.Y, Width: max(0, w), Height: max(0, h)}
	border := core.BorderEdges(s)
	pad := core.PaddingEdges(s)
	insets := AddEdges(border, pad)
	nextContent := Rect{
		X:      next.X + insets.Left,
		Y:      next.Y + insets.Top,
		Width:  max(0, next.Width-insets.Left-insets.Right),
		Height: max(0, next.Height-insets.Top-insets.Bottom),
	}

	// Incremental prune. The subtree below a node is a pure function of the
	// rect the parent assigned to it and of the subtree's own content, so when
	// both the assigned rect and the cached rect agree and nothing inside the
	// subtree is geometrically dirty, re-walking it would reproduce exactly the
	// rects and scroll geometry already stored on those nodes. layoutNode never
	// reads ctx.viewport (only passes ctx down), so the viewport is not a third
	// input to this decision.
	if !n.subtreeGeomDirty && n.Rect == next && n.ContentRect == nextContent {
		// Nothing to do, but childrenLinearY stays valid for the same reason
		// the rects do: child order and child rects are both unchanged. The
		// node is deliberately left unstamped: everything a pruned subtree
		// skipped, including the scroll bookkeeping of this very node, is
		// handled by drainSkippedScroll after the pass.
		return
	}
	n.layoutStamp = ctx.stamp
	n.subtreeGeomDirty = false
	n.Rect = next
	n.ContentRect = nextContent

	if n.Kind == NodeText || n.Kind == NodeRawANSI {
		return
	}

	flow := make([]*Node, 0, len(n.Children))
	absolute := make([]*Node, 0, 4)
	for _, c := range n.Children {
		if c == nil || c.Style.Display == DisplayNone {
			continue
		}
		if c.Style.Position == PositionAbsolute {
			absolute = append(absolute, c)
		} else {
			flow = append(flow, c)
		}
	}

	dir := s.FlexDirection
	if dir == "" {
		dir = Row
	}
	mainAvail, crossAvail := nextContent.Width, nextContent.Height
	if dir == Column || dir == ColumnReverse {
		mainAvail, crossAvail = nextContent.Height, nextContent.Width
	}
	gapMain := max(0, core.GapMain(s, dir))
	gapCross := max(0, core.GapCross(s, dir))

	items := make([]*flexItem, 0, len(flow))
	for _, c := range flow {
		items = append(items, measureItem(c, dir, mainAvail, crossAvail))
	}
	lines := makeFlexLines(items, dir, mainAvail, gapMain, s.FlexWrap)

	// First pass: distribute main sizes and recompute cross sizes under the
	// actual allocated main width. This is necessary for wrapped text.
	for _, line := range lines {
		distributeMain(line, dir, mainAvail, gapMain)
		line.cross = 0
		for _, it := range line.items {
			if dir == Row || dir == RowReverse {
				ms := measureNode(it.node, it.main, crossAvail)
				it.cross = ms.h
			} else {
				ms := measureNode(it.node, crossAvail, it.main)
				it.cross = ms.w
			}
			line.cross = max(line.cross, it.cross+flexCrossMargins(it, dir))
		}
	}
	totalCross := 0
	for i, line := range lines {
		if i > 0 {
			totalCross += gapCross
		}
		totalCross += line.cross
	}

	lineCrossPos := 0
	for _, line := range lines {
		freeMain := mainAvail - line.mainUsed
		start, stepGap := justifyStartGap(s.JustifyContent, freeMain, len(line.items), gapMain)
		mainPos := start

		ordered := line.items
		if dir == RowReverse || dir == ColumnReverse {
			ordered = append([]*flexItem(nil), line.items...)
			for i, j := 0, len(ordered)-1; i < j; i, j = i+1, j-1 {
				ordered[i], ordered[j] = ordered[j], ordered[i]
			}
		}

		for idx, it := range ordered {
			align := it.node.Style.AlignSelf
			if align == "" || align == AlignAuto {
				align = s.AlignItems
			}
			if align == "" {
				align = AlignStretch
			}
			crossSize := it.cross
			crossSpace := line.cross - flexCrossMargins(it, dir)
			if align == AlignStretch && !it.explicitCross {
				crossSize = max(0, crossSpace)
			}
			crossOffset := 0
			switch align {
			case AlignCenter:
				crossOffset = max(0, (crossSpace-crossSize)/2)
			case AlignFlexEnd:
				crossOffset = max(0, crossSpace-crossSize)
			}

			var x, y, cw, ch int
			if dir == Row || dir == RowReverse {
				x = nextContent.X + roundCell(mainPos) + it.margin.Left
				y = nextContent.Y + lineCrossPos + it.margin.Top + crossOffset
				cw, ch = it.main, crossSize
			} else {
				x = nextContent.X + lineCrossPos + it.margin.Left + crossOffset
				y = nextContent.Y + roundCell(mainPos) + it.margin.Top
				cw, ch = crossSize, it.main
			}

			// Relative offsets translate the painted/layout rect but do not
			// consume additional flex space, matching CSS/Yoga.
			if it.node.Style.Position == PositionRelative {
				if v, ok := resolveLength(it.node.Style.Left, nextContent.Width); ok {
					x += v
				} else if v, ok := resolveLength(it.node.Style.Right, nextContent.Width); ok {
					x -= v
				}
				if v, ok := resolveLength(it.node.Style.Top, nextContent.Height); ok {
					y += v
				} else if v, ok := resolveLength(it.node.Style.Bottom, nextContent.Height); ok {
					y -= v
				}
			}
			layoutNode(ctx, it.node, Rect{X: x, Y: y, Width: max(0, cw), Height: max(0, ch)}, true, true)
			mainPos += float64(it.main + flexMainMargins(it, dir))
			if idx < len(ordered)-1 {
				mainPos += stepGap
			}
		}
		lineCrossPos += line.cross + gapCross
	}

	for _, c := range absolute {
		layoutAbsolute(ctx, c, nextContent)
	}

	updateScrollState(n, nextContent)
	n.childrenLinearY = directChildrenLinearY(n)
	_ = totalCross // retained for future align-content parity
}

func directChildrenLinearY(n *Node) bool {
	if n == nil || len(n.Children) < 2 {
		return len(n.Children) > 0
	}
	prevBottom := -1 << 30
	for _, c := range n.Children {
		if c == nil || c.Style.Display == DisplayNone || c.Style.Position == PositionAbsolute {
			return false
		}
		if c.Rect.Y < prevBottom {
			return false
		}
		prevBottom = c.Rect.Y + c.Rect.Height
	}
	return true
}

func layoutAbsolute(ctx *layoutCtx, n *Node, parent Rect) {
	m := measureNode(n, parent.Width, parent.Height)
	w, h := m.w, m.h
	left, hasLeft := resolveLength(n.Style.Left, parent.Width)
	right, hasRight := resolveLength(n.Style.Right, parent.Width)
	top, hasTop := resolveLength(n.Style.Top, parent.Height)
	bottom, hasBottom := resolveLength(n.Style.Bottom, parent.Height)

	if _, hasW := resolveLength(n.Style.Width, parent.Width); !hasW && hasLeft && hasRight {
		w = max(0, parent.Width-left-right)
	}
	if _, hasH := resolveLength(n.Style.Height, parent.Height); !hasH && hasTop && hasBottom {
		h = max(0, parent.Height-top-bottom)
	}
	x, y := parent.X, parent.Y
	if hasLeft {
		x += left
	} else if hasRight {
		x += parent.Width - right - w
	}
	if hasTop {
		y += top
	} else if hasBottom {
		y += parent.Height - bottom - h
	}
	layoutNode(ctx, n, Rect{X: x, Y: y, Width: w, Height: h}, true, true)
}

func drainScrollDelta(n *Node, pending, viewportHeight int) int {
	if n == nil || pending == 0 {
		return 0
	}
	sign := 1
	if pending < 0 {
		sign = -1
	}
	absPending := absInt(pending)
	capStep := max(1, viewportHeight-1)

	// Explicit fixed step is retained as an embedding escape hatch.
	if n.ScrollDrainPerFrame > 0 {
		step := min(absPending, n.ScrollDrainPerFrame, capStep)
		n.PendingScrollDelta = sign * (absPending - step)
		return sign * step
	}

	if n.ScrollAdaptive {
		// xterm.js: small clicks are instant; larger bursts animate in small
		// steps, with excess above 30 rows snapped immediately.
		const instantThreshold = 5
		const highPending = 12
		const stepMedium = 2
		const stepHigh = 3
		const maxPending = 30
		applied := 0
		remaining := absPending
		if remaining > maxPending {
			applied += remaining - maxPending
			remaining = maxPending
		}
		step := remaining
		if remaining > instantThreshold && remaining < highPending {
			step = stepMedium
		} else if remaining >= highPending {
			step = stepHigh
		}
		applied += step
		rem := remaining - step
		if applied > capStep {
			excess := applied - capStep
			n.PendingScrollDelta = sign * (rem + excess)
			return sign * capStep
		}
		n.PendingScrollDelta = sign * rem
		return sign * applied
	}

	// Native terminals: proportional drain catches up quickly while ensuring
	// each step is smaller than the viewport so DECSTBM + SU/SD remains valid.
	step := max(4, (absPending*3)>>2)
	step = min(absPending, step, capStep)
	n.PendingScrollDelta = sign * (absPending - step)
	return sign * step
}

func updateScrollState(n *Node, content Rect) {
	if n.Style.OverflowY != OverflowScroll && n.Style.Overflow != OverflowScroll {
		return
	}
	maxBottom := content.Y
	for _, c := range n.Children {
		if c == nil || c.Style.Display == DisplayNone {
			continue
		}
		maxBottom = max(maxBottom, c.Rect.Y+c.Rect.Height)
	}
	n.ScrollHeight = max(content.Height, maxBottom-content.Y)
	n.ScrollViewportHeight = content.Height
	n.ScrollViewportTop = content.Y
	maxScroll := max(0, n.ScrollHeight-content.Height)

	if n.ScrollAnchor != nil && n.ScrollAnchor.Node != nil {
		target := n.ScrollAnchor.Node
		if target == n || target.IsDescendantOf(n) {
			n.ScrollTop = max(0, target.Rect.Y-content.Y+n.ScrollAnchor.Offset)
		}
		n.ScrollAnchor = nil
	}
	if n.StickyScroll {
		n.ScrollTop = maxScroll
	}
	if n.PendingScrollDelta != 0 {
		step := drainScrollDelta(n, n.PendingScrollDelta, content.Height)
		n.ScrollTop += step
	}

	if n.ScrollClampMin != nil && n.ScrollTop < *n.ScrollClampMin {
		n.ScrollTop = *n.ScrollClampMin
	}
	if n.ScrollClampMax != nil && n.ScrollTop > *n.ScrollClampMax {
		n.ScrollTop = *n.ScrollClampMax
	}
	n.ScrollTop = max(0, min(n.ScrollTop, maxScroll))
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// SortedRects is a small diagnostic helper used by tests and migration tools.
func SortedRects(root *Node) []struct {
	ID   string
	Kind NodeKind
	Rect Rect
} {
	var out []struct {
		ID   string
		Kind NodeKind
		Rect Rect
	}
	if root == nil {
		return out
	}
	root.Walk(func(n *Node) bool {
		out = append(out, struct {
			ID   string
			Kind NodeKind
			Rect Rect
		}{n.ID, n.Kind, n.Rect})
		return true
	})
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Rect.Y != out[j].Rect.Y {
			return out[i].Rect.Y < out[j].Rect.Y
		}
		return out[i].Rect.X < out[j].Rect.X
	})
	return out
}

func hasAlternateScreen(root *Node) bool {
	found := false
	if root == nil {
		return false
	}
	root.Walk(func(n *Node) bool {
		if n.Kind == NodeAlternateScreen {
			found = true
			return false
		}
		return true
	})
	return found
}
