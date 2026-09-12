# Changelog

## Unreleased

- bound the two tab/wrap performance regressions by a deadline instead of checking elapsed time after the call returns: on the quadratic implementation they now fail within 30 s each with a clear diagnostic, where they previously blocked for roughly 75 minutes before reporting (or tripped a CI job timeout);
- advance the release-checklist tag example to the next version, which had been left pointing at an already-published tag while the header named a newer one;

## v0.1.18 — WSL clipboard bridge

- the native clipboard path now reaches the Windows clipboard when running under WSL: `clip.exe` and a `powershell.exe` stdin fallback (`Set-Clipboard -Value ([Console]::In.ReadToEnd())`) are appended to the Linux candidate list, after `wl-copy`/`xclip`/`xsel` so a real X11/Wayland tool still wins;
- the reported path matches the behaviour: with only the Windows utilities on PATH, `GetClipboardPath()` now answers `native` instead of falling back to OSC 52, which Windows Terminal may ignore - a copy used to report success while nothing reached the clipboard;
- the native path stays refused across SSH, bridge included, so it never mutates the clipboard of the machine the user is sitting at;
- both the text candidates and the detection share one list, so they cannot drift apart;

## v0.1.17 — linear tab expansion (wrap performance)

- fix the quadratic cost of tab expansion: `ExpandTabs` segmented the rest of the plain span once per grapheme and kept only the first cluster, so a single long line was scanned once per grapheme. Measured on the validation host before the fix: 1 KB 65 ms, 8 KB 3.9 s, 32 KB 64 s, and 512 KB would have run for hours; after the fix the same inputs take 1.2 ms, 10.9 ms, 40 ms and 0.64 s, i.e. linear in the input size;
- the visible output is unchanged: a differential harness compared the new walk against the previous implementation over 20,000 randomized inputs (tabs, SGR/OSC escapes, CR/LF, combining marks, variation selectors, keycaps, wide CJK and emoji) with byte-identical results;
- consequence for fuzzing: `FuzzWrapTextInvariants` now completes a 60 s run, where mutation growth past roughly 10 KB previously killed the worker as hung; the five-second CI smoke was already inside that bound;
- add two permanent regression tests that fail the quadratic implementation (a 256 KB tab span and a 128 KB ANSI line) with bounds two orders of magnitude above the linear cost, plus the differential check's corpus in the existing wrap tests;
- no public API change; `go generate` leaves `api.go` untouched;

## v0.1.16 — ANSI-in-grapheme tokenizer fix

- tokenize output text after normalizing ANSI/control sequences instead of before: an escape sitting between a base rune and its combining rune (for example `0\x1b[31m\u20e3`, or SGR/OSC inside a keycap or family sequence) does not create a Unicode grapheme boundary, but segmenting first split the cluster and could produce a row wider than the requested wrap width;
- record preserved ANSI sequences by their position in the visible stream and attach them to the grapheme tokens they belong to, so wrapping keeps the visual cluster while slicing and terminal-state restoration still observe the escape effects - including escapes inside a grapheme that falls outside the requested slice range;
- keep the audit reproducer (SGR/OSC between base and combining rune) plus a permanent fuzz seed as regressions;

## v0.1.15 — runtime lifecycle hardening and a wrap trim fix

