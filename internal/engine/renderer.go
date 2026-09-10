package engine

import (
	core "github.com/frudas24/inkgo/internal/core"
	"io"
	"sort"
	"strings"
	"sync"
	"time"
)

type RenderOptions struct {
	Width, Height      int
	Fullscreen         bool
	SynchronizedOutput bool
	HideCursor         bool

	// Extended-key reporting is auto-detected by default. ForceExtendedKeys
	// is useful for a known compatible pty; DisableExtendedKeys is useful
	// when embedding under an unusual terminal multiplexer. Disable wins.
	ForceExtendedKeys   bool
	DisableExtendedKeys bool

	// BorrowFrameScreen skips the stable Frame.Screen snapshot allocation and
	// returns a renderer-owned double buffer. It is intended for high-frequency
	// embedded loops that consume Frame.Screen before the next renders. The
	// default false preserves stable snapshot semantics for general callers.
	BorrowFrameScreen bool
}

type Cursor struct {
	X, Y    int
	Visible bool
}

type CursorDeclaration struct {
	Node                 *Node
	RelativeX, RelativeY int
}

type Frame struct {
	Screen             *Screen
	Size               Size
	Patch              string
	Damage             Damage
	Fullscreen         bool
	Cursor             Cursor
	SearchMatches      []MatchPosition
	ScrollDrainPending bool
}

type Renderer struct {
	mu              sync.Mutex
	prev            *Screen
	back            *Screen
	scratch         *Screen
	options         RenderOptions
	entered         bool
	lastRoot        *Node
	scrollTops      map[string]int
	cursorDecl      *CursorDeclaration
	parked          *Cursor
	selection       *Selection
	selectionBg     Color
	searchQuery     string
	searchCurrent   int
	searchPositions []MatchPosition
	searchRowOffset int
}

func NewRenderer(opts RenderOptions) *Renderer {
	return &Renderer{options: opts, scrollTops: make(map[string]int)}
}

// Invalidate discards physical-screen assumptions so the next render is a
// clean redraw. Use after resize, terminal resume, tmux attach, or wake.
func (r *Renderer) Invalidate() {
	r.mu.Lock()
	if r.prev != nil && r.back == nil {
		r.back = r.prev
	}
	r.prev = nil
	r.parked = nil
	r.scrollTops = make(map[string]int)
	r.mu.Unlock()
}

func (r *Renderer) SetSize(width, height int) {
	r.mu.Lock()
	r.options.Width = width
	r.options.Height = height
	r.mu.Unlock()
}

// Viewport returns the configured terminal viewport. Zero dimensions mean the
// renderer will use its normal fallback during rendering.
func (r *Renderer) Viewport() Size {
	r.mu.Lock()
	defer r.mu.Unlock()
	return Size{Width: r.options.Width, Height: r.options.Height}
}

// IsVisible reports whether a laid-out node intersects the live viewport. It
// accounts for ScrollBox offsets in the same coordinate space used by paint
// and hit-testing.
func (r *Renderer) IsVisible(node *Node) bool {
	if node == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	w, h := r.options.Width, r.options.Height
	if w <= 0 {
		w = 80
	}
	if h <= 0 {
		h = 24
	}
	clip := Rect{Width: w, Height: h}
	nr := visualNodeRect(node)
	for p := node.Parent; p != nil; p = p.Parent {
		if p.Style.OverflowX == OverflowHidden || p.Style.OverflowX == OverflowScroll ||
			p.Style.OverflowY == OverflowHidden || p.Style.OverflowY == OverflowScroll ||
			p.Style.Overflow == OverflowHidden || p.Style.Overflow == OverflowScroll {
			clip = clip.Intersect(visualContentRect(p))
		}
	}
	visible := nr.Intersect(clip)
	return visible.Width > 0 && visible.Height > 0
}

// SetSelection attaches mutable selection state to the renderer. The overlay is
// applied before damage calculation so selection changes use the same diff path
// as ordinary content changes.
func (r *Renderer) SetSelection(selection *Selection) {
	r.mu.Lock()
	r.selection = selection
	r.mu.Unlock()
}

