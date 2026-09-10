# Release checklist

The latest published release is **v0.1.11**. This document is the checklist for
preparing the **next** release; do not reuse an already-published tag for new
source changes.

## Choose the next version

Before tagging, update `VERSION`, `version.go`, the README install example when
appropriate, `docs/STABILITY.md`, and promote the relevant `CHANGELOG.md`
`Unreleased` entries under the new version heading.

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
version (for example `v0.1.12`) and create an annotated tag:

```bash
git tag -a v0.1.12 -m "inkgo v0.1.12"
git push origin v0.1.12
```

Then verify the exact version you published:

```bash
go list -m github.com/frudas24/inkgo@v0.1.12
go get github.com/frudas24/inkgo@v0.1.12
```

## Suggested GitHub metadata

Description:

> Dependency-free native Go TUI engine with flex layout, incremental rendering, Unicode input/selection, and an embeddable terminal runtime.

Suggested topics:

`go`, `tui`, `terminal`, `cli`, `flexbox`, `ansi`, `unicode`, `terminal-ui`, `inkgo`

Stars are a community metric, not a release-readiness requirement.

## License / provenance

The repository now carries an MIT `LICENSE` and `THIRD_PARTY_NOTICES.md` for the
known upstream Ink-derived portions. The customized Ink reference fork used for
behavioral parity still lacks an explicit license declaration for its own
modifications. Keep that outstanding clarification visible unless written
permission or a compatible license declaration for those customizations is
available; do not infer or invent one.