- fix a wrap width-invariant violation in trim mode: the tokenizer splits a literal space away from the zero-width runes sharing its grapheme cluster, and dropping only the space left the orphan rune to re-attach to the preceding base, so a trailing VS16 on a variation base widened a row beyond the width the token accounting had produced; trimming a trailing space now drops its attached zero-width runes too, with the fuzz reproducer committed as a permanent seed;
- fail `Runtime.Start` when a configured terminal input cannot enter raw mode instead of publishing false terminal ownership; `ResumeTerminal` now fails closed on the same boundary;
- make terminal cleanup retryable after transient exit/raw/output-restore failures, preserving the exact pending exit sequence and completing old cleanup before a later `Start`/`ResumeTerminal` re-enters modes;
- make `SuspendTerminal` retain incomplete cleanup instead of silently discarding it, so resume cannot stack a new alternate-screen/raw-mode transition over a failed suspension;
- resolve terminal-query waiters immediately when query/sentinel writes fail or short-write, instead of leaving impossible pending requests until shutdown;
- add a lifecycle state-machine fuzzer covering `Start`/`Close`/`SuspendTerminal`/`ResumeTerminal`/`Stop` under transient output failures;
- make `Runtime.SetRoot` reentrancy-safe at the runtime layer: a nested root transition triggered by focus/blur callbacks now supersedes the older outer transition instead of being overwritten when the callback returns;
- make `Close` a clean reusable lifecycle boundary by discarding incomplete parser fragments and transient pointer/drag/multi-click state before a later `Start`;
- make `Stop` a permanent runtime termination boundary: `Start` returns an error and `ResumeTerminal` is a no-op after stop instead of re-entering terminal modes into an inert runtime;
- propagate terminal-exit/raw-mode/output-restore failures from `Close`, from failed `Start` cleanup, and from the deferred cleanup performed by `Run`; cleanup remains best-effort and joins multiple failures rather than abandoning later restoration steps;
- bound the wide-cell state-machine fuzzer to 256 mutations per input so CI keeps exploring distinct framebuffer states instead of spending a fuzz window replaying oversized mutation streams;
- remove dead helpers/assignments reported by `staticcheck` without changing public API behavior;

## v0.1.14 — terminal Stop contract, nil-writer guard and MIT attribution

- make `Stop` explicitly terminal for a runtime: `Start` on a runtime that has been stopped now returns an error instead of silently re-entering, while `Close` still only cycles terminal modes and may be followed by another `Start`;
- reject a nil writer in `Renderer.WriteFrame` with an error instead of dereferencing it;
- record the MIT license and the upstream Ink attribution for the derived portions in `LICENSE` and `THIRD_PARTY_NOTICES.md`, replacing the previous outstanding-clarification note;

## v0.1.13 — tree ownership, reentrant focus and input lifecycle

- skip runtime keyboard default actions and the global paste callback when a node handler stops the runtime;
- snapshot child lists before reparenting to avoid skipping nodes when the input aliases another parent's children; reject duplicates and ancestor cycles through the mutation API;
- keep focus transitions consistent when focus/blur callbacks redirect focus, disable the manager or replace its root; publish button focus state before callbacks and let nested transitions supersede older ones;
- stop routing keyboard and paste to focused nodes that have been detached or hidden;
- run expired-button render callbacks before locking renderer history so callbacks can query viewport/visibility or change dimensions without deadlocking;
- add adversarial tree/focus/renderer regressions and a permanent tree-mutation fuzzer to CI.

- reject a wide-cell write at the right edge before mutating the existing glyph, preventing orphan spacer tails;
- preserve fitting text (including empty strings, combining marks and ANSI-styled text) when truncating to one column;
- reject programmatic focus outside the managed tree or under hidden ancestors; autofocus selects the first visible eligible node;
- make wide-cell fuzz operations independent of their coordinates and validate invariants immediately after each mutation, covering previously unreachable even-column wide glyphs.

## v0.1.12 — exact GB11, keycap and variation-selector sequences

- make emoji presentation selectors sequence-aware instead of treating `Emoji` as a proxy for a registered variation sequence: add the exact Unicode Emoji 17.0 set of 371 variation bases (183 compact ranges), require FE0E/FE0F to be adjacent to such a base, and preserve the base/default width for displaced or unsupported selectors such as `©\u0301\uFE0F`, `⌚\u0301\uFE0E` and `😀\uFE0E`;
- require keycaps to match the terminal-relevant Unicode shape `[0-9#*] FE0F? U+20E3`; intervening combining marks, VS15 and duplicate VS16 no longer turn a merely keycap-like cluster into a two-cell keycap;
- tighten GB11 state so the join opportunity created by `Extended_Pictographic Extend* ZWJ` is consumed only by the immediately following `Extended_Pictographic`; an Extend, variation selector, modifier or second ZWJ after the ZWJ cancels that opportunity, while valid families and skin-tone ZWJ sequences remain one two-cell grapheme;

## v0.1.11 — Unicode 17 emoji properties and width hot paths

