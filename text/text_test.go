package text

import "testing"

func TestPublicTextDomain(t *testing.T) {
	if StringWidth("A界") != 3 {
		t.Fatal("width")
	}
	if got := WrapText("abcdef", 3, Wrap); got != "abc\ndef" {
		t.Fatalf("wrap=%q", got)
	}
	if len(ParseANSI("\x1b[31mx\x1b[0m", TextStyle{})) == 0 {
		t.Fatal("ansi")
	}
	if !HasRTLCharacters("שלום") {
		t.Fatal("rtl")
	}
}
