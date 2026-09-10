# Round 4 validation — 2026-09-09

## Correctness gates

```text
go test ./...          PASS
go vet ./...           PASS
go test -race ./...    PASS
gofmt -l               clean
go list -m all         github.com/frudas24/inkgo only
```

A separate module at validation time imported `widgets`, `layout`, `render`, `input`, `interaction`, `selection`, `terminal`, `text`, and `scheduler`; it constructed/rendered a tree, parsed input, exercised selection/hit-testing, ran the embedded runtime lifecycle, and passed both `go test` and `go vet`.

## Cross-build gate

```text
linux/amd64    PASS
windows/amd64  PASS
darwin/amd64   PASS
darwin/arm64   PASS
```

Cross-builds use `CGO_ENABLED=0`.

## Fuzz/property smoke

Final smoke run used three independent targets:

```text
FuzzParserNeverPanics             6,371 executions   PASS
FuzzScreenWideCellInvariants     39,856 executions   PASS
FuzzLayoutAndRenderInvariants    74,039 executions   PASS
```

The screen fuzzer previously found a real case where clearing the head of a wide glyph could leave an orphan spacer tail. The minimized input remains committed under `testdata/fuzz/FuzzScreenWideCellInvariants/` as a permanent regression seed.

## Renderer benchmark

Command:

```bash
go test . -run='^$' -bench='BenchmarkRenderer(StableSnapshot|Borrowed)$' -benchmem -benchtime=200x
```

Validation-host result:

```text
BenchmarkRendererStableSnapshot  ~1.865 ms/op   ~966,153 B/op   13,956 allocs/op
BenchmarkRendererBorrowed        ~1.471 ms/op   ~441,282 B/op   13,952 allocs/op
```

The benchmark is comparative, not a portable performance guarantee. The borrowed path reuses renderer-owned screen storage, while the default intentionally pays for a stable snapshot.

## Dependency gate

No third-party Go module is present. No Node/Bun/React/Yoga binding/`bidi-js` runtime or vendor directory is required.
