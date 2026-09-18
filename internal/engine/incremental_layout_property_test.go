package engine

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"time"
)

// The property test below is the adversarial half of the incremental-layout
// coverage. It drives a deterministic pseudo-random mutation sequence through
// the public mutation API of a transcript-shaped tree and, after every single
// step, compares the incrementally laid-out tree against the same tree laid out
// from zero with the same final state.
//
// The equivalence it pins is the one the prune depends on: a pass that skips a
// clean subtree must leave exactly the state a pass that walks every node would
// have produced. Every failure message carries the step number, the mutation
// applied and the path of the node, so the sequence is reproducible.
//
// Nothing here reimplements the layout algorithm: the production path is the
// oracle, so a divergence between the two passes is always a defect of the
// incremental one.

const (
	// ilpSeed keeps the sequence reproducible: rerunning the test with the same
	// seed replays the exact same mutations.
	ilpSeed = 0x5eed
	// ilpEntries is the number of Text rows in the scroll body.
	ilpEntries = 8
)

// ilpPropertySteps bounds the generated sequence. Raise it to fuzz harder; the
// default pays two full passes per step.
var ilpPropertySteps = 400

// ilpViewports are the terminal sizes the sequence switches between. A viewport
// change is not a node mutation, yet it must not leave a stale subtree behind.
var ilpViewports = []Size{
	{Width: 48, Height: 12},
	{Width: 32, Height: 9},
	{Width: 60, Height: 20},
}

// ilpTree collects the handles the mutation sequence needs. Every node the
// sequence can touch is reachable through this struct.
type ilpTree struct {
	root       *Node
	header     *Node
	body       *Node // outer ScrollBox
	content    *Node // ScrollContent() of body
	inner      *Node // nested ScrollBox inside body
	group      *Node // Append/Remove target inside body
	panel      *Node
	panelMid   *Node
	fixed      *Node
	fixedInner *Node
	relative   *Node
	absolute   *Node
	raw        *Node
	button     *Node
	stretched  *Node
	footer     *Node
	entries    []*Node
}

// ilpBuildTree assembles the transcript shape: a header row with a
// ButtonRender button, a ScrollBox holding N Text rows plus a group, a raw-ANSI
// row, an absolute overlay and a nested ScrollBox, a flex panel with
// grow/stretch/justify, a fixed-size box, a relative-offset box and the footer.
func ilpBuildTree(entries int, sticky, grow bool) *ilpTree {
	rows := make([]*Node, 0, entries)
	for i := 0; i < entries; i++ {
		rows = append(rows, Text(fmt.Sprintf("row %02d payload", i)))
	}
	group := Box(Style{FlexDirection: Column, Width: Percent(100)}, Text("group a"), Text("group b"))
	raw := RawANSI("\x1b[31mraw ansi\x1b[0m")
	absolute := Box(Style{
		Position: PositionAbsolute, Left: Cells(2), Top: Cells(1),
		Width: Cells(9), Height: Cells(1), FlexDirection: Row,
	}, Text("overlay"))
	inner := ScrollBox(Style{Height: Cells(2), Width: Percent(100)}, false,
		Text("inner a"), Text("inner b"), Text("inner c"), Text("inner d"))
	content := append(append([]*Node(nil), rows...), group, raw, absolute, inner)

	bodyStyle := Style{Width: Percent(100), Height: Cells(5)}
	if grow {
		bodyStyle.FlexGrow = F(1)
		bodyStyle.FlexShrink = F(1)
	}
	body := ScrollBox(bodyStyle, sticky, content...)

	panelTop := Text("panel top")
	panelMid := Text("panel mid")
	panelBottom := Text("panel bottom")
	panel := Box(Style{
		FlexDirection: Column, Width: Percent(100), Height: Cells(5),
		FlexGrow: F(1), FlexShrink: F(1), AlignItems: AlignStretch,
		JustifyContent: JustifySpaceBetween, Gap: I(1),
	}, panelTop, panelMid, panelBottom)

	fixedInner := Text("fixed inner text")
	fixed := Box(Style{Width: Cells(18), Height: Cells(2), FlexDirection: Row, AlignItems: AlignFlexStart}, fixedInner)

	relative := Box(Style{
		Position: PositionRelative, Left: Cells(1), Top: Cells(1),
		Width: Cells(12), Height: Cells(1), FlexDirection: Row,
	}, Text("rel"))

	stretched := TextWithWrap("status", TextWrapTruncateEnd, TextStyle{})
	statusRow := Box(Style{FlexDirection: Row, Width: Percent(100)}, stretched, Text("tail"))

	footer := TextWithWrap("ready", TextWrapTruncateEnd, TextStyle{})
	footer.Style.AlignSelf = AlignFlexStart

	render := func(s ButtonState) []*Node {
		if s.Active {
			return []*Node{Text("saving")}
		}
		return []*Node{Text("idle")}
	}
	button := ButtonWithState(Style{Width: Cells(10), Height: Cells(1), FlexDirection: Row}, func() {}, render)
	header := Text("header")
	headerRow := Box(Style{FlexDirection: Row, Width: Percent(100)}, header, button)

	column := Box(Style{FlexDirection: Column, Width: Percent(100)},
		headerRow, body, panel, fixed, relative, statusRow, footer)
	root := Root(column)

	return &ilpTree{
		root: root, header: header, body: body, content: body.ScrollContent(),
		inner: inner, group: group, panel: panel, panelMid: panelMid,
		fixed: fixed, fixedInner: fixedInner, relative: relative,
		absolute: absolute, raw: raw, button: button, stretched: stretched,
		footer: footer, entries: rows,
	}
}

// ilpTarget names the node (or the viewport) one operation applies to, so an
// operation can be replayed against any tree of the same shape.
type ilpTarget int

const (
	ilpTargetEntry ilpTarget = iota
	ilpTargetFooter
	ilpTargetHeader
	ilpTargetStretched
	ilpTargetBody
	ilpTargetInner
	ilpTargetGroup
	ilpTargetPanel
	ilpTargetPanelMid
	ilpTargetFixed
	ilpTargetFixedInner
	ilpTargetRelative
	ilpTargetAbsolute
	ilpTargetRaw
	ilpTargetButton
	ilpTargetViewport
	ilpTargetCount
)

func (t ilpTarget) String() string {
	switch t {
	case ilpTargetEntry:
		return "entry"
	case ilpTargetFooter:
		return "footer"
	case ilpTargetHeader:
		return "header"
	case ilpTargetStretched:
		return "stretched"
	case ilpTargetBody:
		return "scrollBody"
	case ilpTargetInner:
		return "innerScroll"
	case ilpTargetGroup:
		return "group"
	case ilpTargetPanel:
		return "panel"
	case ilpTargetPanelMid:
		return "panelMid"
	case ilpTargetFixed:
		return "fixedBox"
	case ilpTargetFixedInner:
		return "fixedInner"
	case ilpTargetRelative:
		return "relativeBox"
	case ilpTargetAbsolute:
		return "absoluteBox"
	case ilpTargetRaw:
		return "rawANSI"
	case ilpTargetButton:
		return "button"
	case ilpTargetViewport:
		return "viewport"
	}
	return "unknown"
}

