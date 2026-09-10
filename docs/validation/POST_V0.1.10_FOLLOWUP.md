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
