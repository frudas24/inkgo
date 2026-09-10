package escscan

import "testing"

func TestNextSequenceFamilies(t *testing.T) {
	cases := []string{
		"\x1b[A", "\x1b[<0;10;20M", "\x1b[Mabc", "\x1b]8;;https://x\x07",
		"\x1b]0;title\x1b\\", "\x1bP1;2|payload\x1b\\", "\x1b_payload\x1b\\", "\x1bOP", "\x1bé",
	}
	for _, in := range cases {
		seq, ok := NextSequence(in + "rest")
		if !ok || seq == "" {
			t.Fatalf("failed %q -> %q,%v", in, seq, ok)
		}
	}
	for _, in := range []string{"", "a", "\x1b", "\x1b[", "\x1b]unterminated", "\x1bPunterminated"} {
		if _, ok := NextSequence(in); ok {
			t.Fatalf("unexpected complete %q", in)
		}
	}
}
