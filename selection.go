package inkgo

import (
	"strings"
	"unicode"
)

// SelectionMode controls how drag extension snaps to screen content.
type SelectionMode uint8

const (
	SelectionChar SelectionMode = iota
	SelectionWord
	SelectionLine
)

type SelectionSpan struct {
	Lo, Hi Point
	Mode   SelectionMode
}

// Selection is the persistent fullscreen selection state. The Anchor/Focus
// fields are intentionally public so applications can inspect or construct a
// simple selection directly. Runtime-managed selections also use FocusSet to
// distinguish a bare click from a one-cell drag selection.
type Selection struct {
	Anchor, Focus Point
	FocusSet      bool
	Dragging      bool
	Span          *SelectionSpan

	ScrolledOffAbove   []string
	ScrolledOffBelow   []string
	ScrolledOffAboveSW []bool
	ScrolledOffBelowSW []bool

	VirtualAnchorRow *int
	VirtualFocusRow  *int
	LastPressHadAlt  bool
}

func (s *Selection) Start(col, row int) {
	if s == nil {
		return
	}
	s.Anchor = Point{X: col, Y: row}
	s.Focus = Point{}
	s.FocusSet = false
	s.Dragging = true
	s.Span = nil
	s.ScrolledOffAbove = nil
	s.ScrolledOffBelow = nil
	s.ScrolledOffAboveSW = nil
	s.ScrolledOffBelowSW = nil
	s.VirtualAnchorRow = nil
	s.VirtualFocusRow = nil
	s.LastPressHadAlt = false
}

func (s *Selection) Update(col, row int) {
	if s == nil || !s.Dragging {
		return
	}
	if !s.FocusSet && s.Anchor.X == col && s.Anchor.Y == row {
		return
	}
	s.Focus = Point{X: col, Y: row}
	s.FocusSet = true
}

func (s *Selection) Finish() {
	if s != nil {
		s.Dragging = false
	}
}

func (s *Selection) Clear() {
	if s == nil {
		return
	}
	*s = Selection{}
}

func (s *Selection) HasSelection() bool {
	return s != nil && s.FocusSet
}

func compareSelectionPoints(a, b Point) int {
	if a.Y < b.Y {
		return -1
	}
	if a.Y > b.Y {
		return 1
	}
	if a.X < b.X {
		return -1
	}
	if a.X > b.X {
		return 1
	}
	return 0
}

// normalized ignores FocusSet for backwards compatibility with callers that
// construct Selection{Anchor:..., Focus:...} directly.
func (s Selection) normalized() (Point, Point) {
	a, b := s.Anchor, s.Focus
	if compareSelectionPoints(a, b) > 0 {
		a, b = b, a
	}
	return a, b
}

func (s *Selection) Bounds() (Point, Point, bool) {
	if s == nil || !s.FocusSet {
		return Point{}, Point{}, false
	}
	a, b := s.Anchor, s.Focus
	if compareSelectionPoints(a, b) > 0 {
		a, b = b, a
	}
	return a, b, true
}

func (s Selection) RectForRow(row, width int) (int, int, bool) {
	a, b := s.normalized()
	if row < a.Y || row > b.Y {
		return 0, 0, false
	}
	start, end := 0, width
	if row == a.Y {
		start = a.X
	}
	if row == b.Y {
		end = b.X + 1
	}
	return max(0, start), min(width, end), true
}

func cellTextClass(c Cell) int {
	ch := c.Char
	if ch == "" || strings.TrimSpace(ch) == "" {
		return 0
	}
	rs := []rune(ch)
	if len(rs) == 0 {
		return 0
	}
	r := rs[0]
	if unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("_/.-+~\\", r) {
		return 1
	}
	return 2
}

func wordBoundsAt(screen *Screen, col, row int) (int, int, bool) {
	if screen == nil || !screen.InBounds(col, row) {
		return 0, 0, false
	}
	c := col
	if cell, ok := screen.CellAt(c, row); ok && cell.Width == CellSpacerTail && c > 0 {
		c--
	}
	cell, ok := screen.CellAt(c, row)
	if !ok || cell.NoSelect {
		return 0, 0, false
	}
	class := cellTextClass(cell)
	lo := c
	for lo > 0 {
		p, _ := screen.CellAt(lo-1, row)
		if p.NoSelect {
			break
		}
		if p.Width == CellSpacerTail {
			if lo-2 < 0 {
				break
			}
			h, _ := screen.CellAt(lo-2, row)
			if h.NoSelect || cellTextClass(h) != class {
				break
			}
			lo -= 2
			continue
		}
		if cellTextClass(p) != class {
			break
		}
		lo--
	}
	hi := c
	for hi < screen.Width-1 {
		n, _ := screen.CellAt(hi+1, row)
		if n.NoSelect {
			break
		}
		if n.Width == CellSpacerTail {
			hi++
			continue
		}
		if cellTextClass(n) != class {
			break
		}
		hi++
	}
	return lo, hi, true
}

