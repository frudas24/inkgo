package inkgo

import (
	"encoding/base64"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
)

// Capabilities is an immutable snapshot of terminal behavior relevant to the
// renderer/runtime. Keeping detection centralized prevents different domains
// from making contradictory decisions from the same environment.
type Capabilities struct {
	Hyperlinks              bool
	SynchronizedOutput      bool
	ExtendedKeys            bool
	SoftwareBidi            bool
	CursorUpViewportYankBug bool
	ProgressReporting       bool
	XtermJS                 bool
	TerminalName            string
}

// DetectCapabilities snapshots capabilities from the current process
// environment. terminalName may be an XTVERSION response; pass "" before the
// asynchronous identity probe completes.
func DetectCapabilities(terminalName string) Capabilities {
	isTTY := false
	if fi, err := os.Stdout.Stat(); err == nil {
		isTTY = fi.Mode()&os.ModeCharDevice != 0
	}
	return detectCapabilities(os.Getenv, runtime.GOOS, isTTY, terminalName)
}

func detectCapabilities(getenv func(string) string, goos string, isTTY bool, terminalName string) Capabilities {
	p := getenv("TERM_PROGRAM")
	lc := getenv("LC_TERMINAL")
	term := getenv("TERM")
	wt := getenv("WT_SESSION") != ""
	tmux := getenv("TMUX") != ""

	knownLinks := map[string]bool{"ghostty": true, "Hyper": true, "kitty": true, "alacritty": true, "iTerm.app": true, "iTerm2": true, "WezTerm": true, "WarpTerminal": true}
	hyperlinks := knownLinks[p] || knownLinks[lc] || strings.Contains(term, "kitty") || wt

	syncOutput := false
	if !tmux {
		knownSync := map[string]bool{"iTerm.app": true, "WezTerm": true, "WarpTerminal": true, "ghostty": true, "contour": true, "vscode": true, "alacritty": true}
		syncOutput = knownSync[p] || strings.Contains(term, "kitty") || term == "xterm-ghostty" || strings.HasPrefix(term, "foot") || strings.Contains(term, "alacritty") || getenv("KITTY_WINDOW_ID") != "" || getenv("ZED_TERM") != "" || wt
		if !syncOutput {
			if n, _ := strconv.Atoi(getenv("VTE_VERSION")); n >= 6800 {
				syncOutput = true
			}
		}
	}

	extended := tmux || p == "iTerm.app" || p == "WezTerm" || p == "ghostty" || p == "kitty" || strings.Contains(term, "kitty") || wt
	xtermJS := p == "vscode" || strings.HasPrefix(terminalName, "xterm.js")
	softwareBidi := goos == "windows" || wt || p == "vscode" || strings.HasPrefix(terminalName, "xterm.js")
	yankBug := goos == "windows" || wt

	progress := false
	if isTTY && !wt {
		conemu := getenv("ConEmuANSI") != "" || getenv("ConEmuPID") != "" || getenv("ConEmuTask") != ""
		switch {
		case conemu:
			progress = true
		case p == "ghostty" && semverAtLeast(getenv("TERM_PROGRAM_VERSION"), 1, 2, 0):
			progress = true
		case p == "iTerm.app" && semverAtLeast(getenv("TERM_PROGRAM_VERSION"), 3, 6, 6):
			progress = true
		}
	}

	return Capabilities{
		Hyperlinks: hyperlinks, SynchronizedOutput: syncOutput, ExtendedKeys: extended,
		SoftwareBidi: softwareBidi, CursorUpViewportYankBug: yankBug,
		ProgressReporting: progress, XtermJS: xtermJS, TerminalName: terminalName,
	}
}

func semverAtLeast(v string, wantMajor, wantMinor, wantPatch int) bool {
	v = strings.TrimSpace(strings.TrimPrefix(v, "v"))
	if v == "" {
		return false
	}
	parts := strings.FieldsFunc(v, func(r rune) bool { return r == '.' || r == '-' || r == '+' })
	got := [3]int{}
	for i := 0; i < len(parts) && i < 3; i++ {
		n, err := strconv.Atoi(parts[i])
		if err != nil {
			return false
		}
		got[i] = n
	}
	want := [3]int{wantMajor, wantMinor, wantPatch}
	for i := range got {
		if got[i] != want[i] {
			return got[i] > want[i]
		}
	}
	return true
}

// SupportsProgressReporting mirrors the source fork's OSC 9;4 capability
// check, including Windows Terminal exclusion and version gates.
func SupportsProgressReporting() bool { return DetectCapabilities("").ProgressReporting }

func SupportsHyperlinks() bool { return DetectCapabilities("").Hyperlinks }

func SupportsSynchronizedOutput() bool { return DetectCapabilities("").SynchronizedOutput }

func SupportsExtendedKeys() bool       { return DetectCapabilities("").ExtendedKeys }
func HasCursorUpViewportYankBug() bool { return DetectCapabilities("").CursorUpViewportYankBug }
func NeedsSoftwareBidi() bool          { return DetectCapabilities("").SoftwareBidi }

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

// GetClearTerminalSequence clears the visible screen and, on modern
// terminals, scrollback as well. Legacy Windows console uses HVP home.
func GetClearTerminalSequence() string {
	if runtime.GOOS == "windows" {
		modern := os.Getenv("WT_SESSION") != "" ||
			(os.Getenv("TERM_PROGRAM") == "vscode" && os.Getenv("TERM_PROGRAM_VERSION") != "") ||
			os.Getenv("TERM_PROGRAM") == "mintty" || os.Getenv("MSYSTEM") != ""
		if !modern {
			return EraseScreen + CSI(0, "f")
		}
	}
	return EraseScreen + EraseScrollback + CursorHome
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
	return WrapForMultiplexer(OSC(99, fmt.Sprintf("i=%d:d=0:p=title", id), title)) +
		WrapForMultiplexer(OSC(99, fmt.Sprintf("i=%d:p=body", id), message)) +
		WrapForMultiplexer(OSC(99, fmt.Sprintf("i=%d:d=1:a=focus", id), ""))
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

// SupportsTabStatus mirrors the fork's temporary feature gate for OSC 21337.
// The protocol is safe for unknown terminals, but the source only enables it
// for Ant users while the extension remains unstable.
func SupportsTabStatus() bool { return os.Getenv("USER_TYPE") == "ant" }
