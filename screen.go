package inkgo

import (
	"strings"
)

type CellWidth uint8

const (
	CellNormal CellWidth = iota
	CellWide
	CellSpacerTail
	CellSpacerHead
)

type Cell struct {
	Char      string
	Width     CellWidth
	Style     TextStyle
	Hyperlink string
	NoSelect  bool
}

type Screen struct {
	Width, Height int
	Cells         []Cell
	SoftWrap      []bool
	// SoftWrapEnd[row] is the absolute exclusive content-end column of
	// row-1 when SoftWrap[row] is true. It preserves significant spaces at
	// wrap boundaries without copying unwritten terminal padding.
	SoftWrapEnd []int
}

func NewScreen(width, height int) *Screen {
	s := &Screen{}
	s.Reset(width, height)
	return s
}

func (s *Screen) Reset(width, height int) {
	width, height = max(0, width), max(0, height)
	s.Width, s.Height = width, height
	need := width * height
	if cap(s.Cells) < need {
		s.Cells = make([]Cell, need)
	} else {
		s.Cells = s.Cells[:need]
		clear(s.Cells)
	}
	if cap(s.SoftWrap) < height {
		s.SoftWrap = make([]bool, height)
	} else {
		s.SoftWrap = s.SoftWrap[:height]
		clear(s.SoftWrap)
	}
	if cap(s.SoftWrapEnd) < height {
		s.SoftWrapEnd = make([]int, height)
	} else {
		s.SoftWrapEnd = s.SoftWrapEnd[:height]
		clear(s.SoftWrapEnd)
	}
	for i := range s.Cells {
		s.Cells[i].Char = " "
	}
}

func (s *Screen) Clone() *Screen {
	if s == nil {
		return nil
	}
	out := &Screen{Width: s.Width, Height: s.Height, Cells: make([]Cell, len(s.Cells)), SoftWrap: make([]bool, len(s.SoftWrap)), SoftWrapEnd: make([]int, len(s.SoftWrapEnd))}
	copy(out.Cells, s.Cells)
	copy(out.SoftWrap, s.SoftWrap)
	copy(out.SoftWrapEnd, s.SoftWrapEnd)
	return out
}

func (s *Screen) index(x, y int) int     { return y*s.Width + x }
func (s *Screen) InBounds(x, y int) bool { return x >= 0 && y >= 0 && x < s.Width && y < s.Height }
func (s *Screen) CellAt(x, y int) (Cell, bool) {
	if s == nil || !s.InBounds(x, y) {
		return Cell{}, false
	}
	return s.Cells[s.index(x, y)], true
}

func (s *Screen) clearWideNeighbors(x, y int) {
	if !s.InBounds(x, y) {
		return
	}
	i := s.index(x, y)
	c := s.Cells[i]
	if c.Width == CellSpacerTail && x > 0 {
		s.Cells[i-1] = Cell{Char: " "}
	}
	if c.Width == CellWide && x+1 < s.Width {
		s.Cells[i+1] = Cell{Char: " "}
	}
	if c.Width == CellSpacerHead && x+1 < s.Width {
		s.Cells[i+1] = Cell{Char: " "}
	}
}

func (s *Screen) SetCell(x, y int, value string, width int, style TextStyle, hyperlink string) {
	if s == nil || !s.InBounds(x, y) || width <= 0 {
		return
	}
	s.clearWideNeighbors(x, y)
	if width >= 2 {
		if x+1 >= s.Width {
			return
		}
		s.clearWideNeighbors(x+1, y)
		s.Cells[s.index(x, y)] = Cell{Char: value, Width: CellWide, Style: style, Hyperlink: hyperlink}
		s.Cells[s.index(x+1, y)] = Cell{Char: "", Width: CellSpacerTail, Style: style, Hyperlink: hyperlink}
		return
	}
	s.Cells[s.index(x, y)] = Cell{Char: value, Width: CellNormal, Style: style, Hyperlink: hyperlink}
}

func (s *Screen) Fill(rect Rect, ch string, style TextStyle) {
	r := ClampRect(rect, Size{Width: s.Width, Height: s.Height})
	for y := r.Y; y < r.Y+r.Height; y++ {
		for x := r.X; x < r.X+r.Width; x++ {
			s.SetCell(x, y, ch, 1, style, "")
		}
	}
}

