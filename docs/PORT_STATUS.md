# Port status

## Practical status

The Go port is approximately **99% functionally equivalent for practical terminal-UI use**. Round 5 deliberately prioritized production behavior, packaging and maintainability over chasing implementation identity with JavaScript/React/Yoga internals.

The project is stdlib-only, CGO-free, importable as `github.com/frudas24/inkgo`, and `v0.1.6` is publicly tagged. Development checkouts may contain additional `Unreleased` hardening.

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

## Post-v0.1.6 hardening

The current development checkpoint additionally replaces Windows console-size polling with native `ReadConsoleInputW` ownership inside `Runtime.Run`. Resize now arrives as `WINDOW_BUFFER_SIZE_EVENT`; key/mouse/focus records are preserved through the same input owner, and the blocking pump is stopped by a kernel event rather than a periodic timer.

The current development checkpoint removes remaining repeated full-tree ScrollBox discovery from hot xterm.js/selection/wheel paths, replaces a throughput-sensitive scheduler assertion with a synchronization-based non-reentrancy regression, raises the CI coverage floor to 77%, and expands CI to the minimum supported Go line plus current stable Go across all three runner OSes. See `validation/WINDOWS_NATIVE_INPUT.md`.

## Deliberately remaining parity boundary

The final ~1% is dominated by low-ROI edge identity rather than missing everyday capabilities:

- full Unicode UAX #9 behavior for pathological nested bidi embeddings/isolates;
- microscopic Yoga identity on obscure flexbox edge combinations not observed in typical application fixtures;
- terminal-emulator-specific legacy quirks requiring real-host fixtures;
- byte-for-byte parity with the original JavaScript implementation where Go-native behavior is already semantically equivalent.

No vendor should be added merely to erase this percentage. Add complexity only when a real fixture demonstrates a user-visible mismatch.

## Release boundary

The latest published tag is `v0.1.6`. New changes should remain under `Unreleased` until the next validated tag is created. Repository description/topics are GitHub metadata and are not part of source archives. See `RELEASE.md`.
