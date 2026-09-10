# Port status

## Practical status

The Go port is approximately **99% functionally equivalent for practical terminal-UI use**. Round 5 deliberately prioritized production behavior, packaging and maintainability over chasing implementation identity with JavaScript/React/Yoga internals.

The project is stdlib-only, CGO-free, importable as `github.com/frudas24/inkgo`, and `v0.1.10` is publicly tagged. Development checkouts may contain additional `Unreleased` hardening.

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

## v0.1.7 through v0.1.10 hardening (post-v0.1.6)

`v0.1.7` replaces Windows console-size polling with native `ReadConsoleInputW` ownership inside `Runtime.Run`. Resize now arrives as `WINDOW_BUFFER_SIZE_EVENT`; key/mouse/focus records are preserved through the same input owner, and the blocking pump is stopped by a kernel event rather than a periodic timer.

`v0.1.7` also removes remaining repeated full-tree ScrollBox discovery from hot xterm.js/selection/wheel paths, replaces a throughput-sensitive scheduler assertion with a synchronization-based non-reentrancy regression, and expands CI to the minimum supported Go line plus current stable Go across all three runner OSes.

`v0.1.8` closes Windows combined-modifier/wheel/shutdown-priority bugs and substantially tightens ANSI/Unicode text semantics: output escape scanning is now distinct from input sequence parsing, ANSI controls are atomic during wrap/slice/truncate, SGR/OSC-8 state is safely closed/reopened across boundaries, invalid UTF-8/control grapheme handling is consistent, and text-default emoji/keycap/VS16 widths are covered by regressions. Five permanent fuzz domains now cover input parsing, screen wide-cell invariants, layout/render, text wrapping/slicing/truncation and ANSI parsing. Total statement coverage is above 81% on the validation host, with an 80% CI floor. See `validation/WINDOWS_NATIVE_INPUT.md` and `validation/POST_V0.1.7_DEEP_AUDIT.md`.

`v0.1.9` adds an external-observer PTY/ConPTY suite in the separate `test/pty` module, and fixes a grapheme-segmentation bug the new wrapping fuzz target found: a ZWJ chain was joined without the UAX #29 GB11 pictographic base, so a digit ZWJ run fused into a three-cell cluster that could not be wrapped and produced rows wider than the requested width. It closes a narrow Unix startup window: `SIGWINCH` handlers were installed after the initial render, so a resize landing in that gap could stay invisible until unrelated input caused another render. `Runtime.Run` now installs signal handlers before `Start`. The suite's immediate-resize test does not discriminate that reorder on its own because the harness may resize the PTY before the fixture reads its initial geometry. A dedicated internal Unix regression now blocks the first `Start` write and injects `SIGWINCH` in that exact window; reverting to the old ordering makes the regression fail, while the current ordering passes repeated runs. The PTY harness also drains final output to quiescence after process exit so restoration assertions cannot race the reader goroutine. The suite's value remains lifecycle, input and terminal-restoration coverage from outside the process, with no dependency added to the root module. See `validation/PTY_INTEGRATION.md`.

## Deliberately remaining parity boundary

The final ~1% is dominated by low-ROI edge identity rather than missing everyday capabilities:

- full Unicode UAX #9 behavior for pathological nested bidi embeddings/isolates;
- microscopic Yoga identity on obscure flexbox edge combinations not observed in typical application fixtures;
- terminal-emulator-specific legacy quirks requiring real-host fixtures;
- byte-for-byte parity with the original JavaScript implementation where Go-native behavior is already semantically equivalent.

No vendor should be added merely to erase this percentage. Add complexity only when a real fixture demonstrates a user-visible mismatch.

## Release boundary

The latest published tag is `v0.1.10`. New changes should remain under `Unreleased` until the next validated tag is created. Repository description/topics are GitHub metadata and are not part of source archives. See `RELEASE.md`.
