// Package input exposes terminal input parsing and event types.
package input

import tui "github.com/frudas24/inkgo"

type (
	Parser           = tui.InputParser
	Kind             = tui.InputKind
	Parsed           = tui.ParsedInput
	Mouse            = tui.ParsedMouse
	TerminalResponse = tui.TerminalResponse
	Key              = tui.Key
	KeyboardEvent    = tui.KeyboardEvent
	ClickEvent       = tui.ClickEvent
	FocusEvent       = tui.FocusEvent
)

const (
	KeyInput      = tui.InputKey
	MouseInput    = tui.InputMouse
	PasteInput    = tui.InputPaste
	ResponseInput = tui.InputResponse
)

var NewParser = tui.NewInputParser
