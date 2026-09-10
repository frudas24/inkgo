# Post-v0.1.10 follow-up audit

## Scope

This pass starts from the published `v0.1.10` source archive and audits only
the new PTY/ConPTY and Unicode changes plus their immediate invariants. The root
module remains stdlib-only; `test/pty` remains a separate test-only module.

## Findings closed

### Text-default pictographic ZWJ chains could still exceed two cells

`v0.1.9` correctly stopped non-pictographic bases such as digits from joining
through ZWJ, but valid GB11 chains composed of text-default pictographs could
still accumulate one cell per pictograph. For example `☀‍☀‍☀` was one grapheme
with width 3, so wrapping at width 2 could not split it and violated the row
width invariant.

The clusterer now records a successful pictographic GB11 join and caps that
cluster to the framebuffer's two-cell grapheme model, while explicit VS15 text
presentation remains authoritative and ordinary digit-ZWJ runs remain split.
Regression coverage includes `☀`, `♥`, `⚠`, `©`, normal emoji families and
skin-tone ZWJ sequences. `FuzzWrapTextInvariants` now asserts directly that
every grapheme width is in the representable range 0..2.

### PTY Wait could observe process exit before final terminal bytes

Under the race detector, `cmd.Wait()` could win before the asynchronous PTY
reader consumed the runtime's final mode-restoration bytes. Tests that called
`Wait()` and then inspected `Output()` could therefore report a missing
`?1049l` even though the child had emitted it.

`Session.Wait` now waits for the reader to become quiescent for a short bounded
period after process exit. EOF is still not required because some ConPTY
backends keep the PTY read side alive until the parent handle closes. The
formerly flaky Ctrl+C restoration scenario passed 100/100 under `-race`; the
full PTY suite passed `-race -count=20 -shuffle=on` against the supplied modern
Go-1.23-compatible go-pty core.

### Startup SIGWINCH ordering now has a discriminating regression

The black-box immediate-resize PTY scenario can succeed even with the old
`Start -> install signal handlers` order because the PTY may be resized before
the child reads its initial geometry. A new Linux regression blocks the first
terminal write made by `Start`, delivers SIGWINCH in that exact window and
requires resize work to be queued.

Ablation result:

- current `install handlers -> Start` ordering: 100/100 pass;
- old `Start -> install handlers` ordering: deterministic failure.

This converts the startup reorder from plausible defensive hardening into a
proved regression boundary.


### Emoji properties are now exact Unicode 17.0 data

The clusterer previously used a coarse approximation for emoji candidates and
emoji-default presentation. That approximation was adequate for common emoji
but had two bad edges: text-default emoji outside the broad symbol ranges (for
example `©`, `®` and `™`) did not consistently react to VS16, while unrelated
code points inside broad emoji blocks could be treated as emoji by default.
The first exact `Extended_Pictographic` pass also exposed that
`Regional_Indicator` must stay under GB12/GB13-style pairing rather than being
reused as a GB11 base.

The implementation now ships compact sorted range tables from Unicode Emoji
17.0 `emoji-data.txt`:

- `Emoji`: 1,438 code points in 151 ranges;
- `Emoji_Presentation`: 1,219 code points in 81 ranges;
- `Extended_Pictographic`: 2,848 code points in 156 ranges.

Tests verify those counts, range ordering/non-overlap, the invariant
`Emoji_Presentation ⊆ Emoji`, representative text/default presentation cases,
Regional Indicator behavior, and all seven Emoji 17 additions (`U+1F6D8`,
`U+1FA8A`, `U+1FA8E`, `U+1FAC8`, `U+1FACD`, `U+1FAEA`, `U+1FAEF`). A non-Emoji
symbol such as `⌘` remains narrow even when followed by VS16. The fuzz target
continues to enforce that every terminal grapheme width is representable in
0..2 cells.

### Width hot paths recovered after exact tables

Exact property lookups add a small cost to cluster-sensitive Unicode. Rather
than weaken the tables, `StringWidth` now mirrors the source fork's fast-path
shape: pure ASCII is counted directly; ANSI-colored ASCII is counted directly
after stripping controls; and Unicode without ZWJ, VS15/VS16, keycap, Regional
Indicator or emoji-modifier semantics sums rune widths without materializing
graphemes. Malformed UTF-8 still falls back to the sanitizing grapheme path.

Representative measurements on the audit host:

| input | v0.1.10 baseline | follow-up |
| --- | ---: | ---: |
| ASCII sentence | ~5.1 µs, 55 allocs | ~42 ns, 0 allocs |
| ordinary mixed Unicode | ~3.2 µs, 33 allocs | ~0.9–1.0 µs, 0 allocs |
| ANSI-colored ASCII | ~5.4 µs, 56 allocs | ~0.34 µs, 1 alloc |
| cluster-sensitive emoji | ~2.4 µs, 16 allocs | ~2.5 µs, 16 allocs |

The small remaining cost on complex emoji is deliberate: those strings use the
full cluster model. Permanent benchmarks cover the width paths, and
`FuzzWrapTextInvariants` cross-checks the simple Unicode result against the full
grapheme result whenever the optimization declares an input safe.

## Supplied go-pty bundle

The supplied reference clone is upstream commit
`da4e5b0c5d98251e5432e8f18e460bee673d2efe` with its dependency graph pinned
to Go-1.23-compatible versions and a generated vendor tree. On Go 1.23.2 with
`GOPROXY=off`, `go test ./...` and `go vet ./...` pass, and the package
cross-compiles for Linux amd64, Windows amd64/arm64 and Darwin amd64/arm64.
The published `test/pty` module intentionally keeps its public upstream pin at
`bef704d47d97`; the only Go-code delta to current upstream is the newer Windows
non-zero-exit `*exec.ExitError` materialization, which the harness already
normalizes.

## Final validation snapshot

The completed follow-up was revalidated after the exact Unicode tables and
width fast paths were in place:

- `go generate ./...`: pass, with byte-identical generated `api.go`;
- `go test ./...`: pass;
- `go vet ./...`: pass;
- `go test -race ./...`: pass;
- total statement coverage: **82.1%** against the **80%** CI floor;
- `internal/engine`: 80.3%; `internal/textutil`: 92.1%;
  `internal/inputparser`: 92.4%; `scheduler`: 89.0%;
- root suite `-count=10 -shuffle=on`: pass;
- scheduler `-count=500`: pass;
- discriminating startup-SIGWINCH regression `-count=100`: pass;
- external consumer using only public packages: `go test` and `go vet` pass;
- PTY suite: 30 shuffled repetitions pass; 10 shuffled repetitions under
  the race detector pass against the supplied offline-compatible PTY core;
- engine/text and PTY harness cross-build for Linux amd64, Windows amd64/arm64
  and Darwin amd64/arm64: pass;
- all five permanent fuzz domains completed the final audit campaign without
  a failure.

The source inventory at this point contains 159 root-module `Test*` functions,
15 PTY-module `Test*` functions, five fuzz targets and nine benchmarks. The
root module remains stdlib-only; all PTY dependencies remain isolated under the
separate `test/pty` module.
