package inputparser

import "testing"

func TestFlushEmptyPasteRestoresKeyParsing(t *testing.T) {
	for _, chunks := range [][]string{{pasteStart}, {"\x1b[20", "0~"}} {
		p := NewInputParser()
		for _, chunk := range chunks {
			p.Feed([]byte(chunk))
		}
		if !p.InPaste() {
			t.Fatal("paste mode not entered")
		}
		if got := p.Flush(); len(got) != 0 {
			t.Fatalf("empty incomplete paste emitted data: %+v", got)
		}
		if p.Pending() || p.InPaste() {
			t.Fatal("Flush left empty paste active")
		}
		if got := p.Flush(); len(got) != 0 {
			t.Fatalf("second Flush emitted input: %+v", got)
		}
		got := p.Feed([]byte("a"))
		if len(got) != 1 || got[0].Kind != InputKey || got[0].Key.Name != "a" {
			t.Fatalf("key after Flush: %+v", got)
		}
		got = p.Feed([]byte(pasteStart + "next" + pasteEnd))
		if len(got) != 1 || got[0].Kind != InputPaste || got[0].Paste != "next" {
			t.Fatalf("next paste: %+v", got)
		}
	}
}

func TestCompletedEmptyPasteStillEmitsPasteEvent(t *testing.T) {
	p := NewInputParser()
	got := p.Feed([]byte(pasteStart + pasteEnd))
	if len(got) != 1 || got[0].Kind != InputPaste || got[0].Paste != "" || p.Pending() {
		t.Fatalf("completed empty paste: %+v", got)
	}
}
