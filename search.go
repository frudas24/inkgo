package ink

import "strings"

type MatchPosition struct{ Row, Col, Len int }

func ScanPositions(screen *Screen, query string) []MatchPosition {
	if screen == nil || query == "" {
		return nil
	}
	q := strings.ToLower(query)
	var out []MatchPosition
	for y := 0; y < screen.Height; y++ {
		var text strings.Builder
		cellForRune := []int{}
		endForRune := []int{}
		for x := 0; x < screen.Width; x++ {
			c := screen.Cells[screen.index(x, y)]
			if c.Width == CellSpacerTail || c.NoSelect {
				continue
			}
			lc := strings.ToLower(c.Char)
			if lc == "" {
				lc = " "
			}
			for range []rune(lc) {
				cellForRune = append(cellForRune, x)
				end := x + 1
				if c.Width == CellWide {
					end = x + 2
				}
				endForRune = append(endForRune, end)
			}
			text.WriteString(lc)
		}
		runes := []rune(text.String())
		qr := []rune(q)
		if len(qr) == 0 || len(runes) < len(qr) {
			continue
		}
		for i := 0; i+len(qr) <= len(runes); {
			match := true
			for j := range qr {
				if runes[i+j] != qr[j] {
					match = false
					break
				}
			}
			if match {
				start := cellForRune[i]
				end := endForRune[i+len(qr)-1]
				out = append(out, MatchPosition{Row: y, Col: start, Len: end - start})
				i += len(qr)
			} else {
				i++
			}
		}
	}
	return out
}

func ApplySearchHighlight(screen *Screen, query string, current int) []MatchPosition {
	matches := ScanPositions(screen, query)
	for i, m := range matches {
		for x := m.Col; x < m.Col+m.Len && x < screen.Width; x++ {
			if x < 0 || m.Row < 0 || m.Row >= screen.Height {
				continue
			}
			idx := screen.index(x, m.Row)
			c := &screen.Cells[idx]
			if c.Width == CellSpacerTail || c.NoSelect {
				continue
			}
			c.Style.Inverse = true
			if i == current {
				c.Style.Bold = true
				c.Style.Underline = true
				c.Style.Color = ANSIColor(3)
			}
		}
	}
	return matches
}

type Selection struct{ Anchor, Focus Point }

func (s Selection) normalized() (Point, Point) {
	a, b := s.Anchor, s.Focus
	if a.Y > b.Y || (a.Y == b.Y && a.X > b.X) {
		a, b = b, a
	}
	return a, b
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
func (s Selection) Text(screen *Screen) string {
	if screen == nil {
		return ""
	}
	a, b := s.normalized()
	a.Y = max(0, a.Y)
	b.Y = min(screen.Height-1, b.Y)
	var lines []string
	for y := a.Y; y <= b.Y; y++ {
		start, end, ok := s.RectForRow(y, screen.Width)
		if !ok {
			continue
		}
		var line strings.Builder
		for x := start; x < end; x++ {
			c := screen.Cells[screen.index(x, y)]
			if c.NoSelect || c.Width == CellSpacerTail {
				continue
			}
			line.WriteString(c.Char)
		}
		v := strings.TrimRight(line.String(), " ")
		// softWrap is attached to the continuation row, so if this row itself
		// is a continuation append it to the previous logical line.
		if y < len(screen.SoftWrap) && screen.SoftWrap[y] && len(lines) > 0 {
			lines[len(lines)-1] += v
		} else {
			lines = append(lines, v)
		}
	}
	return strings.Join(lines, "\n")
}