func (s *Screen) ClearRegion(rect Rect) {
	r := ClampRect(rect, Size{Width: s.Width, Height: s.Height})
	for y := r.Y; y < r.Y+r.Height; y++ {
		for x := r.X; x < r.X+r.Width; x++ {
			// Wide graphemes are atomic visual units. Clearing only their head
			// or tail must clear the partner even when it lies just outside the
			// requested rectangle, otherwise a skipped spacer can survive as a
			// ghost cell in later diffs.
			s.clearWideNeighbors(x, y)
			s.Cells[s.index(x, y)] = Cell{Char: " "}
		}
	}
}

func (s *Screen) MarkNoSelect(rect Rect) {
	r := ClampRect(rect, Size{Width: s.Width, Height: s.Height})
	for y := r.Y; y < r.Y+r.Height; y++ {
		for x := r.X; x < r.X+r.Width; x++ {
			s.Cells[s.index(x, y)].NoSelect = true
		}
	}
}

func (s *Screen) normalizeWideRow(y int) {
	if s == nil || y < 0 || y >= s.Height {
		return
	}
	for x := 0; x < s.Width; x++ {
		i := s.index(x, y)
		c := s.Cells[i]
		switch c.Width {
		case CellSpacerTail:
			if x == 0 || s.Cells[s.index(x-1, y)].Width != CellWide {
				s.Cells[i] = Cell{Char: " "}
			}
		case CellWide:
			if x+1 >= s.Width {
				s.Cells[i] = Cell{Char: " "}
				continue
			}
			next := s.index(x+1, y)
			if s.Cells[next].Width != CellSpacerTail {
				s.Cells[next] = Cell{Char: "", Width: CellSpacerTail, Style: c.Style, Hyperlink: c.Hyperlink, NoSelect: c.NoSelect}
			}
			x++
		}
	}
}

func (s *Screen) Blit(src *Screen, srcRect Rect, dst Point) {
	if s == nil || src == nil {
		return
	}
	sr := ClampRect(srcRect, Size{Width: src.Width, Height: src.Height})
	for y := 0; y < sr.Height; y++ {
		sy, dy := sr.Y+y, dst.Y+y
		if dy >= 0 && dy < s.Height && sy >= 0 && sy < src.Height {
			s.SoftWrap[dy] = src.SoftWrap[sy]
			if sy < len(src.SoftWrapEnd) && dy < len(s.SoftWrapEnd) {
				end := src.SoftWrapEnd[sy]
				if end > 0 {
					end += dst.X - sr.X
					end = max(0, min(s.Width, end))
				}
				s.SoftWrapEnd[dy] = end
			}
		}
		for x := 0; x < sr.Width; x++ {
			dx, dy := dst.X+x, dst.Y+y
			if !s.InBounds(dx, dy) {
				continue
			}
			s.Cells[s.index(dx, dy)] = src.Cells[src.index(sr.X+x, sr.Y+y)]
		}
		if dy := dst.Y + y; dy >= 0 && dy < s.Height {
			s.normalizeWideRow(dy)
		}
	}
}

func (s *Screen) ShiftRows(top, bottom, n int) {
	if s == nil || s.Width == 0 || n == 0 {
		return
	}
	top = max(0, top)
	bottom = min(s.Height-1, bottom)
	if top > bottom {
		return
	}
	count := bottom - top + 1
	if n > count {
		n = count
	}
	if n < -count {
		n = -count
	}
	if n > 0 {
		for y := top; y <= bottom-n; y++ {
			copy(s.Cells[s.index(0, y):s.index(0, y)+s.Width], s.Cells[s.index(0, y+n):s.index(0, y+n)+s.Width])
			s.SoftWrap[y] = s.SoftWrap[y+n]
			s.SoftWrapEnd[y] = s.SoftWrapEnd[y+n]
		}
		for y := bottom - n + 1; y <= bottom; y++ {
			for x := 0; x < s.Width; x++ {
				s.Cells[s.index(x, y)] = Cell{Char: " "}
			}
			s.SoftWrap[y] = false
			s.SoftWrapEnd[y] = 0
		}
	} else {
		n = -n
		for y := bottom; y >= top+n; y-- {
			copy(s.Cells[s.index(0, y):s.index(0, y)+s.Width], s.Cells[s.index(0, y-n):s.index(0, y-n)+s.Width])
			s.SoftWrap[y] = s.SoftWrap[y-n]
			s.SoftWrapEnd[y] = s.SoftWrapEnd[y-n]
		}
		for y := top; y < top+n; y++ {
			for x := 0; x < s.Width; x++ {
				s.Cells[s.index(x, y)] = Cell{Char: " "}
			}
			s.SoftWrap[y] = false
			s.SoftWrapEnd[y] = 0
		}
	}
}

