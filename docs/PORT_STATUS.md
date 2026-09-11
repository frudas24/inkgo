# Port status

## Practical status

The Go port is approximately **99% functionally equivalent for practical terminal-UI use**. Round 5 deliberately prioritized production behavior, packaging and maintainability over chasing implementation identity with JavaScript/React/Yoga internals.

The project is stdlib-only, CGO-free, importable as `github.com/frudas24/inkgo`, and `v0.1.17` is publicly tagged. Development checkouts may contain additional `Unreleased` hardening.

## Closed in Round 5

- canonical module/repository import path;
- implementation removed from repository root into `internal/engine`;
- generated root facade with drift check;
- package-local/internal test attribution and 75.0% total statement coverage;
- Linux/macOS/Windows CI execution plan plus race/fuzz/coverage/dependency gates;
- real-console manual terminal smoke target;
- 10,000-row warm-scroll scaling via layout reuse and visible-range painting;
- bounded renderer scroll history across repeated root replacement;
- focus-manager disabled traversal contract bug.

## v0.1.7 through v0.1.15 hardening (post-v0.1.6)

`v0.1.7` replaces Windows console-size polling with native `ReadConsoleInputW` ownership inside `Runtime.Run`. Resize now arrives as `WINDOW_BUFFER_SIZE_EVENT`; key/mouse/focus records are preserved through the same input owner, and the blocking pump is stopped by a kernel event rather than a periodic timer.

`v0.1.7` also removes remaining repeated full-tree ScrollBox discovery from hot xterm.js/selection/wheel paths, replaces a throughput-sensitive scheduler assertion with a synchronization-based non-reentrancy regression, and expands CI to the minimum supported Go line plus current stable Go across all three runner OSes.

`v0.1.8` closes Windows combined-modifier/wheel/shutdown-priority bugs and substantially tightens ANSI/Unicode text semantics: output escape scanning is now distinct from input sequence parsing, ANSI controls are atomic during wrap/slice/truncate, SGR/OSC-8 state is safely closed/reopened across boundaries, invalid UTF-8/control grapheme handling is consistent, and text-default emoji/keycap/VS16 widths are covered by regressions. Five permanent fuzz domains now cover input parsing, screen wide-cell invariants, layout/render, text wrapping/slicing/truncation and ANSI parsing. Total statement coverage is above 81% on the validation host, with an 80% CI floor. See `validation/WINDOWS_NATIVE_INPUT.md` and `validation/POST_V0.1.7_DEEP_AUDIT.md`.

`v0.1.9` adds an external-observer PTY/ConPTY suite in the separate `test/pty` module, and fixes a grapheme-segmentation bug the new wrapping fuzz target found: a ZWJ chain was joined without the UAX #29 GB11 pictographic base, so a digit ZWJ run fused into a three-cell cluster that could not be wrapped and produced rows wider than the requested width. It closes a narrow Unix startup window: `SIGWINCH` handlers were installed after the initial render, so a resize landing in that gap could stay invisible until unrelated input caused another render. `Runtime.Run` now installs signal handlers before `Start`. The suite's immediate-resize test does not discriminate that reorder on its own because the harness may resize the PTY before the fixture reads its initial geometry. A dedicated internal Unix regression now blocks the first `Start` write and injects `SIGWINCH` in that exact window; reverting to the old ordering makes the regression fail, while the current ordering passes repeated runs. The PTY harness also drains final output to quiescence after process exit so restoration assertions cannot race the reader goroutine. The suite's value remains lifecycle, input and terminal-restoration coverage from outside the process, with no dependency added to the root module. See `validation/PTY_INTEGRATION.md`.

`v0.1.11` removes the last coarse emoji-property approximations from width/GB11 decisions. `Emoji`, `Emoji_Presentation` and `Extended_Pictographic` now use compact Unicode Emoji 17.0 property ranges, while Regional Indicators remain governed by their separate pairing rule. This closes false GB11 joins for non-pictographic symbols, fixes VS16 width for text-default emoji such as `©`/`®`/`™`, covers the seven Emoji 17 additions and directly asserts table consistency plus the 0..2-cell grapheme contract. Width measurement also regains allocation-free fast paths for ASCII and ordinary non-cluster-sensitive Unicode. Final validation reports 82.1% total statement coverage (92.1% in `internal/textutil`), green race/repetition/fuzz/PTY campaigns and five-target cross-builds. See `validation/POST_V0.1.10_FOLLOWUP.md`.

