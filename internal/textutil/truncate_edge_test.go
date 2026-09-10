package textutil

import (
	core "github.com/frudas24/inkgo/internal/core"
	"testing"
)

func TestTruncateSingleColumnPreservesFittingText(t *testing.T) {
	for _, mode := range []core.TextWrap{core.TextWrapTruncateEnd, core.TextWrapTruncateStart, core.TextWrapTruncateMiddle} {
		for _, input := range []string{"", "a", "e\u0301", "\x1b[31ma\x1b[0m"} {
			if got := TruncateText(input, 1, mode); got != input {
				t.Errorf("mode=%s input=%q got=%q", mode, input, got)
			}
		}
		if got := TruncateText("界", 1, mode); got != Ellipsis {
			t.Fatalf("wide glyph not truncated: %q", got)
		}
	}
}