// ilpOpKind enumerates the mutations. Every one of them goes through the public
// mutation API (SetText, SetStyle, ScrollTo, Append, Remove, button state); the
// property test never writes a Rect or a layout flag by hand.
type ilpOpKind int

const (
	ilpOpSetText ilpOpKind = iota
	ilpOpSetTextStyle
	ilpOpSetStyle
	ilpOpScrollTo
	ilpOpScrollBy
	ilpOpScrollToBottom
	ilpOpScrollToElement
	ilpOpScrollClamp
	ilpOpAppend
	ilpOpRemove
	ilpOpSetChildren
	ilpOpButtonActivate
	ilpOpButtonExpire
	ilpOpViewport
	ilpOpKindCount
)

func (k ilpOpKind) String() string {
	switch k {
	case ilpOpSetText:
		return "SetText"
	case ilpOpSetTextStyle:
		return "SetTextStyle"
	case ilpOpSetStyle:
		return "SetStyle"
	case ilpOpScrollTo:
		return "ScrollTo"
	case ilpOpScrollBy:
		return "ScrollBy"
	case ilpOpScrollToBottom:
		return "ScrollToBottom"
	case ilpOpScrollToElement:
		return "ScrollToElement"
	case ilpOpScrollClamp:
		return "SetScrollClamp"
	case ilpOpAppend:
		return "Append"
	case ilpOpRemove:
		return "Remove"
	case ilpOpSetChildren:
		return "SetChildren"
	case ilpOpButtonActivate:
		return "ButtonActivate"
	case ilpOpButtonExpire:
		return "ButtonExpire"
	case ilpOpViewport:
		return "Viewport"
	}
	return "unknown"
}

// ilpGeomOp reports whether a mutation family must propagate subtreeGeomDirty to
// every ancestor, i.e. whether it can move a rect.
func ilpGeomOp(k ilpOpKind) bool {
	switch k {
	case ilpOpSetText, ilpOpSetStyle, ilpOpAppend, ilpOpRemove, ilpOpSetChildren,
		ilpOpButtonActivate, ilpOpButtonExpire:
		return true
	}
	return false
}

// ilpOp is a fully self-describing mutation: the same value applied to two
// equivalent trees produces two equivalent final states. Nothing is drawn from
// the RNG while the op is applied.
type ilpOp struct {
	kind    ilpOpKind
	target  ilpTarget
	entry   int
	variant int
	amount  int
	flag    bool
	text    string
}

// ilpPayloads mixes widths and shapes so wrapping, auto-sizing and multiline
// text are all reachable.
var ilpPayloads = []string{
	"x",
	"short",
	"mid payload here",
	"a much longer payload that has to wrap over several rows inside its row",
	"two\nlines",
}

// ilpRandomOp draws the next mutation from rng. Everything the application
// needs is decided here, never later.
func ilpRandomOp(rng *rand.Rand, tree *ilpTree) ilpOp {
	kind := ilpOpKind(rng.Intn(int(ilpOpKindCount)))
	target := ilpTarget(rng.Intn(int(ilpTargetCount)))
	entry := 0
	if len(tree.entries) > 0 {
		entry = rng.Intn(len(tree.entries))
	}
	op := ilpOp{
		kind:    kind,
		target:  target,
		entry:   entry,
		variant: rng.Intn(13),
		amount:  rng.Intn(9),
		flag:    rng.Intn(2) == 1,
		text:    ilpPayloads[rng.Intn(len(ilpPayloads))],
	}
	switch kind {
	case ilpOpScrollTo, ilpOpScrollBy, ilpOpScrollToBottom, ilpOpScrollToElement, ilpOpScrollClamp:
		// Scroll requests only make sense on a scroll box.
		if op.flag {
			op.target = ilpTargetInner
		} else {
			op.target = ilpTargetBody
		}
	case ilpOpAppend, ilpOpRemove, ilpOpSetChildren:
		op.target = ilpTargetGroup
	case ilpOpButtonActivate, ilpOpButtonExpire:
		op.target = ilpTargetButton
	case ilpOpViewport:
		op.target = ilpTargetViewport
		op.amount = rng.Intn(len(ilpViewports))
	case ilpOpSetStyle:
		// Bias the structural variants: a display or overflow flip is what a
		// prune or a drain is most likely to get wrong, and the uniform draw
		// would reach them only once every few hundred steps.
		if rng.Intn(3) == 0 {
			if rng.Intn(2) == 0 {
				op.variant = 8
			} else {
				op.variant = 9
			}
		}
	case ilpOpSetText:
		// Text content is meaningful on the text-bearing nodes only.
		switch ilpTarget(rng.Intn(6)) {
		case 0:
			op.target = ilpTargetEntry
		case 1:
			op.target = ilpTargetFooter
		case 2:
			op.target = ilpTargetHeader
		case 3:
			op.target = ilpTargetStretched
		case 4:
			op.target = ilpTargetRaw
		default:
			op.target = ilpTargetPanelMid
		}
	}
	return op
}

// ilpNode resolves the target to a node handle.
func ilpNode(tree *ilpTree, target ilpTarget) *Node {
	switch target {
	case ilpTargetEntry:
		if len(tree.entries) > 0 {
			return tree.entries[0]
		}
		return nil
	case ilpTargetFooter:
		return tree.footer
	case ilpTargetHeader:
		return tree.header
	case ilpTargetStretched:
		return tree.stretched
	case ilpTargetBody:
		return tree.body
	case ilpTargetInner:
		return tree.inner
	case ilpTargetGroup:
		return tree.group
	case ilpTargetPanel:
		return tree.panel
	case ilpTargetPanelMid:
		return tree.panelMid
	case ilpTargetFixed:
		return tree.fixed
	case ilpTargetFixedInner:
		return tree.fixedInner
	case ilpTargetRelative:
		return tree.relative
	case ilpTargetAbsolute:
		return tree.absolute
	case ilpTargetRaw:
		return tree.raw
	case ilpTargetButton:
		return tree.button
	}
	return nil
}

