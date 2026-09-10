package inkgo

import (
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Terminal struct {
	In  *os.File
	Out *os.File
}

func DefaultTerminal() Terminal { return Terminal{In: os.Stdin, Out: os.Stdout} }
func (t Terminal) Size() (Size, error) {
	if t.Out == nil {
		return Size{}, errors.New("nil terminal output")
	}
	return terminalSize(t.Out)
}

type RawTerminal struct {
	file  *os.File
	state any
}

func MakeRaw(file *os.File) (*RawTerminal, error) {
	if file == nil {
		return nil, errors.New("nil terminal input")
	}
	s, err := makeRaw(file)
	if err != nil {
		return nil, err
	}
	return &RawTerminal{file: file, state: s}, nil
}
func (r *RawTerminal) Restore() error {
	if r == nil || r.file == nil || r.state == nil {
		return nil
	}
	err := restoreRaw(r.file, r.state)
	if err == nil {
		r.state = nil
	}
	return err
}

const (
	defaultMultiClickTimeout  = 500 * time.Millisecond
	defaultMultiClickDistance = 1
)

type mousePressState struct {
	active  bool
	button  int
	point   Point
	dragged bool
}

type TerminalFocusState int32

const (
	TerminalFocusUnknown TerminalFocusState = iota
	TerminalFocused
	TerminalBlurred
)

// Runtime owns the terminal-facing event loop while the application owns the
// Node tree. It deliberately keeps UI state explicit and inspectable so it can
// be embedded in larger Go programs without a hidden reconciler/event loop.
type Runtime struct {
	Root     *Node
	Renderer *Renderer
	Focus    *FocusManager
	Parser   *InputParser
	Querier  *TerminalQuerier
	In       io.Reader
	Out      io.Writer
	Terminal *Terminal

	Hover     map[*Node]struct{}
	Selection *Selection

	// Selecting is kept for source compatibility; Selection.Dragging is the
	// canonical state used by the runtime.
	Selecting bool

	OnSelection       func(string)
	OnSelectionChange func(*Selection)
	OnPaste           func(string)
	OnTerminalFocus   func(bool)
	OnResponse        func(TerminalResponse)
	OnHyperlink       func(string)

	MouseClicksDisabled bool
	MultiClickTimeout   time.Duration
	MultiClickDistance  int
	// ReassertAfter restores DEC/extended-key modes after this much input
	// silence. Zero disables the gap detector. The default is five seconds.
	ReassertAfter time.Duration
	EscapeTimeout time.Duration
	PasteTimeout  time.Duration

	// ScrollFrameInterval controls follow-up drain frames after a wheel burst.
	// MaxScrollDrainFrames bounds one settled-render call defensively.
	ScrollFrameInterval  time.Duration
	MaxScrollDrainFrames int

	// TerminalName is populated from XTVERSION when the terminal replies.
	// It is per-runtime so multiple embedded PTYs do not share mutable state.
	TerminalName  string
	terminalFocus atomic.Int32

	lastInputTime time.Time
	lastClickTime time.Time
	lastClick     Point
	clickCount    int
	press         mousePressState
	pendingLink   *time.Timer

	lifecycleMu     sync.Mutex
	writeMu         sync.Mutex
	incompleteMu    sync.Mutex
	incompleteTimer *time.Timer
	raw             *RawTerminal
	outputRestore   func() error
	entered         bool
	stopped         atomic.Bool
}

type runtimeWriter struct{ rt *Runtime }

func (w runtimeWriter) Write(p []byte) (int, error) {
	if w.rt == nil || w.rt.Out == nil {
		return len(p), nil
	}
	w.rt.writeMu.Lock()
	defer w.rt.writeMu.Unlock()
	return w.rt.Out.Write(p)
}

func NewRuntime(root *Node, in io.Reader, out io.Writer, opts RenderOptions) *Runtime {
	fm := NewFocusManager(root)
	fm.AutoFocus()
	sel := &Selection{}
	renderer := NewRenderer(opts)
	renderer.SetSelection(sel)
	rt := &Runtime{
		Root:                 root,
		Renderer:             renderer,
		Focus:                fm,
		Parser:               NewInputParser(),
		In:                   in,
		Out:                  out,
		Hover:                map[*Node]struct{}{},
		Selection:            sel,
		MultiClickTimeout:    defaultMultiClickTimeout,
		MultiClickDistance:   defaultMultiClickDistance,
		ReassertAfter:        5 * time.Second,
		EscapeTimeout:        50 * time.Millisecond,
		PasteTimeout:         500 * time.Millisecond,
		ScrollFrameInterval:  4 * time.Millisecond,
		MaxScrollDrainFrames: 64,
	}
	if out != nil {
		rt.Querier = NewTerminalQuerier(runtimeWriter{rt: rt})
	}
	return rt
}

func (rt *Runtime) Stop()         { rt.stopped.Store(true) }
func (rt *Runtime) Stopped() bool { return rt.stopped.Load() }

// TerminalFocusState returns focused, blurred, or unknown when the terminal
// has not emitted DECSET 1004 focus events yet. Unknown is treated as focused
// by TerminalFocused, matching the source fork's throttling behavior.
func (rt *Runtime) TerminalFocusState() TerminalFocusState {
	if rt == nil {
		return TerminalFocusUnknown
	}
	return TerminalFocusState(rt.terminalFocus.Load())
}

func (rt *Runtime) TerminalFocused() bool {
	return rt.TerminalFocusState() != TerminalBlurred
}

// SetRoot swaps the application tree without rebuilding terminal state. Focus
// is preserved only when the focused node belongs to the new tree; otherwise
// the runtime blurs it and applies the new tree's AutoFocus policy.
func (rt *Runtime) SetRoot(root *Node) {
	if rt == nil || root == nil || rt.Root == root {
		return
	}
	oldFocused := rt.Focus.Focused()
	if oldFocused != nil && oldFocused != root && !oldFocused.IsDescendantOf(root) {
		rt.Focus.Blur()
	}
	rt.Root = root
	rt.Focus.SetRoot(root)
	if rt.Focus.Focused() == nil {
		rt.Focus.AutoFocus()
	}
	if rt.Renderer != nil {
		rt.Renderer.Invalidate()
	}
}

// WriteRaw writes a terminal control sequence or other out-of-band payload.
// It is the Go equivalent of the fork's TerminalWriteContext.
func (rt *Runtime) WriteRaw(data string) error {
	if rt == nil || rt.Out == nil || data == "" {
		return nil
	}
	rt.writeMu.Lock()
	defer rt.writeMu.Unlock()
	_, err := io.WriteString(rt.Out, data)
	return err
}

// ClearTerminal clears the terminal and invalidates renderer assumptions so
// the next frame is a complete redraw rather than a diff against stale cells.
func (rt *Runtime) ClearTerminal() error {
	if rt == nil {
		return nil
	}
	if err := rt.WriteRaw(GetClearTerminalSequence()); err != nil {
		return err
	}
	if rt.Renderer != nil {
		rt.Renderer.Invalidate()
	}
	return nil
}

func (rt *Runtime) RefreshSize() {
	if rt.Terminal == nil {
		return
	}
	if sz, err := rt.Terminal.Size(); err == nil && sz.Width > 0 && sz.Height > 0 {
		previous := rt.Renderer.Viewport()
		rt.Renderer.SetSize(sz.Width, sz.Height)
		if previous.Width != sz.Width || previous.Height != sz.Height {
			DispatchResize(rt.Root, sz.Width, sz.Height)
		}
	}
}

func (rt *Runtime) Render() (Frame, error) {
	rt.RefreshSize()
	rt.configureScrollPolicy()
	frame := rt.Renderer.Render(rt.Root)
	if frame.Patch == "" {
		return frame, nil
	}
	return frame, rt.WriteRaw(frame.Patch)
}

func (rt *Runtime) configureScrollPolicy() {
	if rt == nil || rt.Root == nil || !rt.IsXtermJS() {
		return
	}
	rt.Root.Walk(func(n *Node) bool {
		if n.Style.OverflowY == OverflowScroll || n.Style.Overflow == OverflowScroll {
			n.ScrollAdaptive = true
		}
		return true
	})
}

// RenderSettled renders one frame and then drains any outstanding ScrollBox
// wheel delta across follow-up frames. Embedded applications can call this
// after mutating UI state when they want the same smooth-drain contract as Run.
func (rt *Runtime) RenderSettled() (Frame, error) {
	frame, err := rt.Render()
	if err != nil {
		return frame, err
	}
	limit := rt.MaxScrollDrainFrames
	if limit <= 0 {
		limit = 64
	}
	for i := 0; frame.ScrollDrainPending && i < limit; i++ {
		if rt.ScrollFrameInterval > 0 {
			time.Sleep(rt.ScrollFrameInterval)
		}
		frame, err = rt.Render()
		if err != nil {
			return frame, err
		}
	}
	return frame, nil
}

func (rt *Runtime) HandleInput(data []byte) []ParsedInput {
	now := time.Now()
	rt.lifecycleMu.Lock()
	entered := rt.entered
	rt.lifecycleMu.Unlock()
	if entered && rt.ReassertAfter > 0 && !rt.lastInputTime.IsZero() && now.Sub(rt.lastInputTime) > rt.ReassertAfter {
		rt.ReassertTerminalModes(false)
	}
	rt.lastInputTime = now
	rt.cancelIncompleteTimer()
	inputs := rt.Parser.Feed(data)
	for _, in := range inputs {
		rt.dispatch(in)
	}
	rt.scheduleIncompleteFlush()
	return inputs
}

func (rt *Runtime) cancelIncompleteTimer() {
	rt.incompleteMu.Lock()
	if rt.incompleteTimer != nil {
		rt.incompleteTimer.Stop()
		rt.incompleteTimer = nil
	}
	rt.incompleteMu.Unlock()
}

func (rt *Runtime) scheduleIncompleteFlush() {
	if rt == nil || rt.Parser == nil || !rt.Parser.Pending() {
		return
	}
	d := rt.EscapeTimeout
	if rt.Parser.InPaste() {
		d = rt.PasteTimeout
	}
	if d <= 0 {
		return
	}
	rt.incompleteMu.Lock()
	rt.incompleteTimer = time.AfterFunc(d, func() { rt.flushIncompleteInput() })
	rt.incompleteMu.Unlock()
}

func (rt *Runtime) flushIncompleteInput() []ParsedInput {
	rt.incompleteMu.Lock()
	rt.incompleteTimer = nil
	rt.incompleteMu.Unlock()
	if rt == nil || rt.Parser == nil {
		return nil
	}
	inputs := rt.Parser.Flush()
	for _, in := range inputs {
		rt.dispatch(in)
	}
	if len(inputs) > 0 {
		rt.lifecycleMu.Lock()
		entered := rt.entered
		rt.lifecycleMu.Unlock()
		if entered {
			_, _ = rt.Render()
		}
	}
	return inputs
}

// Viewport returns the renderer's current terminal dimensions.
func (rt *Runtime) Viewport() Size {
	if rt == nil || rt.Renderer == nil {
		return Size{}
	}
	return rt.Renderer.Viewport()
}

// IsVisible reports whether a laid-out node intersects the current terminal
// viewport after accounting for ancestor scroll offsets.
func (rt *Runtime) IsVisible(node *Node) bool {
	if rt == nil || rt.Renderer == nil {
		return false
	}
	return rt.Renderer.IsVisible(node)
}

// SetSearchHighlight enables visible-screen search highlighting.
func (rt *Runtime) SetSearchHighlight(query string, current int) {
	if rt != nil && rt.Renderer != nil {
		rt.Renderer.SetSearchHighlight(query, current)
	}
}

// SetSearchPositions installs pre-scanned search positions for virtualized
// content.
func (rt *Runtime) SetSearchPositions(positions []MatchPosition, rowOffset, current int) {
	if rt != nil && rt.Renderer != nil {
		rt.Renderer.SetSearchPositions(positions, rowOffset, current)
	}
}

// ClearSearch removes both visible scan and virtualized positioned search
// highlights.
func (rt *Runtime) ClearSearch() {
	if rt == nil || rt.Renderer == nil {
		return
	}
	rt.Renderer.SetSearchHighlight("", -1)
	rt.Renderer.SetSearchPositions(nil, 0, -1)
}

// SelectionText returns the current selection without modifying it.
func (rt *Runtime) SelectionText() string {
	if rt == nil || rt.Selection == nil || !rt.Selection.HasSelection() {
		return ""
	}
	screen := rt.selectionScreen()
	if screen == nil {
		return ""
	}
	return rt.Selection.Text(screen)
}

// CopySelection writes the best available clipboard escape sequence and, when
// clear is true, clears the visual selection after copying.
func (rt *Runtime) CopySelection(clear bool) (text string, path ClipboardPath, err error) {
	text = rt.SelectionText()
	if text == "" {
		return "", "", nil
	}
	sequence, path := SetClipboard(text)
	if rt.Out != nil && sequence != "" {
		if err = rt.WriteRaw(sequence); err != nil {
			return text, path, err
		}
	}
	if clear && rt.Selection != nil {
		rt.Selection.Clear()
		rt.notifySelection()
	}
	return text, path, nil
}

func (rt *Runtime) notifySelection() {
	rt.Selecting = rt.Selection != nil && rt.Selection.Dragging
	if rt.OnSelectionChange != nil {
		rt.OnSelectionChange(rt.Selection)
	}
}

func (rt *Runtime) selectionScreen() *Screen {
	if rt.Renderer == nil {
		return nil
	}
	return rt.Renderer.Previous()
}

func (rt *Runtime) dispatch(in ParsedInput) {
	switch in.Kind {
	case InputPaste:
		target := rt.Focus.Focused()
		if target == nil {
			target = rt.Root
		}
		DispatchPaste(target, in.Paste)
		if rt.OnPaste != nil {
			rt.OnPaste(in.Paste)
		}
	case InputResponse:
		if in.Response.Type == "xtversion" && rt.TerminalName == "" {
			rt.TerminalName = in.Response.Name
		}
		if rt.Querier != nil {
			rt.Querier.OnResponse(in.Response)
		}
		if in.Response.Type == "focus-in" {
			rt.terminalFocus.Store(int32(TerminalFocused))
			if rt.OnTerminalFocus != nil {
				rt.OnTerminalFocus(true)
			}
			return
		}
		if in.Response.Type == "focus-out" {
			rt.terminalFocus.Store(int32(TerminalBlurred))
			// A lost mouse release while switching windows must not leave a
			// drag alive forever.
			if rt.Selection != nil && rt.Selection.Dragging {
				rt.finishSelection()
			}
			if rt.OnTerminalFocus != nil {
				rt.OnTerminalFocus(false)
			}
			return
		}
		if rt.OnResponse != nil {
			rt.OnResponse(in.Response)
		}
	case InputMouse:
		rt.dispatchMouse(in.Mouse)
	case InputKey:
		rt.dispatchKey(in.Key)
	}
}

func (rt *Runtime) dispatchMouse(m ParsedMouse) {
	x, y := m.Col-1, m.Row-1
	button := m.Button & 3
	motion := m.Button&0x20 != 0

	// Hover is independent from click/selection. SGR mode 1003 reports
	// no-button motion as button=3 + motion bit.
	rt.Hover = DispatchHover(rt.Root, x, y, rt.Hover)
	if rt.MouseClicksDisabled {
		return
	}

	if motion && button == 3 {
		// Lost-release recovery: the pointer may have been released outside
		// the terminal window, leaving no SGR release packet.
		if rt.Selection != nil && rt.Selection.Dragging {
			rt.finishSelection()
		}
		return
	}

	if button != 0 {
		// The fork's DOM click event is left-button only. A non-left release
		// can still terminate an orphaned text selection.
		if m.Action == "release" && rt.Selection != nil && rt.Selection.Dragging {
			rt.finishSelection()
		}
		return
	}

	if motion {
		if rt.Selection != nil && rt.Selection.Dragging {
			if x != rt.press.point.X || y != rt.press.point.Y {
				rt.press.dragged = true
			}
			screen := rt.selectionScreen()
			if rt.Selection.Span != nil && screen != nil {
				rt.Selection.Extend(screen, x, y)
			} else {
				rt.Selection.Update(x, y)
			}
			rt.dragScrollAt(Point{X: x, Y: y})
			rt.notifySelection()
		}
		return
	}

	if m.Action == "press" {
		rt.handleLeftPress(m, Point{X: x, Y: y})
		return
	}
	if m.Action == "release" {
		rt.handleLeftRelease(Point{X: x, Y: y})
	}
}

func (rt *Runtime) handleLeftPress(m ParsedMouse, p Point) {
	if rt.Selection == nil {
		rt.Selection = &Selection{}
		rt.Renderer.SetSelection(rt.Selection)
	}
	if rt.Selection.Dragging {
		rt.finishSelection()
	}
	if rt.pendingLink != nil {
		rt.pendingLink.Stop()
		rt.pendingLink = nil
	}

	now := time.Now()
	timeout := rt.MultiClickTimeout
	if timeout <= 0 {
		timeout = defaultMultiClickTimeout
	}
	distance := rt.MultiClickDistance
	if distance < 0 {
		distance = defaultMultiClickDistance
	}
	near := !rt.lastClickTime.IsZero() && now.Sub(rt.lastClickTime) < timeout &&
		absInt(p.X-rt.lastClick.X) <= distance && absInt(p.Y-rt.lastClick.Y) <= distance
	if near {
		rt.clickCount++
	} else {
		rt.clickCount = 1
	}
	rt.lastClickTime, rt.lastClick = now, p
	rt.press = mousePressState{active: true, button: 0, point: p}

	rt.Selection.Start(p.X, p.Y)
	rt.Selection.LastPressHadAlt = m.Button&0x08 != 0
	if rt.clickCount >= 2 {
		screen := rt.selectionScreen()
		if screen != nil {
			if rt.clickCount == 2 {
				if !rt.Selection.SelectWordAt(screen, p.X, p.Y) {
					rt.Selection.Focus = rt.Selection.Anchor
					rt.Selection.FocusSet = true
				}
			} else {
				if !rt.Selection.SelectLineAt(screen, p.Y) {
					rt.Selection.Focus = rt.Selection.Anchor
					rt.Selection.FocusSet = true
				}
			}
		}
	}
	rt.notifySelection()
}

func (rt *Runtime) handleLeftRelease(p Point) {
	if rt.Selection == nil {
		return
	}
	wasDragging := rt.Selection.Dragging
	hadSelection := rt.Selection.HasSelection()
	rt.Selection.Finish()
	rt.Selecting = false

	// A single press+release with no real drag is a click. Dispatch only on
	// release, matching browser/Ink semantics and preventing drags from
	// activating buttons.
	if rt.press.active && !rt.press.dragged && !hadSelection {
		screen := rt.selectionScreen()
		_, handled := DispatchClickDetailed(rt.Root, rt.Focus, screen, p.X, p.Y, 0)
		if !handled && !rt.IsXtermJS() {
			if url, ok := rt.GetHyperlinkAt(p.X, p.Y); ok && rt.OnHyperlink != nil {
				timeout := rt.MultiClickTimeout
				if timeout <= 0 {
					timeout = defaultMultiClickTimeout
				}
				rt.pendingLink = time.AfterFunc(timeout, func() { rt.OnHyperlink(url) })
			}
		}
	}
	rt.press = mousePressState{}
	rt.notifySelection()
	if wasDragging && rt.Selection.HasSelection() && rt.OnSelection != nil {
		if screen := rt.selectionScreen(); screen != nil {
			rt.OnSelection(rt.Selection.Text(screen))
		}
	}
}

func (rt *Runtime) finishSelection() {
	if rt.Selection == nil {
		return
	}
	rt.Selection.Finish()
	rt.press = mousePressState{}
	rt.notifySelection()
	if rt.Selection.HasSelection() && rt.OnSelection != nil {
		if screen := rt.selectionScreen(); screen != nil {
			rt.OnSelection(rt.Selection.Text(screen))
		}
	}
}

// GetHyperlinkAt checks OSC-8 metadata first and then plain-text URL
// linkification, including spacer-tail correction for wide characters.
func (rt *Runtime) GetHyperlinkAt(col, row int) (string, bool) {
	screen := rt.selectionScreen()
	if screen == nil {
		return "", false
	}
	cell, ok := screen.CellAt(col, row)
	if !ok {
		return "", false
	}
	if cell.Hyperlink != "" {
		return cell.Hyperlink, true
	}
	if cell.Width == CellSpacerTail && col > 0 {
		if head, ok := screen.CellAt(col-1, row); ok && head.Hyperlink != "" {
			return head.Hyperlink, true
		}
	}
	return FindPlainTextURLAt(screen, col, row)
}

// IsXtermJS detects VS Code/Cursor/Windsurf-style xterm.js terminals. Those
// terminals already activate links themselves and also forward the SGR click,
// so opening again here would launch duplicate browser tabs.
func (rt *Runtime) IsXtermJS() bool {
	if os.Getenv("TERM_PROGRAM") == "vscode" {
		return true
	}
	return strings.HasPrefix(rt.TerminalName, "xterm.js")
}

// ProbeTerminalIdentity sends XTVERSION followed by a DA1 barrier. The reply is
// consumed through the ordinary input parser and cached in TerminalName.
func (rt *Runtime) ProbeTerminalIdentity() {
	if rt == nil || rt.Querier == nil {
		return
	}
	_ = rt.Querier.Send(QueryXTVERSION())
	_ = rt.Querier.Flush()
}

func (rt *Runtime) dispatchKey(key Key) {
	target := rt.Focus.Focused()
	if target == nil {
		target = rt.Root
	}
	e := DispatchKey(target, key)
	if e != nil && e.DefaultPrevented() {
		return
	}

	if key.Name == "tab" && !key.Ctrl && !key.Alt && !key.Meta {
		if key.Shift {
			rt.Focus.FocusPrevious()
		} else {
			rt.Focus.FocusNext()
		}
		return
	}

	if key.Name == "wheelup" || key.Name == "wheeldown" {
		dy := -3
		if key.Name == "wheeldown" {
			dy = 3
		}
		if sc := nearestScrollBox(rt.Focus.Focused()); sc != nil {
			sc.ScrollBy(dy)
			return
		}
		if sc := firstScrollBox(rt.Root); sc != nil {
			sc.ScrollBy(dy)
			return
		}
	}

	if rt.Selection != nil && rt.Selection.HasSelection() {
		if key.Name == "escape" {
			rt.Selection.Clear()
			rt.notifySelection()
			return
		}
		if key.Shift {
			move := ""
			switch key.Name {
			case "left", "right", "up", "down":
				move = key.Name
			case "home":
				move = "lineStart"
			case "end":
				move = "lineEnd"
			}
			if move != "" {
				if screen := rt.selectionScreen(); screen != nil {
					rt.Selection.MoveFocus(move, screen)
					rt.notifySelection()
				}
				return
			}
		}
	}

	// Ctrl+Z is delivered as a byte in raw mode. On Unix we explicitly
	// restore terminal modes, suspend, then re-enter after SIGCONT.
	if key.Ctrl && key.Name == "z" {
		if suspendRuntime(rt) {
			return
		}
	}
}

func (rt *Runtime) dragScrollAt(p Point) {
	var candidate *Node
	if hit := HitTest(rt.Root, p); hit != nil {
		candidate = nearestScrollBox(hit)
	}
	if candidate == nil {
		return
	}
	vr := visualContentRect(candidate)
	if vr.Height <= 0 {
		return
	}
	if p.Y <= vr.Y {
		candidate.ScrollBy(-1)
	} else if p.Y >= vr.Y+vr.Height-1 {
		candidate.ScrollBy(1)
	}
}

func nearestScrollBox(n *Node) *Node {
	for ; n != nil; n = n.Parent {
		if n.Style.OverflowY == OverflowScroll || n.Style.Overflow == OverflowScroll {
			return n
		}
	}
	return nil
}

func firstScrollBox(root *Node) *Node {
	var out *Node
	if root != nil {
		root.Walk(func(n *Node) bool {
			if n.Style.OverflowY == OverflowScroll || n.Style.Overflow == OverflowScroll {
				out = n
				return false
			}
			return true
		})
	}
	return out
}

// ReassertTerminalModes restores idempotent terminal modes after a long stdin
// gap, tmux attach, SSH reconnect, or wake. includeAltScreen should only be
// true for a strong signal that mode 1049 was lost because it clears screen.
// recoverAfterResume distinguishes our own Ctrl+Z handoff (where terminal
// modes are intentionally down) from an unsolicited SIGCONT (where the pty may
// have silently reset modes while Runtime still believes it is entered).
func (rt *Runtime) recoverAfterResume() {
	if rt == nil {
		return
	}
	rt.lifecycleMu.Lock()
	entered := rt.entered
	rt.lifecycleMu.Unlock()
	if entered {
		rt.ReassertTerminalModes(true)
		return
	}
	rt.ResumeTerminal()
}

func (rt *Runtime) ReassertTerminalModes(includeAltScreen bool) {
	if rt == nil || rt.Renderer == nil || rt.Out == nil {
		return
	}
	rt.lifecycleMu.Lock()
	if !rt.entered {
		rt.lifecycleMu.Unlock()
		return
	}
	sequence := rt.Renderer.ReassertSequence(rt.Root, includeAltScreen)
	if sequence != "" {
		_ = rt.WriteRaw(sequence)
	}
	rt.lifecycleMu.Unlock()
	if includeAltScreen {
		rt.RefreshSize()
		_, _ = rt.Render()
	}
}

// SuspendTerminal restores host terminal state without stopping the Runtime.
// It is useful before launching an external editor or on Ctrl+Z.
func (rt *Runtime) SuspendTerminal() {
	rt.lifecycleMu.Lock()
	defer rt.lifecycleMu.Unlock()
	if !rt.entered {
		return
	}
	_ = rt.WriteRaw(rt.Renderer.ExitSequence(rt.Root))
	if rt.raw != nil {
		_ = rt.raw.Restore()
		rt.raw = nil
	}
	if rt.outputRestore != nil {
		_ = rt.outputRestore()
		rt.outputRestore = nil
	}
	rt.entered = false
}

// ResumeTerminal re-enters raw/VT modes and forces a clean redraw.
func (rt *Runtime) ResumeTerminal() {
	rt.lifecycleMu.Lock()
	defer rt.lifecycleMu.Unlock()
	if rt.entered {
		return
	}
	if rt.Terminal != nil && rt.Terminal.In != nil {
		rt.raw, _ = MakeRaw(rt.Terminal.In)
	}
	if rt.Terminal != nil && rt.Terminal.Out != nil {
		if restore, err := prepareOutput(rt.Terminal.Out); err == nil {
			rt.outputRestore = restore
		}
	}
	rt.Renderer.Invalidate()
	_ = rt.WriteRaw(rt.Renderer.EnterSequence(rt.Root))
	rt.entered = true
	rt.RefreshSize()
	_, _ = rt.Render()
}

func (rt *Runtime) enterTerminal() error {
	rt.lifecycleMu.Lock()
	defer rt.lifecycleMu.Unlock()
	if rt.entered {
		return nil
	}
	if rt.Terminal != nil && rt.Terminal.In != nil {
		rt.raw, _ = MakeRaw(rt.Terminal.In)
	}
	if rt.Terminal != nil && rt.Terminal.Out != nil {
		if restore, err := prepareOutput(rt.Terminal.Out); err == nil {
			rt.outputRestore = restore
		}
	}
	if err := rt.WriteRaw(rt.Renderer.EnterSequence(rt.Root)); err != nil {
		return err
	}
	rt.entered = true
	return nil
}

func (rt *Runtime) leaveTerminal() {
	rt.cancelIncompleteTimer()
	rt.lifecycleMu.Lock()
	defer rt.lifecycleMu.Unlock()
	if rt.pendingLink != nil {
		rt.pendingLink.Stop()
		rt.pendingLink = nil
	}
	if rt.Querier != nil {
		rt.Querier.Close()
	}
	if rt.entered {
		_ = rt.WriteRaw(rt.Renderer.ExitSequence(rt.Root))
	}
	if rt.raw != nil {
		_ = rt.raw.Restore()
		rt.raw = nil
	}
	if rt.outputRestore != nil {
		_ = rt.outputRestore()
		rt.outputRestore = nil
	}
	rt.entered = false
}

// Started reports whether the runtime currently owns terminal modes.
func (rt *Runtime) Started() bool {
	if rt == nil {
		return false
	}
	rt.lifecycleMu.Lock()
	defer rt.lifecycleMu.Unlock()
	return rt.entered
}

// Start enters terminal modes and performs the initial settled render without
// taking ownership of the caller's input loop. This is the preferred entry
// point for embedding in an application that already has a reactor/select
// loop. Calls are idempotent while started.
func (rt *Runtime) Start() error {
	if rt == nil || rt.In == nil || rt.Out == nil {
		return errors.New("runtime requires input and output")
	}
	if err := rt.enterTerminal(); err != nil {
		return err
	}
	rt.ProbeTerminalIdentity()
	_, err := rt.RenderSettled()
	return err
}

// Close restores host terminal state and releases runtime-owned timers/query
// waiters. It is idempotent and is the counterpart to Start for embedded use.
func (rt *Runtime) Close() error {
	if rt == nil {
		return nil
	}
	rt.leaveTerminal()
	return nil
}

// Run owns terminal modes until EOF/error. Applications with their own event
// loop can instead call HandleInput + Render and optionally SuspendTerminal /
// ResumeTerminal around external programs.
func (rt *Runtime) Run() error {
	if rt.In == nil || rt.Out == nil {
		return errors.New("runtime requires input and output")
	}
	if err := rt.Start(); err != nil {
		return err
	}
	defer rt.Close()
	removeSignals := installRuntimeSignalHandlers(rt)
	defer removeSignals()
	buf := make([]byte, 8192)
	for {
		if rt.stopped.Load() {
			return nil
		}
		n, err := rt.In.Read(buf)
		if n > 0 {
			rt.HandleInput(buf[:n])
			if rt.stopped.Load() {
				return nil
			}
			if _, rerr := rt.RenderSettled(); rerr != nil {
				return rerr
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

// SetTerminalTitle writes an ANSI-stripped OSC 0 title sequence through the
// runtime's raw-output channel. It is the imperative Go equivalent of
// useTerminalTitle.
func (rt *Runtime) SetTerminalTitle(title string) error {
	return rt.WriteRaw(TerminalTitle(title))
}

// RingBell emits BEL directly. It is intentionally not wrapped for tmux so
// tmux's bell-action remains functional.
func (rt *Runtime) RingBell() error { return rt.WriteRaw(Bell()) }

// NotifyITerm2 emits an iTerm2 notification.
func (rt *Runtime) NotifyITerm2(message, title string) error {
	return rt.WriteRaw(NotifyITerm2(message, title))
}

// NotifyGhostty emits a Ghostty notification.
func (rt *Runtime) NotifyGhostty(message, title string) error {
	return rt.WriteRaw(NotifyGhostty(message, title))
}

// NotifyKitty emits a Kitty notification with a caller-owned notification id.
func (rt *Runtime) NotifyKitty(message, title string, id int) error {
	return rt.WriteRaw(NotifyKitty(message, title, id))
}

// SetProgress emits OSC 9;4 when the current terminal is known to support it.
// Passing nil clears progress. Unsupported terminals are a successful no-op.
func (rt *Runtime) SetProgress(state *ProgressState, percentage int) error {
	if rt == nil || !SupportsProgressReporting() {
		return nil
	}
	if state == nil {
		return rt.WriteRaw(ProgressSequence(ProgressCompleted, 0))
	}
	return rt.WriteRaw(ProgressSequence(*state, percentage))
}

// SetTabStatus emits or clears the fork's OSC 21337 status indicator. Passing
// nil clears an existing status. Unsupported/user-gated environments are a
// successful no-op.
func (rt *Runtime) SetTabStatus(kind *TabStatusKind) error {
	if rt == nil || !SupportsTabStatus() {
		return nil
	}
	if kind == nil {
		return rt.WriteRaw(ClearTabStatus())
	}
	return rt.WriteRaw(TabStatus(*kind))
}