- replace the remaining coarse emoji property heuristics with compact Unicode Emoji 17.0 tables for `Emoji`, `Emoji_Presentation` and `Extended_Pictographic`; this fixes VS16 presentation for text-default emoji such as `©`, `®` and `™`, keeps non-Emoji symbols such as `⌘` narrow, separates Regional_Indicator pairing from GB11, and covers all seven Emoji 17 additions while preserving the framebuffer invariant that one grapheme occupies at most two cells;
- restore the source fork's width hot paths without weakening Unicode semantics: pure ASCII now bypasses grapheme allocation entirely, ANSI-colored ASCII re-enters that fast path after stripping controls, and ordinary Unicode without cluster-sensitive ZWJ/VS/keycap/RI/modifier code points sums rune widths directly; validation measured ASCII at ~42 ns/0 allocs versus ~5.1 µs/55 allocs before the optimization, and ordinary mixed Unicode at ~0.9–1.0 µs/0 allocs versus ~3.2 µs/33 allocs; add permanent text-width benchmarks and fuzz-check that the simple path equals the full grapheme model whenever it is selected;
- close the remaining ZWJ width-invariant hole for text-default Extended_Pictographic chains: valid GB11 sequences such as `☀‍☀‍☀`, `♥‍♥‍♥` and `⚠‍⚠‍⚠` remain one grapheme but are capped to the framebuffer's two-cell grapheme model instead of accumulating width 3; add direct regressions and make the fuzz target assert that every emitted grapheme width stays within 0..2;
- make the PTY harness drain final output after process exit until the reader is quiescent (bounded), so `Wait()` cannot race the read goroutine and falsely report missing alternate-screen/mode restoration under `-race`;
- add a discriminating Unix regression for the SIGWINCH-before-Start ordering: the test blocks the first terminal write, delivers SIGWINCH in the exact startup window and proves by ablation that the old ordering loses it while the current ordering queues it;

## v0.1.10 — PTY CI corrections

- make two PTY assertions platform-aware: ConPTY owns host focus reporting and does not surface the application's `?1004l` on detach, and it tears the console down before a panicking process flushes stderr, so the focus-restore sequence and the panic sentinel are asserted on Unix only; terminal restoration is still asserted everywhere;
- fix the PTY module completeness guard: `go mod tidy -diff` compares `go.sum` byte for byte, so a Windows checkout with `core.autocrlf=true` reported every line as changed and the step failed on line endings alone; the step now runs `go mod tidy` and lets git compare, which normalises EOL while still detecting real module drift;

## v0.1.9 — real PTY/ConPTY integration and ZWJ cluster fix

- stop ZWJ from joining clusters unless UAX #29 GB11 applies, i.e. an Extended_Pictographic base before it; ASCII digits, `#` and `*` are emoji candidates but not pictographic, so a digit ZWJ run fused into a single cluster three cells wide, which exceeded the two-cell model and could not be wrapped, leaving rows wider than the requested width; real emoji ZWJ sequences, including ones with a skin-tone modifier between base and ZWJ, still collapse to one cluster;
- add a separate `test/pty` Go module powered by `github.com/aymanbagabas/go-pty` for real Unix PTY / Windows ConPTY end-to-end tests without adding dependencies to the published `inkgo` module;
- cover alternate-screen entry/exit, keyboard, Unicode, bracketed paste across multiple reads, resize without keyboard wakeup, immediate startup resize, resize bursts, Escape timeout, Ctrl+C raw input, panic unwinding, repeated sessions, focus/mouse where deterministically injectable, and terminal-mode restoration from outside the child process;
- fix a Unix startup race where `Runtime.Run` installed `SIGWINCH` handling only after `Start` performed its initial render, allowing an immediate post-start resize to be lost until unrelated input arrived; signal handlers are now installed before terminal entry/first paint;
- add cross-platform PTY CI on Ubuntu/macOS/Windows plus a Linux PTY race campaign, while keeping the root module stdlib-only and dependency-free.

## v0.1.8 — deep-audit hardening: Windows input, ANSI and Unicode

