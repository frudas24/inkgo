package engine

import (
	"fmt"
	"testing"
)

// The tests below cover the container half of the intrinsic measurement memo
// (nodeFlowCache). measureNode used to memoise NodeText/NodeRawANSI only, so a
// Box/ScrollBox/Root re-walked its whole subtree on every call - and because the
// parent's flex pass measures all of its children before the incremental prune
// in layoutNode can skip any of them, a single leaf text update still cost
// O(subtree) of measurement work per ancestor.
//
// The oracle is always a twin built from scratch in the same final state: a fresh
// tree carries no memo, so its measurement is the cold one by construction. A
// stale entry served on the incremental tree therefore shows up as a divergent
// value, and the probe counter (measureNodeProbe) shows whether the walk really
// happened instead of being served from the cache.

// fmcWalks counts every node measureNode visits while fn runs.
func fmcWalks(fn func()) int {
	walks := 0
	prev := measureNodeProbe
	measureNodeProbe = func(*Node, int, int) { walks++ }
	defer func() { measureNodeProbe = prev }()
	fn()
	return walks
}

// fmcMeasure measures a node and also reports how many nodes the call walked.
func fmcMeasure(n *Node, availW, availH int) (measured, int) {
	var m measured
	walks := fmcWalks(func() { m = measureNode(n, availW, availH) })
	return m, walks
}

// fmcSubtreeNodes counts a node plus its descendants: the number of nodes a cold
// measurement of that node has to visit.
func fmcSubtreeNodes(n *Node) int {
	if n == nil {
		return 0
	}
	count := 0
	n.Walk(func(*Node) bool { count++; return true })
	return count
}

// fmcMeasureKeys runs one pass and reports the largest number of distinct
// available spaces a single *container* was measured under during it, plus the
// same number for text nodes as context. Only containers use nodeFlowMeasureSlots
// (Text/NodeRawANSI are memoised by the single-entry nodeMeasureCache), so this
// is the evidence behind the slot count, and it is measured rather than guessed:
// the flex path measures a child once as an item, once again under the main size
// it was allocated, and once per ancestor level above that.
func fmcMeasureKeys(run func()) (maxContainer int, worst *Node, containerCount, maxText int) {
	seen := map[*Node]map[[2]int]struct{}{}
	prev := measureNodeProbe
	measureNodeProbe = func(n *Node, availW, availH int) {
		if n == nil {
			return
		}
		m := seen[n]
		if m == nil {
			m = map[[2]int]struct{}{}
			seen[n] = m
		}
		m[[2]int{availW, availH}] = struct{}{}
	}
	defer func() { measureNodeProbe = prev }()
	run()
	for n, m := range seen {
		if n.Kind == NodeText || n.Kind == NodeRawANSI {
			if len(m) > maxText {
				maxText = len(m)
			}
			continue
		}
		containerCount++
		if len(m) > maxContainer {
			maxContainer, worst = len(m), n
		}
	}
	return maxContainer, worst, containerCount, maxText
}

