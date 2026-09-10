package inkgo

import (
	"time"

	core "github.com/frudas24/inkgo/internal/core"
)

// Event is the common terminal event base. It follows the DOM-ish semantics
// used by the TypeScript fork: preventDefault and stopImmediatePropagation.
type Event struct {
	Target        *Node
	CurrentTarget *Node
	Timestamp     time.Time

	defaultPrevented bool
	stopped          bool
}

func (e *Event) PreventDefault()           { e.defaultPrevented = true }
func (e *Event) DefaultPrevented() bool    { return e.defaultPrevented }
func (e *Event) StopImmediatePropagation() { e.stopped = true }
func (e *Event) PropagationStopped() bool  { return e.stopped }

type Key = core.Key

type KeyboardEvent struct {
	Event
	Key Key
}

type ClickEvent struct {
	Event
	X, Y        int
	LocalX      int
	LocalY      int
	Button      int
	CellIsBlank bool
}

type FocusEvent struct {
	Event
	RelatedTarget *Node
}

type MouseMoveEvent struct {
	Event
	X, Y int
}

type PasteEvent struct {
	Event
	Text string
}

type ResizeEvent struct {
	Event
	Columns int
	Rows    int
}

type TerminalFocusEvent struct {
	Focused bool
}
