package inkgo

import (
	core "github.com/frudas24/inkgo/internal/core"
	"math"
	"sort"
	"strings"
)

type layoutCtx struct {
	viewport Size
}

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

func nodeText(n *Node) string {
	if n.Kind == NodeRawANSI {
		return StripANSI(n.Text)
	}
	return n.Text
}

func measureNode(n *Node, availW, availH int) measured {
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
		return measured{w, h}
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
	gap := core.GapMain(s, dir)
	contentW, contentH := 0, 0
	if len(flow) > 0 {
		switch dir {
		case Row, RowReverse:
			for i, c := range flow {
				cm := measureNode(c, innerAvailW, innerAvailH)
				m := core.MarginEdges(c.Style)
				if i > 0 {
					contentW += gap
				}
				contentW += cm.w + m.Left + m.Right
				contentH = max(contentH, cm.h+m.Top+m.Bottom)
			}
		default:
			for i, c := range flow {
				cm := measureNode(c, innerAvailW, innerAvailH)
				m := core.MarginEdges(c.Style)
				if i > 0 {
					contentH += gap
				}
				contentH += cm.h + m.Top + m.Bottom
				contentW = max(contentW, cm.w+m.Left+m.Right)
			}
		}
	}

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
	return measured{w, h}
}

// ComputeLayout calculates node rectangles. In normal mode the root has a
// terminal-width constraint and natural height, matching Ink scrollback mode.
// In fullscreen mode root height is constrained to viewport.Height.
func ComputeLayout(root *Node, viewport Size, fullscreen bool) Size {
	if root == nil {
		return Size{}
	}
	ctx := layoutCtx{viewport: viewport}
	h := viewport.Height
	if !fullscreen {
		m := measureNode(root, viewport.Width, -1)
		h = max(0, m.h)
	}
	layoutNode(&ctx, root, Rect{X: 0, Y: 0, Width: max(0, viewport.Width), Height: max(0, h)}, true, true)
	return Size{Width: root.Rect.Width, Height: root.Rect.Height}
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
	} else {
		it.baseMain = ms.h
		if v, ok := resolveLength(n.Style.FlexBasis, mainAvail); ok {
			it.baseMain = v
		}
		it.cross = ms.w
		_, it.explicitCross = resolveLength(n.Style.Width, crossAvail)
	}
	it.main = max(0, it.baseMain)
	return it
}

func makeFlexLines(items []*flexItem, mainAvail, gap int, wrap FlexWrap) []*flexLine {
	if len(items) == 0 {
		return nil
	}
	if wrap == "" || wrap == NoWrap || mainAvail <= 0 {
		line := &flexLine{items: append([]*flexItem(nil), items...)}
		for i, it := range line.items {
			if i > 0 {
				line.mainUsed += gap
			}
			line.mainUsed += it.main + mainMargins(it, false)
		}
		return []*flexLine{line}
	}
	var lines []*flexLine
	cur := &flexLine{}
	for _, it := range items {
		need := it.main + mainMargins(it, false)
		if len(cur.items) > 0 {
			need += gap
		}
		if len(cur.items) > 0 && cur.mainUsed+need > mainAvail {
			lines = append(lines, cur)
			cur = &flexLine{}
			need = it.main + mainMargins(it, false)
		}
		if len(cur.items) > 0 {
			cur.mainUsed += gap
		}
		cur.items = append(cur.items, it)
		cur.mainUsed += it.main + mainMargins(it, false)
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
		remain := free
		for i, it := range line.items {
			add := 0
			if i == len(line.items)-1 {
				add = remain
			} else if it.grow > 0 {
				add = int(math.Floor(float64(free) * it.grow / totalGrow))
				remain -= add
			}
			it.main += add
		}
	} else if free < 0 && totalShrinkWeight > 0 {
		need := -free
		remain := need
		for i, it := range line.items {
			weight := maxFloat(0, it.shrink) * float64(max(1, it.baseMain))
			cut := 0
			if i == len(line.items)-1 {
				cut = min(remain, it.main)
			} else if weight > 0 {
				cut = min(it.main, int(math.Floor(float64(need)*weight/totalShrinkWeight)))
				remain -= cut
			}
			it.main -= cut
		}
	}
	line.mainUsed = 0
	for i, it := range line.items {
		if i > 0 {
			line.mainUsed += gap
		}
		line.mainUsed += it.main + flexMainMargins(it, dir)
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

func layoutNode(ctx *layoutCtx, n *Node, assigned Rect, forceW, forceH bool) {
	if n == nil {
		return
	}
	s := n.Style
	core.ApplyDefaults(&s)
	n.Style = s
	if s.Display == DisplayNone {
		n.Rect = Rect{}
		n.ContentRect = Rect{}
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

	n.Rect = Rect{X: assigned.X, Y: assigned.Y, Width: max(0, w), Height: max(0, h)}
	border := core.BorderEdges(s)
	pad := core.PaddingEdges(s)
	insets := AddEdges(border, pad)
	content := Rect{
		X:      n.Rect.X + insets.Left,
		Y:      n.Rect.Y + insets.Top,
		Width:  max(0, n.Rect.Width-insets.Left-insets.Right),
		Height: max(0, n.Rect.Height-insets.Top-insets.Bottom),
	}
	n.ContentRect = content

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
	mainAvail, crossAvail := content.Width, content.Height
	if dir == Column || dir == ColumnReverse {
		mainAvail, crossAvail = content.Height, content.Width
	}
	gapMain := max(0, core.GapMain(s, dir))
	gapCross := max(0, core.GapCross(s, dir))

	items := make([]*flexItem, 0, len(flow))
	for _, c := range flow {
		items = append(items, measureItem(c, dir, mainAvail, crossAvail))
	}
	lines := makeFlexLines(items, mainAvail, gapMain, s.FlexWrap)

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
				x = content.X + roundCell(mainPos) + it.margin.Left
				y = content.Y + lineCrossPos + it.margin.Top + crossOffset
				cw, ch = it.main, crossSize
			} else {
				x = content.X + lineCrossPos + it.margin.Left + crossOffset
				y = content.Y + roundCell(mainPos) + it.margin.Top
				cw, ch = crossSize, it.main
			}

			// Relative offsets translate the painted/layout rect but do not
			// consume additional flex space, matching CSS/Yoga.
			if it.node.Style.Position == PositionRelative {
				if v, ok := resolveLength(it.node.Style.Left, content.Width); ok {
					x += v
				} else if v, ok := resolveLength(it.node.Style.Right, content.Width); ok {
					x -= v
				}
				if v, ok := resolveLength(it.node.Style.Top, content.Height); ok {
					y += v
				} else if v, ok := resolveLength(it.node.Style.Bottom, content.Height); ok {
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
		layoutAbsolute(ctx, c, content)
	}

	updateScrollState(n, content)
	_ = totalCross // retained for future align-content parity
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

func intrinsicPlainText(n *Node) string {
	var b strings.Builder
	if n == nil {
		return ""
	}
	n.Walk(func(cur *Node) bool {
		if cur.Kind == NodeText || cur.Kind == NodeRawANSI {
			b.WriteString(nodeText(cur))
		}
		return true
	})
	return b.String()
}