// TestFlowMeasureCacheServesRepeatedMeasurement pins the basic equivalence: for
// an unchanged subtree and the same available space, the second measurement of a
// container is served from the memo and walks nothing but the container itself,
// while a measurement under a different available space is a different key and
// has to walk the subtree. The value is compared against a twin that never saw a
// first measurement.
func TestFlowMeasureCacheServesRepeatedMeasurement(t *testing.T) {
	// Built but never laid out: nothing is memoised yet, so the first
	// measurement below is cold by construction.
	tree := ilpAutoBuild()
	subtree := fmcSubtreeNodes(tree.outer)
	if subtree < 5 {
		t.Fatalf("the measured subtree is only %d nodes; the walk counts below would be vacuous", subtree)
	}

	first, coldWalks := fmcMeasure(tree.outer, 40, 12)
	if coldWalks != subtree {
		t.Fatalf("the first measurement walked %d nodes, want the whole subtree (%d)", coldWalks, subtree)
	}
	second, hotWalks := fmcMeasure(tree.outer, 40, 12)
	if hotWalks != 1 {
		t.Fatalf("the repeated measurement walked %d nodes, want 1 (the container itself): the subtree was re-measured", hotWalks)
	}
	if first != second {
		t.Fatalf("the memoised measurement %+v differs from the computed one %+v", second, first)
	}

	// A different available space is a different key: it must recompute, and the
	// two keys must both stay valid (the cache holds more than one).
	other, otherWalks := fmcMeasure(tree.outer, 26, 12)
	if otherWalks != subtree {
		t.Fatalf("measuring under a new available space walked %d nodes, want a full walk (%d)", otherWalks, subtree)
	}
	if _, againWalks := fmcMeasure(tree.outer, 40, 12); againWalks != 1 {
		t.Fatalf("the first key was evicted by a second one (%d nodes walked, want 1)", againWalks)
	}
	if _, againWalks := fmcMeasure(tree.outer, 26, 12); againWalks != 1 {
		t.Fatalf("the second key was evicted by a third measurement (%d nodes walked, want 1)", againWalks)
	}

	// The values must be what a tree without any memo produces.
	twin := ilpAutoBuild()
	if want, _ := fmcMeasure(twin.outer, 40, 12); want != first {
		t.Fatalf("memoised measurement %+v != cold twin %+v", first, want)
	}
	if want, _ := fmcMeasure(twin.outer, 26, 12); want != other {
		t.Fatalf("memoised measurement at the second key %+v != cold twin %+v", other, want)
	}
}

// TestFlowMeasureCacheAlternatesFlexAvails drives the two keys a flex child is
// measured under in the same pass (the item measurement and the post-distribution
// re-measurement) in alternation, both ways, and then overflows the cache with
// more keys than it holds. A cache that serves another key's result, or that
// keeps only one key, is caught here; so is a cap that is too small to make the
// alternating pattern useful.
func TestFlowMeasureCacheAlternatesFlexAvails(t *testing.T) {
	tree := ilpAutoBuild()
	// The two spaces the flex path uses for one child: (mainAvail, crossAvail)
	// and (allocatedMain, crossAvail).
	itemKey := [2]int{40, 12}
	allocatedKey := [2]int{28, 12}

	item, itemWalks := fmcMeasure(tree.outer, itemKey[0], itemKey[1])
	allocated, allocatedWalks := fmcMeasure(tree.outer, allocatedKey[0], allocatedKey[1])
	if itemWalks <= 1 || allocatedWalks <= 1 {
		t.Fatalf("a cold measurement walked %d/%d nodes, want a full subtree walk", itemWalks, allocatedWalks)
	}
	if item == allocated {
		t.Fatalf("the two flex keys produced the same measurement %+v; the case would pass on a cache that ignores the key", item)
	}

	for round := 0; round < 3; round++ {
		for _, key := range [][2]int{itemKey, allocatedKey} {
			got, walks := fmcMeasure(tree.outer, key[0], key[1])
			if walks != 1 {
				t.Fatalf("round %d, key %v: walked %d nodes, want 1: alternating the two flex keys re-measured the subtree", round, key, walks)
			}
			want := item
			if key == allocatedKey {
				want = allocated
			}
			if got != want {
				t.Fatalf("round %d, key %v: measurement %+v, want %+v: a result was served under the wrong key", round, key, got, want)
			}
		}
	}

	// More keys than the cache holds: going over the cap may recompute, but every
	// key has to keep returning exactly what a cold measurement returns.
	overflow := [][2]int{{39, 12}, {38, 12}, {37, 12}, {36, 12}, {35, 12}}
	twin := ilpAutoBuild()
	for _, key := range overflow {
		want, _ := fmcMeasure(twin.outer, key[0], key[1])
		got, _ := fmcMeasure(tree.outer, key[0], key[1])
		if got != want {
			t.Fatalf("key %v past the cap: measurement %+v, want the cold one %+v", key, got, want)
		}
	}
	for _, key := range append(append([][2]int(nil), overflow...), itemKey, allocatedKey) {
		want, _ := fmcMeasure(ilpAutoBuild().outer, key[0], key[1])
		got, walks := fmcMeasure(tree.outer, key[0], key[1])
		if got != want {
			t.Fatalf("key %v after the cache wrapped: measurement %+v, want the cold one %+v", key, got, want)
		}
		if walks == 0 {
			t.Fatalf("key %v: measureNode walked nothing", key)
		}
	}
}

