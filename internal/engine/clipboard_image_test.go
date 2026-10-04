package engine

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

// The clipboard image read is the one path by which a screenshot can be pasted into
// a terminal application, which no terminal protocol carries. These tests pin the
// three things a caller depends on: that an image is validated and bounded before it
// is returned, that "no image on the clipboard" is distinguishable from "this host
// cannot read a clipboard image at all", and that a route which is not installed is
// never reported as a failure.

func clipboardPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{G: 0xff, A: 0xff})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// withClipboardImageRunner installs a scripted runner for one test. Clipboard
// helpers are external programs and no headless host has a real clipboard, so the
// seam is the only way to exercise the routes deterministically.
func withClipboardImageRunner(t *testing.T, run clipboardImageTool) {
	t.Helper()
	previous := clipboardImageRun
	clipboardImageRun = run
	t.Cleanup(func() { clipboardImageRun = previous })
}

func TestParseWindowsClipboardImageDecodesHintAndPayload(t *testing.T) {
	data := clipboardPNG(t, 3, 2)
	out := "screen shot.png\n" + base64.StdEncoding.EncodeToString(data)
	image, err := parseWindowsClipboardImage([]byte(out))
	if err != nil {
		t.Fatal(err)
	}
	if image.MIME != "image/png" {
		t.Fatalf("mime = %q", image.MIME)
	}
	if !bytes.Equal(image.Data, data) {
		t.Fatal("the decoded bytes differ from the encoded ones")
	}
	if image.Name != "screen shot.png" {
		t.Fatalf("hint = %q", image.Name)
	}
}

func TestParseWindowsClipboardImageAcceptsAnEmptyHint(t *testing.T) {
	image, err := parseWindowsClipboardImage([]byte("\n" + base64.StdEncoding.EncodeToString(clipboardPNG(t, 1, 1))))
	if err != nil {
		t.Fatal(err)
	}
	if image.Name != "" {
		t.Fatalf("hint = %q, want empty", image.Name)
	}
}

func TestParseWindowsClipboardImageRejectsMalformedPayloads(t *testing.T) {
	for name, out := range map[string]string{
		"no newline":   "screen.png",
		"empty":        "screen.png\n",
		"whitespace":   "screen.png\n   \n",
		"bad base64":   "screen.png\nnot base64 at all!!",
		"not an image": "screen.png\n" + base64.StdEncoding.EncodeToString([]byte("just text")),
	} {
		if _, err := parseWindowsClipboardImage([]byte(out)); err == nil {
			t.Fatalf("%s was accepted", name)
		}
	}
}

// The transport is bounded before it is decoded: a hostile or malfunctioning helper
// must not be able to make one paste allocate without limit.
func TestParseWindowsClipboardImageBoundsTheTransport(t *testing.T) {
	huge := "screen.png\n" + strings.Repeat("A", base64.StdEncoding.EncodedLen(maxClipboardImageBytes)+64)
	_, err := parseWindowsClipboardImage([]byte(huge))
	if err == nil {
		t.Fatal("an over-long payload was decoded")
	}
	if !strings.Contains(err.Error(), "limit") {
		t.Fatalf("the refusal does not name the limit: %v", err)
	}
}

func TestWindowsClipboardImageMissingRecognizesOnlyTheNoImageExit(t *testing.T) {
	if !windowsClipboardImageMissing(&clipboardToolExitError{code: windowsNoClipboardImageExit}) {
		t.Fatal("the no-image exit code was not recognized")
	}
	if windowsClipboardImageMissing(&clipboardToolExitError{code: 1}) {
		t.Fatal("an arbitrary failure was reported as an empty clipboard")
	}
	if windowsClipboardImageMissing(errors.New("boom")) {
		t.Fatal("a non-exit error was reported as an empty clipboard")
	}
}

