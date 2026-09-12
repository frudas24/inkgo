package textutil

import (
	"math/rand"
	"reflect"
	"strings"
	"testing"

	core "github.com/frudas24/inkgo/internal/core"
)

// wrapTextLinesLegacy is the previous WrapTextLines body, kept here as the
// reference the shared row producer must keep reproducing.
func wrapTextLinesLegacy(text string, maxWidth int, mode core.TextWrap) ([]string, []bool) {
	if maxWidth < 0 {
		maxWidth = 0
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	if mode == core.TextWrapTruncate || mode == core.TextWrapTruncateEnd || mode == core.TextWrapTruncateMiddle || mode == core.TextWrapTruncateStart || (mode != core.TextWrapWrap && mode != core.TextWrapTrim && mode != "") {
		lines := strings.Split(WrapText(text, maxWidth, mode), "\n")
		return lines, make([]bool, len(lines))
	}
	trim := mode == core.TextWrapTrim
	var lines []string
	var soft []bool
	for _, src := range strings.Split(text, "\n") {
		parts := hardWrapLineTokens(src, maxWidth, trim)
		for i, part := range parts {
			lines = append(lines, part)
			soft = append(soft, i > 0)
		}
	}
	return lines, soft
}

// measureTextLegacy is the previous MeasureText body: it wrapped into a string,
// split it again and re-measured every row.
func measureTextLegacy(text string, width int, mode core.TextWrap) core.Size {
	if width <= 0 {
		width = max(1, WidestLine(text))
	}
	wrapped := WrapText(text, width, mode)
	lines := strings.Split(wrapped, "\n")
	maxW := 0
	for _, line := range lines {
		maxW = max(maxW, StringWidth(line))
	}
	return core.Size{Width: min(width, maxW), Height: max(1, len(lines))}
}

// TestWrapPlainASCIIMatchesTokenPath is the differential guard for the byte-level
// ASCII row producer: on printable-ASCII input it must return byte-identical rows
// to the token path it replaces, for every width and both trim modes. It covers
// space runs at both boundaries, empty words, overlong words and widths at and
// below the word length, which are the cases where the two break decisions are
// free to disagree.
func TestWrapPlainASCIIMatchesTokenPath(t *testing.T) {
	type testCase struct {
		line  string
		width int
		trim  bool
	}
	targeted := []testCase{
		{"", 4, false}, {"", 4, true},
		{" ", 1, false}, {" ", 4, true}, {"   ", 2, false}, {"   ", 2, true},
		{"  ab", 5, false}, {"  ab", 5, true}, {"ab ", 5, false}, {"ab ", 5, true},
		{"ab cd", 2, false}, {"ab cd", 2, true}, {"ab cd", 3, false}, {"a b c", 1, false},
		{"abcdefgh", 3, false}, {"abcdefgh", 3, true}, {"a abcdefgh", 3, false},
		{"a abcdefgh", 3, true}, {"ab abcdefgh", 3, false}, {"abc defghij", 1, true},
		{"alpha bravo charlie delta", 7, false}, {"alpha bravo charlie delta", 7, true},
		{"alpha  bravo", 6, false}, {"alpha  bravo", 6, true},
	}
	for _, tc := range targeted {
		got := wrapPlainASCIILine(tc.line, tc.width, tc.trim)
		want := hardWrapLineTokens(tc.line, tc.width, tc.trim)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("fast path drift: line=%q width=%d trim=%v\n got=%q\nwant=%q", tc.line, tc.width, tc.trim, got, want)
		}
	}

	rng := rand.New(rand.NewSource(20260912))
	words := []string{"a", "ab", "abc", "alpha", "bravo", "charlie"}
	for _, width := range []int{1, 2, 3, 5, 8, 17, 100} {
		for _, trim := range []bool{false, true} {
			for i := 0; i < 600; i++ {
				var sb strings.Builder
				for j := 0; j <= rng.Intn(10); j++ {
					if j > 0 {
						sb.WriteString(strings.Repeat(" ", 1+rng.Intn(3)))
					}
					if rng.Intn(4) == 0 {
						continue // empty word between spaces
					}
					word := words[rng.Intn(len(words))]
					if rng.Intn(4) == 0 {
						word = strings.Repeat(word, 1+rng.Intn(5))
					}
					sb.WriteString(word)
				}
				line := sb.String()
				got := wrapPlainASCIILine(line, width, trim)
				want := hardWrapLineTokens(line, width, trim)
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("fast path drift: line=%q width=%d trim=%v\n got=%q\nwant=%q", line, width, trim, got, want)
				}
			}
		}
	}
}