func cellsEqual(a, b Cell) bool { return a == b }

type Damage struct {
	Rect    Rect
	Changed int
}

func ScreenDamage(prev, next *Screen) Damage {
	if next == nil {
		return Damage{}
	}
	if prev == nil || prev.Width != next.Width || prev.Height != next.Height {
		return Damage{Rect: Rect{Width: next.Width, Height: next.Height}, Changed: next.Width * next.Height}
	}
	minX, minY, maxX, maxY := next.Width, next.Height, -1, -1
	changed := 0
	for y := 0; y < next.Height; y++ {
		for x := 0; x < next.Width; x++ {
			i := next.index(x, y)
			if cellsEqual(prev.Cells[i], next.Cells[i]) {
				continue
			}
			changed++
			minX = min(minX, x)
			minY = min(minY, y)
			maxX = max(maxX, x)
			maxY = max(maxY, y)
		}
	}
	if changed == 0 {
		return Damage{}
	}
	return Damage{Rect: Rect{X: minX, Y: minY, Width: maxX - minX + 1, Height: maxY - minY + 1}, Changed: changed}
}

// DiffScreens emits a terminal patch. It intentionally optimizes for correctness
// and narrow damage rather than byte-perfect parity with Ink's JS optimizer.
func DiffScreens(prev, next *Screen) string {
	if next == nil {
		return ""
	}
	full := prev == nil || prev.Width != next.Width || prev.Height != next.Height
	var b strings.Builder
	var lastStyle TextStyle
	haveStyle := false
	lastLink := ""
	cursorX, cursorY := -1, -1
	setLink := func(link string) {
		if link == lastLink {
			return
		}
		if lastLink != "" {
			b.WriteString(OSC(8, "", ""))
		}
		if link != "" {
			b.WriteString(OSC(8, "", link))
		}
		lastLink = link
	}
	for y := 0; y < next.Height; y++ {
		for x := 0; x < next.Width; x++ {
			i := next.index(x, y)
			c := next.Cells[i]
			if c.Width == CellSpacerTail {
				continue
			}
			changed := full
			if !full {
				changed = !cellsEqual(prev.Cells[i], c)
				if !changed && c.Width == CellWide && x+1 < next.Width {
					changed = !cellsEqual(prev.Cells[i+1], next.Cells[i+1])
				}
			}
			if !changed {
				continue
			}
			if cursorX != x || cursorY != y {
				b.WriteString(CursorPosition(y, x))
				cursorX, cursorY = x, y
			}
			setLink(c.Hyperlink)
			if !haveStyle || c.Style != lastStyle {
				b.WriteString(StyleSequence(c.Style))
				lastStyle = c.Style
				haveStyle = true
			}
			ch := c.Char
			if ch == "" {
				ch = " "
			}
			b.WriteString(ch)
			w := 1
			if c.Width == CellWide {
				w = 2
			}
			cursorX += w
		}
	}
	if lastLink != "" {
		b.WriteString(OSC(8, "", ""))
	}
	if haveStyle {
		b.WriteString("\x1b[0m")
	}
	return b.String()
}

