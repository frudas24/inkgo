# Changelog

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
