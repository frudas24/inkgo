package inkgo

import (
	"io"
	"strings"
	"sync"
	"time"
)

type RenderOptions struct {
	Width, Height      int
	Fullscreen         bool
	SynchronizedOutput bool
	HideCursor         bool
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
	Screen     *Screen
	Size       Size
	Patch      string
	Damage     Damage
	Fullscreen bool
	Cursor     Cursor
}

type Renderer struct {
	mu         sync.Mutex
	prev       *Screen
	options    RenderOptions
	entered    bool
	lastRoot   *Node
	scrollTops map[string]int
	cursorDecl *CursorDeclaration
	parked     *Cursor
}

func NewRenderer(opts RenderOptions) *Renderer {
	return &Renderer{options: opts, scrollTops: make(map[string]int)}
}

func (r *Renderer) SetSize(width, height int) {
	r.mu.Lock()
	r.options.Width = width
	r.options.Height = height
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
	r.mu.Lock()
	defer r.mu.Unlock()
	refreshExpiredButtonStates(root)
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
	screenH := sz.Height
	if fullscreen {
		screenH = height
	}
	screenH = max(1, screenH)
	next := NewScreen(width, screenH)
	paintTree(next, root)
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
			simulated := r.prev.Clone()
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
	r.prev = next.Clone()
	r.lastRoot = root
	return Frame{Screen: next, Size: sz, Patch: patch, Damage: dmg, Fullscreen: fullscreen, Cursor: cursor}
}

func RenderToScreen(root *Node, width int) (*Screen, int) {
	r := NewRenderer(RenderOptions{Width: width, Height: 1})
	f := r.Render(root)
	return f.Screen, f.Size.Height
}

func (r *Renderer) captureScrollTops(root *Node) {
	if r.scrollTops == nil {
		r.scrollTops = make(map[string]int)
	}
	seen := make(map[string]struct{})
	if root != nil {
		root.Walk(func(n *Node) bool {
			if n.Style.OverflowY == OverflowScroll || n.Style.Overflow == OverflowScroll {
				r.scrollTops[n.ID] = n.ScrollTop
				seen[n.ID] = struct{}{}
			}
			return true
		})
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
	root.Walk(func(n *Node) bool {
		if n.Style.OverflowY != OverflowScroll && n.Style.Overflow != OverflowScroll {
			return true
		}
		old, exists := r.scrollTops[n.ID]
		if !exists || old == n.ScrollTop {
			return true
		}
		d := n.ScrollTop - old
		if d == 0 {
			return true
		}
		vr := visualContentRect(n)
		if vr.X != 0 || vr.Width != screen.Width || vr.Height <= 0 || absInt(d) >= vr.Height {
			return true
		}
		if candidate != nil {
			candidate = nil
			delta = 0
			ok = false
			return false
		}
		candidate = n
		top = max(0, vr.Y)
		bottom = min(screen.Height-1, vr.Y+vr.Height-1)
		delta = d
		ok = top <= bottom
		return true
	})
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
	root.Walk(func(n *Node) bool {
		if n.Kind == NodeButton && n.ButtonState.Active && !n.ActiveUntil.IsZero() && now.After(n.ActiveUntil) {
			next := n.ButtonState
			next.Active = false
			n.ActiveUntil = time.Time{}
			n.setButtonState(next)
		}
		return true
	})
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
	for _, c := range n.Children {
		paintNode(screen, c, childCtx)
	}
	markNoSelect()
}

func paintBox(screen *Screen, n *Node, r, clip Rect) {
	rc := r.Intersect(clip)
	if rc.Empty() {
		return
	}
	if n.Style.BackgroundColor.Kind != ColorUnset || n.Style.Opaque {
		// Fill inside the border (padding included), matching the fork. Opaque
		// uses terminal-default spaces; backgroundColor uses styled spaces.
		b := n.Style.borderEdges()
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
	b := n.Style.borderEdges()
	chars := n.Style.BorderStyle.Chars
	base := TextStyle{Color: n.Style.BorderColor}
	if n.Style.BorderDimColor != nil {
		base.Dim = *n.Style.BorderDimColor
	}
	set := func(x, y int, ch string, st TextStyle) {
		if ch != "" && clip.Contains(Point{x, y}) {
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
		if clip.Contains(Point{x, y}) {
			screen.SetCell(x, y, g.Text, g.Width, style, "")
		}
		x += g.Width
	}
}

func paintText(screen *Screen, n *Node, r, clip Rect, style TextStyle, href string) {
	width := max(0, r.Width)
	if width == 0 {
		return
	}
	if n.Kind == NodeRawANSI {
		paintANSILines(screen, n.Text, r, clip, style, href, n.Style.TextWrap)
		return
	}
	lines, soft := WrapTextLines(n.Text, width, n.Style.TextWrap)
	y := r.Y
	for lineIndex, line := range lines {
		if y >= 0 && y < screen.Height && lineIndex < len(soft) && soft[lineIndex] {
			screen.SoftWrap[y] = true
		}
		x := r.X
		graphemes := Graphemes(line)
		graphemes = ReorderBidiGraphemes(graphemes)
		for _, g := range graphemes {
			if g.Width > 0 && x+g.Width <= r.X+r.Width && clip.Contains(Point{x, y}) {
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

func paintANSILines(screen *Screen, text string, r, clip Rect, base TextStyle, baseHref string, mode TextWrap) {
	// Preserve style/hyperlink while wrapping by cells rather than stripping ANSI.
	parsed := ParseANSI(text, base)
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
			x = r.X
			y++
			if y >= r.Y+r.Height {
				return
			}
			if y >= 0 && y < screen.Height {
				screen.SoftWrap[y] = true
			}
		}
		gh := g.Hyperlink
		if gh == "" {
			gh = baseHref
		}
		if g.Width > 0 && clip.Contains(Point{x, y}) {
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
	return f, err
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
	if r.options.Fullscreen || hasAlternateScreen(root) {
		b.WriteString(ExitAltScreen)
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
