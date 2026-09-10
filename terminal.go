package ink

import (
	"errors"
	"io"
	"os"
	"sync/atomic"
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

// Runtime is the non-React equivalent of Ink's App/root/input plumbing.
// Callers retain the Node tree and mutate it directly; Runtime handles focus,
// terminal input dispatch, scroll wheel routing and incremental rendering.
type Runtime struct {
	Root            *Node
	Renderer        *Renderer
	Focus           *FocusManager
	Parser          *InputParser
	In              io.Reader
	Out             io.Writer
	Terminal        *Terminal
	Hover           map[*Node]struct{}
	Selection       *Selection
	Selecting       bool
	OnSelection     func(string)
	OnPaste         func(string)
	OnTerminalFocus func(bool)
	OnResponse      func(TerminalResponse)
	stopped         atomic.Bool
}

func NewRuntime(root *Node, in io.Reader, out io.Writer, opts RenderOptions) *Runtime {
	fm := NewFocusManager(root)
	fm.AutoFocus()
	return &Runtime{Root: root, Renderer: NewRenderer(opts), Focus: fm, Parser: NewInputParser(), In: in, Out: out, Hover: map[*Node]struct{}{}}
}

// Stop requests a clean Run() exit after the current input dispatch.
func (rt *Runtime) Stop()         { rt.stopped.Store(true) }
func (rt *Runtime) Stopped() bool { return rt.stopped.Load() }

func (rt *Runtime) RefreshSize() {
	if rt.Terminal == nil {
		return
	}
	if sz, err := rt.Terminal.Size(); err == nil && sz.Width > 0 && sz.Height > 0 {
		rt.Renderer.SetSize(sz.Width, sz.Height)
	}
}
func (rt *Runtime) Render() (Frame, error) {
	rt.RefreshSize()
	return rt.Renderer.WriteFrame(rt.Out, rt.Root)
}

func (rt *Runtime) HandleInput(data []byte) []ParsedInput {
	inputs := rt.Parser.Feed(data)
	for _, in := range inputs {
		rt.dispatch(in)
	}
	return inputs
}
func (rt *Runtime) dispatch(in ParsedInput) {
	switch in.Kind {
	case InputPaste:
		if rt.OnPaste != nil {
			rt.OnPaste(in.Paste)
		}
	case InputResponse:
		if in.Response.Type == "focus-in" {
			if rt.OnTerminalFocus != nil {
				rt.OnTerminalFocus(true)
			}
		} else if in.Response.Type == "focus-out" {
			if rt.OnTerminalFocus != nil {
				rt.OnTerminalFocus(false)
			}
		} else if rt.OnResponse != nil {
			rt.OnResponse(in.Response)
		}
	case InputMouse:
		x, y := in.Mouse.Col-1, in.Mouse.Row-1
		rt.Hover = DispatchHover(rt.Root, x, y, rt.Hover)
		button := in.Mouse.Button & 3
		motion := in.Mouse.Button&0x20 != 0
		if button == 0 {
			if in.Mouse.Action == "press" && !motion {
				sel := Selection{Anchor: Point{X: x, Y: y}, Focus: Point{X: x, Y: y}}
				rt.Selection = &sel
				rt.Selecting = true
				DispatchClick(rt.Root, x, y, button)
			} else if motion && rt.Selecting && rt.Selection != nil {
				rt.Selection.Focus = Point{X: x, Y: y}
			} else if in.Mouse.Action == "release" && rt.Selecting && rt.Selection != nil {
				rt.Selection.Focus = Point{X: x, Y: y}
				rt.Selecting = false
				if rt.OnSelection != nil && rt.Renderer.prev != nil {
					rt.OnSelection(rt.Selection.Text(rt.Renderer.prev))
				}
			}
		} else if in.Mouse.Action == "press" && !motion {
			DispatchClick(rt.Root, x, y, button)
		}
	case InputKey:
		if in.Key.Name == "tab" && !in.Key.Ctrl && !in.Key.Alt {
			if in.Key.Shift {
				rt.Focus.FocusPrevious()
			} else {
				rt.Focus.FocusNext()
			}
			return
		}
		if in.Key.Name == "wheelup" || in.Key.Name == "wheeldown" {
			dy := -3
			if in.Key.Name == "wheeldown" {
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
		target := rt.Focus.Focused()
		if target == nil {
			target = rt.Root
		}
		DispatchKey(target, in.Key)
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

// Run owns terminal raw mode until EOF/error. It is deliberately synchronous;
// applications that already have an event loop can use HandleInput + Render.
func (rt *Runtime) Run() error {
	if rt.In == nil || rt.Out == nil {
		return errors.New("runtime requires input and output")
	}
	var raw *RawTerminal
	if rt.Terminal != nil && rt.Terminal.In != nil {
		raw, _ = MakeRaw(rt.Terminal.In)
		if raw != nil {
			defer raw.Restore()
		}
	}
	if rt.Terminal != nil && rt.Terminal.Out != nil {
		if restore, err := prepareOutput(rt.Terminal.Out); err == nil && restore != nil {
			defer restore()
		}
	}
	if _, err := io.WriteString(rt.Out, rt.Renderer.EnterSequence(rt.Root)); err != nil {
		return err
	}
	defer io.WriteString(rt.Out, rt.Renderer.ExitSequence(rt.Root))
	if _, err := rt.Render(); err != nil {
		return err
	}
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
			if _, rerr := rt.Render(); rerr != nil {
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
