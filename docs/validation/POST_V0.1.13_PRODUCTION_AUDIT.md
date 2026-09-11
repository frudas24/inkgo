# Post-v0.1.13 production audit

Date: 2026-09-10

This audit treats the published `v0.1.13` source as the baseline and records
hardening intended for the next immutable release. The `v0.1.13` tag must not
be rewritten.

## Confirmed defects fixed

- `Runtime.SetRoot` could overwrite a newer nested root transition triggered by
  focus/blur callbacks.
- `Close` did not reset incomplete parser/pointer state before a later `Start`.
- `Start` after permanent `Stop` could enter terminal modes into an inert
  runtime.
- terminal exit/raw/output restoration errors were discarded; transient
  failures were not retryable.
- `Start` ignored raw-mode setup failure and could return success with
  `Started()==true` on a non-TTY input.
- `ResumeTerminal` had the same false-ownership failure mode.
- `SuspendTerminal` could discard a failed exit transition and then re-enter
  modes on top of incomplete cleanup.
- terminal query and DA1 sentinel write failures could leave impossible waiters
  pending.
- static analysis found dead helpers/assignments; they were removed without
  changing public behavior.

## Validation performed

On Go 1.23.2 linux/amd64:

- `go test ./...`
- `go vet ./...`
- `staticcheck ./...`
- `go test -race ./...`
- `go test -shuffle=on -count=10 ./...`
- 100x race stress of lifecycle/reentrancy cases
- lifecycle state-machine fuzz: ~424k executions in one 20s campaign
- input parser fuzz: ~173k executions
- ANSI parser fuzz: ~216k executions
- layout/render fuzz: ~260k executions
- tree mutation/focus fuzz: ~124k executions
- wide-cell fuzz smoke: ~6k executions
- real PTY module through an offline `go-pty` recovery bundle: 14/14 cases,
  then 20 shuffled repetitions under `-race`
- cross-compilation of all packages for linux/arm64, windows/amd64,
  windows/arm64, darwin/amd64 and darwin/arm64
- external reopencode integration: full tests/vet/race/checkpoint and 5/5 real
  Linux PTY permission+SIGWINCH+Enter flows
- total statement coverage: 82.6% (80% release floor)
- 10k warm scroll remains in the ~0.12 ms/frame class on the audit host

## Remaining release gate

A real Windows Terminal/PowerShell ConPTY smoke is still required by the
project's release checklist. Cross-builds validate build tags and APIs but do
not substitute for executing Windows console input, resize, raw-mode restore and
shutdown on an actual host.

`govulncheck` could not fetch `https://vuln.go.dev` in the isolated audit
environment. This is an unexecuted external-data gate, not a clean result.
