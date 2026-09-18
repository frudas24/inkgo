package engine

import (
	"fmt"
	"math/rand"
	"testing"
	"time"
)

// Incremental layout pruning (Node.subtreeGeomDirty, the prune in layoutNode
// and drainSkippedScroll) is only worth anything if it is observationally
// equivalent to a full pass. These tests pin that equivalence: for every
// mutation family a tree mutated in place must end up with exactly the same
// geometry and scroll bookkeeping as a twin that reached the same state from
// scratch, and a paint-only mutation must keep skipping the layout pass.
//
// The pre-existing fuzzers FuzzLayoutAndRenderInvariants and
// FuzzTreeMutationInvariants must keep passing unchanged: they push random
// trees through Render/ComputeLayout and catch a prune that loses geometry on
// shapes these tables do not enumerate. Nothing in this file edits them.

const incrementalWidth = 48

var incrementalViewport = Size{Width: incrementalWidth, Height: 12}

// layoutFields is the subset of node state the layout pass owns. A pruned
// subtree keeps every one of these fields by definition, so any difference
// between the incremental and the conservative path is a defect.
type layoutFields struct {
	Rect                 Rect
	ContentRect          Rect
	ScrollTop            int
	ScrollHeight         int
	ScrollViewportHeight int
	ScrollViewportTop    int
	ChildrenLinearY      bool
}

func layoutFieldsOf(n *Node) layoutFields {
	return layoutFields{
		Rect:                 n.Rect,
		ContentRect:          n.ContentRect,
		ScrollTop:            n.ScrollTop,
		ScrollHeight:         n.ScrollHeight,
		ScrollViewportHeight: n.ScrollViewportHeight,
		ScrollViewportTop:    n.ScrollViewportTop,
		ChildrenLinearY:      n.childrenLinearY,
	}
}

// compareIncrementalTrees walks two trees of identical shape and compares the
// full layout state node by node.
func compareIncrementalTrees(t *testing.T, label string, want, got *Node) {
	t.Helper()
	compareIncrementalNodes(t, label, "root", want, got)
}

func compareIncrementalNodes(t *testing.T, label, path string, want, got *Node) {
	t.Helper()
	if want == nil || got == nil {
		if want != got {
			t.Fatalf("%s: %s: node present in only one tree (want=%v got=%v)", label, path, want != nil, got != nil)
		}
		return
	}
	if want.Kind != got.Kind {
		t.Fatalf("%s: %s: kind %d != %d", label, path, want.Kind, got.Kind)
	}
	if len(want.Children) != len(got.Children) {
		t.Fatalf("%s: %s: %d children != %d", label, path, len(want.Children), len(got.Children))
	}
	wantHidden := want.Style.Display == DisplayNone
	gotHidden := got.Style.Display == DisplayNone
	if wantHidden != gotHidden {
		t.Fatalf("%s: %s: display none mismatch (want=%v got=%v)", label, path, wantHidden, gotHidden)
	}
	if wantHidden {
		// A hidden child is skipped by its parent's flow loop, so no path ever
		// assigns geometry to a hidden subtree (HEAD behaves the same way: the
		// DisplayNone early return in layoutNode is only reachable for the
		// root). Only the shape is contractual, and that was checked above.
		return
	}
	if wf, gf := layoutFieldsOf(want), layoutFieldsOf(got); wf != gf {
		t.Fatalf("%s: %s: layout mismatch\n want=%+v\n  got=%+v", label, path, wf, gf)
	}
	for i := range want.Children {
		compareIncrementalNodes(t, label, fmt.Sprintf("%s/[%d]", path, i), want.Children[i], got.Children[i])
	}
}

// conservativeLayout runs one pass with pruning disabled: marking every node as
// holding a dirty subtree makes layoutNode walk the whole tree, which is
// exactly the behaviour before the incremental prune existed.
func conservativeLayout(root *Node, viewport Size, fullscreen bool) {
	root.Walk(func(n *Node) bool {
		n.subtreeGeomDirty = true
		return true
	})
	root.layoutDirty = true
	ComputeLayout(root, viewport, fullscreen)
}

// incrRefs carries the handles a mutation needs. The same closure runs against
// the incremental tree and against the twin, so every node it touches must be
// reachable through this struct.
type incrRefs struct {
	root    *Node
	entries []*Node
	nodes   map[string]*Node
}

func (r *incrRefs) must(name string) *Node {
	n, ok := r.nodes[name]
	if !ok || n == nil {
		panic("incremental layout test: missing node ref " + name)
	}
	return n
}

func incrRefsOf(root *Node, entries []*Node, named map[string]*Node) *incrRefs {
	return &incrRefs{root: root, entries: entries, nodes: named}
}

// incrRoot assembles the transcript shell: a full-width column.
func incrRoot(parts ...*Node) *Node {
	return Root(Box(Style{FlexDirection: Column, Width: Percent(100)}, parts...))
}

// incrBody builds the scrollable transcript body the PERF-001 defect came from:
// one Text row per entry inside a ScrollBox with a fixed viewport height.
func incrBody(entries int, sticky bool) (*Node, []*Node) {
	rows := make([]*Node, 0, entries)
	for i := 0; i < entries; i++ {
		rows = append(rows, Text(fmt.Sprintf("entry %03d payload", i)))
	}
	body := ScrollBox(Style{Height: Cells(3), Width: Percent(100)}, sticky, rows...)
	return body, rows
}

// incrStretchedFooter is a footer Text: the column stretches it, so its rect
// follows the container width rather than the text.
func incrStretchedFooter(text string) *Node {
	return TextWithWrap(text, TextWrapTruncateEnd, TextStyle{})
}

// incrAutoFooter is an auto-sized footer: AlignSelf start keeps the natural
// width, so changing the text really moves geometry.
func incrAutoFooter(text string) *Node {
	f := TextWithWrap(text, TextWrapTruncateEnd, TextStyle{})
	f.Style.AlignSelf = AlignFlexStart
	return f
}

// incrementalLayoutCase is one mutation family. build assembles a fresh tree in
// the pre-mutation state; when mut is non-nil it is applied to that fresh tree
// before its first layout, which is how the twin reaches the final state
// without ever taking the incremental path. mutate is the incremental mutation:
// it runs on a tree that has already been laid out once and may only use the
// public mutation API.
type incrementalLayoutCase struct {
	name       string
	viewport   Size
	fullscreen bool
	build      func(mut func(*incrRefs)) (*Node, *incrRefs)
	mutate     func(*incrRefs)
}

// runIncrementalEquivalence drives one case through both paths:
//
//	mutated: layout -> mutation -> incremental layout (may prune)
//	twin:    same state built from scratch -> one conservative layout
//
// and requires the two trees to agree node by node.
func runIncrementalEquivalence(t *testing.T, tc incrementalLayoutCase) {
	t.Helper()
	mutated, refs := tc.build(nil)
	ComputeLayout(mutated, tc.viewport, tc.fullscreen)
	tc.mutate(refs)
	ComputeLayout(mutated, tc.viewport, tc.fullscreen)

	twin, _ := tc.build(tc.mutate)
	ComputeLayout(twin, tc.viewport, tc.fullscreen)

	compareIncrementalTrees(t, tc.name, twin, mutated)
}

