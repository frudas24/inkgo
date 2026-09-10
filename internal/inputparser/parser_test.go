package inputparser

import (
	"reflect"
	"testing"
)

func TestFragmentedInputMatchesWholeInput(t *testing.T) {
	cases := [][]byte{
		[]byte("hello"),
		[]byte("\x1b[A"),
		[]byte("\x1b[<0;12;4M\x1b[<0;12;4m"),
		[]byte("\x1b[200~hello界\x1b[201~"),
		[]byte("\x1b[97;5u"),
		[]byte("\x1b]11;rgb:ffff/0000/ffff\x07"),
		[]byte("\x1bP>|xterm.js(5.5.0)\x1b\\"),
	}
	for _, data := range cases {
		whole := NewInputParser().Feed(data)
		p := NewInputParser()
		var fragmented []ParsedInput
		for _, b := range data {
			fragmented = append(fragmented, p.Feed([]byte{b})...)
		}
		fragmented = append(fragmented, p.Flush()...)
		if !reflect.DeepEqual(whole, fragmented) {
			t.Fatalf("fragment mismatch for %q\nwhole=%#v\nfragmented=%#v", data, whole, fragmented)
		}
	}
}

func FuzzParserNeverPanics(f *testing.F) {
	seeds := [][]byte{
		{}, []byte("x"), []byte("\x1b"), []byte("\x1b["),
		[]byte("\x1b[200~unterminated"), []byte("\x1b[<999;999;999M"),
		[]byte{0xff, 0xfe, 0x1b, '[', 'A'},
	}
	for _, seed := range seeds {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		p := NewInputParser()
		for i := 0; i < len(data); {
			n := 1 + (i % 7)
			if i+n > len(data) {
				n = len(data) - i
			}
			_ = p.Feed(data[i : i+n])
			i += n
		}
		_ = p.Flush()
	})
}

func TestParserStateKeysMouseAndResponses(t *testing.T) {
	p := NewInputParser()
	if p.Pending() || p.InPaste() || p.Buffer() != "" {
		t.Fatal("new parser state")
	}
	if got := p.Feed([]byte("\x1b[200~partial")); len(got) != 0 || !p.Pending() || !p.InPaste() {
		t.Fatalf("paste state got=%+v", got)
	}
	got := p.Feed([]byte(" paste\x1b[201~"))
	if len(got) != 1 || got[0].Kind != InputPaste || got[0].Paste != "partial paste" || p.Pending() {
		t.Fatalf("paste=%+v pending=%v", got, p.Pending())
	}

	cases := []struct {
		seq  string
		kind InputKind
		name string
	}{
		{"\r", InputKey, "return"}, {"\t", InputKey, "tab"}, {"\x7f", InputKey, "backspace"}, {"\x01", InputKey, "a"},
		{"\x1b", InputKey, "escape"}, {"\x1b[A", InputKey, "up"}, {"\x1b[Z", InputKey, "tab"},
		{"\x1b[1;5A", InputKey, "up"}, {"\x1b[97;5u", InputKey, "a"}, {"\x1b[27;3;97~", InputKey, "a"},
		{"\x1ba", InputKey, "a"}, {"\x1b[<0;2;3M", InputMouse, ""}, {"\x1b[<0;2;3m", InputMouse, ""},
		{"\x1b[<64;2;3M", InputKey, "wheelup"}, {"\x1b[<65;2;3M", InputKey, "wheeldown"},
		{"\x1b[I", InputResponse, ""}, {"\x1b[O", InputResponse, ""},
		{"\x1b[?1000;1$y", InputResponse, ""}, {"\x1b[?1;2c", InputResponse, ""}, {"\x1b[>1;2;3c", InputResponse, ""},
		{"\x1b[?3u", InputResponse, ""}, {"\x1b[?4;5R", InputResponse, ""},
		{"\x1b]10;rgb:ffff/0000/0000\x07", InputResponse, ""}, {"\x1bP>|xterm.js(5.5.0)\x1b\\", InputResponse, ""},
	}
	for _, tc := range cases {
		p := NewInputParser()
		out := p.Feed([]byte(tc.seq))
		if len(out) == 0 {
			out = append(out, p.Flush()...)
		}
		if len(out) != 1 || out[0].Kind != tc.kind {
			t.Fatalf("%q => %+v", tc.seq, out)
		}
		if tc.name != "" && out[0].Key.Name != tc.name {
			t.Fatalf("%q name=%q", tc.seq, out[0].Key.Name)
		}
	}

	// X10 press/release and malformed packets.
	for _, seq := range []string{"\x1b[M" + string([]byte{32, 34, 35}), "\x1b[M" + string([]byte{35, 34, 35})} {
		out := NewInputParser().Feed([]byte(seq))
		if len(out) != 1 || out[0].Kind != InputMouse {
			t.Fatalf("x10=%+v", out)
		}
	}
	if parseMouse("\x1b[M\x00\x00\x00") != nil {
		t.Fatal("invalid x10 accepted")
	}

	if got := parseInts("1;x;3"); !reflect.DeepEqual(got, []int{1, 3}) {
		t.Fatalf("parseInts=%v", got)
	}
	if got := keycodeName(57413); got != "+" {
		t.Fatalf("keycode=%q", got)
	}
	if got := keycodeName(999999); got != "" {
		t.Fatalf("unknown keycode=%q", got)
	}
}

func TestFlushIncompleteUTF8AndPaste(t *testing.T) {
	p := NewInputParser()
	_ = p.Feed([]byte("\x1b[200~unterminated"))
	out := p.Flush()
	if len(out) != 1 || out[0].Kind != InputPaste || out[0].Paste != "unterminated" {
		t.Fatalf("flush paste=%+v", out)
	}

	p = NewInputParser()
	_ = p.Feed([]byte{0xff})
	if !p.Pending() {
		t.Fatal("incomplete utf8 should remain pending")
	}
	if got := p.Flush(); len(got) != 1 || got[0].Kind != InputKey {
		t.Fatalf("flush utf8=%+v", got)
	}
}