// SetSelectionBackground configures a solid selection background while
// preserving each cell's foreground. An unset color falls back to inverse.
func (r *Renderer) SetSelectionBackground(color Color) {
	r.mu.Lock()
	r.selectionBg = color
	r.mu.Unlock()
}

// SetSearchHighlight enables case-insensitive scan highlighting for all visible
// matches. current is optional; use -1 when no scan result should receive the
// emphasized current-match style.
func (r *Renderer) SetSearchHighlight(query string, current int) {
	r.mu.Lock()
	r.searchQuery = query
	r.searchCurrent = current
	r.mu.Unlock()
}

// SetSearchPositions installs pre-scanned, element-relative matches. These are
// useful for virtualized lists: positions stay stable while rowOffset changes
// as the item scrolls. Pass nil to clear.
func (r *Renderer) SetSearchPositions(positions []MatchPosition, rowOffset, current int) {
	r.mu.Lock()
	r.searchPositions = append(r.searchPositions[:0], positions...)
	r.searchRowOffset = rowOffset
	r.searchCurrent = current
	r.mu.Unlock()
}

func (r *Renderer) Previous() *Screen { r.mu.Lock(); defer r.mu.Unlock(); return r.prev.Clone() }

// DeclareCursor is the Go equivalent of useDeclaredCursor. The declaration
// is resolved after layout so it follows scrolling and clipping.
func (r *Renderer) DeclareCursor(node *Node, line, column int, active bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !active || node == nil {
		r.cursorDecl = nil
		return
	}
	r.cursorDecl = &CursorDeclaration{Node: node, RelativeX: column, RelativeY: line}
}

func (r *Renderer) Render(root *Node) Frame {
	// The UI owner updates the tree. Button render callbacks may query or
	// resize this renderer, so run them before locking screen history.
	refreshExpiredButtonStates(root)
	r.mu.Lock()
	defer r.mu.Unlock()
	width := max(0, r.options.Width)
	height := max(0, r.options.Height)
	fullscreen := r.options.Fullscreen || hasAlternateScreen(root)
	if width == 0 {
		width = 80
	}
	if height == 0 {
		height = 24
	}
	sz := ComputeLayout(root, Size{Width: width, Height: height}, fullscreen)
	r.reconcileSelectionScroll(root)
	screenH := sz.Height
	if fullscreen {
		screenH = height
	}
	screenH = max(1, screenH)
	next := r.back
	if next == nil {
		next = NewScreen(width, screenH)
	} else {
		next.Reset(width, screenH)
	}
	paintTree(next, root)
	ApplySelectionOverlay(next, r.selection, r.selectionBg)
	var searchMatches []MatchPosition
	if fullscreen && r.searchQuery != "" {
		current := r.searchCurrent
		if len(r.searchPositions) > 0 {
			// Element-position highlighting owns the current-result emphasis.
			current = -1
		}
		searchMatches = ApplySearchHighlight(next, r.searchQuery, current)
	}
	if fullscreen && len(r.searchPositions) > 0 {
		ApplyPositionedHighlight(next, r.searchPositions, r.searchRowOffset, r.searchCurrent)
	}
	cursor := r.resolveDeclaredCursor(next)
	dmg := ScreenDamage(r.prev, next)
	preamble := ""
	if !fullscreen && r.parked != nil && r.parked.Visible {
		dy := (screenH - 1) - r.parked.Y
		if dy > 0 {
			preamble += CursorDown(dy)
		} else if dy < 0 {
			preamble += CursorUp(-dy)
		}
		preamble += CursorTo(0)
	}
	diffPatch := ""
	if fullscreen && r.prev != nil && r.prev.Width == next.Width && r.prev.Height == next.Height {
		if top, bottom, delta, ok := r.hardwareScrollHint(root, next); ok {
			if r.scratch == nil {
				r.scratch = &Screen{}
			}
			copyScreenInto(r.scratch, r.prev)
			simulated := r.scratch
			simulated.ShiftRows(top, bottom, delta)
			diffPatch = SetScrollRegion(top, bottom)
			if delta > 0 {
				diffPatch += CursorPosition(top, 0) + ScrollUp(delta)
			} else {
				diffPatch += CursorPosition(top, 0) + ScrollDown(-delta)
			}
			diffPatch += ResetScrollRegion + DiffScreens(simulated, next)
		}
	}
	if diffPatch == "" {
		if fullscreen {
			diffPatch = DiffScreens(r.prev, next)
		} else {
			diffPatch = DiffScreensRelative(r.prev, next)
		}
	}
	patch := preamble + diffPatch
	r.captureScrollTops(root)
	if cursor.Visible {
		if fullscreen {
			patch += CursorPosition(cursor.Y, cursor.X)
		} else {
			dy := (screenH - 1) - cursor.Y
			if dy > 0 {
				patch += CursorUp(dy)
			} else if dy < 0 {
				patch += CursorDown(-dy)
			}
			patch += CursorTo(cursor.X)
		}
		patch += ShowCursor
		copyCursor := cursor
		r.parked = &copyCursor
	} else {
		if r.parked != nil && r.parked.Visible {
			patch += HideCursor
		}
		r.parked = nil
	}
	if r.options.SynchronizedOutput && patch != "" {
		patch = BeginSyncUpdate + patch + EndSyncUpdate
	}
	frameScreen := next
	if !r.options.BorrowFrameScreen {
		frameScreen = next.Clone()
	}
	oldPrev := r.prev
	r.prev = next
	r.back = oldPrev
	r.lastRoot = root
	drainPending := hasPendingScroll(root)
	if root != nil {
		root.clearDirtyRecursive()
	}
	return Frame{Screen: frameScreen, Size: sz, Patch: patch, Damage: dmg, Fullscreen: fullscreen, Cursor: cursor, SearchMatches: searchMatches, ScrollDrainPending: drainPending}
}