func (s *Selection) SelectWordAt(screen *Screen, col, row int) bool {
	lo, hi, ok := wordBoundsAt(screen, col, row)
	if !ok {
		return false
	}
	s.Anchor = Point{X: lo, Y: row}
	s.Focus = Point{X: hi, Y: row}
	s.FocusSet = true
	s.Dragging = true
	s.Span = &SelectionSpan{Lo: s.Anchor, Hi: s.Focus, Mode: SelectionWord}
	return true
}

func (s *Selection) SelectLineAt(screen *Screen, row int) bool {
	if s == nil || screen == nil || row < 0 || row >= screen.Height {
		return false
	}
	lo := Point{X: 0, Y: row}
	hi := Point{X: max(0, screen.Width-1), Y: row}
	s.Anchor, s.Focus, s.FocusSet, s.Dragging = lo, hi, true, true
	s.Span = &SelectionSpan{Lo: lo, Hi: hi, Mode: SelectionLine}
	return true
}

func (s *Selection) Extend(screen *Screen, col, row int) {
	if s == nil || !s.Dragging {
		return
	}
	if s.Span == nil {
		s.Update(col, row)
		return
	}
	span := s.Span
	var lo, hi Point
	if span.Mode == SelectionWord {
		wl, wh, ok := wordBoundsAt(screen, col, row)
		if ok {
			lo, hi = Point{X: wl, Y: row}, Point{X: wh, Y: row}
		} else {
			lo, hi = Point{X: col, Y: row}, Point{X: col, Y: row}
		}
	} else {
		rr := max(0, min(row, screen.Height-1))
		lo, hi = Point{X: 0, Y: rr}, Point{X: max(0, screen.Width-1), Y: rr}
	}
	if compareSelectionPoints(hi, span.Lo) < 0 {
		s.Anchor, s.Focus = span.Hi, lo
	} else if compareSelectionPoints(lo, span.Hi) > 0 {
		s.Anchor, s.Focus = span.Lo, hi
	} else {
		s.Anchor, s.Focus = span.Lo, span.Hi
	}
	s.FocusSet = true
}

func (s *Selection) MoveFocus(move string, screen *Screen) {
	if s == nil || !s.FocusSet || screen == nil || screen.Width <= 0 || screen.Height <= 0 {
		return
	}
	col, row := s.Focus.X, s.Focus.Y
	maxCol, maxRow := screen.Width-1, screen.Height-1
	switch move {
	case "left":
		if col > 0 {
			col--
		} else if row > 0 {
			col, row = maxCol, row-1
		}
	case "right":
		if col < maxCol {
			col++
		} else if row < maxRow {
			col, row = 0, row+1
		}
	case "up":
		if row > 0 {
			row--
		}
	case "down":
		if row < maxRow {
			row++
		}
	case "lineStart":
		col = 0
	case "lineEnd":
		col = maxCol
	default:
		return
	}
	s.Focus = Point{X: col, Y: row}
	s.Span = nil
	s.VirtualFocusRow = nil
}

func intp(v int) *int { x := v; return &x }

