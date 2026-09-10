package inkgo

import parser "github.com/frudas24/inkgo/internal/inputparser"

type (
	InputKind        = parser.InputKind
	ParsedInput      = parser.ParsedInput
	ParsedMouse      = parser.ParsedMouse
	TerminalResponse = parser.TerminalResponse
	InputParser      = parser.InputParser
)

const (
	InputKey      = parser.InputKey
	InputMouse    = parser.InputMouse
	InputPaste    = parser.InputPaste
	InputResponse = parser.InputResponse
)

var NewInputParser = parser.NewInputParser

func nextEscapeSequence(s string) (string, bool) { return parser.NextEscapeSequence(s) }
