package engine

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// ClipboardImage is one image read from the local clipboard after an explicit
// application gesture.
//
// Data is always a complete encoded image (the Windows route re-encodes whatever
// the clipboard held as PNG, the X11/Wayland route returns the PNG the clipboard
// advertised), so a caller never has to guess a format from a byte sniff it cannot
// perform. Name is an optional operator-facing hint - the base name of the file the
// image was copied from, when the clipboard still knows one - and is empty when the
// image came from a screenshot or an application that does not name it.
type ClipboardImage struct {
	Data []byte
	MIME string
	Name string
}

// ErrNoClipboardImage reports a clipboard that was reached but holds no image.
//
// It is deliberately distinct from a route error: a caller that pastes only when
// the gesture produced an image (and falls back to text otherwise) needs to tell
// "there is no image here" apart from "this host cannot read a clipboard image at
// all", which is what it has to report to the operator.
var ErrNoClipboardImage = errors.New("clipboard holds no image")

// errClipboardImageTooLarge reports clipboard content that was recognized as an image but
// is larger than one read may return.
//
// It is deliberately not ErrNoClipboardImage: the clipboard does hold an image, so
// reporting it as empty would make a caller fall back to a text paste of nothing and hide
// the reason. A screenshot from a modern camera or a large display is the ordinary way to
// reach this limit.
var errClipboardImageTooLarge = errors.New("clipboard image exceeds the size limit")

// maxClipboardImageBytes bounds one read. It matches the text reader's limit for the
// same reason: the clipboard is written by arbitrary applications, so a hosted or
// malfunctioning provider must not be able to turn one paste into unbounded memory.
const maxClipboardImageBytes = maxClipboardReadBytes

// clipboardImageTimeout bounds one read. It is longer than the text reader's two
// seconds because the Windows route has to encode the image, and a full-screen
// screenshot is not instant, while still being short enough that a hung helper
// cannot hold an interactive paste.
const clipboardImageTimeout = 5 * time.Second

// windowsNoClipboardImageExit is the exit code the Windows helper uses to report an
// empty image clipboard. An exit code, and not empty output, because a base64
// payload can legitimately be empty-looking on a failure that still printed a hint
// line.
const windowsNoClipboardImageExit = 3

// windowsClipboardImageScript extracts the clipboard image as PNG on Windows.
//
// It prints two lines: the operator-facing name hint (empty when unknown) and the
// base64 PNG. Base64, and not the raw bytes, because the same script also serves the
// WSL bridge, where a child process' stdout is the only channel that needs no
// Windows-to-WSL path translation.
//
// Get-Clipboard -Format Image requires a single-threaded apartment, which the
// Windows PowerShell console host provides by default and which the caller also
// requests explicitly with -Sta.
const windowsClipboardImageScript = `$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Drawing
$img = $null
try { $img = Get-Clipboard -Format Image } catch { $img = $null }
if ($null -eq $img) { exit 3 }
$hint = ''
try {
  $files = Get-Clipboard -Format FileDropList
  if ($files -and $files.Count -gt 0) { $hint = [System.IO.Path]::GetFileName([string]$files[0]) }
} catch { $hint = '' }
$ms = New-Object System.IO.MemoryStream
$img.Save($ms, [System.Drawing.Imaging.ImageFormat]::Png)
[Console]::OutputEncoding = [System.Text.UTF8Encoding]::new($false)
[Console]::Out.Write($hint)
[Console]::Out.Write("` + "`n" + `")
[Console]::Out.Write([System.Convert]::ToBase64String($ms.ToArray()))
`

var windowsClipboardImageArgs = []string{"-NoProfile", "-NonInteractive", "-Sta", "-Command", windowsClipboardImageScript}

// nativeClipboardImageReaders lists the Linux utilities that can read an image from
// the local clipboard, in preference order. The names deliberately mirror the text
// readers: the same two clients that a desktop already needs for text are the ones
// that carry an image, so a host that can copy text from the TUI can paste an
// image too.
var nativeClipboardImageReaders = []struct {
	name string
	args []string
}{
	{"wl-paste", []string{"--type", "image/png"}},
	{"xclip", []string{"-selection", "clipboard", "-t", "image/png", "-o"}},
}

// clipboardImageTool runs one external clipboard helper.
//
// found reports whether the helper exists at all, so an uninstalled utility is
// skipped instead of reported: naming a LookPath failure of a program that was never
// on the host is exactly the misleading diagnostic the text readers were fixed to
// stop producing (v0.1.31). A helper that exists and fails returns a non-nil err.
type clipboardImageTool func(ctx context.Context, name string, args []string) (out []byte, found bool, err error)

// clipboardImageTransportLimit bounds a helper's stdout: the base64 payload the Windows
// route can print for an image at the byte limit, plus room for the name line ahead of
// it. The parsed payload is bounded again below, so this is the transport cap that stops
// a provider from being read to EOF in the first place.
var clipboardImageTransportLimit = base64.StdEncoding.EncodedLen(maxClipboardImageBytes) + 4096

