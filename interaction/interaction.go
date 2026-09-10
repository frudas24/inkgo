// Package interaction exposes focus, hit-testing, and DOM-like terminal event
// dispatch. It is intentionally separate from raw input parsing: applications
// can synthesize interactions in tests without constructing escape sequences.
package interaction

import tui "github.com/frudas24/inkgo"

type (
	Event          = tui.Event
	Handlers       = tui.EventHandlers
	KeyboardEvent  = tui.KeyboardEvent
	ClickEvent     = tui.ClickEvent
	FocusEvent     = tui.FocusEvent
	MouseMoveEvent = tui.MouseMoveEvent
	PasteEvent     = tui.PasteEvent
	ResizeEvent    = tui.ResizeEvent
	FocusManager   = tui.FocusManager
	Node           = tui.Node
	Key            = tui.Key
	Screen         = tui.Screen
)

var (
	NewFocusManager       = tui.NewFocusManager
	HitTest               = tui.HitTest
	DispatchClick         = tui.DispatchClick
	DispatchClickDetailed = tui.DispatchClickDetailed
	DispatchKey           = tui.DispatchKey
	DispatchPaste         = tui.DispatchPaste
	DispatchResize        = tui.DispatchResize
	DispatchHover         = tui.DispatchHover
)
