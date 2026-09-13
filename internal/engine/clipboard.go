package engine

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
	"unicode/utf16"
)

type ClipboardPath string

const (
	ClipboardNative     ClipboardPath = "native"
	ClipboardTmuxBuffer ClipboardPath = "tmux-buffer"
	ClipboardOSC52Path  ClipboardPath = "osc52"
)

// nativeClipboardCandidates lists the Linux utilities that can reach the local
// clipboard, in preference order. On WSL the GOOS is still linux and none of the X11 or
// Wayland tools exist, but the Windows utilities are on PATH: clip.exe is the system
// clipboard and the PowerShell fallback reads stdin, so both write to the clipboard the
// user is actually looking at. Run through runClipboardTool, whose stdin carries the
// text, so neither needs argument quoting.
var nativeClipboardCandidates = []struct {
	name string
	args []string
}{
	{"wl-copy", nil},
	{"xclip", []string{"-selection", "clipboard"}},
	{"xsel", []string{"--clipboard", "--input"}},
	{"clip.exe", nil},
	{"powershell.exe", []string{"-NoProfile", "-NonInteractive", "-Command", "$ErrorActionPreference='Stop'; [Console]::InputEncoding = [System.Text.UTF8Encoding]::new($false); Set-Clipboard -Value ([Console]::In.ReadToEnd())"}},
}

// GetClipboardPath reports the strongest clipboard path currently available.
// It mirrors the fork's policy: native tools are never used across SSH.
func GetClipboardPath() ClipboardPath {
	if os.Getenv("SSH_CONNECTION") == "" {
		switch runtime.GOOS {
		case "darwin", "windows":
			return ClipboardNative
		case "linux":
			for _, candidate := range nativeClipboardCandidates {
				if _, err := exec.LookPath(candidate.name); err == nil {
					return ClipboardNative
				}
			}
		}
	}
	if os.Getenv("TMUX") != "" {
		return ClipboardTmuxBuffer
	}
	return ClipboardOSC52Path
}

func runClipboardTool(ctx context.Context, name string, args []string, text string) error {
	return runClipboardToolReader(ctx, name, args, strings.NewReader(text))
}

func runClipboardToolReader(ctx context.Context, name string, args []string, stdin io.Reader) error {
	path, err := exec.LookPath(name)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Stdin = stdin
	return cmd.Run()
}

func windowsClipboardBytes(text string) []byte {
	units := utf16.Encode([]rune(text))
	data := make([]byte, 2*len(units))
	for i, u := range units {
		binary.LittleEndian.PutUint16(data[2*i:], u)
	}
	return data
}

func runWindowsClip(ctx context.Context, text string) error {
	return runClipboardToolReader(ctx, "clip.exe", nil, bytes.NewReader(windowsClipboardBytes(text)))
}

// CopyNativeClipboard writes to a local OS clipboard utility. It refuses to
// run across SSH because that would mutate the remote machine's clipboard.
func CopyNativeClipboard(ctx context.Context, text string) error {
	if os.Getenv("SSH_CONNECTION") != "" {
		return errors.New("native clipboard disabled across SSH")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	switch runtime.GOOS {
	case "darwin":
		return runClipboardTool(ctx, "pbcopy", nil, text)
	case "windows":
		return runWindowsClip(ctx, text)
	case "linux":
		var last error
		for _, candidate := range nativeClipboardCandidates {
			var err error
			if candidate.name == "clip.exe" {
				err = runWindowsClip(ctx, text)
			} else {
				err = runClipboardTool(ctx, candidate.name, candidate.args, text)
			}
			if err == nil {
				return nil
			}
			last = err
		}
		if last != nil {
			return last
		}
	}
	return errors.New("no native clipboard utility available")
}

// TmuxLoadBuffer loads tmux's paste buffer and asks recent tmux versions to
// propagate it to the outer clipboard. iTerm2 deliberately omits -w to avoid
// its problematic tmux OSC-52 propagation path.
func TmuxLoadBuffer(ctx context.Context, text string) error {
	if os.Getenv("TMUX") == "" {
		return errors.New("not running inside tmux")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	args := []string{"load-buffer", "-w", "-"}
	if os.Getenv("LC_TERMINAL") == "iTerm2" {
		args = []string{"load-buffer", "-"}
	}
	return runClipboardTool(ctx, "tmux", args, text)
}

// SetClipboard performs the fork's best-effort clipboard policy and returns
// the OSC-52 sequence that the caller should write to the terminal. Native
// copy is attempted as a local safety net; tmux's buffer is loaded when
// available. The native write runs in the background, so the returned path says
// which route exists, not that it succeeded: a caller that reports success to
// the user must use SetClipboardSync instead.
func SetClipboard(text string) (sequence string, path ClipboardPath) {
	sequence, path, _ = setClipboard(text, false)
	return sequence, path
}

// SetClipboardSync is SetClipboard with the native write awaited, so a caller can
// report what actually happened instead of assuming it. path is the route that was
// taken:
//
//   - ClipboardNative: the native utility ran and succeeded, and nativeErr is nil;
//   - ClipboardTmuxBuffer: tmux loaded the buffer and sequence is the passthrough;
//   - ClipboardOSC52Path: sequence is the only route, which the terminal may ignore.
//
// nativeErr is non-nil when a native write was attempted and failed, or when no
// native utility exists: exactly the case a caller must not describe as a copy that
// reached the clipboard.
func SetClipboardSync(text string) (sequence string, path ClipboardPath, nativeErr error) {
	return setClipboard(text, true)
}

func setClipboard(text string, wait bool) (sequence string, path ClipboardPath, nativeErr error) {
	b64 := base64.StdEncoding.EncodeToString([]byte(text))
	raw := OSC(52, "c", b64)

	native := GetClipboardPath() == ClipboardNative
	if os.Getenv("SSH_CONNECTION") == "" {
		copyNative := func() error {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			return CopyNativeClipboard(ctx, text)
		}
		if wait {
			nativeErr = copyNative()
		} else {
			go func() { _ = copyNative() }()
		}
	}

	if os.Getenv("TMUX") != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err := TmuxLoadBuffer(ctx, text)
		cancel()
		if err == nil {
			payload := ESC + "]52;c;" + b64 + BEL
			return ESC + "Ptmux;" + strings.ReplaceAll(payload, ESC, ESC+ESC) + ST, ClipboardTmuxBuffer, nativeErr
		}
	}
	if native && nativeErr == nil {
		return raw, ClipboardNative, nil
	}
	return raw, ClipboardOSC52Path, nativeErr
}