// clipboardImageRun is the seam tests replace to exercise the Windows and Linux
// routes - including their ordering and their exit-code handling - without a real
// clipboard, which no headless test host has. Its output is capped: a helper that keeps
// printing is stopped at clipboardImageTransportLimit bytes instead of being buffered.
var clipboardImageRun clipboardImageTool = func(ctx context.Context, name string, args []string) (out []byte, found bool, err error) {
	path, lookErr := exec.LookPath(name)
	if lookErr != nil {
		return nil, false, nil
	}
	out, err = runClipboardToolPathLimited(ctx, path, args, clipboardImageTransportLimit)
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			// The exit status is normalized here, at the one place that talks to the
			// process, so the route logic reads a code instead of selecting on a
			// process API and a test can produce one without spawning anything.
			return out, true, &clipboardToolExitError{code: exitErr.ExitCode(), err: err}
		}
		return out, true, err
	}
	return out, true, nil
}

// clipboardToolExitError is a helper's exit status, normalized for the route logic.
type clipboardToolExitError struct {
	code int
	err  error
}

func (e *clipboardToolExitError) Error() string {
	if e.err != nil {
		return e.err.Error()
	}
	return fmt.Sprintf("clipboard helper exited with status %d", e.code)
}

func (e *clipboardToolExitError) Unwrap() error { return e.err }

// ReadClipboardImageSync reads an image from the locally reachable clipboard after
// an explicit application gesture.
//
// It returns ErrNoClipboardImage when a clipboard was reached but holds no image, a
// route error when this host has no way to read a clipboard image at all, an error naming
// the limit when the clipboard does hold an image that is too large to return (never a
// truncated payload), and a bounded ClipboardImage on success. Like the text reader it
// refuses to touch the clipboard across SSH: that clipboard belongs to the machine the
// user is sitting at, and a paste from a remote session that silently read the wrong one
// would be worse than no paste.
func ReadClipboardImageSync() (ClipboardImage, ClipboardPath, error) {
	ctx, cancel := context.WithTimeout(context.Background(), clipboardImageTimeout)
	defer cancel()
	if os.Getenv("SSH_CONNECTION") != "" {
		return ClipboardImage{}, "", errors.New("clipboard image read disabled across SSH")
	}
	switch runtime.GOOS {
	case "windows":
		image, err := readClipboardImageWindows(ctx)
		return image, ClipboardNative, err
	case "linux":
		return readClipboardImageLinux(ctx)
	case "darwin":
		// macOS has no native reader here yet. OSC 52 is write-only, so there is no
		// fallback: reporting the gap is the only honest answer.
		return ClipboardImage{}, "", errors.New("clipboard image read is not supported on darwin")
	}
	return ClipboardImage{}, "", errors.New("clipboard image read is not supported on this platform")
}

// readClipboardImageWindows runs the Windows helper and decodes what it printed.
func readClipboardImageWindows(ctx context.Context) (ClipboardImage, error) {
	out, found, err := clipboardImageRun(ctx, "powershell.exe", windowsClipboardImageArgs)
	if !found {
		return ClipboardImage{}, errors.New("clipboard image read needs powershell.exe, which is not on PATH")
	}
	if err != nil {
		if windowsClipboardImageMissing(err) {
			return ClipboardImage{}, ErrNoClipboardImage
		}
		return ClipboardImage{}, err
	}
	return parseWindowsClipboardImage(out)
}

// windowsClipboardImageMissing recognizes the helper's "no image on the clipboard"
// exit code. Every other failure stays a failure: an absent clipboard image and a
// broken PowerShell must not look the same to the caller.
func windowsClipboardImageMissing(err error) bool {
	var exitErr *clipboardToolExitError
	return errors.As(err, &exitErr) && exitErr.code == windowsNoClipboardImageExit
}

// parseWindowsClipboardImage decodes the helper's two-line payload and validates the
// bytes before they are handed to a caller.
func parseWindowsClipboardImage(out []byte) (ClipboardImage, error) {
	hint, payload, ok := strings.Cut(string(out), "\n")
	if !ok {
		return ClipboardImage{}, errors.New("clipboard image helper produced no payload")
	}
	hint = strings.TrimSpace(hint)
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return ClipboardImage{}, errors.New("clipboard image helper produced an empty payload")
	}
	// Bound the transport before decoding: base64 is 4/3 of the decoded size, so a
	// bounded input is what keeps the decode itself bounded.
	if len(payload) > base64.StdEncoding.EncodedLen(maxClipboardImageBytes)+4 {
		return ClipboardImage{}, fmt.Errorf("%w (%d bytes)", errClipboardImageTooLarge, maxClipboardImageBytes)
	}
	data, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return ClipboardImage{}, fmt.Errorf("clipboard image payload is not valid base64: %w", err)
	}
	return newClipboardImage(data, hint)
}

