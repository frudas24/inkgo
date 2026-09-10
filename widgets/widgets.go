// Package widgets contains the declarative host-node constructors used to
// build a TUI tree. Nodes are ordinary Go values with stable references.
package widgets

import tui "github.com/frudas24/inkgo"

type (
	Node          = tui.Node
	NodeKind      = tui.NodeKind
	Style         = tui.Style
	TextStyle     = tui.TextStyle
	ButtonState   = tui.ButtonState
	EventHandlers = tui.EventHandlers
	ScrollAnchor  = tui.ScrollAnchor
)

const (
	NodeRoot            = tui.NodeRoot
	NodeBox             = tui.NodeBox
	NodeText            = tui.NodeText
	NodeRawANSI         = tui.NodeRawANSI
	NodeLink            = tui.NodeLink
	NodeButton          = tui.NodeButton
	NodeAlternateScreen = tui.NodeAlternateScreen
)

var (
	Root             = tui.Root
	Box              = tui.Box
	Text             = tui.Text
	TextWithWrap     = tui.TextWithWrap
	RawANSI          = tui.RawANSI
	Link             = tui.Link
	Spacer           = tui.Spacer
	Newline          = tui.Newline
	NoSelectBox      = tui.NoSelectBox
	NoSelectFromLeft = tui.NoSelectFromLeft
	ScrollBox        = tui.ScrollBox
	Button           = tui.Button
	ButtonWithState  = tui.ButtonWithState
	AlternateScreen  = tui.AlternateScreen
	ErrorOverview    = tui.ErrorOverview
	ErrorfOverview   = tui.ErrorfOverview
)
