package inkgo

import (
	"encoding/base64"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
)

func SupportsHyperlinks() bool {
	p := os.Getenv("TERM_PROGRAM")
	lc := os.Getenv("LC_TERMINAL")
	term := os.Getenv("TERM")
	known := map[string]bool{"ghostty": true, "Hyper": true, "kitty": true, "alacritty": true, "iTerm.app": true, "iTerm2": true, "WezTerm": true, "WarpTerminal": true}
	return known[p] || known[lc] || strings.Contains(term, "kitty") || os.Getenv("WT_SESSION") != ""
}

func SupportsSynchronizedOutput() bool {
	if os.Getenv("TMUX") != "" {
		return false
	}
	p, term := os.Getenv("TERM_PROGRAM"), os.Getenv("TERM")
	known := map[string]bool{"iTerm.app": true, "WezTerm": true, "WarpTerminal": true, "ghostty": true, "contour": true, "vscode": true, "alacritty": true}
	if known[p] || strings.Contains(term, "kitty") || term == "xterm-ghostty" || strings.HasPrefix(term, "foot") || strings.Contains(term, "alacritty") || os.Getenv("KITTY_WINDOW_ID") != "" || os.Getenv("ZED_TERM") != "" || os.Getenv("WT_SESSION") != "" {
		return true
	}
	if n, _ := strconv.Atoi(os.Getenv("VTE_VERSION")); n >= 6800 {
		return true
	}
	return false
}

func SupportsExtendedKeys() bool {
	if os.Getenv("TMUX") != "" {
		return true
	}
	p := os.Getenv("TERM_PROGRAM")
	term := os.Getenv("TERM")
	return p == "iTerm.app" || p == "WezTerm" || p == "ghostty" || p == "kitty" || strings.Contains(term, "kitty") || os.Getenv("WT_SESSION") != ""
}
func HasCursorUpViewportYankBug() bool {
	return runtime.GOOS == "windows" || os.Getenv("WT_SESSION") != ""
}
func NeedsSoftwareBidi() bool {
	return runtime.GOOS == "windows" || os.Getenv("WT_SESSION") != "" || os.Getenv("TERM_PROGRAM") == "vscode"
}

func WrapForMultiplexer(seq string) string {
	if os.Getenv("TMUX") != "" {
		return ESC + "Ptmux;" + strings.ReplaceAll(seq, ESC, ESC+ESC) + ST
	}
	if os.Getenv("STY") != "" {
		return ESC + "P" + seq + ST
	}
	return seq
}
func ClipboardOSC52(text string) string {
	return WrapForMultiplexer(OSC(52, "c", base64.StdEncoding.EncodeToString([]byte(text))))
}
func TerminalTitle(title string) string { return OSC(0, StripANSI(title)) }
func Bell() string                      { return BEL }
func NotifyITerm2(message, title string) string {
	if title != "" {
		message = title + ":\n" + message
	}
	return WrapForMultiplexer(OSC(9, "\n\n"+message))
}
func NotifyGhostty(message, title string) string {
	return WrapForMultiplexer(OSC(777, "notify", title, message))
}
func NotifyKitty(message, title string, id int) string {
	return WrapForMultiplexer(OSC(99, fmt.Sprintf("i=%d:d=0:p=title", id), title)) + WrapForMultiplexer(OSC(99, fmt.Sprintf("i=%d:p=body", id), message))
}

type ProgressState string

const (
	ProgressRunning       ProgressState = "running"
	ProgressCompleted     ProgressState = "completed"
	ProgressError         ProgressState = "error"
	ProgressIndeterminate ProgressState = "indeterminate"
)

func ProgressSequence(state ProgressState, percentage int) string {
	percentage = max(0, min(100, percentage))
	op := 0
	val := ""
	switch state {
	case ProgressRunning:
		op = 1
		val = strconv.Itoa(percentage)
	case ProgressError:
		op = 2
		val = strconv.Itoa(percentage)
	case ProgressIndeterminate:
		op = 3
	case ProgressCompleted:
		op = 0
	}
	return WrapForMultiplexer(OSC(9, 4, op, val))
}

type TabStatusKind string

const (
	TabIdle    TabStatusKind = "idle"
	TabBusy    TabStatusKind = "busy"
	TabWaiting TabStatusKind = "waiting"
)

func TabStatus(kind TabStatusKind) string {
	indicator, status, color := "#00d75f", "Idle", "#888888"
	switch kind {
	case TabBusy:
		indicator, status, color = "#ff9500", "Working…", "#ff9500"
	case TabWaiting:
		indicator, status, color = "#5f87ff", "Waiting", "#5f87ff"
	}
	return WrapForMultiplexer(OSC(21337, "indicator="+indicator+";status="+escapeTabStatus(status)+";status-color="+color))
}
func ClearTabStatus() string {
	return WrapForMultiplexer(OSC(21337, "indicator=;status=;status-color="))
}
func escapeTabStatus(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	return strings.ReplaceAll(s, ";", "\\;")
}