- harden Windows native-console key translation for combined Ctrl/Shift/Alt modifiers and preserve repeat semantics across reconstructed VT/CSI-u input;
- preserve the magnitude and remainder of Windows mouse-wheel deltas instead of collapsing multi-notch or partial wheel records to a single step;
- prioritize the native Windows stop event ahead of console readiness so shutdown cannot consume pending input intended for the next shell/application;
- split input-sequence scanning from output-ANSI scanning so text wrapping, slicing and rendering no longer classify arbitrary Alt/meta input as terminal-output control sequences;
- make ANSI-aware wrapping treat CSI/OSC/DCS/APC/PM/ESC controls as atomic zero-width units, preserve tab-stop semantics, and close/reopen SGR and OSC-8 state at soft-wrap boundaries;
- harden width/slice/truncate behavior for invalid UTF-8, control boundaries, text-default emoji, VS16 presentation, keycaps, regional indicators and ZWJ clusters;
- prevent sliced/truncated ANSI text from leaking SGR or OSC-8 state, and avoid replaying non-style controls that appeared before a slice boundary;
- unify `ParseANSI`, `StripANSI` and the output escape scanner for generic DEC escape families such as `ESC ( 0`, PM/DCS/APC, incomplete sequences and control-character boundaries;
- add `FuzzWrapTextInvariants` and `FuzzParseANSIInvariants`, bringing the permanent fuzz surface to five domains, and raise the CI coverage floor from 77% to 80%.

## v0.1.7 — native Windows console input and hot-path hardening

- the `terminal-smoke` example echoes every key it receives (count, name, text, sequence, modifiers), so an unresponsive-looking demo can be told apart from a genuinely broken input path;
- replace Windows' 60 ms console-size polling with an event-driven `ReadConsoleInputW` pump owned exclusively by `Runtime.Run`; `WINDOW_BUFFER_SIZE_EVENT` now feeds the existing coalesced resize queue directly, while `WaitForMultipleObjects` waits on console input plus a stop event with no periodic timer;
- preserve the full Windows console input stream while taking native ownership: `KEY_EVENT`, `MOUSE_EVENT` and `FOCUS_EVENT` records are translated into the existing parser-compatible VT stream, including UTF-16 surrogate pairs, AltGr, repeat counts and viewport-relative mouse coordinates;
- extend legacy xterm modifier parsing for Insert/Delete/PageUp/PageDown and F1-F12 so native Windows special keys retain Shift/Alt/Ctrl semantics;
- request `ENABLE_WINDOW_INPUT`/`ENABLE_MOUSE_INPUT` in Windows raw mode and disable Quick Edit while the runtime owns the console, restoring the exact prior mode on exit;
- remove repeated full-tree scroll-node walks from the xterm.js policy, selection-scroll reconciliation and wheel fallback by reusing the layout-owned scroll-node index on stable trees, while retaining dirty-layout discovery for correctness;
- replace the slow-subscriber scheduler regression's throughput-sensitive 100 ms assertion with a synchronization-based non-reentrancy test;
- test the minimum supported Go line (`1.23.x`) and current `stable` Go across Linux, macOS and Windows in CI; run race, coverage and fuzz smoke on stable and raise the coverage gate from 74% to 77%;
- refresh release documentation for the already-published `v0.1.6` tag and keep the customized-fork provenance clarification explicit.

## v0.1.6 — Windows-flaky clock assertion fix

- fix a Windows-only flaky assertion in `TestNowUsesTickTimestampOnlyDuringCallbacks`: the time since clock start can still read as zero immediately after a tick, which made a live `Now()` indistinguishable from a frozen one; the assertion now sleeps past the clock resolution and no longer fails the Windows CI job. No library behavior changed.

## v0.1.5 — resize recovery and interactive smoke

- resize no longer re-enters or clears the alternate screen on every `SIGWINCH`; the runtime coalesces signal bursts into one UI event and re-reads the PTY size over a bounded retry window, so a size that lags the signal (ConPTY/WSL) is picked up without another key press or input;
- Windows consoles deliver no `SIGWINCH`, so the runtime now polls the console size while it owns the loop and refreshes on change instead of waiting for the next keypress;
- the `terminal-smoke` example exits on `q`, Ctrl+C or Escape and shows pasted text, making the interactive smoke escapable and paste observable;
- the `terminal-smoke` example renders a visible focus indicator plus a focus readout across three focusable buttons, so the `Tab`/`Shift+Tab` traversal direction is observable rather than silent (with two buttons both keys land on the same node);

