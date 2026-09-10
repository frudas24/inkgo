package inkgo

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

type ClipboardPath string

const (
	ClipboardNative     ClipboardPath = "native"
	ClipboardTmuxBuffer ClipboardPath = "tmux-buffer"
	ClipboardOSC52Path  ClipboardPath = "osc52"
)

// GetClipboardPath reports the strongest clipboard path currently available.
// It mirrors the fork's policy: native tools are never used across SSH.
func GetClipboardPath() ClipboardPath {
	if os.Getenv("SSH_CONNECTION") == "" {
		switch runtime.GOOS {
		case "darwin", "windows":
			return ClipboardNative
		case "linux":
			for _, tool := range []string{"wl-copy", "xclip", "xsel"} {
				if _, err := exec.LookPath(tool); err == nil {
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
	path, err := exec.LookPath(name)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
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
		return runClipboardTool(ctx, "clip.exe", nil, text)
	case "linux":
		var last error
		for _, candidate := range []struct {
			name string
			args []string
		}{{"wl-copy", nil}, {"xclip", []string{"-selection", "clipboard"}}, {"xsel", []string{"--clipboard", "--input"}}} {
			if err := runClipboardTool(ctx, candidate.name, candidate.args, text); err == nil {
				return nil
			} else {
				last = err
			}
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
// available. No third-party library or vendor code is required.
func SetClipboard(text string) (sequence string, path ClipboardPath) {
	b64 := base64.StdEncoding.EncodeToString([]byte(text))
	raw := OSC(52, "c", b64)

	if os.Getenv("SSH_CONNECTION") == "" {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = CopyNativeClipboard(ctx, text)
		}()
	}

	if os.Getenv("TMUX") != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err := TmuxLoadBuffer(ctx, text)
		cancel()
		if err == nil {
			payload := ESC + "]52;c;" + b64 + BEL
			return ESC + "Ptmux;" + strings.ReplaceAll(payload, ESC, ESC+ESC) + ST, ClipboardTmuxBuffer
		}
	}
	if GetClipboardPath() == ClipboardNative {
		return raw, ClipboardNative
	}
	return raw, ClipboardOSC52Path
}
