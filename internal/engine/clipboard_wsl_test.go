package engine

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// On WSL the GOOS is linux but none of the X11/Wayland tools exist: the Windows utilities
// are on PATH instead. Without them the native path was reported unavailable and every copy
// silently fell back to OSC 52, which Windows Terminal may ignore - a copy that reported
// success while nothing reached the clipboard.
func TestNativeClipboardUsesTheWindowsBridgeOnWSL(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the Windows bridge candidates only apply to the linux branch")
	}
	dir := t.TempDir()
	captured := filepath.Join(dir, "captured.txt")
	// A fake clip.exe that records what it receives on stdin, and must not be preferred
	// over a real X11/Wayland tool when one exists.
	script := "#!/bin/sh\ncat > \"" + captured + "\"\n"
	if err := os.WriteFile(filepath.Join(dir, "clip.exe"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("SSH_CONNECTION", "")
	t.Setenv("TMUX", "")

	if got := GetClipboardPath(); got != ClipboardNative {
		t.Fatalf("clip.exe on PATH must select the native path, got %q", got)
	}
	const text = "hola desde WSL"
	if err := CopyNativeClipboard(context.Background(), text); err != nil {
		t.Fatalf("copy via clip.exe: %v", err)
	}
	data, err := os.ReadFile(captured)
	if err != nil {
		t.Fatalf("clip.exe did not receive the text: %v", err)
	}
	if string(data) != text {
		t.Fatalf("clipboard got %q, want %q", data, text)
	}
}

// The preference order must not change: a real X11/Wayland tool wins over the WSL bridge.
func TestNativeClipboardPrefersTheLocalToolOverTheBridge(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the candidate order only applies to the linux branch")
	}
	dir := t.TempDir()
	wlCapture := filepath.Join(dir, "wl.txt")
	bridgeCapture := filepath.Join(dir, "bridge.txt")
	if err := os.WriteFile(filepath.Join(dir, "wl-copy"), []byte("#!/bin/sh\ncat > \""+wlCapture+"\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "clip.exe"), []byte("#!/bin/sh\ncat > \""+bridgeCapture+"\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("SSH_CONNECTION", "")
	t.Setenv("TMUX", "")

	if err := CopyNativeClipboard(context.Background(), "prefer local"); err != nil {
		t.Fatalf("copy: %v", err)
	}
	if _, err := os.Stat(wlCapture); err != nil {
		t.Fatal("wl-copy must be preferred when it exists")
	}
	if _, err := os.Stat(bridgeCapture); err == nil {
		t.Fatal("the WSL bridge must not be used while a local tool exists")
	}
}

// Across SSH the native path stays refused, bridge included: it would mutate the clipboard
// of the machine the user is sitting at.
func TestNativeClipboardRefusesAcrossSSHIncludingTheBridge(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "clip.exe"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("SSH_CONNECTION", "10.0.0.1 22 10.0.0.2 22")
	if err := CopyNativeClipboard(context.Background(), "x"); err == nil {
		t.Fatal("the native clipboard must stay refused across SSH")
	}
}
