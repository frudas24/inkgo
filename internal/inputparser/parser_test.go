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