## v0.1.4 — runtime, terminal and clock boundary hardening

- invalidate replaced clock timers to prevent overlapping callbacks; keep `Now()` consistent within an active tick;
- retry failed output through both `Renderer.WriteFrame` and `Runtime.Render`;
- leave paste mode when flushing an empty incomplete paste, preserving subsequent keyboard input;
- reject terminal controls and invalid UTF-8 in emitted hyperlink URLs while preserving Unicode links;
- stop dispatching the current input batch immediately after `Stop`;
- match terminal-query responses only within the batch before the first pending barrier;
- release lifecycle locks before resume callbacks; repaint after reopening and after failed frame writes;
- flush pending Escape and partial-paste input on EOF;
- validate manifest inventory as well as hashes, including new non-ignored files;
- align release documentation with 0.1.3 and document its embedded-loop API changes.

## v0.1.3 — runtime event-loop race fix

- signal handlers (SIGWINCH/SIGCONT) and timer callbacks no longer touch the renderer from their own goroutines: runtime work is enqueued and dispatched by the UI owner (`enqueueEvent` / `ProcessEvents`), so `Run` and embedded loops drain it on the calling goroutine;
- `Stop` is idempotent and event dispatch closes cleanly;
- link/incomplete-sequence generations invalidate stale timer callbacks;
- added a regression test that timeout callbacks (escape/paste) run on the UI owner;
- added public `Events()` and `ProcessEvents()` methods; embedded UI loops must service these events for timeout and delayed-link callbacks; zero external modules.

## v0.1.2 — release hygiene

- fixed public release documentation that still pointed consumers at `v0.1.0` after `v0.1.1` was published;
- fixed `MANIFEST.sha256` generation so ignored/generated `coverage.out` is never listed in a clean source manifest;
- added reproducible manifest update/check scripts and a CI policy gate so a manifest cannot reference files absent from a clean checkout;
- retained zero external Go modules and made no runtime/API behavior changes.

## v0.1.1 — manifest portability

- fixed `MANIFEST.sha256` so clean checkouts no longer reference `.git/*` VCS internals; published as a new immutable tag because `v0.1.0` had already been cached by the Go module proxy.

## v0.1.0 — first public Go release

- published the first tagged `github.com/frudas24/inkgo` release from the Round-5 production-hardening baseline.

## Round 5 — 2026-09-09

- changed the canonical module path to `github.com/frudas24/inkgo` and prepared source version `0.1.0`;
- moved stateful implementation out of the repository root into `internal/engine`; root is now a generated compatibility facade;
- added reproducible `go generate` tooling for `api.go` plus generator-drift CI enforcement;
- moved detailed project documents under `docs/` and cross-package smoke tests under `integration/`;
- added package-local tests across every public domain plus direct tests for internal leaf packages; total measured statement coverage reached 75.0%;
- added GitHub Actions CI executing tests/vet/builds on Linux, macOS and Windows, plus Linux race, coverage gate, dependency/version policy and fuzz smoke jobs;
- added a manual real-console `examples/terminal-smoke` for raw/VT lifecycle validation, especially Windows Terminal/PowerShell;
- split layout-dirty from paint/scroll-dirty state and added stable-layout reuse;
- optimized certified linear vertical ScrollBox painting to binary-search and render only the visible child range;
- reduced a 10,000-row warm-scroll benchmark from the pre-optimization ~122 ms/frame class to ~0.62 ms/frame on the validation host while keeping first materialized layout O(n);
- added large-history stress tests, cache-churn tests and detached-scroll-state retention regression coverage;
- indexed scroll/button nodes as layout metadata to remove repeated full-tree control scans from hot scroll frames;
- added a reusable renderer scratch framebuffer so fullscreen `SU/SD` simulation no longer allocates a full screen clone per frame;
- improved the final 10k-row warm-scroll benchmark to ~0.114 ms/frame and ~6.7 KB/frame on the validation host;
- fixed disabled focus traversal returning nodes that were not actually focused;
- fixed a real `scheduler.Clock.Every` data race discovered by the full race campaign and made shared-clock subscriber ticks non-reentrant;
- added idiomatic `selection.New()` while preserving zero-value selection semantics;
- retained zero external Go modules, zero vendor code and CGO-free cross-platform builds.