// Shift moves both endpoints with content during keyboard scrolling. Virtual
// rows preserve the unclamped coordinates so reversing the scroll restores the
// exact selection. Captured off-screen rows are trimmed as that scroll debt is
// paid back, preventing copied text from being duplicated after round-trips.
func (s *Selection) Shift(dRow, minRow, maxRow, width int) {
	if s == nil || !s.FocusSet || dRow == 0 {
		return
	}
	oldA, oldF := s.Anchor.Y, s.Focus.Y
	if s.VirtualAnchorRow != nil {
		oldA = *s.VirtualAnchorRow
	}
	if s.VirtualFocusRow != nil {
		oldF = *s.VirtualFocusRow
	}
	newA, newF := oldA+dRow, oldF+dRow
	if (newA < minRow && newF < minRow) || (newA > maxRow && newF > maxRow) {
		s.Clear()
		return
	}

	oldMin, oldMax := min(oldA, oldF), max(oldA, oldF)
	newMin, newMax := min(newA, newF), max(newA, newF)
	oldAboveDebt := max(0, minRow-oldMin)
	oldBelowDebt := max(0, oldMax-maxRow)
	newAboveDebt := max(0, minRow-newMin)
	newBelowDebt := max(0, newMax-maxRow)

	if newAboveDebt < oldAboveDebt {
		drop := oldAboveDebt - newAboveDebt
		if drop >= len(s.ScrolledOffAbove) {
			s.ScrolledOffAbove = nil
			s.ScrolledOffAboveSW = nil
		} else {
			s.ScrolledOffAbove = s.ScrolledOffAbove[:len(s.ScrolledOffAbove)-drop]
			s.ScrolledOffAboveSW = s.ScrolledOffAboveSW[:min(len(s.ScrolledOffAboveSW), len(s.ScrolledOffAbove))]
		}
	}
	if newBelowDebt < oldBelowDebt {
		drop := oldBelowDebt - newBelowDebt
		if drop >= len(s.ScrolledOffBelow) {
			s.ScrolledOffBelow = nil
			s.ScrolledOffBelowSW = nil
		} else {
			s.ScrolledOffBelow = s.ScrolledOffBelow[drop:]
			if drop >= len(s.ScrolledOffBelowSW) {
				s.ScrolledOffBelowSW = nil
			} else {
				s.ScrolledOffBelowSW = s.ScrolledOffBelowSW[drop:]
			}
		}
	}

	// Invariant: captured accumulator length never exceeds current virtual
	// debt. Keep the entries nearest the visible edge when trimming.
	if len(s.ScrolledOffAbove) > newAboveDebt {
		if newAboveDebt == 0 {
			s.ScrolledOffAbove = nil
			s.ScrolledOffAboveSW = nil
		} else {
			start := len(s.ScrolledOffAbove) - newAboveDebt
			s.ScrolledOffAbove = append([]string(nil), s.ScrolledOffAbove[start:]...)
			if len(s.ScrolledOffAboveSW) >= newAboveDebt {
				s.ScrolledOffAboveSW = append([]bool(nil), s.ScrolledOffAboveSW[len(s.ScrolledOffAboveSW)-newAboveDebt:]...)
			}
		}
	}
	if len(s.ScrolledOffBelow) > newBelowDebt {
		if newBelowDebt == 0 {
			s.ScrolledOffBelow = nil
			s.ScrolledOffBelowSW = nil
		} else {
			s.ScrolledOffBelow = append([]string(nil), s.ScrolledOffBelow[:newBelowDebt]...)
			if len(s.ScrolledOffBelowSW) >= newBelowDebt {
				s.ScrolledOffBelowSW = append([]bool(nil), s.ScrolledOffBelowSW[:newBelowDebt]...)
			}
		}
	}

	clampPoint := func(p Point, row int) Point {
		if row < minRow {
			return Point{X: 0, Y: minRow}
		}
		if row > maxRow {
			return Point{X: max(0, width-1), Y: maxRow}
		}
		p.Y = row
		return p
	}
	s.Anchor = clampPoint(s.Anchor, newA)
	s.Focus = clampPoint(s.Focus, newF)
	if newA < minRow || newA > maxRow {
		s.VirtualAnchorRow = intp(newA)
	} else {
		s.VirtualAnchorRow = nil
	}
	if newF < minRow || newF > maxRow {
		s.VirtualFocusRow = intp(newF)
	} else {
		s.VirtualFocusRow = nil
	}
	if s.Span != nil {
		shift := func(p Point) Point {
			row := p.Y + dRow
			if row < minRow {
				return Point{X: 0, Y: minRow}
			}
			if row > maxRow {
				return Point{X: max(0, width-1), Y: maxRow}
			}
			p.Y = row
			return p
		}
		s.Span.Lo, s.Span.Hi = shift(s.Span.Lo), shift(s.Span.Hi)
	}
}

// ShiftForFollow moves both endpoints with sticky/streaming content. Unlike a
// keyboard jump, following content only clears the selection after both ends
// have moved above the viewport; a lower clamp may still become visible again.
// It returns true when the selection was cleared.
func (s *Selection) ShiftForFollow(dRow, minRow, maxRow int) bool {
	if s == nil || !s.FocusSet || dRow == 0 {
		return false
	}
	rawA, rawF := s.Anchor.Y, s.Focus.Y
	if s.VirtualAnchorRow != nil {
		rawA = *s.VirtualAnchorRow
	}
	if s.VirtualFocusRow != nil {
		rawF = *s.VirtualFocusRow
	}
	rawA += dRow
	rawF += dRow
	if rawA < minRow && rawF < minRow {
		s.Clear()
		return true
	}
	s.Anchor.Y = max(minRow, min(maxRow, rawA))
	s.Focus.Y = max(minRow, min(maxRow, rawF))
	if rawA < minRow || rawA > maxRow {
		s.VirtualAnchorRow = intp(rawA)
	} else {
		s.VirtualAnchorRow = nil
	}
	if rawF < minRow || rawF > maxRow {
		s.VirtualFocusRow = intp(rawF)
	} else {
		s.VirtualFocusRow = nil
	}
	if s.Span != nil {
		shift := func(p Point) Point {
			p.Y = max(minRow, min(maxRow, p.Y+dRow))
			return p
		}
		s.Span.Lo, s.Span.Hi = shift(s.Span.Lo), shift(s.Span.Hi)
	}
	return false
}