// TestWrapPlainASCIIRowsAreSourceRanges pins the property the fast path relies
// on: every row is a slice of the line it came from, with only the spaces trim
// mode removes missing.
func TestWrapPlainASCIIRowsAreSourceRanges(t *testing.T) {
	line := "alpha bravo charlie delta echo"
	for _, width := range []int{1, 2, 4, 9, 40} {
		for _, trim := range []bool{false, true} {
			rows := wrapPlainASCIILine(line, width, trim)
			if len(rows) == 0 {
				t.Fatalf("no rows: width=%d trim=%v", width, trim)
			}
			offset := 0
			for _, row := range rows {
				if row == "" {
					t.Fatalf("empty row: width=%d trim=%v rows=%q", width, trim, rows)
				}
				if StringWidth(row) > width {
					t.Fatalf("row wider than the requested width: width=%d row=%q", width, row)
				}
				at := strings.Index(line[offset:], row)
				if at < 0 {
					t.Fatalf("row %q is not a slice of %q after offset %d", row, line, offset)
				}
				offset += at + len(row)
			}
		}
	}
}

// TestMeasureTextMatchesWrapDerivedRows pins the measurement contract against the
// previous implementation: height is the wrapped row count and width is the widest
// row, both derived from the wrap that the shared row producer performs once.
func TestMeasureTextMatchesWrapDerivedRows(t *testing.T) {
	corpus := []string{
		"",
		" ",
		"plain ascii line of text",
		"one  two   three",
		"supercalifragilisticexpialidocious",
		"a\tb\tc",
		"alpha bravo\r\ncharlie",
		"\x1b[31mred text that wraps\x1b[0m",
		"\x1b]8;;https://example.com\x07link text\x1b]8;;\x07",
		"界界界界界",
		"👨‍👩‍👧‍👦 hello",
		"café ☀\u200d☀",
		strings.Repeat("v", 200),
		"mix \x1b[1mbold\x1b[0m 界 and ascii",
	}
	modes := []core.TextWrap{
		core.TextWrapWrap, core.TextWrapTrim, core.TextWrapTruncate,
		core.TextWrapTruncateEnd, core.TextWrapTruncateMiddle, core.TextWrapTruncateStart,
		core.TextWrap("unknown"),
	}
	for _, text := range corpus {
		for _, mode := range modes {
			for _, width := range []int{-1, 0, 1, 2, 3, 7, 16, 100} {
				got := MeasureText(text, width, mode)
				want := measureTextLegacy(text, width, mode)
				if got != want {
					t.Fatalf("MeasureText drift: text=%q mode=%q width=%d got=%+v want=%+v", text, mode, width, got, want)
				}
			}
		}
	}
}

// TestWrapTextLinesMatchesLegacyDerivation pins the row and soft-wrap bitmap
// contract of the shared producer against the previous per-line derivation.
func TestWrapTextLinesMatchesLegacyDerivation(t *testing.T) {
	corpus := []string{
		"",
		" ",
		"alpha bravo charlie delta",
		"one  two   three",
		"alpha bravo\r\ncharlie delta",
		"a\tb",
		"\x1b[31mred text that wraps\x1b[0m",
		"界界界界界",
		"supercalifragilisticexpialidocious trailer",
	}
	modes := []core.TextWrap{
		core.TextWrapWrap, core.TextWrapTrim, core.TextWrapTruncate,
		core.TextWrapTruncateEnd, core.TextWrapTruncateMiddle, core.TextWrapTruncateStart,
		core.TextWrap("unknown"),
	}
	for _, text := range corpus {
		for _, mode := range modes {
			for _, width := range []int{-1, 0, 2, 5, 9, 64} {
				gotLines, gotSoft := WrapTextLines(text, width, mode)
				wantLines, wantSoft := wrapTextLinesLegacy(text, width, mode)
				if !reflect.DeepEqual(gotLines, wantLines) || !reflect.DeepEqual(gotSoft, wantSoft) {
					t.Fatalf("WrapTextLines drift: text=%q mode=%q width=%d\n got=%q %v\nwant=%q %v",
						text, mode, width, gotLines, gotSoft, wantLines, wantSoft)
				}
				// The soft bitmap has to stay aligned with the rows, which is what
				// fullscreen selection indexes.
				if len(gotSoft) != len(gotLines) {
					t.Fatalf("soft bitmap misaligned: %d rows vs %d flags", len(gotLines), len(gotSoft))
				}
			}
		}
	}
}

// TestRestoreVisualStateSkipsEscapeFreeRows pins the fast path that removed a full
// re-tokenization pass per row: escape-free rows are returned as they are, while
// rows carrying escapes still get their inherited state reopened.
func TestRestoreVisualStateSkipsEscapeFreeRows(t *testing.T) {
	rows := []string{"plain one", "plain two", "plain three"}
	got := restoreVisualStateAcrossRows(rows)
	if len(got) != len(rows) || &got[0] != &rows[0] {
		t.Fatalf("escape-free rows were rebuilt: %q (same backing = %v)", got, len(got) == len(rows) && &got[0] == &rows[0])
	}

	// The pass closes the state at the end of a non-final row and reopens it on
	// the next one, so an escape row must keep that behaviour.
	styled := []string{"\x1b[31mred", "still red"}
	want := []string{"\x1b[31mred\x1b[0m", "\x1b[31mstill red"}
	if got := restoreVisualStateAcrossRows(styled); !reflect.DeepEqual(got, want) {
		t.Fatalf("styled rows lost their inherited state: got=%q want=%q", got, want)
	}
}