// ilpStyleVariant derives the style a SetStyle op installs. It reads only the
// op, never the RNG, so replaying the op reproduces the same style.
func ilpStyleVariant(s Style, op ilpOp) Style {
	switch op.variant {
	case 0:
		s.Width = Cells(float64(6 + 3*op.amount))
	case 1:
		s.Height = Cells(float64(1 + op.amount%4))
	case 2:
		if op.flag {
			s.TextWrap = TextWrapTruncateEnd
		} else {
			s.TextWrap = TextWrapWrap
		}
	case 3:
		if op.flag {
			s.AlignSelf = AlignFlexStart
		} else {
			s.AlignSelf = AlignStretch
		}
	case 4:
		if op.flag {
			s.FlexGrow = F(1)
			s.FlexShrink = F(1)
		} else {
			s.FlexGrow = F(0)
		}
	case 5:
		switch op.amount % 3 {
		case 0:
			s.JustifyContent = JustifySpaceBetween
		case 1:
			s.JustifyContent = JustifyCenter
		default:
			s.JustifyContent = JustifySpaceEvenly
		}
	case 6:
		if op.flag {
			s.AlignItems = AlignCenter
		} else {
			s.AlignItems = AlignFlexStart
		}
	case 7:
		if op.flag {
			s.Position = PositionRelative
			s.Left = Cells(float64(1 + op.amount%4))
			s.Top = Cells(float64(op.amount % 3))
		} else {
			s.Position = PositionAbsolute
			s.Left = Cells(float64(op.amount % 5))
			s.Top = Cells(float64(1 + op.amount%3))
			s.Width = Cells(7)
			s.Height = Cells(2)
		}
	case 8:
		if op.flag {
			s.Display = DisplayNone
		} else {
			s.Display = DisplayFlex
		}
	case 9:
		overflow := OverflowVisible
		if op.flag {
			overflow = OverflowScroll
		}
		s.Overflow, s.OverflowX, s.OverflowY = overflow, overflow, overflow
	case 10:
		if op.flag {
			s.Gap = I(1)
			s.RowGap, s.ColumnGap = nil, nil
		} else {
			s.Gap = nil
		}
	case 11:
		if op.flag {
			s.Padding = I(1)
			s.PaddingX, s.PaddingY = nil, nil
		} else {
			s.Padding, s.PaddingX, s.PaddingY = nil, nil, nil
		}
	default:
		if op.flag {
			border := BorderSingle
			s.BorderStyle = &border
		} else {
			s.BorderStyle = nil
		}
	}
	return s
}

// ilpKindName renders a node kind for a failure path.
func ilpKindName(k NodeKind) string {
	switch k {
	case NodeRoot:
		return "Root"
	case NodeBox:
		return "Box"
	case NodeText:
		return "Text"
	case NodeRawANSI:
		return "RawANSI"
	case NodeLink:
		return "Link"
	case NodeButton:
		return "Button"
	case NodeAlternateScreen:
		return "AltScreen"
	}
	return fmt.Sprintf("Kind(%d)", uint8(k))
}

// ilpPath renders the location of a node as parent/child indexes, so a failure
// message points at the exact node without depending on generated node IDs.
func ilpPath(n *Node) string {
	if n == nil {
		return "<nil>"
	}
	var parts []string
	for cur := n; cur != nil; cur = cur.Parent {
		index := -1
		if cur.Parent != nil {
			for i, c := range cur.Parent.Children {
				if c == cur {
					index = i
					break
				}
			}
		}
		parts = append(parts, fmt.Sprintf("%s[%d]", ilpKindName(cur.Kind), index))
	}
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	return strings.Join(parts, "/")
}

// ilpApplyOp performs one mutation through the public API. It returns the node
// the mutation belongs to, whether the tree state actually changed, and a
// description carrying the node path for failure messages.
func ilpApplyOp(t *testing.T, tree *ilpTree, op ilpOp, viewport *Size) (*Node, bool, string) {
	t.Helper()
	if op.kind == ilpOpViewport {
		next := ilpViewports[op.amount%len(ilpViewports)]
		*viewport = next
		return nil, true, fmt.Sprintf("Viewport{Width:%d Height:%d}", next.Width, next.Height)
	}
	if op.target == ilpTargetViewport {
		return nil, false, "skip: no viewport payload"
	}
	node := ilpNode(tree, op.target)
	if op.target == ilpTargetEntry {
		if len(tree.entries) == 0 {
			return nil, false, "no entries"
		}
		node = tree.entries[op.entry%len(tree.entries)]
	}
	if node == nil {
		t.Fatalf("%s: target %s resolved to no node", op.kind, op.target)
	}
	desc := fmt.Sprintf("%s on %s at %s", op.kind, op.target, ilpPath(node))

	switch op.kind {
	case ilpOpSetText:
		if node.Text == ExpandTabs(op.text) {
			return node, false, desc + " [no-op: text unchanged]"
		}
		node.SetText(op.text)
		return node, true, desc + fmt.Sprintf(" [text %q]", op.text)
	case ilpOpSetTextStyle:
		style := TextStyle{Bold: op.flag, Italic: !op.flag, Underline: op.amount%2 == 0}
		if node.TextStyle == style {
			return node, false, desc + " [no-op: text style unchanged]"
		}
		node.SetTextStyle(style)
		return node, true, desc
	case ilpOpSetStyle:
		node.SetStyle(ilpStyleVariant(node.Style, op))
		return node, true, desc + fmt.Sprintf(" [variant %d flag %v amount %d]", op.variant, op.flag, op.amount)
	case ilpOpScrollTo:
		node.ScrollTo(op.amount)
		return node, true, desc + fmt.Sprintf(" [y=%d]", op.amount)
	case ilpOpScrollBy:
		if op.amount == 0 {
			return node, false, desc + " [no-op: delta 0]"
		}
		node.ScrollBy(op.amount)
		return node, true, desc + fmt.Sprintf(" [dy=%d]", op.amount)
	case ilpOpScrollToBottom:
		node.ScrollToBottom()
		return node, true, desc
	case ilpOpScrollToElement:
		el := tree.footer // outside the box: the anchor is consumed without moving
		if op.flag {
			if len(tree.entries) == 0 {
				return nil, false, "no entries"
			}
			el = tree.entries[op.entry%len(tree.entries)]
		}
		node.ScrollToElement(el, op.amount%3)
		return node, true, desc + fmt.Sprintf(" [anchor %s offset %d]", ilpPath(el), op.amount%3)
	case ilpOpScrollClamp:
		switch op.variant % 3 {
		case 0:
			node.SetScrollClamp(nil, nil)
		case 1:
			minY, maxY := 0, op.amount
			node.SetScrollClamp(&minY, &maxY)
		default:
			minY := 1
			node.SetScrollClamp(&minY, nil)
		}
		return node, true, desc + fmt.Sprintf(" [variant %d amount %d]", op.variant%3, op.amount)
	case ilpOpAppend:
		tree.group.Append(Text(op.text))
		return tree.group, true, desc + fmt.Sprintf(" [append %q]", op.text)
	case ilpOpRemove:
		children := tree.group.Children
		if len(children) == 0 {
			return tree.group, false, desc + " [no-op: group empty]"
		}
		removed := ilpPath(children[len(children)-1])
		tree.group.Remove(children[len(children)-1])
		return tree.group, true, desc + fmt.Sprintf(" [removed %s]", removed)
	case ilpOpSetChildren:
		tree.group.SetChildren(Text(op.text), Text("child two"))
		return tree.group, true, desc + fmt.Sprintf(" [children %q, child two]", op.text)
	case ilpOpButtonActivate:
		if tree.button.ButtonState.Active {
			return tree.button, false, desc + " [no-op: already active]"
		}
		tree.button.activateButton()
		return tree.button, true, desc
	case ilpOpButtonExpire:
		if !tree.button.ButtonState.Active {
			return tree.button, false, desc + " [no-op: not active]"
		}
		tree.button.ActiveUntil = time.Now().Add(-time.Second)
		refreshExpiredButtonStates(tree.root)
		return tree.button, true, desc
	}
	t.Fatalf("unhandled op kind %v", op.kind)
	return nil, false, desc
}

