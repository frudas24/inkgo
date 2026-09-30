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

const maxClipboardReadBytes = 8 << 20

// nativeClipboardCandidates lists the Linux utilities that can reach the local
// clipboard, in preference order. On WSL the GOOS is still linux and none of the X11 or
// Wayland tools exist, but the Windows utilities are on PATH: clip.exe is the system
// clipboard and the PowerShell fallback reads stdin, so both write to the clipboard the
// user is actually looking at. Run through runClipboardTool, whose stdin carries the
// text, so neither needs argument quoting. On a native desktop the two bridge entries are
// simply not on PATH, which the walk below reports as "nothing installed" instead of
// quoting the LookPath failure of a Windows executable in a diagnostic about Linux.
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

var nativeClipboardReaders = []struct {
	name string
	args []string
}{
	{"wl-paste", []string{"--no-newline"}},
	{"xclip", []string{"-selection", "clipboard", "-o"}},
	{"xsel", []string{"--clipboard", "--output"}},
	{"powershell.exe", []string{"-NoProfile", "-NonInteractive", "-Command", "$ErrorActionPreference='Stop'; [Console]::OutputEncoding = [System.Text.UTF8Encoding]::new($false); Get-Clipboard -Raw"}},
}

// errNoNativeClipboard is the report for a linux session where none of the candidate
// utilities is installed. X11 and Wayland expose no clipboard without a client tool, so
// that is what the user has to be told, together with the remedy. The walk below used to
// return the LookPath error of the last candidate instead, which on a native desktop is
// the WSL-only `powershell.exe`: copying from the TUI printed
// `native clipboard: exec: "powershell.exe": executable file not found in $PATH`, a
// Windows executable that host never had, and no hint at what would fix the copy.
var errNoNativeClipboard = errors.New("no native clipboard utility installed (install wl-clipboard, xclip or xsel)")

// nativeToolFailures reports the installed candidates that failed, or the absence of any
// candidate at all. A utility missing from PATH was never attempted, so it is not a
// failure and is deliberately not named.
func nativeToolFailures(failures []string) error {
	if len(failures) == 0 {
		return errNoNativeClipboard
	}
	return errors.New("native clipboard utility failed: " + strings.Join(failures, "; "))
}

// readNativeClipboardLinux walks the reader list under that rule.
func readNativeClipboardLinux(ctx context.Context) (string, ClipboardPath, error) {
	var failures []string
	for _, candidate := range nativeClipboardReaders {
		if _, err := exec.LookPath(candidate.name); err != nil {
			continue
		}
		out, err := runClipboardToolOutput(ctx, candidate.name, candidate.args)
		if err != nil {
			failures = append(failures, candidate.name+": "+err.Error())
			continue
		}
		if len(out) > maxClipboardReadBytes {
			return "", "", errors.New("clipboard content exceeds read limit")
		}
		return string(out), ClipboardNative, nil
	}
	return "", "", nativeToolFailures(failures)
}

// copyNativeClipboardLinux walks the writer list under that rule.
func copyNativeClipboardLinux(ctx context.Context, text string) error {
	var failures []string
	for _, candidate := range nativeClipboardCandidates {
		if _, err := exec.LookPath(candidate.name); err != nil {
			continue
		}
		var err error
		if candidate.name == "clip.exe" {
			err = runWindowsClip(ctx, text)
		} else {
			err = runClipboardTool(ctx, candidate.name, candidate.args, text)
		}
		if err != nil {
			failures = append(failures, candidate.name+": "+err.Error())
			continue
		}
		return nil
	}
	return nativeToolFailures(failures)
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

func runClipboardToolOutput(ctx context.Context, name string, args []string) ([]byte, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return nil, err
	}
	return exec.CommandContext(ctx, path, args...).Output()
}

// ReadClipboardSync reads a locally reachable clipboard after an explicit
// application gesture. OSC-52 is intentionally write-only, so it is never
// treated as a readable clipboard. The bounded result prevents a hostile or
// malfunctioning clipboard provider from turning one paste into unbounded UI
// memory.
func ReadClipboardSync() (string, ClipboardPath, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if os.Getenv("SSH_CONNECTION") == "" {
		if runtime.GOOS == "darwin" {
			out, err := runClipboardToolOutput(ctx, "pbpaste", nil)
			if err == nil {
				if len(out) > maxClipboardReadBytes {
					return "", "", errors.New("clipboard content exceeds read limit")
				}
				return string(out), ClipboardNative, nil
			}
		}
		if runtime.GOOS == "windows" {
			out, err := runClipboardToolOutput(ctx, "powershell.exe", nativeClipboardReaders[len(nativeClipboardReaders)-1].args)
			if err == nil {
				if len(out) > maxClipboardReadBytes {
					return "", "", errors.New("clipboard content exceeds read limit")
				}
				return string(out), ClipboardNative, nil
			}
		}
		if runtime.GOOS == "linux" {
			return readNativeClipboardLinux(ctx)
		}
	}
	if os.Getenv("TMUX") != "" {
		out, err := runClipboardToolOutput(ctx, "tmux", []string{"save-buffer", "-"})
		if err == nil {
			if len(out) > maxClipboardReadBytes {
				return "", "", errors.New("clipboard content exceeds read limit")
			}
			return string(out), ClipboardTmuxBuffer, nil
		}
		return "", "", err
	}
	return "", "", errors.New("no readable clipboard route available")
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
// On Linux it reports errNoNativeClipboard when no candidate is installed, so a caller
// that shows the error can tell the user the remedy instead of quoting the LookPath
// failure of some utility that was never installed.
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
		return copyNativeClipboardLinux(ctx, text)
	}
	return errNoNativeClipboard
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