// TestFlowMeasureCacheStaleEntryAdversarial is the case that can actually break:
// an entry that outlives the mutation of the subtree it was measured from. Each
// case warms the memo for an auto-sized container, mutates a descendant (or the
// container itself) and then requires the container's measurement to equal the
// one a twin built from scratch produces in the same final state - and, whenever
// the mutation moved the measurement, to have been recomputed rather than
// served.
func TestFlowMeasureCacheStaleEntryAdversarial(t *testing.T) {
	const availW, availH = 40, 12
	cases := []struct {
		name   string
		mutate func(*ilpAutoTree)
	}{
		{"leaf text grows", func(a *ilpAutoTree) {
			a.leaf.SetText("a payload long enough to wrap over several rows inside this box")
		}},
		{"leaf text shrinks", func(a *ilpAutoTree) { a.leaf.SetText("tiny") }},
		{"leaf text multiline", func(a *ilpAutoTree) { a.leaf.SetText("two\nlines\nhere") }},
		{"leaf text cleared", func(a *ilpAutoTree) { a.leaf.SetText("") }},
		{"inner width fixed", func(a *ilpAutoTree) {
			a.inner.SetStyle(Style{FlexDirection: Column, Width: Cells(30)})
		}},
		{"inner width back to auto", func(a *ilpAutoTree) {
			a.inner.SetStyle(Style{FlexDirection: Column})
		}},
		{"inner padding", func(a *ilpAutoTree) {
			a.inner.SetStyle(Style{FlexDirection: Column, Padding: I(3)})
		}},
		{"mid padding", func(a *ilpAutoTree) {
			a.mid.SetStyle(Style{FlexDirection: Row, Padding: I(2)})
		}},
		{"mid gap", func(a *ilpAutoTree) {
			a.mid.SetStyle(Style{FlexDirection: Row, Gap: I(1)})
		}},
		{"mid becomes a column", func(a *ilpAutoTree) {
			a.mid.SetStyle(Style{FlexDirection: Column})
		}},
		{"leaf flex grow", func(a *ilpAutoTree) {
			a.leaf.SetStyle(Style{FlexDirection: Row, TextWrap: TextWrapWrap, FlexGrow: F(1), FlexShrink: F(1)})
		}},
		{"leaf display none", func(a *ilpAutoTree) {
			a.leaf.SetStyle(Style{Display: DisplayNone})
		}},
		{"mid display none", func(a *ilpAutoTree) {
			a.mid.SetStyle(Style{FlexDirection: Row, Display: DisplayNone})
		}},
		{"mid absolute", func(a *ilpAutoTree) {
			a.mid.SetStyle(Style{
				FlexDirection: Row, Position: PositionAbsolute,
				Left: Cells(1), Top: Cells(1), Width: Cells(20), Height: Cells(4),
			})
		}},
		{"leaf appended", func(a *ilpAutoTree) { a.inner.Append(Text("appended row")) }},
		{"spare appended to the inner box", func(a *ilpAutoTree) { a.inner.Append(a.spare) }},
		{"subtree removed", func(a *ilpAutoTree) {
			if n := len(a.inner.Children); n > 0 {
				a.inner.Remove(a.inner.Children[n-1])
			}
		}},
		{"children replaced", func(a *ilpAutoTree) {
			a.inner.SetChildren(Text("alpha"), Text("beta payload that wraps over a couple of rows"))
		}},
		{"two mutations without a pass", func(a *ilpAutoTree) {
			a.leaf.SetText("first mutation that is longer than the original row one")
			a.inner.Append(Text("second mutation"))
			a.mid.SetStyle(Style{FlexDirection: Row, Padding: I(1)})
		}},
		{"text after a style change", func(a *ilpAutoTree) {
			a.inner.SetStyle(Style{FlexDirection: Column, Width: Cells(34)})
			a.leaf.SetText("and then the leaf changes as well")
		}},
		{"deep descendant style change", func(a *ilpAutoTree) {
			a.inner.Children[len(a.inner.Children)-1].SetStyle(Style{
				FlexDirection: Column, Width: Cells(12), Height: Cells(2),
			})
		}},
		{"outer wraps", func(a *ilpAutoTree) {
			a.outer.SetStyle(Style{FlexDirection: Row, FlexWrap: Wrap, Width: Percent(100)})
		}},
	}

	moved := 0
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			incremental := ilpAutoBuild()
			ComputeLayout(incremental.root, Size{Width: availW, Height: availH}, true)
			before, _ := fmcMeasure(incremental.outer, availW, availH)
			// The key has to be memoised before the mutation, otherwise the case
			// would pass without ever putting an entry at risk.
			warm, warmWalks := fmcMeasure(incremental.outer, availW, availH)
			if warmWalks != 1 || warm != before {
				t.Fatalf("the memo was not warm before the mutation (%d nodes walked, %+v != %+v)", warmWalks, warm, before)
			}

			tc.mutate(incremental)
			after, walks := fmcMeasure(incremental.outer, availW, availH)

			twin := ilpAutoBuild()
			tc.mutate(twin)
			want, _ := fmcMeasure(twin.outer, availW, availH)
			if after != want {
				t.Fatalf("the ancestor measurement was stale: %+v, want the cold one %+v", after, want)
			}
			if after != before {
				moved++
				if walks <= 1 {
					t.Fatalf("the mutation moved the measurement (%+v -> %+v) but the cached entry was served without recomputing (%d nodes walked)", before, after, walks)
				}
			}

			// The whole tree has to agree with the twin as well: a measurement
			// wrong in a way that does not move this particular key still moves
			// rects somewhere.
			ComputeLayout(incremental.root, Size{Width: availW, Height: availH}, true)
			ComputeLayout(twin.root, Size{Width: availW, Height: availH}, true)
			ilpCompareSnapshots(t, tc.name, ilpSnapshot(twin.root), ilpSnapshot(incremental.root))
		})
	}
	if moved < len(cases)/2 {
		t.Fatalf("only %d of %d cases changed the ancestor measurement; the stale-entry check was mostly vacuous", moved, len(cases))
	}
	t.Logf("%d of %d cases moved the measured ancestor", moved, len(cases))
}

