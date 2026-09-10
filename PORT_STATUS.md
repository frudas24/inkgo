# Port status and parity boundary — round 3

## Source audited

Input archive SHA-256:

`d88df225488bc110916d0b3bfbc21950d0748406bafa4e9fed5559501e68e8fc`

The supplied `ink/` tree contains about 20k TypeScript/TSX lines and is a customized Ink fork: alternate-screen rendering, scroll virtualization, selection, mouse/hit testing, search overlays, terminal response parsing, ANSI internals, screen diffing and performance/lifecycle work are local behavior.

## Functional parity estimate

Round 1 was conservatively assessed at about 90%, round 2 at about 97%. After the round-3 architecture and protocol campaign, practical behavioral parity is **about 98%** for functionality exposed to a Go TUI consumer.

This remains a behavioral estimate, not a byte-for-byte implementation-identity claim. React Fiber, JS object caches and the omitted native Yoga wrapper are intentionally not dependencies of the Go design.

Round 3 adds or strengthens:

- real leaf implementations for canonical value/style/geometry, input parsing, escape-boundary scanning, text measurement/wrapping/tabstops and scheduler timing;
- root compatibility aliases so existing round-1/2 consumers keep the same public type identity;
- contextual bidi levels for mixed RTL + numbers + neutrals, preserving numeric order during visual RTL reversal;
- centralized terminal capability snapshots, including XTVERSION/xterm.js identity, synchronized output, extended keys, software bidi, cursor-yank quirks and OSC 9;4 progress support;
- source-compatible OSC 21337 tab-status gating;
- imperative Go equivalents for title, bell, terminal notifications, progress and tab-status hooks;
- `Runtime.Start` / `Runtime.Close` lifecycle for applications that own their own event loop;
- parser fragmentation regression tests and a fuzz target; a smoke campaign executed about 90k inputs without panic;
- repeat validation from a completely separate Go module that imports the domain packages.

## Validation

Round-3 checkpoint validation:

- `gofmt` clean;
- `go test ./...` green;
- `go vet ./...` green;
- `go test -race ./...` green;
- parser fuzz smoke: ~90.6k executions / 3 seconds, no panic;
- `go list -m all`: only `github.com/frudas24/inkgo`;
- `GOOS=linux GOARCH=amd64 go build ./...` green;
- `GOOS=windows GOARCH=amd64 go build ./...` green;
- `GOOS=darwin GOARCH=amd64 go build ./...` green;
- `GOOS=darwin GOARCH=arm64 go build ./...` green;
- external consumer module using `widgets/layout/render/input/selection/terminal/scheduler` green under `go test` + `go vet`.

## Remaining ~2% boundary

1. **Exact Yoga edge behavior.** Common flex behavior and the exposed style surface are implemented. Obscure upstream Yoga rounding/cache/min-content corner cases are not promised byte-for-byte because the archive omitted its native Yoga implementation.

2. **Full Unicode Bidirectional Algorithm.** Round 3 handles substantially better mixed RTL/numeric/neutral terminal text. Explicit nested UAX #9 embeddings/isolates and every pathological bidi-control combination are still outside the dependency-free claim.

3. **Terminal-emulator micro-quirks.** Important xterm.js/tmux/Windows/Kitty/VTE paths are represented. Rare legacy-emulator protocol behavior still requires real integration fixtures to justify special handling.

4. **Error source excerpts.** `ErrorOverview` renders Go error chains, but ordinary Go `error` values do not universally carry JS-style source-location excerpts, so the port does not manufacture them.

5. **Internal optimization identity.** Damage diffing, safe main-screen output and fullscreen hardware scroll are present. JS-specific Fiber/node/blit cache heuristics are not parity targets unless profiling demonstrates a Go-side need.

The project remains **stdlib-only**. Vendor code should only be introduced if a real failing integration fixture demonstrates behavior that is both important and unreasonable to implement natively.