func TestReadClipboardImageWindowsMapsNoImageAndSuccess(t *testing.T) {
	withClipboardImageRunner(t, func(_ context.Context, name string, args []string) ([]byte, bool, error) {
		if name != "powershell.exe" {
			t.Fatalf("unexpected helper %q", name)
		}
		if !strings.Contains(strings.Join(args, " "), "-Sta") {
			t.Fatalf("the single-threaded apartment was not requested: %v", args)
		}
		return nil, true, &clipboardToolExitError{code: windowsNoClipboardImageExit}
	})
	if _, err := readClipboardImageWindows(context.Background()); !errors.Is(err, ErrNoClipboardImage) {
		t.Fatalf("err = %v, want ErrNoClipboardImage", err)
	}

	data := clipboardPNG(t, 2, 2)
	withClipboardImageRunner(t, func(context.Context, string, []string) ([]byte, bool, error) {
		return []byte("clip.png\n" + base64.StdEncoding.EncodeToString(data)), true, nil
	})
	image, err := readClipboardImageWindows(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if image.Name != "clip.png" || !bytes.Equal(image.Data, data) {
		t.Fatalf("image = %+v", image)
	}
}

// An absent helper is a route problem, not an empty clipboard.
func TestReadClipboardImageWindowsReportsAMissingHelper(t *testing.T) {
	withClipboardImageRunner(t, func(context.Context, string, []string) ([]byte, bool, error) {
		return nil, false, nil
	})
	_, err := readClipboardImageWindows(context.Background())
	if err == nil || errors.Is(err, ErrNoClipboardImage) {
		t.Fatalf("err = %v, want a route error distinct from ErrNoClipboardImage", err)
	}
	if !strings.Contains(err.Error(), "powershell.exe") {
		t.Fatalf("the route error does not name the missing helper: %v", err)
	}
}

func TestReadClipboardImageLinuxPrefersTheFirstWorkingTool(t *testing.T) {
	data := clipboardPNG(t, 1, 1)
	var calls []string
	withClipboardImageRunner(t, func(_ context.Context, name string, _ []string) ([]byte, bool, error) {
		calls = append(calls, name)
		switch name {
		case "wl-paste":
			return nil, false, nil // not installed: skipped silently
		case "xclip":
			return data, true, nil
		}
		return nil, false, nil
	})
	image, path, err := readClipboardImageLinux(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if path != ClipboardNative || !bytes.Equal(image.Data, data) {
		t.Fatalf("image = %+v path = %q", image, path)
	}
	if strings.Join(calls, ",") != "wl-paste,xclip" {
		t.Fatalf("calls = %v, want the preference order with the missing tool skipped", calls)
	}
}

// A local tool that is installed and fails (the ordinary "the clipboard holds text,
// not an image" case) must be reported as an empty clipboard, not as a broken host.
func TestReadClipboardImageLinuxReportsNoImageWhenAToolFails(t *testing.T) {
	withClipboardImageRunner(t, func(_ context.Context, name string, _ []string) ([]byte, bool, error) {
		if name == "wl-paste" {
			return nil, true, &clipboardToolExitError{code: 1}
		}
		return nil, false, nil
	})
	_, _, err := readClipboardImageLinux(context.Background())
	if !errors.Is(err, ErrNoClipboardImage) {
		t.Fatalf("err = %v, want ErrNoClipboardImage", err)
	}
}

// On WSL none of the X11/Wayland tools exist and the Windows bridge is the only
// clipboard there is, so it must be tried and its answer must be authoritative.
func TestReadClipboardImageLinuxFallsBackToTheWindowsBridge(t *testing.T) {
	data := clipboardPNG(t, 4, 4)
	withClipboardImageRunner(t, func(_ context.Context, name string, _ []string) ([]byte, bool, error) {
		if name == "powershell.exe" {
			return []byte("shot.png\n" + base64.StdEncoding.EncodeToString(data)), true, nil
		}
		return nil, false, nil
	})
	image, path, err := readClipboardImageLinux(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if path != ClipboardNative || image.Name != "shot.png" || !bytes.Equal(image.Data, data) {
		t.Fatalf("image = %+v path = %q", image, path)
	}

	withClipboardImageRunner(t, func(_ context.Context, name string, _ []string) ([]byte, bool, error) {
		if name == "powershell.exe" {
			return nil, true, &clipboardToolExitError{code: windowsNoClipboardImageExit}
		}
		return nil, false, nil
	})
	if _, _, err := readClipboardImageLinux(context.Background()); !errors.Is(err, ErrNoClipboardImage) {
		t.Fatalf("err = %v, want the bridge's own no-image answer", err)
	}
}

func TestReadClipboardImageLinuxWithNoRouteAtAll(t *testing.T) {
	withClipboardImageRunner(t, func(context.Context, string, []string) ([]byte, bool, error) {
		return nil, false, nil
	})
	_, _, err := readClipboardImageLinux(context.Background())
	if err == nil || errors.Is(err, ErrNoClipboardImage) {
		t.Fatalf("err = %v, want the no-route report", err)
	}
	if !strings.Contains(err.Error(), "wl-clipboard") {
		t.Fatalf("the no-route report does not name the remedy: %v", err)
	}
}

func TestReadClipboardImageRefusesAcrossSSH(t *testing.T) {
	t.Setenv("SSH_CONNECTION", "10.0.0.1 22 10.0.0.2 22")
	withClipboardImageRunner(t, func(context.Context, string, []string) ([]byte, bool, error) {
		t.Fatal("a helper was invoked across SSH")
		return nil, false, nil
	})
	if _, _, err := ReadClipboardImageSync(); err == nil {
		t.Fatal("the clipboard image was read across SSH")
	}
}

func TestNewClipboardImageBoundsAndValidates(t *testing.T) {
	if _, err := newClipboardImage(nil, ""); !errors.Is(err, ErrNoClipboardImage) {
		t.Fatalf("empty clipboard data: err = %v", err)
	}
	if _, err := newClipboardImage(make([]byte, maxClipboardImageBytes+1), ""); err == nil {
		t.Fatal("an oversized image was accepted")
	}
	if _, err := newClipboardImage([]byte("plain text, not an image"), ""); err == nil {
		t.Fatal("non-image bytes were accepted")
	}
	for name, data := range map[string][]byte{
		"png":  clipboardPNG(t, 1, 1),
		"jpeg": {0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10},
		"gif":  []byte("GIF89a\x00\x00\x00\x00"),
		"webp": append([]byte("RIFF\x00\x00\x00\x00WEBP"), make([]byte, 12)...),
	} {
		image, err := newClipboardImage(data, "")
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if image.MIME == "" || len(image.Data) != len(data) {
			t.Fatalf("%s: image = %+v", name, image)
		}
	}
}

func TestSanitizeClipboardImageHintKeepsOnlyAFileName(t *testing.T) {
	for raw, want := range map[string]string{
		"shot.png":                "shot.png",
		`C:\Users\me\shot.png`:    "shot.png",
		"/home/me/shot.png":       "shot.png",
		"  spaced name.png  ":     "spaced name.png",
		"../../etc/passwd":        "passwd",
		"..":                      "",
		".":                       "",
		"":                        "",
		"weird\u0000name.png":     "weirdname.png",
		"emoji\U0001F600shot.png": "emojishot.png",
		strings.Repeat("x", 200):  strings.Repeat("x", 96),
	} {
		if got := sanitizeClipboardImageHint(raw); got != want {
			t.Fatalf("sanitizeClipboardImageHint(%q) = %q, want %q", raw, got, want)
		}
	}
}