// ilpFields is the layout state a pass owns. A pruned subtree keeps every one of
// these by definition, so any difference between the incremental pass and the
// pass from zero is a defect of the incremental one.
type ilpFields struct {
	Rect                 Rect
	ContentRect          Rect
	ScrollTop            int
	ScrollHeight         int
	ScrollViewportHeight int
	ScrollViewportTop    int
	ChildrenLinearY      bool
}

// ilpEntry is one node of a snapshot.
type ilpEntry struct {
	path             string
	kind             NodeKind
	hidden           bool
	fields           ilpFields
	scrollComparable bool
}

// ilpSnapshot records the layout state of the whole tree in preorder. A hidden
// node is recorded as such and its subtree is not descended into: layoutNode
// returns before touching the children of a hidden node, so their geometry is
// stale by design and not part of the contract.
func ilpSnapshot(root *Node) []ilpEntry {
	var out []ilpEntry
	var walk func(n *Node, path string)
	walk = func(n *Node, path string) {
		hidden := n.Style.Display == DisplayNone
		out = append(out, ilpEntry{
			path:   path,
			kind:   n.Kind,
			hidden: hidden,
			fields: ilpFields{
				Rect:                 n.Rect,
				ContentRect:          n.ContentRect,
				ScrollTop:            n.ScrollTop,
				ScrollHeight:         n.ScrollHeight,
				ScrollViewportHeight: n.ScrollViewportHeight,
				ScrollViewportTop:    n.ScrollViewportTop,
				ChildrenLinearY:      n.childrenLinearY,
			},
			// layoutNode returns before updateScrollState for text content, so
			// its scroll fields are only ever written by the scroll-only cached
			// path; they are not comparable between the two passes.
			scrollComparable: !hidden && n.Kind != NodeText && n.Kind != NodeRawANSI,
		})
		if hidden {
			return
		}
		for i, c := range n.Children {
			walk(c, fmt.Sprintf("%s/%d", path, i))
		}
	}
	if root != nil {
		walk(root, "root")
	}
	return out
}

// ilpNormalizedFields returns the comparable part of a node's layout state.
// layoutNode returns before updateScrollState for text content, so a scroll
// style on a Text/NodeRawANSI node is only ever served by the scroll-only cached
// path (refreshScrollState) and never by a layout pass; that asymmetry predates
// the incremental prune, so those fields are not part of this comparison.
func ilpNormalizedFields(e ilpEntry) ilpFields {
	f := e.fields
	if !e.scrollComparable {
		f.ScrollTop, f.ScrollHeight, f.ScrollViewportHeight, f.ScrollViewportTop = 0, 0, 0, 0
	}
	return f
}

// ilpDigest collapses a snapshot into the coarse aggregate: the summed geometry
// must not diverge either.
func ilpDigest(entries []ilpEntry) string {
	var x, y, w, h, cw, ch, scrollH int
	for _, e := range entries {
		if e.hidden {
			continue
		}
		f := ilpNormalizedFields(e)
		x += f.Rect.X
		y += f.Rect.Y
		w += f.Rect.Width
		h += f.Rect.Height
		cw += f.ContentRect.Width
		ch += f.ContentRect.Height
		scrollH += f.ScrollHeight
	}
	return fmt.Sprintf("Σx=%d Σy=%d Σw=%d Σh=%d Σcw=%d Σch=%d ΣscrollH=%d", x, y, w, h, cw, ch, scrollH)
}

// ilpCompareSnapshots compares two snapshots of the same shape node by node and
// field by field.
func ilpCompareSnapshots(t *testing.T, label string, want, got []ilpEntry) {
	t.Helper()
	if len(want) != len(got) {
		t.Fatalf("%s: %d compared nodes != %d (the two paths disagree on the tree shape)", label, len(want), len(got))
	}
	for i := range want {
		w, g := want[i], got[i]
		if w.path != g.path || w.kind != g.kind || w.hidden != g.hidden {
			t.Fatalf("%s: node %d: shape mismatch %s/%s/%v != %s/%s/%v",
				label, i, w.path, ilpKindName(w.kind), w.hidden, g.path, ilpKindName(g.kind), g.hidden)
		}
		if w.hidden {
			continue
		}
		wf, gf := ilpNormalizedFields(w), ilpNormalizedFields(g)
		if wf != gf {
			t.Fatalf("%s: %s (%s): layout mismatch\n  from-zero=%+v\n incremental=%+v", label, w.path, ilpKindName(w.kind), wf, gf)
		}
	}
}

// ilpScrollInput is the stateful scroll input of one node. It has to be put back
// before the from-zero pass, otherwise that pass would consume a second drain
// step and the two passes would not be comparing the same inputs.
type ilpScrollInput struct {
	node     *Node
	top      int
	pending  int
	sticky   bool
	anchor   *ScrollAnchor
	clampMin *int
	clampMax *int
}

func ilpCaptureScrollInputs(root *Node) []ilpScrollInput {
	var out []ilpScrollInput
	if root == nil {
		return out
	}
	root.Walk(func(n *Node) bool {
		if n.ScrollTop != 0 || n.PendingScrollDelta != 0 || n.StickyScroll ||
			n.ScrollAnchor != nil || n.ScrollClampMin != nil || n.ScrollClampMax != nil {
			out = append(out, ilpScrollInput{
				node: n, top: n.ScrollTop, pending: n.PendingScrollDelta,
				sticky: n.StickyScroll, anchor: n.ScrollAnchor,
				clampMin: n.ScrollClampMin, clampMax: n.ScrollClampMax,
			})
		}
		return true
	})
	return out
}

func ilpRestoreScrollInputs(inputs []ilpScrollInput) {
	for _, in := range inputs {
		in.node.ScrollTop = in.top
		in.node.PendingScrollDelta = in.pending
		in.node.StickyScroll = in.sticky
		in.node.ScrollAnchor = in.anchor
		in.node.ScrollClampMin = in.clampMin
		in.node.ScrollClampMax = in.clampMax
	}
}

