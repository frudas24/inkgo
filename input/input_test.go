package input

import "testing"

func TestPublicParser(t *testing.T) {
	p := NewParser()
	got := p.Feed([]byte("a"))
	if len(got) != 1 || got[0].Kind != KeyInput || got[0].Key.Text != "a" {
		t.Fatalf("got=%+v", got)
	}
}
