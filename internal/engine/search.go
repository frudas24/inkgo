package engine

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

// ApplyPositionedHighlight paints one current result from a pre-scanned set.
// Positions are relative to an element; rowOffset maps them to the live screen.
func ApplyPositionedHighlight(screen *Screen, positions []MatchPosition, rowOffset, current int) bool {
	if screen == nil || current < 0 || current >= len(positions) {
		return false
	}
	m := positions[current]
	row := m.Row + rowOffset
	if row < 0 || row >= screen.Height {
		return false
	}
	applied := false
	for x := m.Col; x < m.Col+m.Len && x < screen.Width; x++ {
		if x < 0 {
			continue
		}
		c := &screen.Cells[screen.index(x, row)]
		if c.NoSelect || c.Width == CellSpacerTail || c.Width == CellSpacerHead {
			continue
		}
		c.Style.Bold = true
		c.Style.Underline = true
		c.Style.Color = ANSIColor(3)
		applied = true
	}
	return applied
}
