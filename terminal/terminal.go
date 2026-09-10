// Package terminal exposes terminal lifecycle, runtime orchestration,
// capability queries, clipboard helpers, and escape-sequence helpers.
package terminal

import (
	"io"

	tui "github.com/frudas24/inkgo"
)

type (
	Capabilities  = tui.Capabilities
	Terminal      = tui.Terminal
	RawTerminal   = tui.RawTerminal
	Runtime       = tui.Runtime
	Query         = tui.TerminalQuery
	Querier       = tui.TerminalQuerier
	Response      = tui.TerminalResponse
	RenderOptions = tui.RenderOptions
	ClipboardPath = tui.ClipboardPath
	ProgressState = tui.ProgressState
	TabStatusKind = tui.TabStatusKind
	FocusState    = tui.TerminalFocusState
)

const (
	ClipboardNative     = tui.ClipboardNative
	ClipboardTmuxBuffer = tui.ClipboardTmuxBuffer
	ClipboardOSC52      = tui.ClipboardOSC52Path

	ProgressRunning       = tui.ProgressRunning
	ProgressCompleted     = tui.ProgressCompleted
	ProgressError         = tui.ProgressError
	ProgressIndeterminate = tui.ProgressIndeterminate

	TabIdle    = tui.TabIdle
	TabBusy    = tui.TabBusy
	TabWaiting = tui.TabWaiting

	FocusUnknown = tui.TerminalFocusUnknown
	Focused      = tui.TerminalFocused
	Blurred      = tui.TerminalBlurred
)

func NewRuntime(root *tui.Node, in io.Reader, out io.Writer, opts RenderOptions) *Runtime {
	return tui.NewRuntime(root, in, out, opts)
}

var (
	Default             = tui.DefaultTerminal
	MakeRaw             = tui.MakeRaw
	NewQuerier          = tui.NewTerminalQuerier
	DECRQM              = tui.QueryDECRQM
	DA1                 = tui.QueryDA1
	DA2                 = tui.QueryDA2
	KittyKeyboard       = tui.QueryKittyKeyboard
	CursorPosition      = tui.QueryCursorPosition
	OSCColor            = tui.QueryOSCColor
	XTVERSION           = tui.QueryXTVERSION
	DetectCapabilities  = tui.DetectCapabilities
	SupportsHyperlinks  = tui.SupportsHyperlinks
	SupportsSyncOutput  = tui.SupportsSynchronizedOutput
	SupportsProgress    = tui.SupportsProgressReporting
	SupportsTabStatus   = tui.SupportsTabStatus
	SupportsExtendedKey = tui.SupportsExtendedKeys
	SetClipboard        = tui.SetClipboard
	GetClipboardPath    = tui.GetClipboardPath
	TerminalTitle       = tui.TerminalTitle
	Bell                = tui.Bell
	ProgressSequence    = tui.ProgressSequence
	TabStatus           = tui.TabStatus
	ClearTabStatus      = tui.ClearTabStatus
	ClearSequence       = tui.GetClearTerminalSequence
	NotifyITerm2        = tui.NotifyITerm2
	NotifyGhostty       = tui.NotifyGhostty
	NotifyKitty         = tui.NotifyKitty
)
