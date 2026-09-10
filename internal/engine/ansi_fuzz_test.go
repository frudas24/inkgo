package engine

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func FuzzParseANSIInvariants(f *testing.F) {
	for _, seed := range []string{
		"plain",
		"\x1b[31mred\x1b[0m",
		"\x1b]8;;https://example.com\x07link\x1b]8;;\x07",
		"\x1b(0abc\x1b(B",
		"\x1bPpayload\x1b\\text",
		"\x1b^private\x1b\\text",
		"\x1b[31",
		string([]byte{'a', 0xff, 'b'}),
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input string) {
		items := ParseANSI(input, TextStyle{})
		var visible strings.Builder
		for _, item := range items {
			if !utf8.ValidString(item.Value) {
				t.Fatalf("invalid UTF-8 grapheme: input=%q value=%q", input, item.Value)
			}
			if strings.Contains(item.Value, ESC) {
				t.Fatalf("raw ESC leaked as visible grapheme: input=%q value=%q", input, item.Value)
			}
			if item.Width != StringWidth(item.Value) {
				t.Fatalf("width mismatch: input=%q value=%q got=%d want=%d", input, item.Value, item.Width, StringWidth(item.Value))
			}
			visible.WriteString(item.Value)
		}
		want := strings.ReplaceAll(StripANSI(input), "\r", "")
		if visible.String() != want {
			t.Fatalf("visible mismatch: input=%q parsed=%q stripped=%q", input, visible.String(), want)
		}
	})
}
