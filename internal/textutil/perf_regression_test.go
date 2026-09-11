package textutil

import (
	"strings"
	"testing"
	"time"

	core "github.com/frudas24/inkgo/internal/core"
)

// ExpandTabs used to segment the remainder of the plain span once per grapheme and
// keep only the first cluster, which made a single long span quadratic: 32 KB took
// about 64 s and 512 KB would have taken hours. The bounds below are orders of
// magnitude above the linear cost (~0.3 s and ~0.6 s on the validation host) and
// orders of magnitude below the quadratic one, so they fail the old implementation
// without being sensitive to a loaded machine.
func TestExpandTabsStaysLinearOnLongSpans(t *testing.T) {
	plain := strings.Repeat("v", 256*1024)
	in := "\t" + plain
	start := time.Now()
	out := ExpandTabs(in, 8)
	elapsed := time.Since(start)
	if elapsed > 30*time.Second {
		t.Fatalf("ExpandTabs took %s for a 256 KB span: the walk is not linear", elapsed)
	}
	if want := 8 + len(plain); len(out) != want {
		t.Fatalf("expanded length = %d, want %d", len(out), want)
	}
	if strings.Contains(out, "\t") {
		t.Fatal("expanded output still contains a tab")
	}
}

// The user-visible path: one long line mixing ANSI, a tab and plain text. This is the
// shape that killed the fuzz worker before the fix.
func TestWrapTextStaysLinearOnLongANSISpan(t *testing.T) {
	in := "\x1b[31m\t" + strings.Repeat("v", 128*1024) + "\x1b[0m0"
	start := time.Now()
	out := WrapText(in, 2, core.TextWrapTrim)
	elapsed := time.Since(start)
	if elapsed > 30*time.Second {
		t.Fatalf("WrapText took %s for a 128 KB line: wrapping is not linear", elapsed)
	}
	if StripANSI(out) == "" {
		t.Fatal("wrapped output lost its visible text")
	}
	if got := WidestLine(out); got > 2 {
		t.Fatalf("wrapped output exceeds the requested width: %d", got)
	}
}
