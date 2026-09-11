# inkgo v0.1.13 audit hardening checkpoint

Date: 2026-09-10
Status: production-candidate hardening snapshot; not yet promoted beyond upstream VERSION 0.1.13.

## Fixes applied during aggressive audit

- Runtime.SetRoot is reentrancy-safe: a nested root switch from focus/blur callbacks supersedes the stale outer switch.
- Close/Start lifecycle drops incomplete parser fragments and transient pointer/drag/multiclick state so state cannot leak across terminal ownership epochs.
- Stop is permanent: Start errors and ResumeTerminal is a no-op after Stop.
- Terminal enter/exit/raw/output restoration errors are propagated and joined; failed cleanup is retained and retried instead of silently discarded.
- Suspend/Resume completes pending terminal cleanup before re-entering modes.
- TerminalQuerier detects write errors and short writes, removes impossible pending waiters, and resolves them instead of blocking forever.
- Wide-cell fuzzer input stream is bounded to keep CI exploration effective.
- staticcheck dead helpers/unused assignments/inefficiencies were removed without public API changes.
- Regression tests added for lifecycle cleanup/retry, Stop semantics, reentrant SetRoot, terminal-query write failure, and related boundaries.

## Validation completed

- go test ./... : PASS
- go vet ./... : PASS
- staticcheck ./... : PASS
- go test -race ./... : PASS
- real PTY module with recovered local go-pty dependency: PASS
- real PTY module under -race: PASS
- PTY repeated stress (5 runs in prior audit continuation): PASS
- fuzzers: WrapText, ParserNeverPanics, ScreenWideCellInvariants, LayoutAndRenderInvariants, TreeMutationInvariants, ParseANSIInvariants: PASS
- cross compile/test-build: windows/amd64, windows/arm64, darwin/amd64, darwin/arm64, linux/arm64: PASS
- downstream reopencode integration: vendored source sync PASS; go test internal/cmd PASS; go vet PASS; TUI+CLI -race PASS

## Remaining external gates

- govulncheck could not be made authoritative in the sandbox because the vulnerability DB requires network access.
- Native physical-terminal smoke on Windows ConPTY and macOS terminal remains an external release gate; cross-builds are green but are not substitutes for real OS terminal behavior.