// TestFlowMeasureCacheRootMeasuredOnlyOutsideFullscreen is the regression test
// for the invalidation signal. Gating the memo on Node.subtreeGeomDirty is
// unsound, because a layout pass clears that flag whenever it visits the node
// and the root is never measured in fullscreen mode: the mutation below sets the
// flag, the fullscreen pass clears it without replacing the entry, and the next
// natural-height pass finds a clean, stale entry. The epoch MarkDirty bumps is
// what makes the entry unusable regardless of who measured what in between.
func TestFlowMeasureCacheRootMeasuredOnlyOutsideFullscreen(t *testing.T) {
	build := func(rows int) *Node {
		children := make([]*Node, 0, rows)
		for i := 0; i < rows; i++ {
			children = append(children, Text(fmt.Sprintf("row %d", i)))
		}
		return Root(Box(Style{FlexDirection: Column}, children...))
	}
	viewport := Size{Width: 20}

	incremental := build(2)
	if got := ComputeLayout(incremental, viewport, false); got.Height != 2 {
		t.Fatalf("the first natural-height pass reports %+v, want height 2", got)
	}

	// A row is added: the root's auto height must grow.
	incremental.Children[0].Append(Text("row 2"))
	if !incremental.subtreeGeomDirty {
		t.Fatal("the mutation did not mark the root as holding a changed subtree")
	}

	// A fullscreen pass visits the root and never measures it: fullscreen uses
	// the viewport height instead of the natural one.
	ComputeLayout(incremental, Size{Width: 20, Height: 10}, true)
	if incremental.subtreeGeomDirty {
		t.Fatal("the fullscreen pass did not clear the root's subtree flag; the case no longer exercises the hole it pins")
	}

	got := ComputeLayout(incremental, viewport, false)
	twin := build(3)
	want := ComputeLayout(twin, viewport, false)
	if got != want {
		t.Fatalf("the natural-height pass served a stale root measurement: %+v, want the from-zero twin's %+v", got, want)
	}
	if got.Height != 3 {
		t.Fatalf("root height %d after adding a row, want 3", got.Height)
	}
}