// TestIncrementalLayoutEquivalence compares an in-place mutated tree against a
// twin built from scratch in the same final state, node by node and field by
// field (Rect, ContentRect, ScrollTop, ScrollHeight, ScrollViewportHeight,
// ScrollViewportTop, childrenLinearY). It also pins the visited-node counter:
// the footer mutation must not enter the large ScrollBox branch at all.
func TestIncrementalLayoutEquivalence(t *testing.T) {
	for _, tc := range incrementalLayoutCases() {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			runIncrementalEquivalence(t, tc)
		})
	}
	t.Run("footer-same-width-prunes-large-branch", testFooterSameWidthPrunesLargeBranch)
	t.Run("display-none-round-trip", testDisplayNoneRoundTrip)
	t.Run("previously-pruned-subtree-repropagates", testPreviouslyPrunedSubtreeRepropagates)
}

func incrementalLayoutCases() []incrementalLayoutCase {
	cases := incrementalTextLayoutCases()
	cases = append(cases, incrementalSizeLayoutCases()...)
	cases = append(cases, incrementalScrollLayoutCases()...)
	return cases
}

// incrementalTextLayoutCases covers the text/content mutation families: the
// footer of the transcript (same width, wider, wrapping), raw ANSI rows, button
// render state and Display:None.
func incrementalTextLayoutCases() []incrementalLayoutCase {
	return []incrementalLayoutCase{
		{
			name:       "footer-same-width-text",
			viewport:   incrementalViewport,
			fullscreen: true,
			build: func(mut func(*incrRefs)) (*Node, *incrRefs) {
				body, rows := incrBody(6, false)
				footer := incrStretchedFooter("ready")
				root := incrRoot(Text("header"), body, footer)
				refs := incrRefsOf(root, rows, map[string]*Node{"body": body, "footer": footer})
				if mut != nil {
					mut(refs)
				}
				return root, refs
			},
			mutate: func(r *incrRefs) { r.must("footer").SetText("busy!") },
		},
		{
			name:       "footer-auto-width-text-widens",
			viewport:   incrementalViewport,
			fullscreen: true,
			build: func(mut func(*incrRefs)) (*Node, *incrRefs) {
				body, rows := incrBody(6, false)
				footer := incrAutoFooter("ready")
				status := Box(Style{FlexDirection: Row, Width: Percent(100)}, footer, Text("tail"))
				root := incrRoot(Text("header"), body, status)
				refs := incrRefsOf(root, rows, map[string]*Node{"body": body, "footer": footer})
				if mut != nil {
					mut(refs)
				}
				return root, refs
			},
			mutate: func(r *incrRefs) { r.must("footer").SetText("ready-ready-ready") },
		},
		{
			name: "footer-text-wraps-to-two-lines",
			// Natural-height root: the wrapped footer must grow the root itself.
			viewport:   incrementalViewport,
			fullscreen: false,
			build: func(mut func(*incrRefs)) (*Node, *incrRefs) {
				body, rows := incrBody(4, false)
				footer := TextWithWrap("ready", TextWrapWrap, TextStyle{})
				root := incrRoot(Text("header"), body, footer, Text("tail"))
				refs := incrRefsOf(root, rows, map[string]*Node{"body": body, "footer": footer})
				if mut != nil {
					mut(refs)
				}
				return root, refs
			},
			mutate: func(r *incrRefs) {
				r.must("footer").SetText("a wrapped footer line that no longer fits on one row")
			},
		},
		{
			name:       "raw-ansi-text-change",
			viewport:   incrementalViewport,
			fullscreen: true,
			build: func(mut func(*incrRefs)) (*Node, *incrRefs) {
				body, rows := incrBody(4, false)
				raw := RawANSI("\x1b[31mabc\x1b[0m")
				status := Box(Style{FlexDirection: Row, Width: Percent(100)}, raw, Text("tail"))
				root := incrRoot(Text("header"), body, status, incrStretchedFooter("ready"))
				refs := incrRefsOf(root, rows, map[string]*Node{"body": body, "raw": raw})
				if mut != nil {
					mut(refs)
				}
				return root, refs
			},
			mutate: func(r *incrRefs) {
				r.must("raw").SetText("\x1b[32mabcdefghij\x1b[0m")
			},
		},
		{
			name:       "button-render-active-expiry",
			viewport:   incrementalViewport,
			fullscreen: true,
			build: func(mut func(*incrRefs)) (*Node, *incrRefs) {
				body, rows := incrBody(4, false)
				render := func(s ButtonState) []*Node {
					if s.Active {
						return []*Node{Text("active-state")}
					}
					return []*Node{Text("idle")}
				}
				button := ButtonWithState(Style{Width: Cells(20), Height: Cells(1), FlexDirection: Row}, func() {}, render)
				button.activateButton()
				status := Box(Style{FlexDirection: Row, Width: Percent(100)}, button, Text("after"))
				root := incrRoot(Text("header"), body, status, incrStretchedFooter("ready"))
				refs := incrRefsOf(root, rows, map[string]*Node{"body": body, "button": button})
				if mut != nil {
					mut(refs)
				}
				return root, refs
			},
			mutate: func(r *incrRefs) {
				// Renderer.Render calls refreshExpiredButtonStates before laying
				// out, which is what rebuilds the ButtonRender children and marks
				// the button geometrically dirty. ActiveUntil in the past is the
				// elapsed-time condition that trigger observes.
				r.must("button").ActiveUntil = time.Now().Add(-time.Second)
				refreshExpiredButtonStates(r.root)
			},
		},
		{
			name:       "display-none-hides-branch",
			viewport:   incrementalViewport,
			fullscreen: true,
			build: func(mut func(*incrRefs)) (*Node, *incrRefs) {
				body, rows := incrBody(4, false)
				hidden := Box(Style{FlexDirection: Column, Width: Percent(100)},
					Text("hidden one"), Text("hidden two"))
				root := incrRoot(Text("header"), body, hidden, incrStretchedFooter("ready"))
				refs := incrRefsOf(root, rows, map[string]*Node{"body": body, "hidden": hidden})
				if mut != nil {
					mut(refs)
				}
				return root, refs
			},
			mutate: func(r *incrRefs) {
				h := r.must("hidden")
				s := h.Style
				s.Display = DisplayNone
				h.SetStyle(s)
			},
		},
		{
			name:       "display-none-leaf-in-row",
			viewport:   incrementalViewport,
			fullscreen: true,
			build: func(mut func(*incrRefs)) (*Node, *incrRefs) {
				body, rows := incrBody(4, false)
				leaf := Text("middle")
				row := Box(Style{FlexDirection: Row, Width: Percent(100)}, Text("alpha"), leaf, Text("omega"))
				root := incrRoot(Text("header"), body, row, incrStretchedFooter("ready"))
				refs := incrRefsOf(root, rows, map[string]*Node{"body": body, "leaf": leaf})
				if mut != nil {
					mut(refs)
				}
				return root, refs
			},
			mutate: func(r *incrRefs) {
				leaf := r.must("leaf")
				s := leaf.Style
				s.Display = DisplayNone
				leaf.SetStyle(s)
			},
		},
	}
}