func (s *Screen) PlainText() string {
	if s == nil {
		return ""
	}
	lines := make([]string, s.Height)
	for y := 0; y < s.Height; y++ {
		var b strings.Builder
		for x := 0; x < s.Width; x++ {
			c := s.Cells[s.index(x, y)]
			if c.Width == CellSpacerTail {
				continue
			}
			if c.Char == "" {
				b.WriteByte(' ')
			} else {
				b.WriteString(c.Char)
			}
		}
		lines[y] = strings.TrimRight(b.String(), " ")
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n")
}

func (s *Screen) SelectableText(rect Rect) string {
	if s == nil {
		return ""
	}
	r := ClampRect(rect, Size{Width: s.Width, Height: s.Height})
	lines := make([]string, 0, r.Height)
	for y := r.Y; y < r.Y+r.Height; y++ {
		var b strings.Builder
		for x := r.X; x < r.X+r.Width; x++ {
			c := s.Cells[s.index(x, y)]
			if c.Width == CellSpacerTail || c.NoSelect {
				continue
			}
			b.WriteString(c.Char)
		}
		lines = append(lines, strings.TrimRight(b.String(), " "))
	}
	return strings.Join(lines, "\n")
}

// DiffScreensRelative emits a patch assuming the physical cursor is parked at
// column 0 of the last rendered row. This is the safe main-screen equivalent
// of Ink/log-update: it never addresses absolute viewport rows, so existing
// shell scrollback above the app is not overwritten.
func DiffScreensRelative(prev, next *Screen) string {
	if next == nil {
		return ""
	}
	if prev == nil || prev.Width != next.Width || prev.Height != next.Height {
		return redrawRelative(prev, next)
	}
	if next.Height == 0 {
		return ""
	}
	type run struct{ y, x1, x2 int }
	var runs []run
	for y := 0; y < next.Height; y++ {
		x1, x2 := -1, -1
		for x := 0; x < next.Width; x++ {
			i := next.index(x, y)
			if cellsEqual(prev.Cells[i], next.Cells[i]) {
				continue
			}
			if x1 < 0 {
				x1 = x
			}
			x2 = x
		}
		if x1 >= 0 {
			if next.Cells[next.index(x1, y)].Width == CellSpacerTail && x1 > 0 {
				x1--
			}
			if x2+1 < next.Width && next.Cells[next.index(x2, y)].Width == CellWide {
				x2++
			}
			runs = append(runs, run{y, x1, x2})
		}
	}
	if len(runs) == 0 {
		return ""
	}
	var b strings.Builder
	curY := next.Height - 1
	for _, r := range runs {
		if r.y < curY {
			b.WriteString(CursorUp(curY - r.y))
		} else if r.y > curY {
			b.WriteString(CursorDown(r.y - curY))
		}
		curY = r.y
		b.WriteString(CursorTo(r.x1))
		b.WriteString(encodeCellRange(next, r.y, r.x1, r.x2+1))
	}
	if curY < next.Height-1 {
		b.WriteString(CursorDown(next.Height - 1 - curY))
	}
	b.WriteString(CursorTo(0))
	return b.String()
}

func redrawRelative(prev, next *Screen) string {
	var b strings.Builder
	prevH := 0
	if prev != nil {
		prevH = prev.Height
	}
	if prevH > 0 {
		b.WriteString("\r")
		if prevH > 1 {
			b.WriteString(CursorUp(prevH - 1))
		}
	}
	rows := max(prevH, next.Height)
	if rows == 0 {
		return ""
	}
	for y := 0; y < rows; y++ {
		b.WriteString(EraseLine)
		if y < next.Height {
			b.WriteString(encodeCellRange(next, y, 0, next.Width))
		}
		if y < rows-1 {
			b.WriteString("\r\n")
		}
	}
	if rows > next.Height && next.Height > 0 {
		b.WriteString(CursorUp(rows - next.Height))
	}
	b.WriteString(CursorTo(0))
	return b.String()
}

func encodeCellRange(s *Screen, y, x1, x2 int) string {
	if s == nil || y < 0 || y >= s.Height {
		return ""
	}
	x1 = max(0, x1)
	x2 = min(s.Width, x2)
	var b strings.Builder
	var style TextStyle
	haveStyle := false
	link := ""
	setLink := func(v string) {
		if v == link {
			return
		}
		if link != "" {
			b.WriteString(OSC(8, "", ""))
		}
		if v != "" {
			b.WriteString(OSC(8, "", v))
		}
		link = v
	}
	for x := x1; x < x2; x++ {
		c := s.Cells[s.index(x, y)]
		if c.Width == CellSpacerTail {
			continue
		}
		setLink(c.Hyperlink)
		if !haveStyle || c.Style != style {
			b.WriteString(StyleSequence(c.Style))
			style = c.Style
			haveStyle = true
		}
		ch := c.Char
		if ch == "" {
			ch = " "
		}
		b.WriteString(ch)
	}
	if link != "" {
		b.WriteString(OSC(8, "", ""))
	}
	if haveStyle {
		b.WriteString("\x1b[0m")
	}
	return b.String()
}
