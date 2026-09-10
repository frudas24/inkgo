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

func TestNextANSISequenceRejectsInputMetaAndControls(t *testing.T) {
	for _, in := range []string{"\x1b\t0", "\x1bé", "\x1b\x00"} {
		if _, ok := NextANSISequence(in); ok {
			t.Fatalf("unexpected ANSI sequence for %q", in)
		}
	}
	for _, in := range []string{"\x1b[31mrest", "\x1b]0;title\x07rest", "\x1bPabc\x1b\\rest", "\x1b0rest"} {
		if seq, ok := NextANSISequence(in); !ok || seq == "" {
			t.Fatalf("failed ANSI %q -> %q,%v", in, seq, ok)
		}
	}
}
