# Port status and parity boundary

## Source audited

Input archive SHA-256:

`d88df225488bc110916d0b3bfbc21950d0748406bafa4e9fed5559501e68e8fc`

The supplied `ink/` tree contains **102 TS/TSX files / 20,011 lines**. It is a customized Ink fork, not stock Ink: alternate-screen rendering, scroll virtualization, selection, mouse/hit testing, search overlays, terminal response parsing, ANSI internals, screen diffing and several performance paths are local behavior.

The archive references a few files outside the supplied `ink/` directory, most importantly `src/native-ts/yoga-layout/index.js`. Debug/log/bootstrap imports are not needed by the Go runtime.

## Ported behavior

The Go tree ports the exposed TUI semantics rather than React internals. React Fiber and Yoga object wrappers are intentionally replaced by Go-native tree/layout ownership. Screen rendering, input, focus, scroll, selection, terminal modes and ANSI are all native Go.

Validation at handoff:

- `gofmt`: clean
- `go vet ./...`: clean
- `go test ./...`: **10/10 tests pass**
- `go test -race ./...`: pass
- cross-compilation test binary: Linux amd64, Windows amd64, macOS amd64: pass

## Deliberately not claimed as byte-for-byte parity

1. **Yoga edge-case rounding.** The exposed style surface is implemented in the self-contained Go flex engine and the common behavior is covered, but the missing native-TS Yoga implementation prevents a truthful claim that every obscure Yoga rounding/cache case is identical. If exact fixture parity becomes necessary, the only vendor slice I need is `src/native-ts/yoga-layout/` (and its direct local dependencies), not the whole `node_modules` tree.

2. **Full Unicode Bidirectional Algorithm.** The Go port includes a software RTL fallback that preserves grapheme clusters and reorders contiguous RTL runs on Windows/Windows Terminal/xterm.js. The TS fork delegates complex embedding-level resolution to `bidi-js`, whose source was not in the archive. Nested mixed-direction embeddings are therefore a known precision boundary. A vendor copy of `bidi-js` or permission to take a Go Unicode-bidi dependency would close it.

3. **Terminal capability probing.** Environment-based capability detection and terminal-response parsing are present. The fork's asynchronous startup query manager/XTVERSION cache is not reproduced as a background subsystem; applications can consume `Runtime.OnResponse` and choose policy explicitly.

4. **Native clipboard helpers.** OSC52/tmux passthrough encoding is present. The TS-specific `pbcopy` / `tmux load-buffer -w` subprocess policy is not baked into the library; Go callers can own that OS policy.

5. **Optimization implementation details.** The behaviorally important screen damage diff and fullscreen hardware-scroll path are ported. The exact JS node-cache/blit-cache heuristics are not copied line-for-line; Go rebuilds the next cell buffer and diffs it. This keeps semantics simple and deterministic while still avoiding unchanged terminal writes.

Those are explicit boundaries, not hidden TODOs. The port is usable without a vendor bundle today.
