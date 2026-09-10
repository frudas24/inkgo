package textutil

import (
	"strings"
	"testing"

	core "github.com/frudas24/inkgo/internal/core"
)

func FuzzWrapTextInvariants(f *testing.F) {
	for _, seed := range []string{
		"hello world",
		"one   two",
		"a\tb",
		"界界界",
		"👨‍👩‍👧‍👦 hello",
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
		got := WrapText(input, width, mode)
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
	})
}