// incrementalSizeLayoutCases covers families where the mutated node has an
// explicit size, is auto-sized in a row/column, lives in a nested flex
// container with grow/stretch/justify, is positioned absolute/relative, or has
// border+padding insets that define its ContentRect.
func incrementalSizeLayoutCases() []incrementalLayoutCase {
	return []incrementalLayoutCase{
		{
			name:       "explicit-size-box-and-inner-wrap",
			viewport:   incrementalViewport,
			fullscreen: true,
			build: func(mut func(*incrRefs)) (*Node, *incrRefs) {
				body, rows := incrBody(4, false)
				inner := Text("short")
				box := Box(Style{Width: Cells(20), Height: Cells(2), FlexDirection: Row, AlignItems: AlignFlexStart}, inner)
				root := incrRoot(Text("header"), body, box, incrStretchedFooter("ready"))
				refs := incrRefsOf(root, rows, map[string]*Node{"body": body, "box": box, "inner": inner})
				if mut != nil {
					mut(refs)
				}
				return root, refs
			},
			mutate: func(r *incrRefs) {
				r.must("inner").SetText("a long sentence that has to wrap inside the explicit box")
				box := r.must("box")
				s := box.Style
				s.Width = Cells(30)
				box.SetStyle(s)
			},
		},
		{
			name:       "auto-sized-row-child-grows",
			viewport:   incrementalViewport,
			fullscreen: true,
			build: func(mut func(*incrRefs)) (*Node, *incrRefs) {
				body, rows := incrBody(4, false)
				grow := Text("aa")
				status := Box(Style{FlexDirection: Row, Width: Percent(100)}, grow, Text("bb"))
				root := incrRoot(Text("header"), body, status, incrStretchedFooter("ready"))
				refs := incrRefsOf(root, rows, map[string]*Node{"body": body, "grow": grow})
				if mut != nil {
					mut(refs)
				}
				return root, refs
			},
			mutate: func(r *incrRefs) { r.must("grow").SetText("aaaaaaaaaaaaaaaa") },
		},
		{
			name:       "auto-sized-column-child-grows",
			viewport:   incrementalViewport,
			fullscreen: true,
			build: func(mut func(*incrRefs)) (*Node, *incrRefs) {
				body, rows := incrBody(4, false)
				grow := Text("x")
				status := Box(Style{FlexDirection: Column, Width: Percent(100), AlignItems: AlignFlexStart},
					grow, Text("y"))
				root := incrRoot(Text("header"), body, status, incrStretchedFooter("ready"))
				refs := incrRefsOf(root, rows, map[string]*Node{"body": body, "grow": grow})
				if mut != nil {
					mut(refs)
				}
				return root, refs
			},
			mutate: func(r *incrRefs) { r.must("grow").SetText("xxxxxxxxxxxxxxxx") },
		},
		{
			name:       "nested-flex-grow-stretch-justify",
			viewport:   incrementalViewport,
			fullscreen: true,
			build: func(mut func(*incrRefs)) (*Node, *incrRefs) {
				body, rows := incrBody(4, false)
				top := Text("top")
				middle := Text("middle")
				bottom := Text("bottom")
				panel := Box(Style{
					FlexDirection:  Column,
					Width:          Percent(100),
					Height:         Cells(8),
					FlexGrow:       F(1),
					FlexShrink:     F(1),
					AlignItems:     AlignStretch,
					JustifyContent: JustifySpaceBetween,
					Gap:            I(1),
				}, top, middle, bottom)
				wrapper := Box(Style{FlexDirection: Column, Width: Percent(100)}, panel)
				root := incrRoot(Text("header"), body, wrapper, incrStretchedFooter("ready"))
				refs := incrRefsOf(root, rows, map[string]*Node{"body": body, "middle": middle})
				if mut != nil {
					mut(refs)
				}
				return root, refs
			},
			mutate: func(r *incrRefs) {
				r.must("middle").SetText("middle line that wraps onto two rows inside the panel")
			},
		},
		{
			name:       "bordered-panel-padding-content-rect",
			viewport:   incrementalViewport,
			fullscreen: true,
			build: func(mut func(*incrRefs)) (*Node, *incrRefs) {
				body, rows := incrBody(4, false)
				inner := Text("abcd")
				panel := Box(Style{
					Width:         Percent(100),
					FlexDirection: Column,
					BorderStyle:   &BorderSingle,
					Padding:       I(2),
				}, inner)
				root := incrRoot(Text("header"), body, panel, incrStretchedFooter("ready"))
				refs := incrRefsOf(root, rows, map[string]*Node{"body": body, "panel": panel, "inner": inner})
				if mut != nil {
					mut(refs)
				}
				return root, refs
			},
			mutate: func(r *incrRefs) {
				r.must("inner").SetText("abcdefghijklmnopqrstuvwxyz0123456789")
			},
		},
		{
			name:       "append-and-remove-child",
			viewport:   incrementalViewport,
			fullscreen: true,
			build: func(mut func(*incrRefs)) (*Node, *incrRefs) {
				body, rows := incrBody(4, false)
				group := Box(Style{FlexDirection: Column, Width: Percent(100)}, Text("g1"), Text("g2"))
				root := incrRoot(Text("header"), body, group, incrStretchedFooter("ready"))
				refs := incrRefsOf(root, rows, map[string]*Node{"body": body, "group": group})
				if mut != nil {
					mut(refs)
				}
				return root, refs
			},
			mutate: func(r *incrRefs) {
				group := r.must("group")
				group.Append(Text("g3"))
				group.Remove(group.Children[0])
			},
		},
		{
			name:       "position-relative-offset-and-text",
			viewport:   incrementalViewport,
			fullscreen: true,
			build: func(mut func(*incrRefs)) (*Node, *incrRefs) {
				body, rows := incrBody(4, false)
				label := Text("rel")
				rel := Box(Style{
					Position:      PositionRelative,
					Left:          Cells(3),
					Top:           Cells(1),
					Width:         Cells(14),
					Height:        Cells(1),
					FlexDirection: Row,
				}, label)
				root := incrRoot(Text("header"), body, rel, incrStretchedFooter("ready"))
				refs := incrRefsOf(root, rows, map[string]*Node{"body": body, "rel": rel, "label": label})
				if mut != nil {
					mut(refs)
				}
				return root, refs
			},
			mutate: func(r *incrRefs) {
				r.must("label").SetText("rel rel rel rel")
				rel := r.must("rel")
				s := rel.Style
				s.Left = Cells(5)
				rel.SetStyle(s)
			},
		},
		{
			name:       "absolute-overlay-inside-pruned-branch",
			viewport:   incrementalViewport,
			fullscreen: true,
			build: func(mut func(*incrRefs)) (*Node, *incrRefs) {
				body, rows := incrBody(5, false)
				overlay := Box(Style{
					Position:      PositionAbsolute,
					Left:          Cells(2),
					Top:           Cells(1),
					Width:         Cells(8),
					Height:        Cells(1),
					FlexDirection: Row,
				}, Text("overlay"))
				body.ScrollContent().Append(overlay)
				footer := incrStretchedFooter("ready")
				root := incrRoot(Text("header"), body, footer)
				refs := incrRefsOf(root, rows, map[string]*Node{"body": body, "overlay": overlay, "footer": footer})
				if mut != nil {
					mut(refs)
				}
				return root, refs
			},
			// The overlay sits inside the subtree the prune skips, so its
			// absolute geometry must survive the pruned pass untouched.
			mutate: func(r *incrRefs) { r.must("footer").SetText("busy!") },
		},
		{
			name:       "absolute-child-content-and-offset",
			viewport:   incrementalViewport,
			fullscreen: true,
			build: func(mut func(*incrRefs)) (*Node, *incrRefs) {
				body, rows := incrBody(4, false)
				inner := Text("overlay")
				overlay := Box(Style{
					Position:      PositionAbsolute,
					Left:          Cells(2),
					Top:           Cells(1),
					Width:         Cells(18),
					Height:        Cells(1),
					FlexDirection: Row,
				}, inner)
				status := Box(Style{FlexDirection: Row, Width: Percent(100)}, overlay, Text("sibling"))
				root := incrRoot(Text("header"), body, status, incrStretchedFooter("ready"))
				refs := incrRefsOf(root, rows, map[string]*Node{"body": body, "overlay": overlay, "inner": inner})
				if mut != nil {
					mut(refs)
				}
				return root, refs
			},
			mutate: func(r *incrRefs) {
				r.must("inner").SetText("overlay overlay overlay")
				overlay := r.must("overlay")
				s := overlay.Style
				s.Left = Cells(9)
				s.Height = Cells(2)
				overlay.SetStyle(s)
			},
		},
	}
}