`v0.1.12` makes the remaining terminal-facing sequence decisions exact rather than property-based: FE0E/FE0F only affect width when adjacent to one of the 371 Unicode Emoji 17.0 variation bases, keycaps must have the exact `[0-9#*] FE0F? U+20E3` shape, and a GB11 join is cancelled by anything inserted after the ZWJ before the target pictograph. The focused audit raised measured coverage to 82.1% overall and 92.4% in `internal/textutil`, with all five fuzz targets, race/repetition, offline PTY stress, external-consumer smoke and five-target cross-builds green. See `validation/POST_V0.1.11_UNICODE_SEQUENCE_AUDIT.md`.

`v0.1.13` closes ownership and lifecycle gaps the earlier audits left behind. Tree mutation snapshots incoming child lists before reparenting, so passing another node's `Children` slice no longer skips elements, and it rejects nil, duplicate and ancestor-cycle entries. Focus transitions publish state before callbacks and use a revision counter so a nested focus/blur supersedes the outer one, programmatic focus outside the managed tree or under a hidden ancestor is refused, and input no longer routes to a detached or hidden focused node. Expired-button render callbacks run before the renderer's history lock, which removes a real deadlock when a callback inspects the viewport or changes its size. A wide-cell write at the right edge is rejected before it clears the existing pair, truncation to one column preserves text that already fits, and a runtime stopped by a node handler no longer delivers the global paste callback. Coverage reached 82.2% with a sixth permanent fuzz target, `FuzzTreeMutationInvariants`, added to CI.

`v0.1.14` makes `Stop` explicitly terminal: starting a runtime that has been stopped now returns an error rather than silently re-entering, while `Close` still only cycles terminal modes and may be followed by another `Start`. `Renderer.WriteFrame` also rejects a nil writer with a typed error instead of dereferencing it, and `LICENSE`/`THIRD_PARTY_NOTICES.md` now record the MIT terms and the upstream Ink attribution for the derived portions.

`v0.1.17` makes tab expansion linear: it segmented the remainder of each plain span once per grapheme, so wrapping a single long line cost O(n^2) (32 KB about 64 s before, 40 ms after). The visible output is unchanged (verified by a 20,000-input differential comparison against the previous implementation), and the affected fuzz target now completes a 60 s run where it previously killed the worker.

`v0.1.16` fixes an ANSI-in-grapheme tokenizer bug: output escapes were stripped after grapheme segmentation, so an ANSI or control sequence between a base rune and its combining rune split the cluster and could widen a wrapped row beyond the requested width. Tokenization now normalizes controls first, segments the visible stream, and attaches the preserved sequences to their tokens; slicing still observes their terminal-state effects. The audit reproducer and a permanent fuzz seed are kept as regressions.

`v0.1.15` hardens the runtime lifecycle boundary. `Start` now fails when a configured terminal input cannot enter raw mode instead of publishing false terminal ownership, terminal cleanup is retryable after transient exit/raw/output-restore failures with the pending exit sequence retained, `SuspendTerminal` keeps incomplete cleanup instead of discarding it, terminal-query waiters resolve as soon as a query or sentinel write fails or short-writes, and `Close` resets incomplete parser and transient pointer state so a later `Start` begins from a clean epoch. Failures from `Close`, failed `Start` cleanup and the deferred cleanup in `Run` are joined and propagated rather than abandoned. A seventh permanent fuzz target, `FuzzRuntimeLifecycleInvariants`, drives `Start`/`Close`/`SuspendTerminal`/`ResumeTerminal`/`Stop` under transient output failures. The same release fixes a wrap width-invariant violation in trim mode where an orphaned variation selector could widen a row beyond its computed width.

## Deliberately remaining parity boundary

The final ~1% is dominated by low-ROI edge identity rather than missing everyday capabilities:

- full Unicode UAX #9 behavior for pathological nested bidi embeddings/isolates;
- microscopic Yoga identity on obscure flexbox edge combinations not observed in typical application fixtures;
- terminal-emulator-specific legacy quirks requiring real-host fixtures;
- byte-for-byte parity with the original JavaScript implementation where Go-native behavior is already semantically equivalent.

No vendor should be added merely to erase this percentage. Add complexity only when a real fixture demonstrates a user-visible mismatch.

## Release boundary

The latest published tag is `v0.1.15`. New changes should remain under `Unreleased` until the next validated tag is created. Repository description/topics are GitHub metadata and are not part of source archives. See `RELEASE.md`.