// ShiftAnchor follows content while the mouse remains fixed during drag-scroll.
func (s *Selection) ShiftAnchor(dRow, minRow, maxRow int) {
	if s == nil || dRow == 0 {
		return
	}
	raw := s.Anchor.Y
	if s.VirtualAnchorRow != nil {
		raw = *s.VirtualAnchorRow
	}
	raw += dRow
	s.Anchor.Y = max(minRow, min(maxRow, raw))
	if raw < minRow || raw > maxRow {
		s.VirtualAnchorRow = intp(raw)
	} else {
		s.VirtualAnchorRow = nil
	}
	if s.Span != nil {
		s.Span.Lo.Y = max(minRow, min(maxRow, s.Span.Lo.Y+dRow))
		s.Span.Hi.Y = max(minRow, min(maxRow, s.Span.Hi.Y+dRow))
	}
}

func extractSelectionRow(screen *Screen, row, start, end int) string {
	if screen == nil || row < 0 || row >= screen.Height {
		return ""
	}
	start = max(0, start)
	end = min(screen.Width-1, end)
	contentEnd := 0
	if row+1 < screen.Height && row+1 < len(screen.SoftWrap) && screen.SoftWrap[row+1] && row+1 < len(screen.SoftWrapEnd) {
		contentEnd = screen.SoftWrapEnd[row+1]
		if contentEnd > 0 {
			end = min(end, contentEnd-1)
		}
	}
	if start > end {
		return ""
	}
	var b strings.Builder
	for x := start; x <= end; x++ {
		c := screen.Cells[screen.index(x, row)]
		if c.NoSelect || c.Width == CellSpacerTail || c.Width == CellSpacerHead {
			continue
		}
		if c.Char == "" {
			b.WriteByte(' ')
		} else {
			b.WriteString(c.Char)
		}
	}
	if contentEnd > 0 {
		return b.String()
	}
	return strings.TrimRight(b.String(), " ")
}

func joinSelectionRow(lines *[]string, text string, soft bool) {
	if soft && len(*lines) > 0 {
		(*lines)[len(*lines)-1] += text
	} else {
		*lines = append(*lines, text)
	}
}

// CaptureScrolledRows preserves selected text that is about to leave a scroll viewport.
func (s *Selection) CaptureScrolledRows(screen *Screen, firstRow, lastRow int, above bool) {
	if s == nil || !s.FocusSet || screen == nil || firstRow > lastRow {
		return
	}
	start, end, ok := s.Bounds()
	if !ok {
		return
	}
	lo, hi := max(firstRow, start.Y), min(lastRow, end.Y)
	if lo > hi {
		return
	}
	texts := make([]string, 0, hi-lo+1)
	sw := make([]bool, 0, hi-lo+1)
	for row := lo; row <= hi; row++ {
		cs, ce := 0, screen.Width-1
		if row == start.Y {
			cs = start.X
		}
		if row == end.Y {
			ce = end.X
		}
		texts = append(texts, extractSelectionRow(screen, row, cs, ce))
		sw = append(sw, row < len(screen.SoftWrap) && screen.SoftWrap[row])
	}
	if above {
		s.ScrolledOffAbove = append(s.ScrolledOffAbove, texts...)
		s.ScrolledOffAboveSW = append(s.ScrolledOffAboveSW, sw...)
		if s.Anchor.Y == start.Y && lo == start.Y {
			s.Anchor.X = 0
		}
	} else {
		s.ScrolledOffBelow = append(texts, s.ScrolledOffBelow...)
		s.ScrolledOffBelowSW = append(sw, s.ScrolledOffBelowSW...)
		if s.Anchor.Y == end.Y && hi == end.Y {
			s.Anchor.X = max(0, screen.Width-1)
		}
	}
}

