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
