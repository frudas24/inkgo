package textutil

import (
	"strings"
	"testing"
	"unicode/utf8"

	core "github.com/frudas24/inkgo/internal/core"
)

func FuzzWrapTextInvariants(f *testing.F) {
	for _, seed := range []string{
		"hello world",
		"one   two",
		"a\tb",
		"界界界",
		"👨‍👩‍👧‍👦 hello",
		"☀\u200d☀\u200d☀",
		"♥\u200d♥\u200d♥",
		"🇦\u200d🇧",
		"🇦\u200d🇧\u200d🇨",
		"🇦🇧🇨",
		"\x1b[31mhello world\x1b[0m",
		"\x1b]8;;https://example.com\x07link text\x1b]8;;\x07",
		"a\r\nb",
	} {
		f.Add(seed, uint8(8), false)
		f.Add(seed, uint8(5), true)
	}
	f.Fuzz(func(t *testing.T, input string, rawWidth uint8, trim bool) {
		width := 2 + int(rawWidth%63)
		mode := core.TextWrapWrap
		if trim {
			mode = core.TextWrapTrim
		}
		for _, g := range Graphemes(StripANSI(input)) {
			if g.Width < 0 || g.Width > 2 {
				t.Fatalf("grapheme width outside screen model: input=%q grapheme=%q width=%d", input, g.Text, g.Width)
			}
		}

		got := WrapText(input, width, mode)
		if !utf8.ValidString(got) {
			t.Fatalf("wrap returned invalid UTF-8: input=%q output=%q", input, got)
		}
		for _, line := range strings.Split(got, "\n") {
			if w := StringWidth(line); w > width {
				t.Fatalf("line width %d > %d: input=%q output=%q line=%q", w, width, input, got, line)
			}
			if trim {
				plain := StripANSI(line)
				if strings.HasPrefix(plain, " ") || strings.HasSuffix(plain, " ") {
					t.Fatalf("trim left visible spaces: input=%q output=%q line=%q", input, got, line)
				}
			}
		}

		visible := StringWidth(input)
		if visible > 0 {
			start := visible / 3
			end := min(visible, start+width)
			slice := SliceByWidth(input, start, end)
			if !utf8.ValidString(slice) {
				t.Fatalf("slice returned invalid UTF-8: input=%q slice=%q", input, slice)
			}
			if w := StringWidth(slice); w > end-start {
				t.Fatalf("slice width %d > %d: input=%q slice=%q", w, end-start, input, slice)
			}
		}

		truncated := TruncateText(input, width, core.TextWrapTruncateEnd)
		if !utf8.ValidString(truncated) {
			t.Fatalf("truncate returned invalid UTF-8: input=%q output=%q", input, truncated)
		}
		if w := StringWidth(truncated); w > width {
			t.Fatalf("truncate width %d > %d: input=%q output=%q", w, width, input, truncated)
		}
	})
}
