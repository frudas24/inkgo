# Port status and parity boundary — round 4

## Source audited

Input archive SHA-256:

`d88df225488bc110916d0b3bfbc21950d0748406bafa4e9fed5559501e68e8fc`

The supplied `ink/` tree is a customized Ink fork with alternate-screen rendering, scroll virtualization, selection, mouse/hit testing, search overlays, terminal protocol negotiation, ANSI internals, screen diffing and lifecycle/performance work.

## Functional parity estimate

Round 1 was conservatively assessed at about 90%, round 2 at about 97%, and round 3 at about 98%. After the round-4 differential/property campaign, practical behavioral parity is **about 99%** for functionality exposed to a Go TUI consumer.

This is deliberately a behavioral estimate rather than a byte-for-byte implementation-identity claim. React Fiber, JavaScript cache identity, the omitted native Yoga wrapper and `bidi-js` are not runtime dependencies of the Go design.

Round 4 closes several real parity defects rather than adding cosmetic API:

- flex wrapping now uses the correct main-axis margins in row and column modes;
- flex grow/shrink excludes zero-factor siblings from remainder distribution and redistributes space around min/max constraints;
- wrapped containers measure their natural cross dimension from actual flex lines;
- runtime output is serialized so render patches, queries, notifications and other escape sequences cannot interleave under concurrency;
- selection retains exact soft-wrap content-end provenance across painting, scroll shifts and translated blits;
- wide glyph heads/tails remain atomic through overlapping writes, clears and blits;
- renderer-owned double buffering reduces frame allocation pressure while default frames remain stable snapshots;
- high-frequency embedders can opt into `BorrowFrameScreen`;
- bounded per-node last-key caches reuse text measurement, wrapping/graphemes and ANSI parsing without an unbounded global cache;
- fuzz campaigns cover raw input parsing, screen wide-cell invariants, and randomized flex-layout/render invariants.

## Round-4 validation

Final validation is recorded in `ROUND4_VALIDATION.md`. The release gate includes:

- `gofmt` clean;
- `go test ./...` green;
- `go vet ./...` green;
- `go test -race ./...` green;
- parser fuzz smoke green;
- wide-cell screen fuzz smoke green, including a minimized permanent regression corpus;
- randomized layout/render fuzz smoke green;
- `go list -m all`: only `github.com/frudas24/inkgo`;
- Linux amd64, Windows amd64, macOS amd64 and macOS arm64 cross-builds green;
- separate external consumer module green under `go test` + `go vet`.

## Remaining ~1% boundary

1. **Full Unicode Bidirectional Algorithm.** Practical mixed LTR/RTL/numeric/neutral terminal text is handled, but every nested UAX #9 embedding/isolate/control-path combination is not claimed without bringing in a full Unicode bidi implementation.

2. **Microscopic Yoga identity.** Common and advanced exposed flex behavior is implemented and property-tested. Byte-identical rounding/cache/min-content behavior for obscure Yoga cases is not promised because the archive omitted its native Yoga source and the project intentionally avoids a vendor dependency.

3. **Rare terminal-emulator quirks.** Important Kitty/xterm.js/tmux/VTE/Windows paths are covered. Legacy emulator quirks should only be added from a reproducible failing fixture, not speculative TERM-name branches.

4. **Implementation-specific optimizers.** The Go renderer now has damage diffing, hardware fullscreen scroll, double buffering and bounded text caches. JavaScript/Fiber-specific optimizer identity is not a parity target when observable behavior is equivalent.

For the project's stated goal—**same practical function, maintainable native Go, fewer dependencies**—the remaining boundary does not justify vendoring code today.
