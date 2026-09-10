// Package selection exposes text-selection and search-highlight behavior.
package selection

import tui "github.com/frudas24/inkgo"

type (
	State         = tui.Selection
	Mode          = tui.SelectionMode
	Span          = tui.SelectionSpan
	MatchPosition = tui.MatchPosition
	Screen        = tui.Screen
	Point         = tui.Point
	Color         = tui.Color
)

const (
	Char = tui.SelectionChar
	Word = tui.SelectionWord
	Line = tui.SelectionLine
)

var (
	FindPlainTextURLAt = tui.FindPlainTextURLAt
	ApplyOverlay       = tui.ApplySelectionOverlay
	ScanPositions      = tui.ScanPositions
	ApplySearch        = tui.ApplySearchHighlight
	ApplyPositioned    = tui.ApplyPositionedHighlight
)
