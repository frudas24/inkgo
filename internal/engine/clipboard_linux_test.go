package engine

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// A native Linux desktop has neither the Windows bridge nor, without a client tool, any
// way to reach X11's or Wayland's clipboard. The report used to be the LookPath error of
// the last candidate - the WSL-only powershell.exe - so copying from the TUI printed
// `native clipboard: exec: "powershell.exe": executable file not found in $PATH`: a
// Windows executable this host never had, and no hint at what the remedy is.
func TestNativeClipboardWithoutAnyUtilityReportsTheMissingTool(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the candidate list this pins belongs to the linux branch")
	}
	t.Setenv("PATH", t.TempDir())
	t.Setenv("SSH_CONNECTION", "")
	t.Setenv("TMUX", "")

	if got := GetClipboardPath(); got != ClipboardOSC52Path {
		t.Fatalf("with no utility installed the only route left is OSC 52, got %q", got)
	}

	err := CopyNativeClipboard(context.Background(), "texto ñ🙂")
	if err == nil {
		t.Fatal("no candidate is installed here, so the native copy cannot succeed")
	}
	for _, unwanted := range []string{"powershell.exe", "clip.exe", "exec:"} {
		if strings.Contains(err.Error(), unwanted) {
			t.Fatalf("an uninstalled utility must not appear in the report (%q): %v", unwanted, err)
		}
	}
	for _, remedy := range []string{"wl-clipboard", "xclip", "xsel"} {
		if !strings.Contains(err.Error(), remedy) {
			t.Fatalf("the report must name the remedy %q: %v", remedy, err)
		}
	}

	// The read path shares the rule: the absence of a reader is not a failure of one.
	if _, _, readErr := ReadClipboardSync(); readErr == nil || strings.Contains(readErr.Error(), "powershell.exe") {
		t.Fatalf("the read report must not name the Windows bridge either: %v", readErr)
	}

	// The copy still falls back to OSC 52, and the caller is told that is the route plus
	// why the native one is unavailable.
	sequence, path, nativeErr := SetClipboardSync("texto ñ🙂")
	if path != ClipboardOSC52Path {
		t.Fatalf("path = %q, want %q", path, ClipboardOSC52Path)
	}
	if sequence == "" {
		t.Fatal("the OSC 52 sequence is the only route left and must still be returned")
	}
	if nativeErr == nil || !strings.Contains(nativeErr.Error(), "no native clipboard utility installed") {
		t.Fatalf("nativeErr = %v, want the absent-utility report", nativeErr)
	}
}

// An installed utility that fails is a real failure and must be named; the bridge that was
// never reached must not be named in its place.
func TestNativeClipboardNamesTheUtilityThatFailed(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("this pins the linux candidate list")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "xclip"), []byte("#!/bin/sh\nexit 9\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("SSH_CONNECTION", "")
	t.Setenv("TMUX", "")

	err := CopyNativeClipboard(context.Background(), "x")
	if err == nil {
		t.Fatal("xclip exits 9 here and that failure must not be swallowed")
	}
	if !strings.Contains(err.Error(), "xclip") {
		t.Fatalf("the failing utility must be named: %v", err)
	}
	if strings.Contains(err.Error(), "powershell.exe") {
		t.Fatalf("a utility that was never reached must not be named: %v", err)
	}
}
