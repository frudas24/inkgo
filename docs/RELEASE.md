# Release checklist

The latest published release is **v0.1.35**. This document is the checklist for
preparing the **next** release; do not reuse an already-published tag for new
source changes.

## Choose the next version

Before tagging, update `VERSION`, `version.go`, the README install example when
appropriate, `docs/STABILITY.md`, `docs/PORT_STATUS.md`, and promote the relevant
`CHANGELOG.md` `Unreleased` entries under the new version heading.
`./scripts/check-version.sh` fails when the documents disagree about the latest
published version.

## Validate before tagging

```bash
gofmt -w .
go generate ./...
git diff --exit-code -- api.go
go test ./...
go vet ./...
go test -race ./...
./scripts/check-coverage.sh 80.0 coverage.out
./scripts/check-no-external-deps.sh
./scripts/check-version.sh
(cd test/pty && go mod download && go mod verify && GOFLAGS=-mod=readonly go test -count=5 -shuffle=on -timeout=120s ./... && GOFLAGS=-mod=readonly go vet ./...)
# Include new source files in the commit; the manifest also lists non-ignored
# untracked files, so inspect git status before committing the release.
git status --short
./scripts/update-manifest.sh
./scripts/check-manifest.sh
go run ./examples/terminal-smoke
```

Run the final interactive smoke on at least one real Windows Terminal/PowerShell
console. Linux/macOS real-terminal smoke is also recommended.

CI tests the minimum supported Go line (`1.23.x`) and the current stable Go
release across Ubuntu, macOS and Windows. Race, coverage and fuzz-smoke use the
current stable Go toolchain; policy checks remain on the minimum supported line.
The separate `test/pty` module runs real PTY/ConPTY end-to-end scenarios on
Ubuntu, macOS and Windows, with an additional Linux race campaign.

## Tag

After the validated tree is committed to `main`, choose a new immutable semantic
version (for example `vX.Y.Z`, never a tag that is already published) and create
an annotated tag:

```bash
git tag -a vX.Y.Z -m "inkgo vX.Y.Z"
git push origin vX.Y.Z
```

Then verify the exact version you published:

```bash
go list -m github.com/frudas24/inkgo@vX.Y.Z
go get github.com/frudas24/inkgo@vX.Y.Z
```

## Suggested GitHub metadata

Description:

> Dependency-free native Go TUI engine with flex layout, incremental rendering, Unicode input/selection, and an embeddable terminal runtime.

Suggested topics:

`go`, `tui`, `terminal`, `cli`, `flexbox`, `ansi`, `unicode`, `terminal-ui`, `inkgo`

Stars are a community metric, not a release-readiness requirement.

## License / provenance

The repository now carries an MIT `LICENSE` and `THIRD_PARTY_NOTICES.md` for the
upstream Ink-derived portions. The audited reference repository declares the
same MIT terms and the Vadym Demedes/Sindre Sorhus attribution; retain that
notice when distributing the Go port. Keep the clean-room/native-port boundary
documented in `THIRD_PARTY_NOTICES.md`.
