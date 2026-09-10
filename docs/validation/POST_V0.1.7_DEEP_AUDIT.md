# Post-v0.1.7 deep audit

Date: 2026-09-10

This checkpoint is a hardening pass on top of the public `v0.1.7` baseline, released as **v0.1.8** after the failures it fixes were reproduced and cleared on Linux, macOS and Windows runners.

## Audit outcome

The architecture, race discipline, large-scroll hot path and public package composition held up under the campaign. The audit did, however, find real edge bugs that ordinary statement coverage had not exposed. The fixes were applied in the implementation and captured as regression/property/fuzz coverage.

### Windows native console input

- reconstruct Ctrl-based printable keys from the virtual-key record before CSI-u encoding so combined Ctrl/Shift/Alt modifiers are retained;
- preserve Windows wheel magnitude and partial `WHEEL_DELTA` remainder instead of collapsing each record to one notch;
- give the stop event priority over console readiness in the native wait set so shutdown cannot consume input intended for the next shell/application;
- retain the existing single-owner `ReadConsoleInputW` design, viewport-relative mouse translation, UTF-16 surrogate handling, AltGr and repeat-count semantics.

### ANSI / Unicode / text pipeline

- split input-sequence scanning from strict terminal-output ANSI scanning;
- treat output CSI/OSC/DCS/APC/PM/ESC controls as atomic zero-width units during wrapping/slicing/truncation;
- make soft-wrapped lines self-contained by closing/reopening SGR and OSC-8 state at line boundaries;
- prevent slices/truncation from leaking SGR/OSC-8 state or replaying unrelated controls from before the slice;
- unify `ParseANSI`, `StripANSI` and output scanning for generic DEC ESC families and incomplete controls;
- ignore invalid UTF-8 bytes consistently while preserving an explicitly encoded U+FFFD;
- make terminal controls grapheme boundaries on both sides rather than allowing combining marks to hide them;
- align tab semantics so expansion is the sole source of tab-column width;
- correct text-default emoji vs emoji-presentation width, including VS16, keycaps, regional indicators and ZWJ clusters.

## Permanent test surface

The checkpoint contains 152 named tests, 5 fuzz targets and 6 benchmarks.

Fuzz domains:

1. `FuzzParserNeverPanics` — fragmented/arbitrary input bytes;
2. `FuzzScreenWideCellInvariants` — wide glyph/spacer atomicity;
3. `FuzzLayoutAndRenderInvariants` — randomized layout/render trees;
4. `FuzzWrapTextInvariants` — ANSI/Unicode wrap, slice and truncate invariants;
5. `FuzzParseANSIInvariants` — parser/strip visible-text and width agreement.

CI now runs all five fuzz targets as smoke gates and requires at least 80.0% total statement coverage.

## Final local validation

```text
go generate ./...              PASS
go test ./...                  PASS
go vet ./...                   PASS
go test -race ./...            PASS
go test ./... -shuffle=on -count=10   PASS
go test ./scheduler -count=500         PASS

statement coverage             81.8%
CI minimum                     80.0%
internal/engine                80.3%
internal/textutil              90.0%
internal/inputparser           92.4%
scheduler                      89.0%
selection                      100.0%
terminal                       100.0%

external Go dependencies       0
vendor                         0
```

Final isolated fuzz smoke on this tree:

```text
ParserNeverPanics              ~218k executions   PASS
ScreenWideCellInvariants       ~131k executions   PASS
LayoutAndRenderInvariants      ~119k executions   PASS
WrapTextInvariants             ~16k executions    PASS
ParseANSIInvariants            ~12.8k executions  PASS
```

Cross-build / compile checks:

```text
linux/amd64                    PASS
windows/amd64                  PASS
windows/arm64                  PASS
darwin/amd64                   PASS
darwin/arm64                   PASS
Windows engine test binary     PASS (amd64, arm64)
Windows parser test binary     PASS (amd64)
external consumer test/vet     PASS
```

## Coverage conclusion

The audit does not recommend chasing a vanity 90% statement-coverage number. The useful threshold is now `>=80%` plus targeted race, fuzz, property, stress, real-console and external-consumer coverage. This campaign found multiple bugs while the project was already near 80%, demonstrating why risk-oriented tests are the stronger release gate.

## Remaining boundaries

No new architectural P0 was found. The remaining known boundaries are deliberately low-ROI parity edges already documented in `PORT_STATUS.md`: pathological full UAX #9 bidi identity, microscopic Yoga identity and host-specific legacy terminal quirks.

A real Windows Terminal/PowerShell interactive smoke remains valuable for `ReadConsoleInputW` behavior because cross-builds and headless Windows CI cannot fully reproduce a user's physical/interactive console session.

Customized-fork provenance/licensing clarification also remains an external release-hygiene item and must not be invented by code changes.