// Text extracts logical source text, joining rows marked as soft-wrap continuations.
func (s Selection) Text(screen *Screen) string {
	if screen == nil || screen.Height == 0 {
		return ""
	}
	a, b := s.normalized()
	a.Y = max(0, a.Y)
	b.Y = min(screen.Height-1, b.Y)
	if a.Y > b.Y {
		return ""
	}
	lines := []string{}
	for i, text := range s.ScrolledOffAbove {
		soft := i < len(s.ScrolledOffAboveSW) && s.ScrolledOffAboveSW[i]
		joinSelectionRow(&lines, text, soft)
	}
	for y := a.Y; y <= b.Y; y++ {
		start, end := 0, screen.Width-1
		if y == a.Y {
			start = a.X
		}
		if y == b.Y {
			end = b.X
		}
		soft := y < len(screen.SoftWrap) && screen.SoftWrap[y]
		joinSelectionRow(&lines, extractSelectionRow(screen, y, start, end), soft)
	}
	for i, text := range s.ScrolledOffBelow {
		soft := i < len(s.ScrolledOffBelowSW) && s.ScrolledOffBelowSW[i]
		joinSelectionRow(&lines, text, soft)
	}
	return strings.Join(lines, "\n")
}

func isURLASCII(c string) bool {
	if len(c) != 1 {
		return false
	}
	b := c[0]
	if b < 0x21 || b > 0x7e {
		return false
	}
	return !strings.ContainsRune("<>\"'` ", rune(b))
}

// FindPlainTextURLAt mirrors terminal linkification when mouse tracking intercepts native Cmd/Ctrl-click.
func FindPlainTextURLAt(screen *Screen, col, row int) (string, bool) {
	if screen == nil || !screen.InBounds(col, row) {
		return "", false
	}
	if c, _ := screen.CellAt(col, row); c.Width == CellSpacerTail && col > 0 {
		col--
	}
	c, _ := screen.CellAt(col, row)
	if c.NoSelect || !isURLASCII(c.Char) {
		return "", false
	}
	lo, hi := col, col
	for lo > 0 {
		p, _ := screen.CellAt(lo-1, row)
		if p.NoSelect || p.Width != CellNormal || !isURLASCII(p.Char) {
			break
		}
		lo--
	}
	for hi < screen.Width-1 {
		n, _ := screen.CellAt(hi+1, row)
		if n.NoSelect || n.Width != CellNormal || !isURLASCII(n.Char) {
			break
		}
		hi++
	}
	var b strings.Builder
	for x := lo; x <= hi; x++ {
		cc, _ := screen.CellAt(x, row)
		b.WriteString(cc.Char)
	}
	token := b.String()
	click := col - lo
	starts := []int{}
	for _, scheme := range []string{"http://", "https://", "file://"} {
		off := 0
		for {
			i := strings.Index(token[off:], scheme)
			if i < 0 {
				break
			}
			starts = append(starts, off+i)
			off += i + 1
		}
	}
	if len(starts) == 0 {
		return "", false
	}
	best := -1
	next := len(token)
	for _, st := range starts {
		if st <= click && st > best {
			best = st
		}
	}
	if best < 0 {
		return "", false
	}
	for _, st := range starts {
		if st > best && st < next {
			next = st
		}
	}
	url := token[best:next]
	for len(url) > 0 && strings.ContainsRune(".,;:!?", rune(url[len(url)-1])) {
		url = url[:len(url)-1]
	}
	for len(url) > 0 {
		last := url[len(url)-1]
		var open byte
		switch last {
		case ')':
			open = '('
		case ']':
			open = '['
		case '}':
			open = '{'
		default:
			return url, click < best+len(url)
		}
		if strings.Count(url, string(last)) > strings.Count(url, string(open)) {
			url = url[:len(url)-1]
		} else {
			break
		}
	}
	return url, click < best+len(url)
}

// ApplySelectionOverlay modifies a rendered screen copy before diffing. If bg is unset,
// inverse is used as a safe theme-independent fallback.
func ApplySelectionOverlay(screen *Screen, s *Selection, bg Color) {
	if screen == nil || s == nil {
		return
	}
	start, end, ok := s.Bounds()
	if !ok {
		return
	}
	for y := max(0, start.Y); y <= end.Y && y < screen.Height; y++ {
		x1, x2 := 0, screen.Width-1
		if y == start.Y {
			x1 = start.X
		}
		if y == end.Y {
			x2 = end.X
		}
		for x := max(0, x1); x <= x2 && x < screen.Width; x++ {
			c := &screen.Cells[screen.index(x, y)]
			if c.NoSelect {
				continue
			}
			if bg.Kind != ColorUnset {
				c.Style.BackgroundColor = bg
				c.Style.Inverse = false
			} else {
				c.Style.Inverse = true
			}
		}
	}
}