// incrementalScrollLayoutCases covers scroll requests issued on a ScrollBox
// that the very same pass prunes. Their bookkeeping has to happen anyway, so
// each case pairs a paint-only scroll request with a geometry mutation
// elsewhere: the scroll box is skipped and drainSkippedScroll must produce the
// same ScrollTop/ScrollHeight a conservative pass would.
func incrementalScrollLayoutCases() []incrementalLayoutCase {
	return []incrementalLayoutCase{
		{
			name:       "scroll-sticky-bottom-through-drain",
			viewport:   incrementalViewport,
			fullscreen: true,
			build: func(mut func(*incrRefs)) (*Node, *incrRefs) {
				body, rows := incrBody(8, false)
				footer := incrStretchedFooter("ready")
				root := incrRoot(Text("header"), body, footer)
				refs := incrRefsOf(root, rows, map[string]*Node{"body": body, "footer": footer})
				if mut != nil {
					mut(refs)
				}
				return root, refs
			},
			mutate: func(r *incrRefs) {
				r.must("body").ScrollToBottom()
				r.must("footer").SetText("busy!")
			},
		},
		{
			name:       "scroll-anchor-through-drain",
			viewport:   incrementalViewport,
			fullscreen: true,
			build: func(mut func(*incrRefs)) (*Node, *incrRefs) {
				body, rows := incrBody(8, false)
				footer := incrStretchedFooter("ready")
				root := incrRoot(Text("header"), body, footer)
				refs := incrRefsOf(root, rows, map[string]*Node{"body": body, "footer": footer})
				if mut != nil {
					mut(refs)
				}
				return root, refs
			},
			mutate: func(r *incrRefs) {
				r.must("body").ScrollToElement(r.entries[5], 0)
				r.must("footer").SetText("busy!")
			},
		},
		{
			name:       "scroll-clamp-through-drain",
			viewport:   incrementalViewport,
			fullscreen: true,
			build: func(mut func(*incrRefs)) (*Node, *incrRefs) {
				body, rows := incrBody(8, false)
				footer := incrStretchedFooter("ready")
				root := incrRoot(Text("header"), body, footer)
				refs := incrRefsOf(root, rows, map[string]*Node{"body": body, "footer": footer})
				if mut != nil {
					mut(refs)
				}
				return root, refs
			},
			mutate: func(r *incrRefs) {
				minY, maxY := 0, 1
				body := r.must("body")
				body.SetScrollClamp(&minY, &maxY)
				body.ScrollTo(9)
				r.must("footer").SetText("busy!")
			},
		},
		{
			name:       "scroll-by-through-drain",
			viewport:   incrementalViewport,
			fullscreen: true,
			build: func(mut func(*incrRefs)) (*Node, *incrRefs) {
				body, rows := incrBody(8, false)
				footer := incrStretchedFooter("ready")
				root := incrRoot(Text("header"), body, footer)
				refs := incrRefsOf(root, rows, map[string]*Node{"body": body, "footer": footer})
				if mut != nil {
					mut(refs)
				}
				return root, refs
			},
			mutate: func(r *incrRefs) {
				r.must("body").ScrollBy(3)
				r.must("footer").SetText("busy!")
			},
		},
	}
}

// collectLayoutFields snapshots the layout state of every node in preorder, so
// two states of the same tree can be compared without rebuilding it.
func collectLayoutFields(root *Node) []layoutFields {
	var out []layoutFields
	root.Walk(func(n *Node) bool {
		out = append(out, layoutFieldsOf(n))
		return true
	})
	return out
}

func compareLayoutFieldSlices(t *testing.T, label string, want, got []layoutFields) {
	t.Helper()
	if len(want) != len(got) {
		t.Fatalf("%s: %d nodes != %d nodes", label, len(want), len(got))
	}
	for i := range want {
		if want[i] != got[i] {
			t.Fatalf("%s: node %d mismatch\n want=%+v\n  got=%+v", label, i, want[i], got[i])
		}
	}
}

// testFooterSameWidthPrunesLargeBranch pins the node counter: the pass that
// follows a same-width footer mutation must not walk into the large ScrollBox
// branch at all, and the geometry it leaves behind must be the geometry a
// conservative pass produces.
func testFooterSameWidthPrunesLargeBranch(t *testing.T) {
	for _, entries := range []int{200, 800} {
		entries := entries
		t.Run(fmt.Sprintf("entries-%d", entries), func(t *testing.T) {
			body, rows := incrBody(entries, false)
			footer := incrStretchedFooter("ready")
			root := incrRoot(Text("header"), body, footer)

			_, first := computeLayout(root, incrementalViewport, true)
			if first < entries {
				t.Fatalf("first pass visited %d nodes, want the whole tree (>= %d)", first, entries)
			}
			stamps := make([]uint64, len(rows))
			for i, row := range rows {
				stamps[i] = row.layoutStamp
			}

			footer.SetText("busy!")
			_, second := computeLayout(root, incrementalViewport, true)
			if second > 10 {
				t.Fatalf("pass after a same-width footer mutation visited %d nodes; the %d-entry branch was not pruned", second, entries)
			}
			if second >= first {
				t.Fatalf("pruned pass visited %d nodes, first pass %d", second, first)
			}
			t.Logf("entries=%d: full pass visited=%d nodes, pruned pass visited=%d nodes", entries, first, second)
			for i, row := range rows {
				if row.layoutStamp != stamps[i] {
					t.Fatalf("entry %d was re-entered by the pruned pass (stamp %d -> %d)", i, stamps[i], row.layoutStamp)
				}
			}

			// The pruned result must be exactly what a full pass produces.
			pruned := collectLayoutFields(root)
			conservativeLayout(root, incrementalViewport, true)
			compareLayoutFieldSlices(t, fmt.Sprintf("pruned-vs-conservative-%d", entries), pruned, collectLayoutFields(root))
		})
	}
}