## Round 4 — 2026-09-09

- fixed flex-wrap main-axis margin selection for column/column-reverse layouts;
- fixed grow/shrink remainder allocation so zero-factor siblings never absorb rounding leftovers;
- made grow/shrink redistribute remaining space after min/max constraints freeze an item;
- fixed natural cross-size measurement for wrapped row/column containers;
- serialized every runtime terminal write, including asynchronous terminal queries, behind one write gate so ANSI sequences cannot interleave;
- preserved exact soft-wrap provenance (`SoftWrapEnd`) for selection, row shifting and translated blits, preventing significant spaces from disappearing when copying wrapped text;
- hardened wide-cell atomicity in `SetCell`, `ClearRegion` and `Blit`; fuzzing found and permanently captured an orphan-spacer regression corpus;
- switched the renderer to reusable internal double buffers while retaining stable `Frame.Screen` snapshots by default; added opt-in `BorrowFrameScreen` for high-frequency embedded loops;
- added bounded per-node last-key caches for measurement, wrapping/graphemes and parsed ANSI, avoiding global cache growth;
- made tab-expanded `SetText` idempotent;
- added layout/render and screen invariant fuzzers, renderer benchmarks, concurrent-output regression tests and cache/provenance tests;
- revalidated race detector, vet, Linux/Windows/macOS cross-builds and a separate external consumer importing every public domain package;
- remains stdlib-only with zero vendor/runtime dependencies.

## Round 3 — 2026-09-09

- inverted foundational dependencies: canonical style/color/length/geometry/key values now live in `internal/core` and the root keeps compatibility aliases;
- extracted escape-boundary scanning into `internal/escscan` and the stateful input parser into `internal/inputparser`;
- extracted Unicode grapheme/cell-width, wrapping/truncation and ANSI-preserving tab expansion into `internal/textutil`;
- made `scheduler` a real independent implementation package rather than a root alias facade;
- improved software bidi for mixed RTL text, numbers and contextual neutral characters while preserving numeric visual order;
- centralized terminal detection in a `Capabilities` snapshot and added OSC 9;4 progress-reporting/version gates;
- added source-compatible OSC 21337 tab-status support gate;
- added runtime methods for title, bell, iTerm2/Ghostty/Kitty notifications, progress and tab status;
- added `Runtime.Start`, `Started` and `Close` for clean embedding into caller-owned event loops; `Run` now reuses the same lifecycle;
- added parser fragmentation regression coverage and `FuzzParserNeverPanics`;
- validated ~90.6k fuzz executions, race detector, vet, Linux/Windows/macOS cross-builds and a separate external-consumer module;
- remains stdlib-only.

## Round 2 — 2026-09-09

- reorganized the public surface into `widgets`, `layout`, `text`, `render`, `input`, `interaction`, `selection`, `terminal`, and `scheduler` domains while preserving canonical root type identity;
- corrected mouse click semantics to activate on release only and never after a drag;
- added multi-click selection, keyboard extension, scroll capture/debt and sticky-follow behavior;
- added X10 mouse, expanded terminal-response parsing, XTVERSION identity and xterm.js duplicate-link suppression;
- added fragment timeout handling for ESC and bracketed paste;
- added terminal query orchestration and DA1 barrier semantics;
- hardened suspend/resume, SIGCONT, resize/reconnect and extended-key mode reassertion;
- added smooth settled scroll draining and xterm.js adaptive wheel policy;
- added native/tmux/OSC52 clipboard behavior;
- added macOS raw TTY support;
- added `Runtime.SetRoot`, `WriteRaw`, `ClearTerminal`, `ClearSearch`, viewport/focus state helpers;
- added dependency-free `ErrorOverview` and shared animation `Clock`;
- expanded parity/regression tests and embedding examples.
