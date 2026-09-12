# inkgo v0.1.13 audit hardening checkpoint

Date: 2026-09-10
Status: production-candidate hardening snapshot; not yet promoted beyond upstream VERSION 0.1.13.

## Fixes applied during aggressive audit

- Runtime.SetRoot is reentrancy-safe: a nested root switch from focus/blur callbacks supersedes the stale outer switch.
- Close/Start lifecycle drops incomplete parser fragments and transient pointer/drag/multiclick state so state cannot leak across terminal ownership epochs.
- Stop is permanent: Start errors and ResumeTerminal is a no-op after Stop.
- Terminal enter/exit/raw/output restoration errors are propagated and joined; failed cleanup is retained and retried instead of silently discarded.
- Suspend/Resume completes pending terminal cleanup before re-entering modes.
- TerminalQuerier detects write errors and short writes, removes impossible pending waiters, and resolves them instead of blocking forever.
- Wide-cell fuzzer input stream is bounded to keep CI exploration effective.
- staticcheck dead helpers/unused assignments/inefficiencies were removed without public API changes.
- Regression tests added for lifecycle cleanup/retry, Stop semantics, reentrant SetRoot, terminal-query write failure, and related boundaries.

## Validation completed

- go test ./... : PASS
- go vet ./... : PASS
- staticcheck ./... : PASS
- go test -race ./... : PASS
- real PTY module with recovered local go-pty dependency: PASS
- real PTY module under -race: PASS
- PTY repeated stress (5 runs in prior audit continuation): PASS
- fuzzers: WrapText, ParserNeverPanics, ScreenWideCellInvariants, LayoutAndRenderInvariants, TreeMutationInvariants, ParseANSIInvariants: PASS
- cross compile/test-build: windows/amd64, windows/arm64, darwin/amd64, darwin/arm64, linux/arm64: PASS
- downstream reopencode integration: vendored source sync PASS; go test internal/cmd PASS; go vet PASS; TUI+CLI -race PASS

## Remaining external gates

- govulncheck could not be made authoritative in the sandbox because the vulnerability DB requires network access.
- Native physical-terminal smoke on Windows ConPTY and macOS terminal remains an external release gate; cross-builds are green but are not substitutes for real OS terminal behavior.


## Resize re-wrap performance (2026-09-11, v0.1.20)

Reported symptom: the TUI slows down noticeably after a long session, and every
drag-resize of a long transcript re-wraps all entries. Profiled on the transcript
shape a consumer holds - 200 wrapped 1024-byte ASCII entries inside a ScrollBox,
renderer 120x40, one width step - with `go tool pprof` over a resize loop.

What the profile showed (`/tmp/ndemo1/probe-fix`, pre-fix tree):

- `textutil.terminalTokens` 40.9% cumulative and `textutil.Graphemes` 35.9%: the
  wrap path segmented every line into grapheme tokens, and then
  `restoreVisualStateAcrossRows` segmented each produced row *again* to reopen the
  inherited terminal state - a pass that changes nothing when no row contains an
  escape sequence, which is the case for plain transcript text;
- `splitWrapWords` 29.1% and the allocator (`growslice`, `mallocgc`, `scanObject`)
  over 20%: a 1024-byte line built ~1030 grapheme tokens of 56 bytes plus a
  `strings.Builder` per cluster;
- `MeasureText` wrapped into a string, split it again and re-measured every row, so
  the layout pass re-derived what the wrap pass had just computed.

Fixes, all inside `internal/textutil` (no engine layout, viewport or API change):

- `restoreVisualStateAcrossRows` returns escape-free rows untouched;
- `hardWrapLine` dispatches printable-ASCII lines (bytes 0x20-0x7e only) to
  `wrapPlainASCIILine`, which walks bytes and slices the source line - no tokens, no
  per-grapheme builder, no state pass;
- `WrapText`, `WrapTextLines` and `MeasureText` share one row producer
  (`wrappedRows`), and `MeasureText` measures the rows it was given instead of
  re-splitting a joined string.

Measured before/after, same machine, both trees built from the same module
(`/tmp/inkgo-prefix` = `git archive HEAD`, i.e. 14107cc):

- resize render, 200 entries, 5 resizes averaged, three runs: 404.6 / 399.4 /
  398.8 ms before, 54.1 / 52.7 / 55.3 ms after (7.4x);
- `MeasureText` on a 1024-byte entry: 481-507 us before, 4 us after;
- renderer resize benchmark: 314 ms/op (2,595,239 allocs/op) before, 58 ms/op
  (155,370 allocs/op) after;
- warm render at an unchanged size, averaged over 10 renders: 32.4-40.3 ms (200
  entries) and 68.5-70.5 ms (400 entries) before, 29.7-33.0 ms and 60.0-66.6 ms
  after - i.e. no regression on the path that does not resize.

Correctness evidence: `TestWrapPlainASCIIMatchesTokenPath` (differential, byte
identical rows against the token path over 8,400 targeted and randomized inputs),
`TestWrapPlainASCIIRowsAreSourceRanges`, `TestMeasureTextMatchesWrapDerivedRows`
and `TestWrapTextLinesMatchesLegacyDerivation` (against the previous
implementations, over ASCII/ANSI/tab/CRLF/CJK/emoji corpora and every wrap mode),
plus `TestRestoreVisualStateSkipsEscapeFreeRows` for the state-pass fast path.
`FuzzWrapTextInvariants` ran 30 s (619,103 execs) without a failure.

Gate status: `go test ./...`, `go vet ./...`, `go test -race ./...` PASS;
`scripts/check-version.sh` (0.1.20), `scripts/check-manifest.sh` and
`scripts/check-no-external-deps.sh` PASS; `go generate ./...` leaves `api.go`
untouched. `staticcheck` was not run (not installed on the validation host).