// ilpForceFullPass lays the tree out from zero with the same inputs: every node
// is made geometrically dirty, cached geometry and the pure layout outputs are
// cleared, and the pass that follows therefore visits every node — exactly what
// a tree built fresh in this state would do. Only the layout cache metadata
// (which the pass overwrites) is touched; no node style, text or child list is
// modified.
func ilpForceFullPass(root *Node, viewport Size, fullscreen bool) Size {
	root.Walk(func(n *Node) bool {
		n.Rect = Rect{}
		n.ContentRect = Rect{}
		n.childrenLinearY = false
		n.subtreeGeomDirty = true
		n.layoutStamp = 0
		// Only the nodes whose scroll bookkeeping the pass actually rewrites may
		// be cleared. A node that is not a scroll node right now (a Box whose
		// overflow was just switched off, text content, a hidden subtree) keeps
		// whatever the last call left on it in both paths.
		scroll := n.Style.OverflowY == OverflowScroll || n.Style.Overflow == OverflowScroll
		if scroll && layoutPassReaches(n, root) {
			n.ScrollHeight = 0
			n.ScrollViewportHeight = 0
			n.ScrollViewportTop = 0
		}
		return true
	})
	root.layoutCached = false
	root.layoutDirty = true
	root.layoutViewport = Size{}
	root.layoutFullscreen = false
	return ComputeLayout(root, viewport, fullscreen)
}

// ilpFullPassSize counts the nodes a fully conservative pass would visit, so the
// property test can prove it is not vacuous: fewer visited nodes than this means
// the pass really pruned something.
func ilpFullPassSize(root *Node) int {
	count := 0
	if root == nil {
		return 0
	}
	root.Walk(func(n *Node) bool {
		count++
		return n.Style.Display != DisplayNone
	})
	return count
}

// single mutation can be attributed to itself: the property test runs a full
// pass at the end of every step, which leaves the tree clean.
func ilpClearLayoutFlags(root *Node) {
	root.Walk(func(n *Node) bool {
		n.subtreeGeomDirty = false
		n.layoutDirty = false
		return true
	})
}

// ilpAssertFlagPropagation pins the contract MarkDirty provides and the prune
// depends on: a geometry mutation flags the mutated node and every ancestor as
// holding a dirty subtree, while a paint-only mutation must leave those flags
// alone. The check walks to the root, node by node.
func ilpAssertFlagPropagation(t *testing.T, label string, node *Node, geometry bool) {
	t.Helper()
	if node == nil {
		t.Fatalf("%s: mutation reported no target node", label)
	}
	for cur := node; cur != nil; cur = cur.Parent {
		if geometry {
			if !cur.subtreeGeomDirty {
				t.Fatalf("%s: subtreeGeomDirty not propagated to %s", label, ilpPath(cur))
			}
			if !cur.layoutDirty {
				t.Fatalf("%s: layoutDirty not propagated to %s", label, ilpPath(cur))
			}
			continue
		}
		if cur.subtreeGeomDirty {
			t.Fatalf("%s: paint-only mutation marked %s as geometrically dirty", label, ilpPath(cur))
		}
		if cur.layoutDirty {
			t.Fatalf("%s: paint-only mutation marked %s as needing a layout pass", label, ilpPath(cur))
		}
	}
}

// TestIncrementalLayoutProperty drives a deterministic pseudo-random mutation
// sequence through the public mutation API and, after every step, compares the
// incrementally laid-out tree against the same tree laid out from zero with the
// same final state and the same scroll inputs.
//
// A failure prints the step number, the mutation (with the path of the node it
// touched) and the first node whose layout state diverged, so the sequence can
// be replayed exactly.
func TestIncrementalLayoutProperty(t *testing.T) {
	run := func(t *testing.T, fullscreen bool) {
		t.Helper()
		rng := rand.New(rand.NewSource(ilpSeed))
		viewport := ilpViewports[0]
		sticky := rng.Intn(2) == 1
		grow := rng.Intn(2) == 1
		tree := ilpBuildTree(ilpEntries, sticky, grow)
		t.Logf("seed=%#x entries=%d steps=%d fullscreen=%v sticky=%v grow=%v", ilpSeed, ilpEntries, ilpPropertySteps, fullscreen, sticky, grow)
		ComputeLayout(tree.root, viewport, fullscreen)

		fullPassSize := ilpFullPassSize(tree.root)
		prunedSteps, drainedSteps := 0, 0
		opMix := make(map[ilpOpKind]int)
		displayNoneSteps, multilineSteps := 0, 0

		for step := 0; step < ilpPropertySteps; step++ {
			op := ilpRandomOp(rng, tree)
			// The previous step ended with a full pass, so the tree is clean;
			// clearing the flags again makes a single mutation attributable to
			// itself (the property under test: which flags one mutation raises).
			ilpClearLayoutFlags(tree.root)
			mutated, changed, desc := ilpApplyOp(t, tree, op, &viewport)
			opMix[op.kind]++
			if op.kind == ilpOpSetStyle && op.variant == 8 && op.flag {
				displayNoneSteps++
			}
			if strings.Contains(op.text, "\n") {
				multilineSteps++
			}
			label := fmt.Sprintf("step %d: %s", step, desc)
			if changed && mutated != nil {
				ilpAssertFlagPropagation(t, label, mutated, ilpGeomOp(op.kind))
			}

			// Inputs the pass reads and writes; they have to go back before the
			// from-zero pass runs, otherwise it would consume a second drain.
			inputs := ilpCaptureScrollInputs(tree.root)
			pendingBefore := false
			for _, in := range inputs {
				if in.pending != 0 {
					pendingBefore = true
				}
			}
			incrementalSize, visited := computeLayout(tree.root, viewport, fullscreen)
			if visited < fullPassSize {
				prunedSteps++
				if pendingBefore {
					drainedSteps++
				}
			}
			incremental := ilpSnapshot(tree.root)

			ilpRestoreScrollInputs(inputs)
			fullSize := ilpForceFullPass(tree.root, viewport, fullscreen)
			full := ilpSnapshot(tree.root)

			if incrementalSize != fullSize {
				t.Fatalf("%s: root size %+v diverged from the from-zero pass %+v", label, incrementalSize, fullSize)
			}
			ilpCompareSnapshots(t, label, full, incremental)
			if inc, zero := ilpDigest(incremental), ilpDigest(full); inc != zero {
				t.Fatalf("%s: geometry digest diverged\n incremental=%s\n  from-zero=%s", label, inc, zero)
			}
		}

		// A sequence that never pruned anything would compare two identical
		// full passes and prove nothing.
		t.Logf("pruned steps=%d/%d, of them with a pending scroll request=%d, full pass size=%d",
			prunedSteps, ilpPropertySteps, drainedSteps, fullPassSize)
		if prunedSteps < ilpPropertySteps/4 {
			t.Fatalf("only %d of %d steps pruned a subtree; the sequence barely exercised the incremental path", prunedSteps, ilpPropertySteps)
		}
		if drainedSteps < 10 {
			t.Fatalf("only %d steps applied a scroll request through the drain; that path is barely covered", drainedSteps)
		}

		// Every mutation family the adversarial mix promises has to have been
		// applied at least once, otherwise the coverage claim is decorative.
		t.Logf("op mix over %d steps: %v display-none steps=%d multiline-text steps=%d", ilpPropertySteps, opMix, displayNoneSteps, multilineSteps)
		for _, kind := range []ilpOpKind{
			ilpOpSetText, ilpOpSetTextStyle, ilpOpSetStyle, ilpOpScrollTo, ilpOpScrollBy,
			ilpOpScrollToBottom, ilpOpScrollToElement, ilpOpScrollClamp, ilpOpAppend,
			ilpOpRemove, ilpOpSetChildren, ilpOpButtonActivate, ilpOpButtonExpire, ilpOpViewport,
		} {
			if opMix[kind] == 0 {
				t.Fatalf("the sequence never applied %s", kind)
			}
		}
		if displayNoneSteps == 0 {
			t.Fatal("the sequence never set Display: none; the hidden-subtree path was not covered")
		}
		if multilineSteps == 0 {
			t.Fatal("the sequence never applied multiline text")
		}
	}

	t.Run("fullscreen", func(t *testing.T) { run(t, true) })
	t.Run("natural-height", func(t *testing.T) { run(t, false) })
	t.Run("mutation-inside-previously-pruned-subtree", func(t *testing.T) { ilpPrunedSubtreeCase(t) })
	t.Run("scroll-requests-through-pruned-branches", func(t *testing.T) { ilpPrunedScrollCase(t) })
	t.Run("drain-skips-unreachable-scroll-nodes", func(t *testing.T) { ilpUnreachableScrollCase(t) })
}

