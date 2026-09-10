package ink

// HasRTLCharacters is the same fast gate used by the TypeScript fork.
func HasRTLCharacters(s string) bool {
	for _, r := range s {
		if isRTL(r) {
			return true
		}
	}
	return false
}
func isRTL(r rune) bool {
	return (r >= 0x0590 && r <= 0x05ff) || (r >= 0xfb1d && r <= 0xfb4f) || (r >= 0x0600 && r <= 0x06ff) || (r >= 0x0750 && r <= 0x077f) || (r >= 0x08a0 && r <= 0x08ff) || (r >= 0xfb50 && r <= 0xfdff) || (r >= 0xfe70 && r <= 0xfeff) || (r >= 0x0780 && r <= 0x07bf) || (r >= 0x0700 && r <= 0x074f)
}

// ReorderBidiGraphemes provides the software RTL fallback needed by Windows
// Terminal/conhost/xterm.js. It preserves grapheme clusters and reverses
// contiguous RTL runs. The TS fork delegates embedding-level resolution to
// bidi-js; complex nested embeddings are listed in PORT_STATUS.md as the one
// place where an exact external Unicode-Bidi implementation can improve parity.
func ReorderBidiGraphemes(gs []Grapheme) []Grapheme {
	if !NeedsSoftwareBidi() || len(gs) == 0 {
		return gs
	}
	has := false
	for _, g := range gs {
		for _, r := range g.Text {
			if isRTL(r) {
				has = true
				break
			}
		}
	}
	if !has {
		return gs
	}
	out := append([]Grapheme(nil), gs...)
	for i := 0; i < len(out); {
		rtl := false
		for _, r := range out[i].Text {
			if isRTL(r) {
				rtl = true
				break
			}
		}
		if !rtl {
			i++
			continue
		}
		j := i + 1
		for j < len(out) {
			next := false
			for _, r := range out[j].Text {
				if isRTL(r) {
					next = true
					break
				}
			}
			if !next {
				break
			}
			j++
		}
		for a, b := i, j-1; a < b; a, b = a+1, b-1 {
			out[a], out[b] = out[b], out[a]
		}
		i = j
	}
	return out
}

func ReorderBidiStyled(gs []StyledGrapheme) []StyledGrapheme {
	if !NeedsSoftwareBidi() || len(gs) == 0 {
		return gs
	}
	has := false
	for _, g := range gs {
		for _, r := range g.Value {
			if isRTL(r) {
				has = true
				break
			}
		}
	}
	if !has {
		return gs
	}
	out := append([]StyledGrapheme(nil), gs...)
	for i := 0; i < len(out); {
		rtl := false
		for _, r := range out[i].Value {
			if isRTL(r) {
				rtl = true
				break
			}
		}
		if !rtl {
			i++
			continue
		}
		j := i + 1
		for j < len(out) {
			next := false
			for _, r := range out[j].Value {
				if isRTL(r) {
					next = true
					break
				}
			}
			if !next {
				break
			}
			j++
		}
		for a, b := i, j-1; a < b; a, b = a+1, b-1 {
			out[a], out[b] = out[b], out[a]
		}
		i = j
	}
	return out
}