// TestFlowMeasureCacheResizeWithoutMutation changes the available space without
// mutating anything. The key has to carry the available space, so a measurement
// taken under the old viewport must never be served for the new one, and coming
// back to the old one must restore exactly the old geometry.
func TestFlowMeasureCacheResizeWithoutMutation(t *testing.T) {
	incremental := ilpAutoBuild()
	small := Size{Width: 24, Height: 8}
	large := Size{Width: 60, Height: 20}

	// Two cold references: one tree laid out once at each viewport.
	twinSmall := ilpAutoBuild()
	ComputeLayout(twinSmall.root, small, true)
	twinLarge := ilpAutoBuild()
	ComputeLayout(twinLarge.root, large, true)

	ComputeLayout(incremental.root, small, true)
	ilpCompareSnapshots(t, "first pass at the small viewport", ilpSnapshot(twinSmall.root), ilpSnapshot(incremental.root))

	// Grow, shrink, grow, shrink: every entry reused on the way back has to be
	// exactly right, because nothing was mutated in between.
	for round := 0; round < 2; round++ {
		ComputeLayout(incremental.root, large, true)
		ilpCompareSnapshots(t, fmt.Sprintf("grow round %d", round), ilpSnapshot(twinLarge.root), ilpSnapshot(incremental.root))
		ComputeLayout(incremental.root, small, true)
		ilpCompareSnapshots(t, fmt.Sprintf("shrink round %d", round), ilpSnapshot(twinSmall.root), ilpSnapshot(incremental.root))
	}
}

// TestFlowMeasureCachePaintOnlyMutationKeepsEntries pins the other half of the
// contract: what cannot change a measurement must not drop the memo. TextStyle
// and the scroll state are not read by measureNode (nor by measureFlowContent),
// so a restyle or a scroll request has to leave the entries in place.
func TestFlowMeasureCachePaintOnlyMutationKeepsEntries(t *testing.T) {
	tree := ilpAutoBuild()
	ComputeLayout(tree.root, ilpViewports[0], true)

	before, _ := fmcMeasure(tree.outer, 40, 12)
	if _, walks := fmcMeasure(tree.outer, 40, 12); walks != 1 {
		t.Fatalf("the memo was not warm (%d nodes walked)", walks)
	}

	tree.leaf.SetTextStyle(TextStyle{Bold: true, Underline: true})
	tree.spare.ScrollBy(4)
	tree.leaf.ScrollToElement(tree.spare, 1)
	if tree.outer.subtreeGeomDirty {
		t.Fatal("a paint-only mutation marked the ancestor as geometrically dirty")
	}

	after, walks := fmcMeasure(tree.outer, 40, 12)
	if walks != 1 {
		t.Fatalf("a paint-only mutation dropped the memo: %d nodes walked, want 1", walks)
	}
	if after != before {
		t.Fatalf("a paint-only mutation changed the measurement: %+v, want %+v", after, before)
	}
}