// testDisplayNoneRoundTrip hides and shows a branch again: the hidden subtree is
// never visited, so showing it must re-layout it from scratch and end up exactly
// where a tree that never hid it is.
func testDisplayNoneRoundTrip(t *testing.T) {
	hidden := Box(Style{FlexDirection: Column, Width: Percent(100)}, Text("one"), Text("two"))
	body, _ := incrBody(4, false)
	footer := incrStretchedFooter("ready")
	root := incrRoot(Text("header"), body, hidden, footer)

	ComputeLayout(root, incrementalViewport, true)
	hide := func() {
		s := hidden.Style
		s.Display = DisplayNone
		hidden.SetStyle(s)
	}
	show := func() {
		s := hidden.Style
		s.Display = DisplayFlex
		hidden.SetStyle(s)
	}
	hide()
	ComputeLayout(root, incrementalViewport, true)
	show()
	ComputeLayout(root, incrementalViewport, true)

	twinHidden := Box(Style{FlexDirection: Column, Width: Percent(100)}, Text("one"), Text("two"))
	twinBody, _ := incrBody(4, false)
	twinRoot := incrRoot(Text("header"), twinBody, twinHidden, incrStretchedFooter("ready"))
	ComputeLayout(twinRoot, incrementalViewport, true)
	compareIncrementalTrees(t, "display-none-round-trip", twinRoot, root)

	// A converged tree must not move when the conservative path walks it: the
	// prune may not leave geometry behind that a full pass would recompute.
	before := collectLayoutFields(root)
	conservativeLayout(root, incrementalViewport, true)
	compareLayoutFieldSlices(t, "display-none-round-trip-conservative", before, collectLayoutFields(root))
}

// testPreviouslyPrunedSubtreeRepropagates covers the failure mode a prune is
// most likely to have: a subtree the previous pass skipped because it was
// clean, which then receives a second mutation and must be re-entered.
func testPreviouslyPrunedSubtreeRepropagates(t *testing.T) {
	const entries = 12
	body, rows := incrBody(entries, false)
	footer := incrStretchedFooter("ready")
	root := incrRoot(Text("header"), body, footer)

	if _, first := computeLayout(root, incrementalViewport, true); first < entries {
		t.Fatalf("first pass visited %d nodes, want the whole tree", first)
	}

	// First mutation: a same-width footer change, so the body branch is skipped.
	footer.SetText("busy!")
	entry := rows[7]
	beforeStamp := entry.layoutStamp
	if _, pruned := computeLayout(root, incrementalViewport, true); pruned > 10 {
		t.Fatalf("pass after a same-width footer mutation visited %d nodes, want the body pruned", pruned)
	}
	if entry.layoutStamp != beforeStamp {
		t.Fatal("entry inside the body was re-entered by a pass that should have pruned it")
	}
	if entry.subtreeGeomDirty {
		t.Fatal("a pruned entry must not look geometrically dirty")
	}

	// Second mutation on the same entry: the flag has to travel up to the root
	// again, otherwise the next pass would prune over a real change.
	entry.SetText("entry 007 payload that is noticeably longer than before")
	for cur := entry; cur != nil; cur = cur.Parent {
		if !cur.subtreeGeomDirty {
			t.Fatalf("subtreeGeomDirty was not propagated to ancestor %s", cur.ID)
		}
	}
	if !root.layoutDirty {
		t.Fatal("root.layoutDirty was not set by a geometry mutation inside a pruned subtree")
	}
	_, reentered := computeLayout(root, incrementalViewport, true)
	if reentered < entries {
		t.Fatalf("pass re-entered only %d nodes, want the scroll body re-laid out (>= %d)", reentered, entries)
	}

	reenteredFields := collectLayoutFields(root)
	conservativeLayout(root, incrementalViewport, true)
	compareLayoutFieldSlices(t, "repropagated-subtree-conservative", reenteredFields, collectLayoutFields(root))

	twinBody, twinRows := incrBody(entries, false)
	twinRoot := incrRoot(Text("header"), twinBody, incrStretchedFooter("busy!"))
	twinRows[7].SetText("entry 007 payload that is noticeably longer than before")
	ComputeLayout(twinRoot, incrementalViewport, true)
	compareIncrementalTrees(t, "repropagated-subtree", twinRoot, root)
}

// geometryFields is the geometry half of the layout state: paint-only
// mutations may change scroll bookkeeping, but never a rect.
type geometryFields struct {
	Rect            Rect
	ContentRect     Rect
	ChildrenLinearY bool
}

func collectGeometry(root *Node) []geometryFields {
	var out []geometryFields
	root.Walk(func(n *Node) bool {
		out = append(out, geometryFields{Rect: n.Rect, ContentRect: n.ContentRect, ChildrenLinearY: n.childrenLinearY})
		return true
	})
	return out
}

func compareGeometrySlices(t *testing.T, label string, want, got []geometryFields) {
	t.Helper()
	if len(want) != len(got) {
		t.Fatalf("%s: %d nodes != %d nodes", label, len(want), len(got))
	}
	for i := range want {
		if want[i] != got[i] {
			t.Fatalf("%s: node %d geometry changed\n want=%+v\n  got=%+v", label, i, want[i], got[i])
		}
	}
}

