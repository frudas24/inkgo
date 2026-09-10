package inkgo

import (
	"bytes"
	"strings"
	"testing"
)

func joinGraphemeText(gs []Grapheme) string {
	var b strings.Builder
	for _, g := range gs {
		b.WriteString(g.Text)
	}
	return b.String()
}

func TestBidiMixedRTLNumbersAndNeutrals(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "vscode")
	in := Graphemes("אבג 123 דהו")
	got := joinGraphemeText(ReorderBidiGraphemes(in))
	if got != "והד 123 גבא" {
		t.Fatalf("mixed bidi = %q", got)
	}

	// A base-LTR paragraph keeps the outer LTR runs in place while moving the
	// numeric RTL segment into visual order.
	got = joinGraphemeText(ReorderBidiGraphemes(Graphemes("left אבג 123 right")))
	if got != "left 123 גבא right" {
		t.Fatalf("ltr/rtl/ltr bidi = %q", got)
	}
}

func TestDetectCapabilitiesProgressAndIdentity(t *testing.T) {
	env := map[string]string{
		"TERM_PROGRAM":         "ghostty",
		"TERM_PROGRAM_VERSION": "1.2.0",
	}
	get := func(k string) string { return env[k] }
	c := detectCapabilities(get, "linux", true, "")
	if !c.ProgressReporting || !c.SynchronizedOutput {
		t.Fatalf("ghostty capabilities = %+v", c)
	}

	env["WT_SESSION"] = "1"
	c = detectCapabilities(get, "linux", true, "xterm.js(5.5.0)")
	if c.ProgressReporting || !c.XtermJS || !c.SoftwareBidi || !c.CursorUpViewportYankBug {
		t.Fatalf("windows-terminal/xterm capabilities = %+v", c)
	}

	env = map[string]string{"TERM_PROGRAM": "iTerm.app", "TERM_PROGRAM_VERSION": "3.6.5"}
	c = detectCapabilities(get, "darwin", true, "")
	if c.ProgressReporting {
		t.Fatalf("old iTerm unexpectedly supports progress: %+v", c)
	}
	env["TERM_PROGRAM_VERSION"] = "3.6.6"
	if !detectCapabilities(get, "darwin", true, "").ProgressReporting {
		t.Fatal("iTerm 3.6.6 progress gate failed")
	}
}

func TestRuntimeImperativeTerminalHelpers(t *testing.T) {
	var out bytes.Buffer
	rt := NewRuntime(Root(Text("x")), bytes.NewReader(nil), &out, RenderOptions{Width: 5, Height: 2})
	if err := rt.SetTerminalTitle("\x1b[31mred\x1b[0m"); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); strings.Contains(got, "[31m") || !strings.Contains(got, "red") {
		t.Fatalf("title was not ANSI-stripped: %q", got)
	}
	out.Reset()
	if err := rt.RingBell(); err != nil || out.String() != BEL {
		t.Fatalf("bell output=%q err=%v", out.String(), err)
	}
}

func TestSemverCapabilityGate(t *testing.T) {
	cases := []struct {
		v           string
		maj, min, p int
		want        bool
	}{
		{"1.2.0", 1, 2, 0, true},
		{"1.2.1", 1, 2, 0, true},
		{"1.1.99", 1, 2, 0, false},
		{"v3.6.6", 3, 6, 6, true},
		{"3.6", 3, 6, 6, false},
		{"garbage", 1, 0, 0, false},
	}
	for _, tc := range cases {
		if got := semverAtLeast(tc.v, tc.maj, tc.min, tc.p); got != tc.want {
			t.Fatalf("semverAtLeast(%q,%d,%d,%d)=%v want %v", tc.v, tc.maj, tc.min, tc.p, got, tc.want)
		}
	}
}

func TestEmbeddedRuntimeStartCloseLifecycle(t *testing.T) {
	root := Root(AlternateScreen(Text("embedded")))
	var out bytes.Buffer
	rt := NewRuntime(root, bytes.NewReader(nil), &out, RenderOptions{Width: 12, Height: 3, Fullscreen: true})
	if rt.Started() {
		t.Fatal("new runtime already started")
	}
	if err := rt.Start(); err != nil {
		t.Fatal(err)
	}
	if !rt.Started() || !strings.Contains(out.String(), EnterAltScreen) {
		t.Fatalf("start state=%v output=%q", rt.Started(), out.String())
	}
	before := out.Len()
	if err := rt.Start(); err != nil {
		t.Fatal(err)
	}
	// Start is terminal-mode idempotent. It may render/query but must not push
	// another alternate-screen entry sequence.
	if strings.Count(out.String(), EnterAltScreen) != 1 {
		t.Fatalf("duplicate alt-screen entry after repeated Start: %q (before=%d)", out.String(), before)
	}
	if err := rt.Close(); err != nil {
		t.Fatal(err)
	}
	if rt.Started() || !strings.Contains(out.String(), ExitAltScreen) {
		t.Fatalf("close state=%v output=%q", rt.Started(), out.String())
	}
	if err := rt.Close(); err != nil {
		t.Fatal(err)
	}
}