// TestFlowMeasureCacheKeySetIsBounded measures what the cache is sized against:
// how many distinct available spaces a single container is measured under during
// one pass. Only containers use the table (Text and NodeRawANSI have their own
// memo), so this is the evidence behind nodeFlowMeasureSlots rather than a guess,
// and the test fails if a future layout change starts using more keys per pass
// than the cache holds.
func TestFlowMeasureCacheKeySetIsBounded(t *testing.T) {
	observed, worstLabel := 0, ""
	// measurePass runs one pass and folds its key-set evidence into the log.
	measurePass := func(label string, run func()) {
		max, worst, containers, maxText := fmcMeasureKeys(run)
		if max > observed {
			observed, worstLabel = max, label
		}
		t.Logf("%-42s containers measured=%3d max keys=%d (worst %s), text max=%d",
			label, containers, max, ilpPath(worst), maxText)
	}

	// The transcript shape of the PERF-001 benchmarks: a same-width footer update
	// in the steady state, in both layout modes.
	root, footer := benchTextUpdateTranscript(t, 200)
	for _, fullscreen := range []bool{true, false} {
		vp := Size{Width: 120, Height: 40}
		ComputeLayout(root, vp, fullscreen)
		for i := 0; i < 4; i++ {
			footer.SetText(benchTextUpdateSpinner(i))
			measurePass(fmt.Sprintf("transcript footer update fullscreen=%v", fullscreen),
				func() { ComputeLayout(root, vp, fullscreen) })
		}
	}

	// The auto-sized shape, one mutation per pass, alternating both layout modes
	// so the root's natural-height key is exercised too.
	tree := ilpAutoBuild()
	ComputeLayout(tree.root, ilpViewports[0], true)
	for i, op := range ilpAutoOps {
		op.apply(tree)
		vp := ilpViewports[i%len(ilpViewports)]
		fullscreen := i%2 == 0
		measurePass(fmt.Sprintf("auto op %d %s", i, op.name),
			func() { ComputeLayout(tree.root, vp, fullscreen) })
	}

	// A deeper transcript shape than the benchmark's: entries wrapped in a padded
	// row box inside a ScrollBox inside a column.
	deep, deepFooter := fmcDeepTranscript(40)
	for i := 0; i < 4; i++ {
		deepFooter.SetText(benchTextUpdateSpinner(i))
		measurePass("deep transcript shape", func() {
			ComputeLayout(deep, Size{Width: 100, Height: 30}, true)
		})
	}

	if observed > nodeFlowMeasureSlots {
		t.Fatalf("one pass measured a single container under %d available spaces (%s), more than the %d the cache holds: a key past the table can never be served",
			observed, worstLabel, nodeFlowMeasureSlots)
	}
	if observed < 2 {
		t.Fatalf("the widest container was measured under %d available spaces; the case no longer proves the cache needs more than one slot", observed)
	}
	t.Logf("observed maximum: %d distinct keys per container per pass (%s); nodeFlowMeasureSlots=%d", observed, worstLabel, nodeFlowMeasureSlots)
}

// fmcDeepTranscript builds a transcript with one more nesting level than the
// benchmark shape: each entry is a padded row box holding two text nodes.
func fmcDeepTranscript(entries int) (*Node, *Node) {
	rows := make([]*Node, 0, entries)
	for i := 0; i < entries; i++ {
		rows = append(rows, Box(Style{FlexDirection: Row, PaddingX: I(1)},
			Text(fmt.Sprintf("entry %02d body text that has to wrap around here", i)),
			Text("meta")))
	}
	box := ScrollBox(Style{FlexGrow: F(1), FlexShrink: F(1)}, false, rows...)
	footer := Text(benchTextUpdateSpinner(0))
	root := Root(Box(Style{FlexDirection: Column}, Text("header"), box, footer))
	return root, footer
}
