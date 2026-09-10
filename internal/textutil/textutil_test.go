package textutil

import (
	core "github.com/frudas24/inkgo/internal/core"
	"testing"
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
