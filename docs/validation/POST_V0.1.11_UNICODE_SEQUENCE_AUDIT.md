# Post-v0.1.11 Unicode sequence audit

Date: 2026-09-10

This follow-up audits the terminal-facing sequence boundaries left after the
Unicode Emoji 17.0 property-table work released in `v0.1.11`. It deliberately
does not attempt to turn inkgo into a complete general-purpose UAX #29
implementation; the scope is grapheme segmentation and cell width behavior
that can affect the framebuffer, wrapping and terminal rendering.

## Inputs and release boundary

The development source supplied for this audit was:

```text
inkgo-main-V0.1.12.zip
SHA256 a1cbfd7b5054b2d4bd5fa66e16a1783aadf61df301ce85313d502155bebeec7a
```

Despite the archive name, the source tree declares `0.1.11`, and `v0.1.11` is
the published release boundary. These fixes therefore remain under
`Unreleased`; this audit does not manufacture a `v0.1.12` tag or change the
version constants.

Variation-sequence authority was the Unicode Emoji 17.0 data file supplied for
the audit:

```text
emoji-variation-sequences.txt
Version 17.0
Date 2025-01-30
SHA256 bb3d09ef03f206012c7532dd52dc0a21c9efddba0135ea4cf0d9201b8b9bba7e
742 records
371 unique bases
```

The 371 bases were compacted into 183 sorted, non-overlapping static ranges in
`internal/textutil/emoji_variation.go`. A full set comparison against the input
file was performed during generation. The table is data only and adds no
runtime dependency.

## Defects reproduced by ablation

Three correctness classes were reproduced before patching.

### 1. GB11 join state survived characters after ZWJ

The segmenter could keep a pending GB11 join alive across an Extend-like rune
or another ZWJ after the ZWJ. Consequently sequences such as:

```text
Extended_Pictographic ZWJ Extend Extended_Pictographic
Extended_Pictographic ZWJ ZWJ Extended_Pictographic
```

could be fused as though they matched GB11. The relevant UAX #29 shape is:

```text
Extended_Pictographic Extend* ZWJ × Extended_Pictographic
```

The `Extend*` belongs before the ZWJ. The implementation now treats the join as
a one-step opportunity: after the ZWJ, only the immediately following
`Extended_Pictographic` can consume it. Any intervening Extend, VS, modifier or
ZWJ cancels it. Valid family and skin-tone ZWJ sequences remain intact.

### 2. Keycap detection accepted keycap-like clusters

The width path previously recognized a keycap by seeing a keycap base and a
later U+20E3 in the same grapheme. That admitted malformed shapes such as:

```text
1 + combining acute + U+20E3
1 + VS15 + U+20E3
1 + VS16 + combining acute + U+20E3
```

The detector is now a small structural state machine and accepts only:

```text
[0-9#*] U+20E3
[0-9#*] VS16 U+20E3
```

Intervening marks, VS15 and duplicate VS16 invalidate the keycap shape instead
of forcing width 2.

### 3. VS15/VS16 used the Emoji property instead of a registered sequence

The previous width calculation could let a selector affect the whole grapheme
rather than its adjacent base, and used `Emoji` as an approximation for
whether the base supported a variation sequence. Examples exposed by the
regression tests include:

```text
© + combining acute + VS16
⌚ + combining acute + VS15
😀 + VS15
```

The last example is important: U+1F600 is Emoji but has no registered FE0E
variation sequence, so `Emoji=Yes` is not a valid substitute for the official
variation-sequence set.

Width now changes for FE0E/FE0F only when the selector is adjacent to a base in
the exact Unicode Emoji 17.0 variation-base table. Unsupported or displaced
selectors do not override the base/default terminal width.

## Permanent regressions

`internal/textutil/unicode_terminal_regression_test.go` fixes the new behavior
for:

- GB11 with an intervening combining mark, VS16, modifier or second ZWJ after
  the ZWJ, plus valid Extend-before-ZWJ/family/skin-tone controls;
- every keycap base (`#`, `*`, `0`-`9`) in valid and adversarial sequence
  shapes;
- registered adjacent FE0E/FE0F, displaced selectors, unsupported selectors
  and non-Emoji selectors;
- the variation-base table ordering/non-overlap invariant, membership in
  `Emoji`, and the exact 371-code-point count.

`FuzzWrapTextInvariants` also gained seeds for the adversarial GB11, keycap and
variation-selector forms.

## Validation

The patched source passed:

```text
go generate ./...                              PASS (api.go byte-identical)
go test ./...                                  PASS
go vet ./...                                   PASS
go test -race ./...                            PASS
go test -shuffle=on -count=10 ./...            PASS

FuzzParserNeverPanics                          PASS
FuzzScreenWideCellInvariants                   PASS
FuzzLayoutAndRenderInvariants                  PASS
FuzzWrapTextInvariants                         PASS
FuzzParseANSIInvariants                        PASS

TestRunInstallsResizeHandlerBeforeTerminalEntry x100 PASS
scheduler x500                                 PASS
scheduler -race x100                           PASS
```

Measured statement coverage on the audit host:

```text
overall                    82.1%
internal/textutil          92.4%
CI floor                   80% (unchanged)
```

Representative width benchmarks after the patch remained in the same class as
`v0.1.11`:

```text
ASCII                      ~41.5-42.4 ns/op, 0 allocs
ANSI ASCII                 ~340-348 ns/op, 1 alloc
complex emoji              ~2.4 us/op, 16 allocs
```

The exact variation table is consulted only when the current rune is FE0E or
FE0F, avoiding a table lookup on ordinary grapheme runes.

## PTY / ConPTY validation

The repository's public `test/pty/go.mod` and `go.sum` were left unchanged. For
offline validation, a temporary copy of the harness was pointed at the supplied
Go-1.23 recovery bundle (`go-pty` core plus local `creack/pty` and `x/sys`),
with SSH-only indirect requirements removed only in that disposable copy.

```text
PTY go test -mod=readonly -count=30 -shuffle=on ./...       PASS
PTY go test -race -mod=readonly -count=10 -shuffle=on ./... PASS
PTY go vet ./...                                             PASS
```

The source module hashes remained:

```text
test/pty/go.mod  dfac0d3990a6901453493e9c1850543095f119ea9437c3b877eb16443ccf602d
test/pty/go.sum  4af410f51c56d1e7fef6780d80f0ea6a90fee4ee6b869fb14815c88dedc62830
```

No recovery `replace` directive or bundled dependency is part of the root
module or final source tree.

## Cross-build and external consumer

With `CGO_ENABLED=0`, both the root module and the PTY harness/test binary built
for:

```text
linux/amd64
windows/amd64
windows/arm64
darwin/amd64
darwin/arm64
```

A separate temporary consumer module imported the public root facade plus
`input`, `interaction`, `layout`, `render`, `scheduler`, `selection`,
`terminal`, `text` and `widgets`; `go test` and `go vet` passed. The repository
`scripts` directory was deliberately excluded because it is policy/release
infrastructure containing shell scripts and tests, not an importable public Go
package.

## Result

The remaining terminal-facing Unicode decisions audited here are now based on
sequence structure rather than coarse property approximations:

```text
Extended_Pictographic / GB11     strict post-ZWJ target
Emoji variation selectors       exact registered base + adjacency
keycaps                          exact terminal-relevant sequence shape
framebuffer width                still bounded to 0..2 cells per grapheme
```

The root module remains stdlib-only with zero runtime/module dependencies. The
next version may be promoted only after normal public CI is green; this audit
itself leaves the tree at `0.1.11` with the fixes under `Unreleased`.