func RenderToScreen(root *Node, width int) (*Screen, int) {
	r := NewRenderer(RenderOptions{Width: width, Height: 1})
	f := r.Render(root)
	return f.Screen, f.Size.Height
}

func hasPendingScroll(root *Node) bool {
	for _, n := range layoutScrollNodes(root) {
		if n.PendingScrollDelta != 0 {
			return true
		}
	}
	return false
}

// reconcileSelectionScroll keeps selection screen coordinates attached to
// content when a ScrollBox moves. It also captures selected rows from the
// previous screen before they leave the viewport, matching the fork's
// drag/keyboard-scroll preservation behavior without coupling Node to Runtime.
func (r *Renderer) reconcileSelectionScroll(root *Node) {
	if r.selection == nil || !r.selection.HasSelection() || r.prev == nil || root == nil {
		return
	}
	for _, n := range layoutScrollNodes(root) {
		old, exists := r.scrollTops[n.ID]
		if !exists || old == n.ScrollTop {
			continue
		}
		delta := n.ScrollTop - old
		vr := visualContentRect(n)
		if vr.Height <= 0 {
			continue
		}
		start, end, ok := r.selection.Bounds()
		if !ok || end.Y < vr.Y || start.Y >= vr.Y+vr.Height {
			continue
		}
		top := max(0, vr.Y)
		bottom := min(r.prev.Height-1, vr.Y+vr.Height-1)
		if top > bottom {
			continue
		}
		if delta > 0 {
			leaving := min(delta, bottom-top+1)
			r.selection.CaptureScrolledRows(r.prev, top, top+leaving-1, true)
		} else {
			leaving := min(-delta, bottom-top+1)
			r.selection.CaptureScrolledRows(r.prev, bottom-leaving+1, bottom, false)
		}
		if r.selection.Dragging {
			r.selection.ShiftAnchor(-delta, top, bottom)
		} else if n.StickyScroll {
			r.selection.ShiftForFollow(-delta, top, bottom)
		} else {
			r.selection.Shift(-delta, top, bottom, r.prev.Width)
		}
	}
}

