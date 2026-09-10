# Release checklist

The current source version is **v0.1.3**. For a new release, update `VERSION`, `version.go`, and the version references below before tagging. Publishing the remote Git tag is a repository action and is intentionally separate from building the source ZIP.

## Before tagging

```bash
gofmt -w .
go generate ./...
git diff --exit-code -- api.go
go test ./...
go vet ./...
go test -race ./...
./scripts/check-coverage.sh 74.0 coverage.out
./scripts/check-no-external-deps.sh
./scripts/check-version.sh
# Include new source files in the commit; the manifest also lists non-ignored
# untracked files, so inspect git status before committing the release.
git status --short
./scripts/update-manifest.sh
./scripts/check-manifest.sh
go run ./examples/terminal-smoke
```

Run the final interactive smoke on at least one real Windows Terminal/PowerShell console. Linux/macOS real-terminal smoke is also recommended.

## Tag

After the validated tree is committed to `main`:

```bash
git tag -a v0.1.3 -m "inkgo v0.1.3"
git push origin v0.1.3
```

Then verify:

```bash
go list -m github.com/frudas24/inkgo@v0.1.3
go get github.com/frudas24/inkgo@v0.1.3
```

## Suggested GitHub metadata

Description:

> Dependency-free native Go TUI engine with flex layout, incremental rendering, Unicode input/selection, and an embeddable terminal runtime.

Suggested topics:

`go`, `tui`, `terminal`, `cli`, `flexbox`, `ansi`, `unicode`, `terminal-ui`, `inkgo`

Stars are a community metric, not a release-readiness requirement.

## License / provenance

The supplied TypeScript source archive did not contain a license file. Before publishing a public `v0.1.3` tag, verify the licensing/provenance obligations of the upstream/customized Ink source and add the appropriate license/attribution. This source tree intentionally does not invent a license on the author's behalf.