// TestIncrementalLayoutPaintOnlyDoesNotRelayout pins the other half of the
// contract: SetTextStyle and the scroll API keep reusing cached geometry, hit
// the cached early return of ComputeLayout and leave every rect untouched.
// production_round5_test.go:21-40 and post_v016_hardening_test.go assert the
// same invariant for their own trees and are deliberately left unmodified.
func TestIncrementalLayoutPaintOnlyDoesNotRelayout(t *testing.T) {
	steps := []struct {
		name          string
		scrollRequest bool
		run           func(body *Node, rows []*Node, footer *Node)
	}{
		{
			// Only a lower clamp: it never moves the scroll position away from 0.
			name: "SetScrollClamp",
			run: func(body *Node, rows []*Node, footer *Node) {
				minY := 0
				body.SetScrollClamp(&minY, nil)
			},
		},
		{
			name: "SetTextStyle-entry",
			run: func(body *Node, rows []*Node, footer *Node) {
				rows[2].SetTextStyle(TextStyle{Italic: true, Underline: true})
			},
		},
		{
			name: "SetTextStyle-footer",
			run: func(body *Node, rows []*Node, footer *Node) {
				footer.SetTextStyle(TextStyle{Bold: true})
			},
		},
		{name: "ScrollTo", scrollRequest: true, run: func(body *Node, rows []*Node, footer *Node) { body.ScrollTo(1) }},
		{name: "ScrollBy", scrollRequest: true, run: func(body *Node, rows []*Node, footer *Node) { body.ScrollBy(2) }},
		{
			name: "ScrollToElement", scrollRequest: true,
			run: func(body *Node, rows []*Node, footer *Node) { body.ScrollToElement(rows[4], 0) },
		},
		{
			name: "ScrollToBottom", scrollRequest: true,
			run: func(body *Node, rows []*Node, footer *Node) { body.ScrollToBottom() },
		},
	}
	for _, step := range steps {
		step := step
		t.Run(step.name, func(t *testing.T) {
			// A fresh tree per step keeps the steps independent and starts every
			// scroll request from the top of the transcript.
			body, rows := incrBody(6, false)
			footer := incrStretchedFooter("ready")
			root := incrRoot(Text("header"), body, footer)

			size := ComputeLayout(root, incrementalViewport, true)
			geometry := collectGeometry(root)

			step.run(body, rows, footer)
			if root.layoutDirty {
				t.Fatal("paint-only mutation set layoutDirty on the root")
			}
			if !root.layoutCached {
				t.Fatal("paint-only mutation dropped the cached layout")
			}
			if _, visited := computeLayout(root, incrementalViewport, true); visited != 0 {
				t.Fatalf("paint-only mutation walked %d nodes; the cached layout must be reused", visited)
			}
			if got := ComputeLayout(root, incrementalViewport, true); got != size {
				t.Fatalf("cached layout size = %+v, want %+v", got, size)
			}
			compareGeometrySlices(t, step.name, geometry, collectGeometry(root))
			if step.scrollRequest {
				if body.ScrollTop == 0 && body.PendingScrollDelta == 0 {
					t.Fatal("scroll request did not reach the scroll box")
				}
			} else if body.ScrollTop != 0 || body.PendingScrollDelta != 0 {
				t.Fatalf("plain paint mutation moved scroll state: top=%d pending=%d",
					body.ScrollTop, body.PendingScrollDelta)
			}
		})
	}
}

// TestIncrementalLayoutScrollDrainInFullPass covers the other direction: a full
// pass forced by a geometry mutation elsewhere must still run the scroll
// bookkeeping of the ScrollBoxes it skipped, and run it exactly once.
func TestIncrementalLayoutScrollDrainInFullPass(t *testing.T) {
	t.Run("pending-delta-matches-scroll-only-pass", func(t *testing.T) {
		bodyA, _ := incrBody(8, false)
		rootA := incrRoot(Text("header"), bodyA, incrStretchedFooter("ready"))
		ComputeLayout(rootA, incrementalViewport, true)

		bodyB, _ := incrBody(8, false)
		footerB := incrStretchedFooter("ready")
		rootB := incrRoot(Text("header"), bodyB, footerB)
		ComputeLayout(rootB, incrementalViewport, true)

		// A: the scroll request alone leaves the cached layout valid, so the
		// request is applied by refreshScrollState.
		bodyA.ScrollBy(3)
		ComputeLayout(rootA, incrementalViewport, true)

		// B: the same request in the pass a geometry mutation forces, where the
		// body is pruned and only the drain can apply it.
		bodyB.ScrollBy(3)
		footerB.SetText("busy!")
		if _, visited := computeLayout(rootB, incrementalViewport, true); visited > 10 {
			t.Fatalf("footer mutation visited %d nodes, want the scroll body pruned", visited)
		}
		if bodyB.ScrollTop != bodyA.ScrollTop || bodyB.PendingScrollDelta != bodyA.PendingScrollDelta {
			t.Fatalf("drained scroll state top=%d pending=%d; scroll-only pass top=%d pending=%d",
				bodyB.ScrollTop, bodyB.PendingScrollDelta, bodyA.ScrollTop, bodyA.PendingScrollDelta)
		}

		// C: the same final state built from scratch and laid out once.
		twinBody, _ := incrBody(8, false)
		twinRoot := incrRoot(Text("header"), twinBody, incrStretchedFooter("busy!"))
		twinBody.ScrollBy(3)
		ComputeLayout(twinRoot, incrementalViewport, true)
		compareIncrementalTrees(t, "pending-delta-drain", twinRoot, rootB)
	})

	t.Run("sticky-scroll-reapplied-through-drain", func(t *testing.T) {
		body, _ := incrBody(10, false)
		footer := incrStretchedFooter("ready")
		root := incrRoot(Text("header"), body, footer)
		ComputeLayout(root, incrementalViewport, true)

		body.ScrollToBottom()
		footer.SetText("busy!")
		if _, visited := computeLayout(root, incrementalViewport, true); visited > 10 {
			t.Fatalf("footer mutation visited %d nodes, want the scroll body pruned", visited)
		}
		if !body.StickyScroll {
			t.Fatal("sticky request was lost by the pruned pass")
		}
		maxScroll := body.ScrollHeight - body.ScrollViewportHeight
		if maxScroll <= 0 {
			t.Fatalf("scroll box reports no overflow: height=%d viewport=%d", body.ScrollHeight, body.ScrollViewportHeight)
		}
		if body.ScrollTop != maxScroll {
			t.Fatalf("sticky scroll top = %d, want %d", body.ScrollTop, maxScroll)
		}

		twinBody, _ := incrBody(10, false)
		twinRoot := incrRoot(Text("header"), twinBody, incrStretchedFooter("busy!"))
		twinBody.ScrollToBottom()
		ComputeLayout(twinRoot, incrementalViewport, true)
		compareIncrementalTrees(t, "sticky-drain", twinRoot, root)
	})

	t.Run("nested-scroll-boxes-inside-pruned-branch", func(t *testing.T) {
		type nestedScrollTree struct {
			root   *Node
			inner  *Node
			outer  *Node
			second *Node
			footer *Node
		}
		build := func(footerText string, request func(nestedScrollTree)) nestedScrollTree {
			innerRows := []*Node{Text("i1"), Text("i2"), Text("i3"), Text("i4"), Text("i5")}
			inner := ScrollBox(Style{Height: Cells(2), Width: Percent(100)}, false, innerRows...)
			outer := ScrollBox(Style{Height: Cells(3), Width: Percent(100)}, false,
				Text("outer a"), inner, Text("outer b"))
			second := ScrollBox(Style{Height: Cells(2), Width: Percent(100)}, false,
				Text("s1"), Text("s2"), Text("s3"))
			group := Box(Style{FlexDirection: Column, Width: Percent(100)}, outer, second)
			footer := incrStretchedFooter(footerText)
			root := incrRoot(Text("header"), group, footer)
			tree := nestedScrollTree{root: root, inner: inner, outer: outer, second: second, footer: footer}
			if request != nil {
				request(tree)
			}
			return tree
		}
		request := func(tree nestedScrollTree) {
			tree.inner.ScrollBy(5)
			tree.outer.ScrollToBottom()
			tree.second.ScrollBy(4)
		}

		mutated := build("ready", nil)
		ComputeLayout(mutated.root, incrementalViewport, true)
		request(mutated)
		mutated.footer.SetText("busy!")
		if _, visited := computeLayout(mutated.root, incrementalViewport, true); visited > 10 {
			t.Fatalf("footer mutation visited %d nodes, want the whole group pruned", visited)
		}
		// Two nesting levels plus a sibling scroll box: every one of them has to
		// be drained, not just the outermost.
		for name, box := range map[string]*Node{"inner": mutated.inner, "outer": mutated.outer, "second": mutated.second} {
			if box.ScrollTop == 0 && box.PendingScrollDelta == 0 {
				t.Fatalf("%s ScrollBox inside the pruned subtree was never drained", name)
			}
		}

		twin := build("busy!", request)
		ComputeLayout(twin.root, incrementalViewport, true)
		compareIncrementalTrees(t, "nested-drain", twin.root, mutated.root)
	})

	t.Run("drain-runs-once-per-scroll-box", func(t *testing.T) {
		body, _ := incrBody(12, false)
		footer := incrStretchedFooter("ready")
		root := incrRoot(Text("header"), body, footer)
		ComputeLayout(root, incrementalViewport, true)

		body.ScrollBy(100)
		footer.SetText("busy!")
		if _, visited := computeLayout(root, incrementalViewport, true); visited > 10 {
			t.Fatalf("footer mutation visited %d nodes, want the scroll body pruned", visited)
		}
		// Exactly one updateScrollState call: pending=100 on a 3-row viewport
		// drains a single step, max(4, 75) capped at viewportHeight-1 = 2, and
		// leaves 98 pending. A duplicated drain would leave top=4 pending=96.
		if body.ScrollTop != 2 || body.PendingScrollDelta != 98 {
			t.Fatalf("pruned pass applied top=%d pending=%d, want exactly one drain step (top=2 pending=98)",
				body.ScrollTop, body.PendingScrollDelta)
		}

		twinBody, _ := incrBody(12, false)
		twinRoot := incrRoot(Text("header"), twinBody, incrStretchedFooter("busy!"))
		twinBody.ScrollBy(100)
		ComputeLayout(twinRoot, incrementalViewport, true)
		if twinBody.ScrollTop != body.ScrollTop || twinBody.PendingScrollDelta != body.PendingScrollDelta {
			t.Fatalf("twin top=%d pending=%d; drained top=%d pending=%d",
				twinBody.ScrollTop, twinBody.PendingScrollDelta, body.ScrollTop, body.PendingScrollDelta)
		}
		compareIncrementalTrees(t, "single-drain", twinRoot, root)
	})
}

