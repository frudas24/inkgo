package textutil

import (
	"strings"
	"testing"

	core "github.com/frudas24/inkgo/internal/core"
)

func TestWidthsGraphemesAndANSI(t *testing.T) {
	if StringWidth("abc") != 3 || StringWidth("界") != 2 || StringWidth("e\u0301") != 1 {
		t.Fatal("width")
	}
	if StringWidth("a\tb") != 2 {
		t.Fatal("tabs are controls until ExpandTabs")
	}
	if StringWidth("\x1b[31mred\x1b[0m") != 3 {
		t.Fatal("ansi width")
	}
	if got := WidestLine("a\n界x"); got != 3 {
		t.Fatalf("widest=%d", got)
	}
	if got := SliceByWidth("a界b", 1, 3); got != "界" {
		t.Fatalf("slice=%q", got)
	}
	if gs := Graphemes("👨‍👩‍👧‍👦"); len(gs) != 1 || gs[0].Width != 2 {
		t.Fatalf("emoji=%+v", gs)
	}
}

func TestTabsWrapTruncateAndMeasure(t *testing.T) {
	if got := ExpandTabs("a\tb", 0); got != "a       b" {
		t.Fatalf("tabs=%q", got)
	}
	if got := ExpandTabs("\x1b[31ma\tb", 4); got != "\x1b[31ma   b" {
		t.Fatalf("ansi tabs=%q", got)
	}
	if got := TruncateText("abcdef", 4, core.TextWrapTruncateEnd); got != "abc…" {
		t.Fatalf("truncate=%q", got)
	}
	if got := TruncateText("abcdef", 4, core.TextWrapTruncateStart); got != "…def" {
		t.Fatalf("start=%q", got)
	}
	if got := TruncateText("abcdef", 5, core.TextWrapTruncateMiddle); StringWidth(got) != 5 {
		t.Fatalf("middle=%q", got)
	}
	if got := WrapText("helloworld", 5, core.TextWrapWrap); got != "hello\nworld" {
		t.Fatalf("wrap=%q", got)
	}
	lines, soft := WrapTextLines("helloworld", 5, core.TextWrapWrap)
	if len(lines) != 2 || len(soft) != 2 || !soft[1] {
		t.Fatalf("lines=%v soft=%v", lines, soft)
	}
	if got := MeasureText("helloworld", 5, core.TextWrapWrap); got.Width != 5 || got.Height != 2 {
		t.Fatalf("measure=%+v", got)
	}
}

// ZWJ only joins clusters when UAX #29 GB11 applies, i.e. an
// Extended_Pictographic base before it. Digits are not pictographic, so a
// digit ZWJ run must split: fusing it produced a single cluster 3 cells wide,
// which the two-cell model cannot render and wrapping cannot break.
func TestZWJJoiningRequiresPictographicBase(t *testing.T) {
	digits := "0\u200d0\u200d0"
	gs := Graphemes(digits)
	if len(gs) != 3 {
		t.Fatalf("digit ZWJ run graphemes=%d (%v), want 3", len(gs), gs)
	}
	for _, g := range gs {
		if g.Width != 1 {
			t.Fatalf("digit ZWJ cluster %q width=%d, want 1", g.Text, g.Width)
		}
	}
	if got := StringWidth(digits); got != 3 {
		t.Fatalf("digit ZWJ run width=%d, want 3", got)
	}
	// GB9 keeps the trailing ZWJ with its base, so the two one-cell clusters
	// "0\u200d" fill the first row exactly.
	if got := WrapText(digits, 2, core.TextWrapTrim); got != "0\u200d0\u200d\n0" {
		t.Fatalf("digit ZWJ wrap=%q", got)
	}

	// Text-default pictographs are Extended_Pictographic too. GB11 keeps a
	// ZWJ chain as one grapheme even without VS16; keep that grapheme within
	// the screen model's two-cell maximum instead of accumulating one cell per
	// pictograph (which made wrapping unable to satisfy its width invariant).
	for _, in := range []string{
		"☀\u200d☀\u200d☀",
		"♥\u200d♥\u200d♥",
		"⚠\u200d⚠\u200d⚠",
		"©\u200d©\u200d©",
	} {
		gs := Graphemes(in)
		if len(gs) != 1 || gs[0].Width != 2 {
			t.Fatalf("text-default pictographic ZWJ %q graphemes=%v, want one 2-cell cluster", in, gs)
		}
		if got := WrapText(in, 2, core.TextWrapTrim); StringWidth(got) > 2 {
			t.Fatalf("text-default pictographic ZWJ wrap=%q width=%d, want <=2", got, StringWidth(got))
		}
	}

	// Regional indicators are handled by UAX #29 GB12/GB13, not GB11. A ZWJ
	// therefore must not turn separate RI symbols into an emoji ZWJ sequence.
	// GB9 still keeps each ZWJ with the preceding RI. Plain RI pairs remain a
	// single two-cell flag cluster.
	for _, tc := range []struct {
		in   string
		want []Grapheme
	}{
		{"🇦\u200d🇧", []Grapheme{{Text: "🇦\u200d", Width: 1}, {Text: "🇧", Width: 1}}},
		{"🇦\u200d🇧\u200d🇨", []Grapheme{{Text: "🇦\u200d", Width: 1}, {Text: "🇧\u200d", Width: 1}, {Text: "🇨", Width: 1}}},
		{"🇦🇧", []Grapheme{{Text: "🇦🇧", Width: 2}}},
	} {
		gs := Graphemes(tc.in)
		if len(gs) != len(tc.want) {
			t.Fatalf("regional-indicator graphemes %q=%v, want %v", tc.in, gs, tc.want)
		}
		for i := range gs {
			if gs[i] != tc.want[i] {
				t.Fatalf("regional-indicator grapheme %q[%d]=%v, want %v", tc.in, i, gs[i], tc.want[i])
			}
		}
	}

	// Real emoji ZWJ sequences must still collapse to one two-cell cluster.
	for _, in := range []string{
		"👨\u200d👩\u200d👧\u200d👦", // family
		"🧑\u200d💻",               // technologist
		"👨🏽\u200d👩",              // skin tone between base and ZWJ
	} {
		gs := Graphemes(in)
		if len(gs) != 1 || gs[0].Width != 2 {
			t.Fatalf("emoji ZWJ %q graphemes=%v, want one 2-cell cluster", in, gs)
		}
	}
}