// readClipboardImageLinux walks the local utilities in preference order and then
// falls back to the Windows bridge, which is the only route that exists on WSL.
func readClipboardImageLinux(ctx context.Context) (ClipboardImage, ClipboardPath, error) {
	attempted := false
	var failures []string
	for _, candidate := range nativeClipboardImageReaders {
		out, found, err := clipboardImageRun(ctx, candidate.name, candidate.args)
		if !found {
			continue
		}
		attempted = true
		if err != nil {
			failures = append(failures, candidate.name+": "+err.Error())
			continue
		}
		image, parseErr := newClipboardImage(out, "")
		if parseErr != nil {
			if errors.Is(parseErr, errClipboardImageTooLarge) {
				// An image over the limit is not an empty clipboard: the caller must report
				// the refusal instead of falling back to a text paste of nothing.
				return ClipboardImage{}, ClipboardNative, fmt.Errorf("%s: %w", candidate.name, parseErr)
			}
			failures = append(failures, candidate.name+": "+parseErr.Error())
			continue
		}
		return image, ClipboardNative, nil
	}
	// The WSL bridge: GOOS is linux but the Windows utilities are the only clipboard
	// that exists. It is tried after the local tools so a real desktop never pays for
	// it, and its own "no image" answer is authoritative when it is reached.
	if out, found, err := clipboardImageRun(ctx, "powershell.exe", windowsClipboardImageArgs); found {
		attempted = true
		if err == nil {
			if image, parseErr := parseWindowsClipboardImage(out); parseErr == nil {
				return image, ClipboardNative, nil
			} else if errors.Is(parseErr, errClipboardImageTooLarge) {
				return ClipboardImage{}, ClipboardNative, parseErr
			} else {
				failures = append(failures, "powershell.exe: "+parseErr.Error())
			}
		} else if windowsClipboardImageMissing(err) {
			return ClipboardImage{}, ClipboardNative, ErrNoClipboardImage
		} else {
			failures = append(failures, "powershell.exe: "+err.Error())
		}
	}
	if !attempted {
		return ClipboardImage{}, "", errNoNativeClipboard
	}
	// A tool was reached and none produced an image. For an X11/Wayland client that
	// means the clipboard holds no image, which is the common paste-over-text case;
	// an installed client that genuinely failed is named instead.
	if len(failures) == 0 {
		return ClipboardImage{}, ClipboardNative, ErrNoClipboardImage
	}
	return ClipboardImage{}, ClipboardNative, fmt.Errorf("%w: %s", ErrNoClipboardImage, strings.Join(failures, "; "))
}

// newClipboardImage validates bounded, decodable image bytes and pins them in the
// result. The MIME is derived from the bytes, never taken from the helper, so a
// mis-labelled route cannot make a caller send a payload the provider will reject.
func newClipboardImage(data []byte, hint string) (ClipboardImage, error) {
	if len(data) == 0 {
		return ClipboardImage{}, ErrNoClipboardImage
	}
	if len(data) > maxClipboardImageBytes {
		// The observed length is not named: a transport cap makes it a truncated read, so
		// it would be a lower bound masquerading as the image's size.
		return ClipboardImage{}, fmt.Errorf("%w (%d bytes)", errClipboardImageTooLarge, maxClipboardImageBytes)
	}
	mime := sniffClipboardImageMIME(data)
	if mime == "" {
		return ClipboardImage{}, errors.New("clipboard content is not a recognized image")
	}
	return ClipboardImage{Data: data, MIME: mime, Name: sanitizeClipboardImageHint(hint)}, nil
}

// sniffClipboardImageMIME recognizes the encodings a clipboard can hand over. It
// reads signatures only: the caller transports the bytes unchanged, so a full decode
// would be work whose result nothing consumes.
func sniffClipboardImageMIME(data []byte) string {
	switch {
	case len(data) >= 8 && data[0] == 0x89 && data[1] == 'P' && data[2] == 'N' && data[3] == 'G':
		return "image/png"
	case len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff:
		return "image/jpeg"
	case len(data) >= 6 && (string(data[:6]) == "GIF87a" || string(data[:6]) == "GIF89a"):
		return "image/gif"
	case len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP":
		return "image/webp"
	}
	return ""
}

// sanitizeClipboardImageHint keeps a clipboard-provided name usable as a file-name
// hint: a base name with no path element, no separator and a bounded length. The
// clipboard is written by arbitrary applications, so its text is data, not a
// trusted path.
func sanitizeClipboardImageHint(hint string) string {
	hint = strings.TrimSpace(hint)
	if hint == "" {
		return ""
	}
	hint = strings.ReplaceAll(hint, "\\", "/")
	if i := strings.LastIndexByte(hint, '/'); i >= 0 {
		hint = hint[i+1:]
	}
	hint = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		case r == '-', r == '_', r == '.', r == ' ':
			return r
		default:
			return -1
		}
	}, hint)
	hint = strings.TrimSpace(hint)
	if len(hint) > 96 {
		hint = hint[:96]
	}
	if hint == "." || hint == ".." {
		return ""
	}
	return hint
}