// propertyTree is the transcript-shaped tree the property test mutates: a
// header row with a ButtonRender button, a ScrollBox holding N entries plus a
// group, and a footer.
type propertyTree struct {
	root    *Node
	footer  *Node
	scroll  *Node
	group   *Node
	button  *Node
	entries []*Node
}

func buildPropertyTree(entries int) *propertyTree {
	rows := make([]*Node, 0, entries)
	for i := 0; i < entries; i++ {
		rows = append(rows, Text(fmt.Sprintf("row %02d payload", i)))
	}
	group := Box(Style{FlexDirection: Column, Width: Percent(100)}, Text("group a"), Text("group b"))
	content := append(append([]*Node(nil), rows...), group)
	scroll := ScrollBox(Style{Height: Cells(8), Width: Percent(100)}, false, content...)
	render := func(s ButtonState) []*Node {
		if s.Active {
			return []*Node{Text("saving")}
		}
		return []*Node{Text("idle")}
	}
	button := ButtonWithState(Style{Width: Cells(12), Height: Cells(1), FlexDirection: Row}, func() {}, render)
	headerRow := Box(Style{FlexDirection: Row, Width: Percent(100)}, Text("header"), button)
	footer := incrStretchedFooter("ready")
	root := incrRoot(headerRow, scroll, footer)
	return &propertyTree{root: root, footer: footer, scroll: scroll, group: group, button: button, entries: rows}
}

type propertyOpKind int

const (
	opSetText propertyOpKind = iota
	opSetTextStyle
	opSetStyle
	opScrollTo
	opScrollBy
	opScrollToBottom
	opScrollToElement
	opSetScrollClamp
	opAppendEntry
	opRemoveEntry
	opActivateButton
	opExpireButton
	propertyOpKindCount
)

func (k propertyOpKind) String() string {
	switch k {
	case opSetText:
		return "SetText"
	case opSetTextStyle:
		return "SetTextStyle"
	case opSetStyle:
		return "SetStyle"
	case opScrollTo:
		return "ScrollTo"
	case opScrollBy:
		return "ScrollBy"
	case opScrollToBottom:
		return "ScrollToBottom"
	case opScrollToElement:
		return "ScrollToElement"
	case opSetScrollClamp:
		return "SetScrollClamp"
	case opAppendEntry:
		return "Append"
	case opRemoveEntry:
		return "Remove"
	case opActivateButton:
		return "ButtonActivate"
	case opExpireButton:
		return "ButtonExpire"
	}
	return "unknown"
}

type propertyOp struct {
	kind   propertyOpKind
	entry  int
	text   string
	amount int
	index  int
	style  TextStyle
}

// propertyPayloads mixes widths and shapes so wrapping and auto-sizing are both
// reachable.
var propertyPayloads = []string{
	"x",
	"short",
	"mid payload here",
	"a much longer payload that has to wrap over several rows inside its entry",
}

func randomPropertyOp(rng *rand.Rand, tree *propertyTree) propertyOp {
	entry := -1
	if len(tree.entries) > 0 {
		entry = rng.Intn(len(tree.entries))
	}
	payload := propertyPayloads[rng.Intn(len(propertyPayloads))]
	switch propertyOpKind(rng.Intn(int(propertyOpKindCount))) {
	case opSetText:
		return propertyOp{kind: opSetText, entry: entry, text: payload}
	case opSetTextStyle:
		return propertyOp{kind: opSetTextStyle, entry: entry,
			style: TextStyle{Bold: rng.Intn(2) == 1, Italic: rng.Intn(2) == 1}}
	case opSetStyle:
		return propertyOp{kind: opSetStyle, entry: entry, amount: rng.Intn(6), index: rng.Intn(4)}
	case opScrollTo:
		return propertyOp{kind: opScrollTo, amount: rng.Intn(9)}
	case opScrollBy:
		return propertyOp{kind: opScrollBy, amount: rng.Intn(9) - 4}
	case opScrollToBottom:
		return propertyOp{kind: opScrollToBottom}
	case opScrollToElement:
		return propertyOp{kind: opScrollToElement, entry: entry, amount: rng.Intn(3)}
	case opSetScrollClamp:
		return propertyOp{kind: opSetScrollClamp, index: rng.Intn(3), amount: rng.Intn(4)}
	case opAppendEntry:
		return propertyOp{kind: opAppendEntry, text: payload}
	case opRemoveEntry:
		return propertyOp{kind: opRemoveEntry}
	case opActivateButton:
		return propertyOp{kind: opActivateButton}
	default:
		return propertyOp{kind: opExpireButton}
	}
}