func TestStripANSIControlFamiliesAndInvalidUTF8(t *testing.T) {
	in := "a\x1b[31mb\x1b[0m" +
		"\x1b]8;;https://example.com\x07c\x1b]8;;\x1b\\" +
		"\x1bPignored\x1b\\d\x1b_hidden\x1b\\e\x1b(0f"
	if got := StripANSI(in); got != "abcdef" {
		t.Fatalf("StripANSI=%q", got)
	}
	if got := StripANSI(string([]byte{'x', 0xff, 'y'})); got != "xy" {
		t.Fatalf("invalid UTF-8 strip=%q", got)
	}
}

func TestWrapTrimZeroWidthAndWideGlyphEdges(t *testing.T) {
	if got := WrapText("one   two", 5, core.TextWrapTrim); got != "one\ntwo" {
		t.Fatalf("wrap trim=%q", got)
	}
	if got := WrapText("abc", -1, core.TextWrapTruncateEnd); got != "" {
		t.Fatalf("negative truncate=%q", got)
	}
	if got := TruncateText("abc", 1, core.TextWrapTruncateEnd); got != Ellipsis {
		t.Fatalf("one-column truncate=%q", got)
	}
	if got := WrapText("界", 1, core.TextWrapWrap); got != "界" {
		t.Fatalf("wide glyph wrap=%q", got)
	}
	if got := WrapText("abc", 2, core.TextWrap("unknown")); got != "abc" {
		t.Fatalf("unknown mode=%q", got)
	}
	lines, soft := WrapTextLines("a\nb", 2, core.TextWrapTruncateEnd)
	if len(lines) != 2 || len(soft) != 2 || soft[0] || soft[1] {
		t.Fatalf("truncate lines=%v soft=%v", lines, soft)
	}
}

func TestWrapAndTruncatePreserveANSISequences(t *testing.T) {
	red := "\x1b[31mhello world\x1b[0m"
	wrapped := WrapText(red, 5, core.TextWrapTrim)
	if got := StripANSI(wrapped); got != "hello\nworld" {
		t.Fatalf("ANSI wrap visible=%q raw=%q", got, wrapped)
	}
	if strings.Contains(wrapped, "\x1b[31\n") || strings.Contains(wrapped, "\x1b[0\n") {
		t.Fatalf("ANSI escape was split: %q", wrapped)
	}
	truncated := TruncateText("\x1b[32mabcdef\x1b[0m", 4, core.TextWrapTruncateEnd)
	if got := StripANSI(truncated); got != "abc…" {
		t.Fatalf("ANSI truncate visible=%q raw=%q", got, truncated)
	}
	if StringWidth(truncated) != 4 {
		t.Fatalf("ANSI truncate width=%d raw=%q", StringWidth(truncated), truncated)
	}
}

func TestWrapUsesEightColumnTabsAndNormalizesCRLF(t *testing.T) {
	if got := StripANSI(WrapText("a\tb", 8, core.TextWrapWrap)); got != "a       \nb" {
		t.Fatalf("tab wrap=%q", got)
	}
	if got := WrapText("a\r\nb", 8, core.TextWrapWrap); got != "a\nb" {
		t.Fatalf("CRLF normalize=%q", got)
	}
}

func TestMalformedUTF8AndIncompleteEscapeStayConsistent(t *testing.T) {
	invalid := string([]byte{'a', 0xff, 'b'})
	if got := StripANSI(invalid); got != "ab" {
		t.Fatalf("StripANSI malformed UTF-8 = %q", got)
	}
	if got := StringWidth(invalid); got != 2 {
		t.Fatalf("StringWidth malformed UTF-8 = %d", got)
	}
	if got := WrapText(invalid, 1, core.TextWrapWrap); got != "a\nb" {
		t.Fatalf("WrapText malformed UTF-8 = %q", got)
	}
	if got := WrapText("\x1b\t0", 7, core.TextWrapWrap); got != "       \n 0" {
		t.Fatalf("WrapText stray ESC+TAB = %q", got)
	}
	if got := StripANSI("a\x1b[31"); got != "a" {
		t.Fatalf("StripANSI incomplete CSI = %q", got)
	}
}