// ilpForceConservativePass lays the tree out with pruning disabled: every node
// is marked as holding a dirty subtree, so the pass walks the whole tree, which
// is what the pre-prune code always did. Geometry is left untouched.
func ilpForceConservativePass(root *Node, viewport Size, fullscreen bool) Size {
	root.Walk(func(n *Node) bool {
		n.subtreeGeomDirty = true
		return true
	})
	root.layoutDirty = true
	return ComputeLayout(root, viewport, fullscreen)
}

// ilpPrunedPass runs one pass and requires it to have skipped the scroll branch,
// returning the number of nodes it visited.
func ilpPrunedPass(t *testing.T, label string, root *Node, viewport Size, fullscreen bool, limit int) int {
	t.Helper()
	_, visited := computeLayout(root, viewport, fullscreen)
	if visited > limit {
		t.Fatalf("%s: pass visited %d nodes, want the scroll branch pruned (<= %d)", label, visited, limit)
	}
	return visited
}

// ilpAssertChainFlags walks node by node from a mutated node to the root,
// requiring the subtree-geometry flag the prune depends on to be set on every
// one of them.
func ilpAssertChainFlags(t *testing.T, label string, node *Node) {
	t.Helper()
	chain := 0
	for cur := node; cur != nil; cur = cur.Parent {
		if !cur.subtreeGeomDirty {
			t.Fatalf("%s: subtreeGeomDirty missing on %s", label, ilpPath(cur))
		}
		if !cur.layoutDirty {
			t.Fatalf("%s: layoutDirty missing on %s", label, ilpPath(cur))
		}
		chain++
	}
	if chain < 3 {
		t.Fatalf("%s: the ancestor chain is only %d node(s) long, the propagation check would be vacuous", label, chain)
	}
}

// ilpPrunedSubtreeCase mutates the SAME entry twice: the first mutation is
// applied to an entry whose subtree the previous pass pruned, and the second one
// hits that entry again after it was laid out and went clean. It pins the
// propagation of the dirty subtree flag node by node up to the root, the
// re-entry of the pruned branch, and the geometry of the second pass against a
// twin built from scratch in the same final state.
func ilpPrunedSubtreeCase(t *testing.T) {
	viewport := ilpViewports[0]
	tree := ilpBuildTree(ilpEntries, false, true)
	if _, first := computeLayout(tree.root, viewport, true); first < ilpEntries {
		t.Fatalf("first pass visited %d nodes, want the whole tree (>= %d)", first, ilpEntries)
	}

	// A same-width footer change keeps every other rect in place, so the pass
	// that applies it must skip the whole scroll branch.
	tree.footer.SetText("busy!")
	entry := tree.entries[4]
	prunedStamp := entry.layoutStamp
	if visited := ilpPrunedPass(t, "footer same-width mutation", tree.root, viewport, true, 20); visited == 0 {
		t.Fatal("the pass was served from the layout cache; the prune was not exercised")
	}
	if entry.layoutStamp != prunedStamp {
		t.Fatal("an entry inside the body was re-entered by a pass that should have pruned it")
	}
	if entry.subtreeGeomDirty {
		t.Fatal("a pruned entry must not be left looking geometrically dirty")
	}

	// First mutation inside the pruned subtree.
	entry.SetText("row 04 payload updated")
	ilpAssertChainFlags(t, "first mutation inside the pruned subtree", entry)
	if !tree.root.layoutDirty {
		t.Fatal("root.layoutDirty was not set by a mutation inside a pruned subtree")
	}
	firstStamp := entry.layoutStamp
	if visited := ilpPrunedPass(t, "re-enter after the first entry mutation", tree.root, viewport, true, 1<<30); visited < ilpEntries {
		t.Fatalf("pass after the first entry mutation visited %d nodes, want the scroll body re-laid out (>= %d)", visited, ilpEntries)
	}
	if entry.layoutStamp == firstStamp {
		t.Fatal("the mutated entry was not re-laid out after its subtree had been pruned")
	}

	// Second consecutive mutation of the same entry, which is clean again by
	// now: the flag has to travel to the root a second time and the branch has
	// to be re-laid out.
	entry.SetText("row 04 payload that is longer and wraps over two rows now")
	ilpAssertChainFlags(t, "second mutation on the same entry", entry)
	secondStamp := entry.layoutStamp
	if visited := ilpPrunedPass(t, "re-enter after the second entry mutation", tree.root, viewport, true, 1<<30); visited < ilpEntries {
		t.Fatalf("pass after the second entry mutation visited %d nodes, want the scroll body re-laid out (>= %d)", visited, ilpEntries)
	}
	if entry.layoutStamp == secondStamp {
		t.Fatal("the entry was not re-laid out by the pass that followed its second mutation")
	}

	// Twin: the same final state, but built from scratch and laid out once.
	twin := ilpBuildTree(ilpEntries, false, true)
	twin.footer.SetText("busy!")
	twin.entries[4].SetText("row 04 payload updated")
	twin.entries[4].SetText("row 04 payload that is longer and wraps over two rows now")
	ComputeLayout(twin.root, viewport, true)
	ilpCompareSnapshots(t, "pruned-subtree twin", ilpSnapshot(twin.root), ilpSnapshot(tree.root))

	// And the same tree laid out from zero.
	before := ilpSnapshot(tree.root)
	ilpForceFullPass(tree.root, viewport, true)
	ilpCompareSnapshots(t, "pruned-subtree from zero", before, ilpSnapshot(tree.root))
}