func (r *Renderer) captureScrollTops(root *Node) {
	if r.scrollTops == nil {
		r.scrollTops = make(map[string]int)
	}
	seen := make(map[string]struct{}, len(layoutScrollNodes(root)))
	for _, n := range layoutScrollNodes(root) {
		r.scrollTops[n.ID] = n.ScrollTop
		seen[n.ID] = struct{}{}
	}
	for id := range r.scrollTops {
		if _, ok := seen[id]; !ok {
			delete(r.scrollTops, id)
		}
	}
}

// hardwareScrollHint mirrors the fork's DECSTBM + SU/SD fast path. It is
// deliberately limited to a full-width scroll viewport because DECSTBM is
// row-scoped, not rectangular.
func (r *Renderer) hardwareScrollHint(root *Node, screen *Screen) (top, bottom, delta int, ok bool) {
	var candidate *Node
	if root == nil {
		return
	}
	for _, n := range layoutScrollNodes(root) {
		old, exists := r.scrollTops[n.ID]
		if !exists || old == n.ScrollTop {
			continue
		}
		d := n.ScrollTop - old
		if d == 0 {
			continue
		}
		vr := visualContentRect(n)
		if vr.X != 0 || vr.Width != screen.Width || vr.Height <= 0 || absInt(d) >= vr.Height {
			continue
		}
		if candidate != nil {
			return 0, 0, 0, false
		}
		candidate = n
		top = max(0, vr.Y)
		bottom = min(screen.Height-1, vr.Y+vr.Height-1)
		delta = d
		ok = top <= bottom
	}
	return
}

func visualContentRect(n *Node) Rect {
	if n == nil {
		return Rect{}
	}
	r := n.ContentRect
	for p := n.Parent; p != nil; p = p.Parent {
		if p.Style.OverflowY == OverflowScroll || p.Style.Overflow == OverflowScroll {
			r.Y -= p.ScrollTop
		}
	}
	return r
}
func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func (r *Renderer) resolveDeclaredCursor(screen *Screen) Cursor {
	if r.cursorDecl == nil || r.cursorDecl.Node == nil || screen == nil {
		return Cursor{}
	}
	n := r.cursorDecl.Node
	vr := n.Rect
	clip := Rect{Width: screen.Width, Height: screen.Height}
	for p := n.Parent; p != nil; p = p.Parent {
		if p.Style.OverflowY == OverflowScroll || p.Style.Overflow == OverflowScroll {
			vr.Y -= p.ScrollTop
		}
		if p.Style.OverflowX == OverflowHidden || p.Style.OverflowX == OverflowScroll || p.Style.OverflowY == OverflowHidden || p.Style.OverflowY == OverflowScroll || p.Style.Overflow == OverflowHidden || p.Style.Overflow == OverflowScroll {
			clip = clip.Intersect(visualContentRect(p))
		}
	}
	p := Point{X: vr.X + r.cursorDecl.RelativeX, Y: vr.Y + r.cursorDecl.RelativeY}
	return Cursor{X: p.X, Y: p.Y, Visible: clip.Contains(p) && p.X >= 0 && p.Y >= 0 && p.X < screen.Width && p.Y < screen.Height}
}

func refreshExpiredButtonStates(root *Node) {
	if root == nil {
		return
	}
	now := time.Now()
	for _, n := range layoutButtonNodes(root) {
		if n.ButtonState.Active && !n.ActiveUntil.IsZero() && now.After(n.ActiveUntil) {
			next := n.ButtonState
			next.Active = false
			n.ActiveUntil = time.Time{}
			n.setButtonState(next)
		}
	}
}

type paintContext struct {
	dx, dy    int
	clip      Rect
	textStyle TextStyle
	hyperlink string
}

func paintTree(screen *Screen, root *Node) {
	if screen == nil || root == nil {
		return
	}
	ctx := paintContext{clip: Rect{Width: screen.Width, Height: screen.Height}}
	paintNode(screen, root, ctx)
}

