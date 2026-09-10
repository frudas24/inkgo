# Port status and parity boundary — round 2

## Source audited

Input archive SHA-256:

`d88df225488bc110916d0b3bfbc21950d0748406bafa4e9fed5559501e68e8fc`

The supplied `ink/` tree contains about 20k TypeScript/TSX lines and is a customized Ink fork: alternate-screen rendering, scroll virtualization, selection, mouse/hit testing, search overlays, terminal response parsing, ANSI internals, screen diffing and performance/lifecycle work are local behavior.

## Functional parity estimate

Round 1 was conservatively assessed at about 90% behavioral parity. After the second campaign, the practical parity estimate is **about 97%** for the functionality exposed to a Go TUI consumer.

This is a behavioral estimate, not a claim of byte-for-byte implementation identity. React Fiber, JS object caches and Yoga wrappers are intentionally not part of the target architecture.

Round 2 closes the largest previously known gaps:

- click fires on release only and is suppressed after drag;
- double-click word and triple-click line selection;
- keyboard selection extension, scroll debt/capture and sticky-follow reconciliation;
- SGR + X10 mouse and richer terminal response parsing;
- search integration for visible and virtualized/positioned results;
- fragment timeouts for ESC and bracketed paste;
- asynchronous terminal query manager with DA1 sentinel barrier;
- terminal focus state and long-gap mode recovery;
- suspend/SIGCONT/resize/alternate-screen reassert lifecycle;
- Kitty/modifyOtherKeys negotiation with balanced pop-before-push reassert;
- smooth wheel draining that continues until settled, including xterm.js policy;
- native/tmux/OSC52 clipboard paths;
- Linux/macOS/Windows raw terminal paths;
- terminal raw writer, clear/redraw API and tree replacement for embedding;
- shared animation clock equivalent to the fork's consolidated clock;
- domain-oriented public import surfaces.

## Remaining ~3% boundary

1. **Exact Yoga edge behavior.** Common flex behavior and the style surface are implemented, but obscure upstream Yoga rounding/cache/min-content edge cases are not promised byte-for-byte because the archive omitted its native Yoga implementation. No vendor is required for normal use.

2. **Full Unicode Bidirectional Algorithm.** The Go software bidi path handles practical RTL/mixed terminal text while preserving grapheme clusters, but it is not a complete nested UAX #9 embedding/isolate implementation equivalent to `bidi-js` for every pathological string.

3. **Terminal-emulator micro-quirks.** The important xterm.js/tmux/Windows/Kitty paths are represented. There can still be emulator-specific protocol quirks in rare legacy terminals that only real integration fixtures will expose.

4. **Error source excerpts.** `ErrorOverview` is functional in Go and renders wrapped error chains. Standard Go `error` values do not universally contain JS-style file/line stack metadata, so the port does not fake source-code excerpts.

5. **Internal optimization identity.** The Go renderer has damage diffing and the fullscreen hardware-scroll optimization. It does not copy every JS node-cache/blit-cache heuristic because those are implementation details, not externally observable contracts.

The project remains **stdlib-only**. The preferred policy is to add no vendor unless a real integration fixture demonstrates a behavior that cannot reasonably be implemented natively.