func TestSliceAndTruncateCloseANSIState(t *testing.T) {
	red := "\x1b[31mabcdef\x1b[0m"
	sliced := SliceByWidth(red, 0, 3)
	if sliced != "\x1b[31mabc\x1b[0m" {
		t.Fatalf("styled slice leaked state: %q", sliced)
	}
	truncated := TruncateText(red, 4, core.TextWrapTruncateEnd)
	if !strings.HasSuffix(truncated, "\x1b[0m…") && !strings.HasSuffix(truncated, "…\x1b[0m") {
		t.Fatalf("styled truncation missing reset: %q", truncated)
	}
	if got := StripANSI(truncated); got != "abc…" {
		t.Fatalf("styled truncation visible=%q", got)
	}

	link := "\x1b]8;;https://example.com\x07abcdef\x1b]8;;\x07"
	linkSlice := SliceByWidth(link, 0, 3)
	if !strings.HasSuffix(linkSlice, "\x1b]8;;\x07") {
		t.Fatalf("hyperlink slice missing close: %q", linkSlice)
	}
	if got := StripANSI(linkSlice); got != "abc" {
		t.Fatalf("hyperlink slice visible=%q", got)
	}
}

func TestSliceBoundaryReplaysOnlyVisualANSIState(t *testing.T) {
	in := "\x1b[2J\x1b[31mabcdef\x1b[0m"
	got := SliceByWidth(in, 2, 4)
	if strings.Contains(got, "\x1b[2J") {
		t.Fatalf("slice replayed non-visual control: %q", got)
	}
	if !strings.HasPrefix(got, "\x1b[31m") || !strings.HasSuffix(got, "\x1b[0m") {
		t.Fatalf("slice did not reconstruct color state: %q", got)
	}
	if visible := StripANSI(got); visible != "cd" {
		t.Fatalf("slice visible=%q", visible)
	}
}

func TestWrapRowsRestoreANSIState(t *testing.T) {
	red := "\x1b[31mhello world\x1b[0m"
	rows := strings.Split(WrapText(red, 5, core.TextWrapTrim), "\n")
	if len(rows) != 2 {
		t.Fatalf("rows=%q", rows)
	}
	if rows[0] != "\x1b[31mhello\x1b[0m" {
		t.Fatalf("first wrapped row not self-contained: %q", rows[0])
	}
	if rows[1] != "\x1b[31mworld\x1b[0m" {
		t.Fatalf("second wrapped row did not reopen style: %q", rows[1])
	}

	link := "\x1b]8;;https://example.com\x07hello world\x1b]8;;\x07"
	rows = strings.Split(WrapText(link, 5, core.TextWrapTrim), "\n")
	if len(rows) != 2 || !strings.HasSuffix(rows[0], "\x1b]8;;\x07") || !strings.HasPrefix(rows[1], "\x1b]8;;https://example.com\x07") {
		t.Fatalf("hyperlink state not restored across rows: %q", rows)
	}
	for _, row := range rows {
		if StringWidth(row) != 5 {
			t.Fatalf("row width changed by state restoration: %q", row)
		}
	}
}

func TestEmojiPresentationWidthMatchesForkPolicy(t *testing.T) {
	cases := map[string]int{
		"⚠":       1,
		"⚠️":      2,
		"♥":       1,
		"♥️":      2,
		"☀":       1,
		"☀️":      2,
		"☺":       1,
		"☺️":      2,
		"✅":       2,
		"1️":      1, // incomplete keycap: digit + VS16 only
		"1️⃣":     2,
		"🇨🇴":      2,
		"👨‍👩‍👧‍👦": 2,
	}
	for input, want := range cases {
		if got := StringWidth(input); got != want {
			t.Errorf("StringWidth(%q)=%d want %d", input, got, want)
		}
	}
}

func TestTerminalControlsDoNotMergeIntoNeighboringGraphemes(t *testing.T) {
	gs := Graphemes("a\x12b")
	if len(gs) != 3 || gs[0].Text != "a" || gs[1].Text != "\x12" || gs[1].Width != 0 || gs[2].Text != "b" {
		t.Fatalf("control graphemes=%#v", gs)
	}
	gs = Graphemes("\r\x12")
	if len(gs) != 2 || gs[0].Text != "\r" || gs[1].Text != "\x12" {
		t.Fatalf("CR/control graphemes=%#v", gs)
	}
	gs = Graphemes("\r\u0616")
	if len(gs) != 2 || gs[0].Text != "\r" || gs[1].Text != "\u0616" {
		t.Fatalf("control/combining graphemes=%#v", gs)
	}
}