// ilpPrunedScrollCase interleaves scroll requests on a ScrollBox that the pass
// prunes with full passes forced by a geometry change elsewhere (the footer and
// the status row). The mutated tree never walks the scroll branch — the request
// is applied by drainSkippedScroll — while the twin walks it every round because
// its pass runs from zero, so a drain that is lost or applied twice shows up as
// a divergence in ScrollTop, PendingScrollDelta or ScrollHeight.
func ilpPrunedScrollCase(t *testing.T) {
	viewport := ilpViewports[0]
	incremental := ilpBuildTree(ilpEntries, false, true)
	twin := ilpBuildTree(ilpEntries, false, true)
	ComputeLayout(incremental.root, viewport, true)
	ComputeLayout(twin.root, viewport, true)

	forcing := []ilpOp{
		{kind: ilpOpSetText, target: ilpTargetFooter, text: "busy!"},
		{kind: ilpOpSetText, target: ilpTargetFooter, text: "ready"},
		{kind: ilpOpSetText, target: ilpTargetStretched, text: "state"},
		{kind: ilpOpSetText, target: ilpTargetStretched, text: "status row"},
	}
	scrolls := []ilpOp{
		{kind: ilpOpScrollBy, target: ilpTargetBody, amount: 5},
		{kind: ilpOpScrollToBottom, target: ilpTargetInner},
		{kind: ilpOpScrollToElement, target: ilpTargetBody, entry: 3, flag: true},
		{kind: ilpOpScrollTo, target: ilpTargetBody, amount: 2},
		{kind: ilpOpScrollToElement, target: ilpTargetBody, entry: 5},
		{kind: ilpOpScrollClamp, target: ilpTargetInner, variant: 1, amount: 2},
		{kind: ilpOpScrollBy, target: ilpTargetInner, amount: 9},
		{kind: ilpOpScrollToBottom, target: ilpTargetBody},
	}

	for i, scroll := range scrolls {
		force := forcing[i%len(forcing)]
		for _, tree := range []*ilpTree{incremental, twin} {
			vp := viewport
			ilpApplyOp(t, tree, scroll, &vp)
			ilpApplyOp(t, tree, force, &vp)
		}
		label := fmt.Sprintf("round %d: %s + %s", i, scroll.kind, force.kind)
		ilpPrunedPass(t, label, incremental.root, viewport, true, 20)
		ilpForceFullPass(twin.root, viewport, true)
		ilpCompareSnapshots(t, label, ilpSnapshot(twin.root), ilpSnapshot(incremental.root))
		t.Logf("%s: body top=%d pending=%d height=%d | inner top=%d pending=%d",
			label, incremental.body.ScrollTop, incremental.body.PendingScrollDelta, incremental.body.ScrollHeight,
			incremental.inner.ScrollTop, incremental.inner.PendingScrollDelta)
	}

	body, inner := incremental.body, incremental.inner
	t.Logf("after %d rounds: body top=%d scrollHeight=%d viewportHeight=%d pending=%d; inner top=%d pending=%d",
		len(scrolls), body.ScrollTop, body.ScrollHeight, body.ScrollViewportHeight, body.PendingScrollDelta,
		inner.ScrollTop, inner.PendingScrollDelta)
	if body.ScrollHeight <= body.ScrollViewportHeight {
		t.Fatalf("the scroll body reports no overflow (height=%d viewport=%d), the comparison would be vacuous",
			body.ScrollHeight, body.ScrollViewportHeight)
	}
	if body.ScrollTop == 0 && body.PendingScrollDelta == 0 {
		t.Fatal("no scroll request reached the pruned body")
	}
}