// applyPropertyOp applies one operation through the public mutation API and
// returns the node it mutated, or nil when the operation only changed scroll
// state or was a no-op.
func applyPropertyOp(t *testing.T, op propertyOp, tree *propertyTree) *Node {
	t.Helper()
	switch op.kind {
	case opSetText:
		if op.entry < 0 {
			return nil
		}
		node := tree.entries[op.entry]
		if node.Text == ExpandTabs(op.text) {
			return nil
		}
		node.SetText(op.text)
		return node
	case opSetTextStyle:
		if op.entry < 0 {
			return nil
		}
		tree.entries[op.entry].SetTextStyle(op.style)
		return tree.entries[op.entry]
	case opSetStyle:
		if op.entry < 0 {
			return nil
		}
		node := tree.entries[op.entry]
		s := node.Style
		switch op.index {
		case 0:
			s.Width = Cells(float64(8 + op.amount*3))
		case 1:
			s.Height = Cells(float64(1 + op.amount))
		case 2:
			s.TextWrap = TextWrapTruncateEnd
		default:
			s.AlignSelf = AlignFlexStart
		}
		node.SetStyle(s)
		return node
	case opScrollTo:
		tree.scroll.ScrollTo(op.amount)
	case opScrollBy:
		tree.scroll.ScrollBy(op.amount)
	case opScrollToBottom:
		tree.scroll.ScrollToBottom()
	case opScrollToElement:
		if op.entry < 0 {
			return nil
		}
		tree.scroll.ScrollToElement(tree.entries[op.entry], op.amount)
	case opSetScrollClamp:
		if op.index == 0 {
			tree.scroll.SetScrollClamp(nil, nil)
			return nil
		}
		minY, maxY := 0, op.amount
		tree.scroll.SetScrollClamp(&minY, &maxY)
	case opAppendEntry:
		content := tree.scroll.ScrollContent()
		if content == nil {
			return nil
		}
		content.Append(Text(op.text))
		return content
	case opRemoveEntry:
		if len(tree.group.Children) == 0 {
			return nil
		}
		tree.group.Remove(tree.group.Children[len(tree.group.Children)-1])
		return tree.group
	case opActivateButton:
		if tree.button.ButtonState.Active {
			// activateButton only refreshes ActiveUntil when the button is
			// already Active, so nothing gets marked dirty: report a no-op.
			return nil
		}
		tree.button.activateButton()
		return tree.button
	case opExpireButton:
		if !tree.button.ButtonState.Active {
			return nil
		}
		tree.button.ActiveUntil = time.Now().Add(-time.Second)
		refreshExpiredButtonStates(tree.root)
		return tree.button
	}
	return nil
}

// assertAncestorsFlagged checks the propagation contract of MarkDirty: a
// geometry mutation must mark itself and every ancestor, otherwise the next
// pass would prune over the change.
func assertAncestorsFlagged(t *testing.T, node *Node, context string) {
	t.Helper()
	if node == nil {
		t.Fatalf("%s: mutation reported no target node", context)
	}
	for cur := node; cur != nil; cur = cur.Parent {
		if !cur.subtreeGeomDirty {
			t.Fatalf("%s: subtreeGeomDirty was not propagated to ancestor %s", context, cur.ID)
		}
		if !cur.layoutDirty {
			t.Fatalf("%s: layoutDirty was not propagated to ancestor %s", context, cur.ID)
		}
	}
}

// TestIncrementalLayoutMutationProperty drives a deterministic pseudo-random
// mutation sequence over a transcript-shaped tree. After every single step the
// incrementally laid-out tree is compared field by field against a tree that
// received the same mutation but is always laid out with pruning disabled, so a
// prune or a drain that loses state shows up immediately with the step number
// and the operation that produced it.
func TestIncrementalLayoutMutationProperty(t *testing.T) {
	const (
		seed    = 20260918
		entries = 14
		steps   = 400
	)
	geometryOp := func(kind propertyOpKind) bool {
		switch kind {
		case opSetText, opSetStyle, opAppendEntry, opRemoveEntry, opActivateButton, opExpireButton:
			return true
		}
		return false
	}

	run := func(t *testing.T, fullscreen bool) {
		t.Helper()
		rng := rand.New(rand.NewSource(seed))
		incremental := buildPropertyTree(entries)
		reference := buildPropertyTree(entries)
		ComputeLayout(incremental.root, incrementalViewport, fullscreen)
		ComputeLayout(reference.root, incrementalViewport, fullscreen)
		compareIncrementalTrees(t, "start", reference.root, incremental.root)

		for step := 0; step < steps; step++ {
			op := randomPropertyOp(rng, incremental)
			mutatedNode := applyPropertyOp(t, op, incremental)
			applyPropertyOp(t, op, reference)
			label := fmt.Sprintf("step %d (%s)", step, op.kind)
			if len(incremental.entries) != len(reference.entries) {
				t.Fatalf("%s: trees diverged in shape", label)
			}
			// Checked before the passes clear the flags.
			if geometryOp(op.kind) && mutatedNode != nil {
				assertAncestorsFlagged(t, mutatedNode, label)
			}

			ComputeLayout(incremental.root, incrementalViewport, fullscreen)
			conservativeLayout(reference.root, incrementalViewport, fullscreen)
			compareIncrementalTrees(t, label, reference.root, incremental.root)
		}
	}

	t.Run("fullscreen", func(t *testing.T) { run(t, true) })
	t.Run("natural-height", func(t *testing.T) { run(t, false) })

	// The case a prune is most likely to get wrong: a subtree that the previous
	// pass skipped, then two consecutive mutations inside it.
	t.Run("two-mutations-in-previously-pruned-subtree", func(t *testing.T) {
		incremental := buildPropertyTree(entries)
		ComputeLayout(incremental.root, incrementalViewport, true)

		// A paint-only footer change leaves the body clean, so the next pass
		// prunes over it.
		incremental.footer.SetText("busy!")
		entry := incremental.entries[6]
		stamp := entry.layoutStamp
		if _, visited := computeLayout(incremental.root, incrementalViewport, true); visited > 10 {
			t.Fatalf("footer mutation visited %d nodes, want the scroll body pruned", visited)
		}
		if entry.layoutStamp != stamp {
			t.Fatal("entry inside the body was re-entered by a pass that should have pruned it")
		}
		if entry.subtreeGeomDirty {
			t.Fatal("a pruned entry must not look geometrically dirty")
		}

		// First mutation on the pruned entry.
		first := propertyOp{kind: opSetText, entry: 6, text: "first mutation payload that wraps over two rows"}
		assertAncestorsFlagged(t, applyPropertyOp(t, first, incremental), first.kind.String())
		firstStamp := entry.layoutStamp
		if _, visited := computeLayout(incremental.root, incrementalViewport, true); visited < entries {
			t.Fatalf("pass after the first entry mutation visited %d nodes, want the body re-laid out", visited)
		}
		if entry.layoutStamp == firstStamp {
			t.Fatal("the mutated entry was not re-laid out after its subtree was pruned")
		}

		// Second mutation on the same entry, which is clean again by now.
		second := propertyOp{kind: opSetText, entry: 6, text: "second mutation payload, longer still so the row height changes"}
		assertAncestorsFlagged(t, applyPropertyOp(t, second, incremental), second.kind.String())
		if _, visited := computeLayout(incremental.root, incrementalViewport, true); visited < entries {
			t.Fatalf("pass after the second entry mutation visited %d nodes, want the body re-laid out", visited)
		}

		before := collectLayoutFields(incremental.root)
		conservativeLayout(incremental.root, incrementalViewport, true)
		compareLayoutFieldSlices(t, "two-mutations-conservative", before, collectLayoutFields(incremental.root))
	})
}
