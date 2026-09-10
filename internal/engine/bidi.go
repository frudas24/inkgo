package engine

import "unicode"

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
	return (r >= 0x0590 && r <= 0x05ff) || (r >= 0xfb1d && r <= 0xfb4f) ||
		(r >= 0x0600 && r <= 0x06ff) || (r >= 0x0750 && r <= 0x077f) ||
		(r >= 0x08a0 && r <= 0x08ff) || (r >= 0xfb50 && r <= 0xfdff) ||
		(r >= 0xfe70 && r <= 0xfeff) || (r >= 0x0780 && r <= 0x07bf) ||
		(r >= 0x0700 && r <= 0x074f)
}

type bidiClass uint8

const (
	bidiNeutral bidiClass = iota
	bidiLTR
	bidiRTL
	bidiNumber
)

func classifyBidiCluster(s string) bidiClass {
	class := bidiNeutral
	for _, r := range s {
		if isRTL(r) {
			return bidiRTL
		}
		if unicode.IsDigit(r) {
			if class == bidiNeutral {
				class = bidiNumber
			}
			continue
		}
		if unicode.IsLetter(r) {
			return bidiLTR
		}
	}
	return class
}

// bidiLevelsLTR computes practical UAX #9-compatible embedding levels for the
// terminal's fixed LTR paragraph direction. RTL strong text gets level 1;
// numbers in RTL context get level 2 so their internal order survives the
// level-1 reversal; neutrals inherit RTL only when both surrounding contexts
// are RTL. This handles the mixed-script cases that matter in terminals while
// keeping the implementation dependency-free.
func bidiLevelsLTR(values []string) []int {
	levels := make([]int, len(values))
	classes := make([]bidiClass, len(values))
	contexts := make([]int, len(values))
	lastStrong := 0
	for i, value := range values {
		c := classifyBidiCluster(value)
		classes[i] = c
		switch c {
		case bidiRTL:
			lastStrong = 1
			contexts[i] = 1
			levels[i] = 1
		case bidiLTR:
			lastStrong = 0
			contexts[i] = 0
			levels[i] = 0
		case bidiNumber:
			contexts[i] = lastStrong
			if lastStrong == 1 {
				levels[i] = 2
			}
		case bidiNeutral:
			contexts[i] = -1
			levels[i] = 0
		}
	}

	// Resolve neutral runs from their surrounding resolved contexts. If the
	// directions disagree (or a run touches a paragraph edge), base LTR wins.
	for i := 0; i < len(values); {
		if classes[i] != bidiNeutral {
			i++
			continue
		}
		j := i + 1
		for j < len(values) && classes[j] == bidiNeutral {
			j++
		}
		left, right := 0, 0
		leftOK, rightOK := false, false
		for k := i - 1; k >= 0; k-- {
			if contexts[k] >= 0 {
				left, leftOK = contexts[k], true
				break
			}
		}
		for k := j; k < len(values); k++ {
			if contexts[k] >= 0 {
				right, rightOK = contexts[k], true
				break
			}
		}
		if leftOK && rightOK && left == 1 && right == 1 {
			for k := i; k < j; k++ {
				levels[k] = 1
				contexts[k] = 1
			}
		}
		i = j
	}
	return levels
}

func reorderByBidiLevels[T any](items []T, values func(T) string) []T {
	if len(items) == 0 {
		return items
	}
	plain := make([]string, len(items))
	hasRTL := false
	for i, item := range items {
		plain[i] = values(item)
		if !hasRTL && HasRTLCharacters(plain[i]) {
			hasRTL = true
		}
	}
	if !hasRTL {
		return items
	}
	levels := bidiLevelsLTR(plain)
	maxLevel := 0
	for _, level := range levels {
		if level > maxLevel {
			maxLevel = level
		}
	}
	out := append([]T(nil), items...)
	for level := maxLevel; level >= 1; level-- {
		for i := 0; i < len(out); {
			if levels[i] < level {
				i++
				continue
			}
			j := i + 1
			for j < len(out) && levels[j] >= level {
				j++
			}
			for a, b := i, j-1; a < b; a, b = a+1, b-1 {
				out[a], out[b] = out[b], out[a]
				levels[a], levels[b] = levels[b], levels[a]
			}
			i = j
		}
	}
	return out
}

// ReorderBidiGraphemes provides the software RTL fallback needed by Windows
// Terminal/conhost/xterm.js. It preserves grapheme clusters and resolves mixed
// RTL + numeric runs before applying the standard descending-level reversal.
func ReorderBidiGraphemes(gs []Grapheme) []Grapheme {
	if !NeedsSoftwareBidi() {
		return gs
	}
	return reorderByBidiLevels(gs, func(g Grapheme) string { return g.Text })
}

func ReorderBidiStyled(gs []StyledGrapheme) []StyledGrapheme {
	if !NeedsSoftwareBidi() {
		return gs
	}
	return reorderByBidiLevels(gs, func(g StyledGrapheme) string { return g.Value })
}