func shiftedRect(r Rect, dx, dy int) Rect { r.X += dx; r.Y += dy; return r }

func paintNode(screen *Screen, n *Node, ctx paintContext) {
	if n == nil || n.Style.Display == DisplayNone {
		return
	}
	r := shiftedRect(n.Rect, ctx.dx, ctx.dy)
	if r.Intersect(ctx.clip).Empty() && n.Kind != NodeRoot {
		return
	}
	style := mergeStyle(ctx.textStyle, n.TextStyle)
	// Box background is inherited by descendant text exactly like the TS
	// renderer; otherwise text writes would punch holes in a filled box.
	if n.Kind != NodeText && n.Kind != NodeRawANSI && n.Style.BackgroundColor.Kind != ColorUnset {
		style.BackgroundColor = n.Style.BackgroundColor
	}
	href := ctx.hyperlink
	if n.Kind == NodeLink && n.URL != "" {
		href = n.URL
	}

	if n.Kind != NodeText && n.Kind != NodeRawANSI {
		paintBox(screen, n, r, ctx.clip)
	}
	markNoSelect := func() {
		if n.Style.NoSelect == Selectable {
			return
		}
		nr := r.Intersect(ctx.clip)
		if n.Style.NoSelect == NoSelectFromLeftEdge {
			nr.Width = nr.X + nr.Width
			nr.X = 0
		}
		screen.MarkNoSelect(nr)
	}

	if n.Kind == NodeText || n.Kind == NodeRawANSI {
		paintText(screen, n, r, ctx.clip, style, href)
		markNoSelect()
		return
	}

	childCtx := ctx
	childCtx.textStyle = style
	childCtx.hyperlink = href
	childClip := ctx.clip
	overX := n.Style.OverflowX == OverflowHidden || n.Style.OverflowX == OverflowScroll || n.Style.Overflow == OverflowHidden || n.Style.Overflow == OverflowScroll
	overY := n.Style.OverflowY == OverflowHidden || n.Style.OverflowY == OverflowScroll || n.Style.Overflow == OverflowHidden || n.Style.Overflow == OverflowScroll
	cr := shiftedRect(n.ContentRect, ctx.dx, ctx.dy)
	if overX || overY {
		clip := ctx.clip
		if overX {
			clip.X = max(clip.X, cr.X)
			x2 := min(ctx.clip.X+ctx.clip.Width, cr.X+cr.Width)
			clip.Width = max(0, x2-clip.X)
		}
		if overY {
			clip.Y = max(clip.Y, cr.Y)
			y2 := min(ctx.clip.Y+ctx.clip.Height, cr.Y+cr.Height)
			clip.Height = max(0, y2-clip.Y)
		}
		childClip = clip
	}
	childCtx.clip = childClip
	if n.ScrollTop != 0 && (n.Style.OverflowY == OverflowScroll || n.Style.Overflow == OverflowScroll) {
		childCtx.dy -= n.ScrollTop
	}
	paintChildren(screen, n, childCtx)
	markNoSelect()
}

func paintChildren(screen *Screen, n *Node, ctx paintContext) {
	children := n.Children
	if len(children) == 0 {
		return
	}
	// Large vertical lists are the common ScrollBox workload. The layout pass
	// certifies non-overlap, which makes these predicates monotonic and safe for
	// binary search even when the viewport is translated by ScrollTop.
	if n.childrenLinearY && len(children) >= 64 && ctx.clip.Height > 0 {
		top := ctx.clip.Y
		bottom := ctx.clip.Y + ctx.clip.Height
		first := sort.Search(len(children), func(i int) bool {
			c := children[i]
			return c.Rect.Y+ctx.dy+c.Rect.Height > top
		})
		for i := first; i < len(children); i++ {
			c := children[i]
			if c.Rect.Y+ctx.dy >= bottom {
				break
			}
			paintNode(screen, c, ctx)
		}
		return
	}
	for _, c := range children {
		paintNode(screen, c, ctx)
	}
}

