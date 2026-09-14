package engine

import (
	"strings"
	"testing"
)

// TestRawANSIWrapBreaksBetweenWords pins the transcript wrapping defect: a
// styled (NodeRawANSI) node was painted cell by cell, so the layout measured it
// with the word-wrap producer while the paint pass broke at the first grapheme
// that did not fit the column - slicing a word at the row edge and painting
// rows the layout had never granted. Painted rows must come from the same
// producer the layout measures with, so a word that fits a row of its own moves
// to the next row instead of being cut.
func TestRawANSIWrapBreaksBetweenWords(t *testing.T) {
	cases := []struct {
		name   string
		styled string
		plain  string
		width  int
		// rows is the producer's own output for the visible text
		// (WrapText/WrapTextLines share wrappedRows with MeasureText). The
		// leading space of the second row is the separator space the producer
		// keeps when a row ends exactly at the width - paint must reproduce it
		// rather than re-derive its own breaks.
		rows []string
	}{
		{
			name:   "breaks-at-space",
			styled: "\x1b[36malpha beta gamma\x1b[0m",
			plain:  "alpha beta gamma",
			width:  10,
			rows:   []string{"alpha beta", " gamma"},
		},
		{
			// Before the fix this painted "one wonder" / "ful thing": the word
			// was cut at the column edge even though it fits a row of its own.
			name:   "whole-word-moves-to-next-row",
			styled: "\x1b[36mone wonderful thing\x1b[0m",
			plain:  "one wonderful thing",
			width:  10,
			rows:   []string{"one", "wonderful", "thing"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			node := RawANSI(tc.styled)
			screen, height := RenderToScreen(Root(node), tc.width)
			got := screen.PlainText()

			if want := strings.Join(tc.rows, "\n"); got != want {
				t.Fatalf("width %d painted\n got %q\nwant %q", tc.width, got, want)
			}
			// The producer's visible rows for the same width are what the layout
			// granted the node (MeasureText counts exactly these rows), so the
			// paint pass must not invent a different decomposition.
			if want := producerRows(tc.plain, tc.width); !equalRows(strings.Split(got, "\n"), want) {
				t.Fatalf("width %d painted %q, producer/layout rows %q", tc.width, got, strings.Join(want, "\n"))
			}

			measured := MeasureText(tc.plain, tc.width, TextWrapWrap).Height
			if height != measured {
				t.Fatalf("painted rows %d disagree with measured height %d", height, measured)
			}
			if node.Rect.Height != measured {
				t.Fatalf("node rect height %d disagrees with measured height %d", node.Rect.Height, measured)
			}

			// No word may be split across rows: a sliced word would not appear
			// whole in any painted row.
			painted := strings.Split(got, "\n")
			for _, word := range strings.Fields(tc.plain) {
				if len(word) < 2 {
					continue
				}
				seen := 0
				for _, row := range painted {
					if strings.Contains(row, word) {
						seen++
					}
				}
				if seen != 1 {
					t.Fatalf("word %q appears whole in %d painted rows, want 1: %q", word, seen, painted)
				}
			}

			// Parsing one wrapped row at a time must keep the inherited SGR
			// alive on continuation rows (restoreVisualStateAcrossRows).
			for y, row := range painted {
				first := strings.IndexFunc(row, func(r rune) bool { return r != ' ' })
				if first < 0 {
					continue
				}
				cell, ok := screen.CellAt(first, y)
				if !ok || cell.Style.Color.Kind == ColorUnset {
					t.Fatalf("row %d (%q) lost its SGR colour: %+v", y, row, cell.Style)
				}
			}
		})
	}
}

// producerRows is WrapText's output for the visible text, normalized the same
// way Screen.PlainText normalizes a painted row (trailing spaces trimmed,
// trailing empty rows dropped).
func producerRows(plain string, width int) []string {
	rows := strings.Split(WrapText(plain, width, TextWrapWrap), "\n")
	for i := range rows {
		rows[i] = strings.TrimRight(rows[i], " ")
	}
	for len(rows) > 0 && rows[len(rows)-1] == "" {
		rows = rows[:len(rows)-1]
	}
	return rows
}

func equalRows(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
