package textutil

import "testing"

func TestGB11RequiresPictographImmediatelyAfterZWJ(t *testing.T) {
	invalid := []string{
		"☀\u200d\u0301☀",     // Extend after ZWJ invalidates the GB11 opportunity.
		"☀\u200d\ufe0f☀",     // VS16 is Extend, but Extend* belongs before ZWJ.
		"☀\u200d\U0001F3FB☀", // Emoji_Modifier is Extend and also invalid here.
		"☀\u200d\u200d☀",     // A second ZWJ cannot inherit the first ZWJ's base.
	}
	for _, in := range invalid {
		gs := Graphemes(in)
		if len(gs) != 2 {
			t.Errorf("Graphemes(%q)=%#v, want 2 clusters", in, gs)
		}
		for _, g := range gs {
			if g.Width < 0 || g.Width > 2 {
				t.Fatalf("Graphemes(%q) emitted unrepresentable width: %#v", in, gs)
			}
		}
	}

	valid := []string{
		"☀\u0301\u200d☀", // Extend before ZWJ is the GB11 Extend* position.
		"☀\ufe0f\u200d☀",
		"👨\U0001F3FD\u200d👩",
		"👨\u200d👩\u200d👧\u200d👦",
	}
	for _, in := range valid {
		gs := Graphemes(in)
		if len(gs) != 1 || gs[0].Width != 2 {
			t.Errorf("Graphemes(%q)=%#v, want one 2-cell cluster", in, gs)
		}
	}
}

func TestKeycapRequiresExactSequenceShape(t *testing.T) {
	for _, base := range []rune("#*0123456789") {
		valid := []string{
			string([]rune{base, 0x20e3}),
			string([]rune{base, 0xfe0f, 0x20e3}),
		}
		for _, in := range valid {
			if got := StringWidth(in); got != 2 {
				t.Errorf("valid keycap StringWidth(%q)=%d want 2", in, got)
			}
		}

		invalid := []string{
			string([]rune{base, 0x0301, 0x20e3}),
			string([]rune{base, 0xfe0e, 0x20e3}),
			string([]rune{base, 0xfe0f, 0x0301, 0x20e3}),
			string([]rune{base, 0xfe0f, 0xfe0f, 0x20e3}),
		}
		for _, in := range invalid {
			if got := StringWidth(in); got != 1 {
				t.Errorf("invalid keycap StringWidth(%q)=%d want 1", in, got)
			}
		}
	}
}

func TestVariationSelectorMustBeRegisteredAndAdjacent(t *testing.T) {
	cases := map[string]int{
		"©\ufe0e":       1,
		"©\ufe0f":       2,
		"©\u0301\ufe0f": 1, // FE0F no longer follows the registered base.
		"⌚\ufe0e":       1,
		"⌚\u0301\ufe0e": 2, // Invalid FE0E must not narrow emoji-default WATCH.
		"😀\ufe0e":       2, // U+1F600 has no registered emoji variation sequence.
		"😀\ufe0f":       2,
		"⌘\ufe0e":       1,
		"⌘\ufe0f":       1,
	}
	for in, want := range cases {
		if got := StringWidth(in); got != want {
			t.Errorf("StringWidth(%q)=%d want %d", in, got, want)
		}
	}
}

func TestUnicode17EmojiVariationBaseTable(t *testing.T) {
	count := 0
	for i, rg := range emojiVariationBaseRanges {
		if rg.lo > rg.hi {
			t.Fatalf("variation range %d inverted: %U..%U", i, rg.lo, rg.hi)
		}
		if i > 0 && emojiVariationBaseRanges[i-1].hi >= rg.lo {
			t.Fatalf("variation ranges overlap/out of order: %U..%U then %U..%U",
				emojiVariationBaseRanges[i-1].lo, emojiVariationBaseRanges[i-1].hi, rg.lo, rg.hi)
		}
		count += int(rg.hi-rg.lo) + 1
		for r := rg.lo; r <= rg.hi; r++ {
			if !isEmojiCandidateRune(r) {
				t.Fatalf("emoji variation base must have Emoji=Yes: %U", r)
			}
		}
	}
	if count != 371 {
		t.Fatalf("emoji variation base table has %d code points, want 371", count)
	}
}