func paintBox(screen *Screen, n *Node, r, clip Rect) {
	rc := r.Intersect(clip)
	if rc.Empty() {
		return
	}
	if n.Style.BackgroundColor.Kind != ColorUnset || n.Style.Opaque {
		// Fill inside the border (padding included), matching the fork. Opaque
		// uses terminal-default spaces; backgroundColor uses styled spaces.
		b := core.BorderEdges(n.Style)
		inner := Rect{X: r.X + b.Left, Y: r.Y + b.Top, Width: max(0, r.Width-b.Left-b.Right), Height: max(0, r.Height-b.Top-b.Bottom)}.Intersect(clip)
		st := TextStyle{}
		if n.Style.BackgroundColor.Kind != ColorUnset {
			st.BackgroundColor = n.Style.BackgroundColor
		}
		if !inner.Empty() {
			screen.Fill(inner, " ", st)
		}
	}
	if n.Style.BorderStyle == nil {
		return
	}
	b := core.BorderEdges(n.Style)
	chars := n.Style.BorderStyle.Chars
	base := TextStyle{Color: n.Style.BorderColor}
	if n.Style.BorderDimColor != nil {
		base.Dim = *n.Style.BorderDimColor
	}
	set := func(x, y int, ch string, st TextStyle) {
		if ch != "" && clip.Contains(Point{X: x, Y: y}) {
			screen.SetCell(x, y, ch, max(1, StringWidth(ch)), st, "")
		}
	}
	if b.Top > 0 && r.Height > 0 {
		for x := r.X + 1; x < r.X+r.Width-1; x++ {
			set(x, r.Y, chars.Top, sideBorderStyle(n, "top", base))
		}
	}
	if b.Bottom > 0 && r.Height > 1 {
		for x := r.X + 1; x < r.X+r.Width-1; x++ {
			set(x, r.Y+r.Height-1, chars.Bottom, sideBorderStyle(n, "bottom", base))
		}
	}
	if b.Left > 0 && r.Width > 0 {
		for y := r.Y + 1; y < r.Y+r.Height-1; y++ {
			set(r.X, y, chars.Left, sideBorderStyle(n, "left", base))
		}
	}
	if b.Right > 0 && r.Width > 1 {
		for y := r.Y + 1; y < r.Y+r.Height-1; y++ {
			set(r.X+r.Width-1, y, chars.Right, sideBorderStyle(n, "right", base))
		}
	}
	if b.Top > 0 && b.Left > 0 {
		set(r.X, r.Y, chars.TopLeft, base)
	}
	if b.Top > 0 && b.Right > 0 {
		set(r.X+r.Width-1, r.Y, chars.TopRight, base)
	}
	if b.Bottom > 0 && b.Left > 0 {
		set(r.X, r.Y+r.Height-1, chars.BottomLeft, base)
	}
	if b.Bottom > 0 && b.Right > 0 {
		set(r.X+r.Width-1, r.Y+r.Height-1, chars.BottomRight, base)
	}
	paintBorderText(screen, n, r, clip, base)
}

func sideBorderStyle(n *Node, side string, base TextStyle) TextStyle {
	s := base
	var c Color
	var dim *bool
	switch side {
	case "top":
		c, dim = n.Style.BorderTopColor, n.Style.BorderTopDimColor
	case "bottom":
		c, dim = n.Style.BorderBottomColor, n.Style.BorderBottomDimColor
	case "left":
		c, dim = n.Style.BorderLeftColor, n.Style.BorderLeftDimColor
	case "right":
		c, dim = n.Style.BorderRightColor, n.Style.BorderRightDimColor
	}
	if c.Kind != ColorUnset {
		s.Color = c
	}
	if dim != nil {
		s.Dim = *dim
	}
	return s
}

