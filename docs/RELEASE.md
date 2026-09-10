# Release checklist

This tree is prepared as the **v0.1.0 release candidate**. Publishing the remote Git tag is a repository action and is intentionally separate from building the source ZIP.

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
go run ./examples/terminal-smoke
```

Run the final interactive smoke on at least one real Windows Terminal/PowerShell console. Linux/macOS real-terminal smoke is also recommended.

## Tag

After the validated tree is committed to `main`:

```bash
git tag -a v0.1.0 -m "inkgo v0.1.0"
git push origin v0.1.0
```

Then verify:

```bash
go list -m github.com/frudas24/inkgo@v0.1.0
go get github.com/frudas24/inkgo@v0.1.0
```

## Suggested GitHub metadata

Description:

> Dependency-free native Go TUI engine with flex layout, incremental rendering, Unicode input/selection, and an embeddable terminal runtime.

Suggested topics:

`go`, `tui`, `terminal`, `cli`, `flexbox`, `ansi`, `unicode`, `terminal-ui`, `inkgo`

Stars are a community metric, not a release-readiness requirement.

## License / provenance

The supplied TypeScript source archive did not contain a license file. Before publishing a public `v0.1.0` tag, verify the licensing/provenance obligations of the upstream/customized Ink source and add the appropriate license/attribution. This source tree intentionally does not invent a license on the author's behalf.
