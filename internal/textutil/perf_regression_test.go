package textutil

import (
	"strings"
	"testing"
	"time"

	core "github.com/frudas24/inkgo/internal/core"
)

// linearBudget bounds each measured call. It sits two orders of magnitude above the
// linear cost observed on the validation host (~0.3 s and ~0.6 s) and far below the
// quadratic one, so a regression fails inside this window rather than blocking until
// the walk finishes: a 256 KB span cost about 64 s at 32 KB before the fix and would
// have taken roughly 75 minutes here.
const linearBudget = 30 * time.Second

// measureWithin runs fn on its own goroutine and fails the test as soon as budget
// elapses. The call cannot be cancelled, so a regression leaves the goroutine running
// until the test binary exits; failing fast is what matters for CI.
func measureWithin(t *testing.T, budget time.Duration, what string, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	select {
	case <-done:
	case <-time.After(budget):
		t.Fatalf("%s did not finish within %s: the walk is not linear", what, budget)
	}
}

// ExpandTabs used to segment the remainder of the plain span once per grapheme and
// keep only the first cluster, which made a single long span quadratic.
func TestExpandTabsStaysLinearOnLongSpans(t *testing.T) {
	plain := strings.Repeat("v", 256*1024)
	in := "\t" + plain
	var out string
	measureWithin(t, linearBudget, "ExpandTabs on a 256 KB span", func() {
		out = ExpandTabs(in, 8)
	})
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
	var out string
	measureWithin(t, linearBudget, "WrapText on a 128 KB line", func() {
		out = WrapText(in, 2, core.TextWrapTrim)
	})
	if StripANSI(out) == "" {
		t.Fatal("wrapped output lost its visible text")
	}
	if got := WidestLine(out); got > 2 {
		t.Fatalf("wrapped output exceeds the requested width: %d", got)
	}
}