func paintBorderText(screen *Screen, n *Node, r, clip Rect, style TextStyle) {
	bt := n.Style.BorderText
	if bt == nil || bt.Content == "" || r.Width < 3 {
		return
	}
	y := r.Y
	if bt.Position == "bottom" {
		y = r.Y + r.Height - 1
	}
	maxW := max(0, r.Width-2)
	txt := truncateText(bt.Content, maxW, TextWrapTruncateEnd)
	tw := StringWidth(txt)
	x := r.X + 1 + bt.Offset
	switch bt.Align {
	case "center":
		x = r.X + (r.Width-tw)/2 + bt.Offset
	case "end":
		x = r.X + r.Width - 1 - tw - bt.Offset
	}
	for _, g := range Graphemes(txt) {
		if clip.Contains(Point{X: x, Y: y}) {
			screen.SetCell(x, y, g.Text, g.Width, style, "")
		}
		x += g.Width
	}
}

func cachedWrappedText(n *Node, width int) ([]string, []bool, [][]Grapheme) {
	if n == nil {
		return nil, nil, nil
	}
	mode := n.Style.TextWrap
	if c := &n.wrapCache; c.valid && c.text == n.Text && c.width == width && c.mode == mode {
		return c.lines, c.soft, c.graphemes
	}
	lines, soft := WrapTextLines(n.Text, width, mode)
	clusters := make([][]Grapheme, len(lines))
	for i, line := range lines {
		clusters[i] = Graphemes(line)
	}
	n.wrapCache = nodeWrapCache{valid: true, text: n.Text, width: width, mode: mode, lines: lines, soft: soft, graphemes: clusters}
	return lines, soft, clusters
}

func cachedParsedANSI(n *Node, base TextStyle) []StyledGrapheme {
	if n == nil {
		return nil
	}
	if c := &n.ansiCache; c.valid && c.text == n.Text && c.base == base {
		return c.items
	}
	items := ParseANSI(n.Text, base)
	n.ansiCache = nodeANSICache{valid: true, text: n.Text, base: base, items: items}
	return items
}

func paintText(screen *Screen, n *Node, r, clip Rect, style TextStyle, href string) {
	width := max(0, r.Width)
	if width == 0 {
		return
	}
	if n.Kind == NodeRawANSI {
		paintANSILines(screen, n, r, clip, style, href, n.Style.TextWrap)
		return
	}
	lines, soft, clusters := cachedWrappedText(n, width)
	y := r.Y
	for lineIndex := range lines {
		if y >= 0 && y < screen.Height && lineIndex < len(soft) && soft[lineIndex] {
			screen.SoftWrap[y] = true
			if lineIndex > 0 && y < len(screen.SoftWrapEnd) {
				screen.SoftWrapEnd[y] = min(screen.Width, r.X+StringWidth(lines[lineIndex-1]))
			}
		}
		x := r.X
		graphemes := clusters[lineIndex]
		graphemes = ReorderBidiGraphemes(graphemes)
		for _, g := range graphemes {
			if g.Width > 0 && x+g.Width <= r.X+r.Width && clip.Contains(Point{X: x, Y: y}) {
				screen.SetCell(x, y, g.Text, g.Width, style, href)
			}
			x += g.Width
		}
		y++
		if y >= r.Y+r.Height {
			return
		}
	}
}

func paintANSILines(screen *Screen, n *Node, r, clip Rect, base TextStyle, baseHref string, mode TextWrap) {
	// Preserve style/hyperlink while wrapping by cells rather than stripping ANSI.
	parsed := cachedParsedANSI(n, base)
	parsed = ReorderBidiStyled(parsed)
	x, y := r.X, r.Y
	href := baseHref
	for _, g := range parsed {
		if g.Value == "\n" {
			x = r.X
			y++
			if y >= r.Y+r.Height {
				return
			}
			continue
		}
		if href == "" {
			href = g.Hyperlink
		}
		if x+g.Width > r.X+r.Width {
			if mode == TextWrapTruncate || mode == TextWrapTruncateEnd || mode == TextWrapTruncateMiddle || mode == TextWrapTruncateStart {
				return
			}
			previousEnd := x
			x = r.X
			y++
			if y >= r.Y+r.Height {
				return
			}
			if y >= 0 && y < screen.Height {
				screen.SoftWrap[y] = true
				if y < len(screen.SoftWrapEnd) {
					screen.SoftWrapEnd[y] = min(screen.Width, previousEnd)
				}
			}
		}
		gh := g.Hyperlink
		if gh == "" {
			gh = baseHref
		}
		if g.Width > 0 && clip.Contains(Point{X: x, Y: y}) {
			screen.SetCell(x, y, g.Value, g.Width, g.Style, gh)
		}
		x += g.Width
	}
}

