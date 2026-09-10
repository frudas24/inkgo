// Package render exposes screen-buffer and renderer primitives.
package render

import tui "github.com/frudas24/inkgo"

type (
	Renderer          = tui.Renderer
	RenderOptions     = tui.RenderOptions
	Frame             = tui.Frame
	Screen            = tui.Screen
	Cell              = tui.Cell
	CellWidth         = tui.CellWidth
	Damage            = tui.Damage
	Cursor            = tui.Cursor
	CursorDeclaration = tui.CursorDeclaration
	TextStyle         = tui.TextStyle
	Color             = tui.Color
	ColorKind         = tui.ColorKind
	Node              = tui.Node
)

const (
	CellNormal     = tui.CellNormal
	CellWide       = tui.CellWide
	CellSpacerTail = tui.CellSpacerTail
	CellSpacerHead = tui.CellSpacerHead

	ColorUnset   = tui.ColorUnset
	ColorANSI    = tui.ColorANSI
	ColorANSI256 = tui.ColorANSI256
	ColorRGB     = tui.ColorRGB
)

var (
	New            = tui.NewRenderer
	NewScreen      = tui.NewScreen
	ScreenDamage   = tui.ScreenDamage
	DiffScreens    = tui.DiffScreens
	DiffRelative   = tui.DiffScreensRelative
	RenderToScreen = tui.RenderToScreen
	ANSIColor      = tui.ANSIColor
	ANSI256        = tui.ANSI256
	RGB            = tui.RGB
	Hex            = tui.Hex
	ParseColor     = tui.ParseColor
)