// ilpUnreachableScrollCase pins the boundary of drainSkippedScroll: it may only
// apply the scroll bookkeeping the pass itself would have applied. A node the
// pass cannot reach at all — one that left the tree, sits under a hidden node,
// or is text content — has to keep the state the pre-prune code left on it.
//
// Each family is built three times: with pruning allowed (the pass may skip the
// branch and hand the bookkeeping to the drain), with pruning disabled (the
// conservative pass the pre-prune code always ran) and from a single first pass.
// All three must agree, so a drain that reaches a node the pass never sees shows
// up as a consumed scroll delta.
func ilpUnreachableScrollCase(t *testing.T) {
	viewport := ilpViewports[0]

	// A ScrollBox under a container that is hidden before the scroll request:
	// the box keeps its geometry, is never visited, and only the drain could
	// touch it.
	buildHidden := func() (root, container, box, footer *Node) {
		box = ScrollBox(Style{Height: Cells(2), Width: Percent(100)}, false,
			Text("h1"), Text("h2"), Text("h3"), Text("h4"), Text("h5"), Text("h6"))
		container = Box(Style{FlexDirection: Column, Width: Percent(100)}, box)
		footer = Text("ready")
		root = Root(Box(Style{FlexDirection: Column, Width: Percent(100)},
			Text("header"), container, footer))
		return root, container, box, footer
	}
	hide := func(container *Node) {
		s := container.Style
		s.Display = DisplayNone
		container.SetStyle(s)
	}

	t.Run("hidden-subtree", func(t *testing.T) {
		prunedRoot, containerA, boxA, footerA := buildHidden()
		ComputeLayout(prunedRoot, viewport, true)
		hide(containerA)
		ComputeLayout(prunedRoot, viewport, true)
		boxA.ScrollBy(5)
		footerA.SetText("busy!")
		ilpPrunedPass(t, "hidden subtree", prunedRoot, viewport, true, 20)

		conservativeRoot, containerB, boxB, footerB := buildHidden()
		ComputeLayout(conservativeRoot, viewport, true)
		hide(containerB)
		ilpForceConservativePass(conservativeRoot, viewport, true)
		boxB.ScrollBy(5)
		footerB.SetText("busy!")
		ilpForceConservativePass(conservativeRoot, viewport, true)

		freshRoot, containerC, boxC, footerC := buildHidden()
		hide(containerC)
		boxC.ScrollBy(5)
		footerC.SetText("busy!")
		ComputeLayout(freshRoot, viewport, true)

		t.Logf("hidden scroll box: pruned top=%d pending=%d | conservative top=%d pending=%d | fresh top=%d pending=%d",
			boxA.ScrollTop, boxA.PendingScrollDelta, boxB.ScrollTop, boxB.PendingScrollDelta, boxC.ScrollTop, boxC.PendingScrollDelta)
		if boxC.ScrollTop != boxB.ScrollTop || boxC.PendingScrollDelta != boxB.PendingScrollDelta {
			t.Fatalf("the conservative and the fresh pass disagree: %d/%d != %d/%d",
				boxB.ScrollTop, boxB.PendingScrollDelta, boxC.ScrollTop, boxC.PendingScrollDelta)
		}
		if boxA.ScrollTop != boxB.ScrollTop || boxA.PendingScrollDelta != boxB.PendingScrollDelta {
			t.Fatalf("a ScrollBox inside a hidden subtree was drained by the pruned pass: top=%d pending=%d, want top=%d pending=%d",
				boxA.ScrollTop, boxA.PendingScrollDelta, boxB.ScrollTop, boxB.PendingScrollDelta)
		}
		if boxA.PendingScrollDelta == 0 {
			t.Fatal("the pending scroll delta was consumed; the assertion would be vacuous")
		}
	})

	// A Text node carrying a scroll style: layoutNode returns before
	// updateScrollState for text content, so the pass never updates it either.
	t.Run("text-content", func(t *testing.T) {
		buildText := func() (root, body, leaf, footer *Node) {
			leaf = Text("leaf payload")
			body = Box(Style{FlexDirection: Column, Width: Percent(100)}, leaf, Text("second"))
			footer = Text("ready")
			root = Root(Box(Style{FlexDirection: Column, Width: Percent(100)},
				Text("header"), body, footer))
			return root, body, leaf, footer
		}
		scrollStyle := func(leaf *Node) {
			s := leaf.Style
			s.Overflow, s.OverflowX, s.OverflowY = OverflowScroll, OverflowScroll, OverflowScroll
			s.Height = Cells(2)
			leaf.SetStyle(s)
		}

		prunedRoot, _, leafA, footerA := buildText()
		ComputeLayout(prunedRoot, viewport, true)
		scrollStyle(leafA)
		ComputeLayout(prunedRoot, viewport, true)
		leafA.ScrollBy(5)
		footerA.SetText("busy!")
		ilpPrunedPass(t, "text content", prunedRoot, viewport, true, 20)

		conservativeRoot, _, leafB, footerB := buildText()
		ComputeLayout(conservativeRoot, viewport, true)
		scrollStyle(leafB)
		ilpForceConservativePass(conservativeRoot, viewport, true)
		leafB.ScrollBy(5)
		footerB.SetText("busy!")
		ilpForceConservativePass(conservativeRoot, viewport, true)

		freshRoot, _, leafC, footerC := buildText()
		scrollStyle(leafC)
		leafC.ScrollBy(5)
		footerC.SetText("busy!")
		ComputeLayout(freshRoot, viewport, true)

		t.Logf("scroll-styled Text: pruned top=%d pending=%d height=%d | conservative %d/%d/%d | fresh %d/%d/%d",
			leafA.ScrollTop, leafA.PendingScrollDelta, leafA.ScrollHeight,
			leafB.ScrollTop, leafB.PendingScrollDelta, leafB.ScrollHeight,
			leafC.ScrollTop, leafC.PendingScrollDelta, leafC.ScrollHeight)
		if leafB.ScrollTop != leafC.ScrollTop || leafB.PendingScrollDelta != leafC.PendingScrollDelta || leafB.ScrollHeight != leafC.ScrollHeight {
			t.Fatalf("the conservative and the fresh pass disagree: %d/%d/%d != %d/%d/%d",
				leafB.ScrollTop, leafB.PendingScrollDelta, leafB.ScrollHeight,
				leafC.ScrollTop, leafC.PendingScrollDelta, leafC.ScrollHeight)
		}
		if leafA.ScrollTop != leafB.ScrollTop || leafA.PendingScrollDelta != leafB.PendingScrollDelta || leafA.ScrollHeight != leafB.ScrollHeight {
			t.Fatalf("a scroll-styled Text node was drained by the pruned pass: top=%d pending=%d height=%d, want top=%d pending=%d height=%d",
				leafA.ScrollTop, leafA.PendingScrollDelta, leafA.ScrollHeight,
				leafB.ScrollTop, leafB.PendingScrollDelta, leafB.ScrollHeight)
		}
	})

	// A ScrollBox whose subtree left the tree while a scroll request was
	// pending: the pass cannot see it, so it must not drain it either.
	t.Run("detached-subtree", func(t *testing.T) {
		build := func() (root, column, container, box, footer *Node) {
			box = ScrollBox(Style{Height: Cells(2), Width: Percent(100)}, false,
				Text("d1"), Text("d2"), Text("d3"), Text("d4"), Text("d5"), Text("d6"))
			container = Box(Style{FlexDirection: Column, Width: Percent(100)}, box)
			footer = Text("ready")
			column = Box(Style{FlexDirection: Column, Width: Percent(100)},
				Text("header"), container, footer)
			root = Root(column)
			return root, column, container, box, footer
		}

		prunedRoot, columnA, containerA, boxA, footerA := build()
		ComputeLayout(prunedRoot, viewport, true)
		boxA.ScrollBy(5)
		columnA.Remove(containerA)
		footerA.SetText("busy!")
		ilpPrunedPass(t, "detached subtree", prunedRoot, viewport, true, 20)

		conservativeRoot, columnB, containerB, boxB, footerB := build()
		ComputeLayout(conservativeRoot, viewport, true)
		boxB.ScrollBy(5)
		columnB.Remove(containerB)
		footerB.SetText("busy!")
		ilpForceConservativePass(conservativeRoot, viewport, true)

		t.Logf("detached scroll box: pruned top=%d pending=%d | conservative top=%d pending=%d",
			boxA.ScrollTop, boxA.PendingScrollDelta, boxB.ScrollTop, boxB.PendingScrollDelta)
		if boxA.ScrollTop != boxB.ScrollTop || boxA.PendingScrollDelta != boxB.PendingScrollDelta {
			t.Fatalf("a detached ScrollBox was drained by the pruned pass: top=%d pending=%d, want top=%d pending=%d",
				boxA.ScrollTop, boxA.PendingScrollDelta, boxB.ScrollTop, boxB.PendingScrollDelta)
		}
		if boxA.PendingScrollDelta != 5 || boxA.ScrollTop != 0 {
			t.Fatalf("the detached box did not keep its pre-removal scroll state: top=%d pending=%d, want top=0 pending=5",
				boxA.ScrollTop, boxA.PendingScrollDelta)
		}
	})
}
