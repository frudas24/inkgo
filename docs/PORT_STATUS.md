# Port status

## Practical status

The Go port is approximately **99% functionally equivalent for practical terminal-UI use**. Round 5 deliberately prioritized production behavior, packaging and maintainability over chasing implementation identity with JavaScript/React/Yoga internals.

The project is stdlib-only, CGO-free, importable as `github.com/frudas24/inkgo`, and prepared as a `v0.1.2` release candidate.

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

## Deliberately remaining parity boundary

The final ~1% is dominated by low-ROI edge identity rather than missing everyday capabilities:

- full Unicode UAX #9 behavior for pathological nested bidi embeddings/isolates;
- microscopic Yoga identity on obscure flexbox edge combinations not observed in typical application fixtures;
- terminal-emulator-specific legacy quirks requiring real-host fixtures;
- byte-for-byte parity with the original JavaScript implementation where Go-native behavior is already semantically equivalent.

No vendor should be added merely to erase this percentage. Add complexity only when a real fixture demonstrates a user-visible mismatch.

## Release boundary

The source tree can be validated locally, but `v0.1.2` does not exist for consumers until the validated commit is pushed and the Git tag is published. Repository description/topics are GitHub metadata and are not part of the source archive. See `RELEASE.md`.