func (r *Renderer) WriteFrame(w io.Writer, root *Node) (Frame, error) {
	f := r.Render(root)
	if f.Patch == "" {
		return f, nil
	}
	_, err := io.WriteString(w, f.Patch)
	if err != nil {
		r.Invalidate()
	}
	return f, err
}

func (r *Renderer) extendedKeysEnabled() bool {
	if r.options.DisableExtendedKeys {
		return false
	}
	return r.options.ForceExtendedKeys || SupportsExtendedKeys()
}

func (r *Renderer) EnterSequence(root *Node) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.entered {
		return ""
	}
	r.entered = true
	var b strings.Builder
	if r.options.Fullscreen || hasAlternateScreen(root) {
		b.WriteString(EnterAltScreen)
		b.WriteString(CursorHome)
		b.WriteString(EraseScreen)
	}
	if r.extendedKeysEnabled() {
		b.WriteString(EnableKittyKeyboard)
		b.WriteString(EnableModifyOtherKeys)
	}
	b.WriteString(EnableBracketPaste)
	b.WriteString(EnableFocusEvents)
	if hasMouseTracking(root) {
		b.WriteString(EnableMouseTracking)
	}
	if r.options.HideCursor {
		b.WriteString(HideCursor)
	}
	return b.String()
}

func (r *Renderer) ExitSequence(root *Node) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.entered {
		return ""
	}
	r.entered = false
	var b strings.Builder
	if r.options.HideCursor {
		b.WriteString(ShowCursor)
	}
	if hasMouseTracking(root) {
		b.WriteString(DisableMouseTracking)
	}
	b.WriteString(DisableFocusEvents)
	b.WriteString(DisableBracketPaste)
	if r.extendedKeysEnabled() {
		b.WriteString(DisableModifyOtherKeys)
		b.WriteString(DisableKittyKeyboard)
	}
	if r.options.Fullscreen || hasAlternateScreen(root) {
		b.WriteString(ExitAltScreen)
	}
	return b.String()
}

// ReassertSequence restores terminal modes after tmux detach/attach, SSH
// reconnect, SIGCONT, or a long stdin gap. Kitty's protocol is stack-based, so
// pop-before-push prevents an idle session from accumulating unmatched pushes.
// includeAltScreen is intentionally opt-in because re-entering mode 1049 clears
// the fullscreen surface.
func (r *Renderer) ReassertSequence(root *Node, includeAltScreen bool) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.entered {
		return ""
	}
	var b strings.Builder
	if r.extendedKeysEnabled() {
		b.WriteString(DisableKittyKeyboard)
		b.WriteString(EnableKittyKeyboard)
		b.WriteString(EnableModifyOtherKeys)
	}
	alt := r.options.Fullscreen || hasAlternateScreen(root)
	if alt && hasMouseTracking(root) {
		b.WriteString(EnableMouseTracking)
	}
	if includeAltScreen && alt {
		b.WriteString(EnterAltScreen)
		b.WriteString(EraseScreen)
		b.WriteString(CursorHome)
		if hasMouseTracking(root) {
			b.WriteString(EnableMouseTracking)
		}
		r.prev = nil
		r.parked = nil
		r.scrollTops = make(map[string]int)
	}
	return b.String()
}

func hasMouseTracking(root *Node) bool {
	found := false
	if root != nil {
		root.Walk(func(n *Node) bool {
			if n.MouseTracking {
				found = true
				return false
			}
			return true
		})
	}
	return found
}
