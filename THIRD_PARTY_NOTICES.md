# Third-party notices

## Ink (upstream)

This library is a native Go port of the terminal-UI engine popularized by
**Ink** — https://github.com/vadimdemedes/ink — licensed under the **MIT**
License, Copyright (c) Vadym Demedes and Sindre Sorhus.

The MIT notice for the derived portions is reproduced in [`LICENSE`](LICENSE).

## Customized Ink fork (ported reference)

The behavior ported here was audited against a *customized Ink fork* supplied
as a `src/ink/` tree (reference snapshot 2026-09-09). The source repository
declares the MIT license and the attribution reproduced above. That fork:

- adds its own modifications (alternate-screen lifecycle, scroll
  virtualization, selection, mouse/hit testing, search overlays, terminal
  response parsing, ANSI internals, screen diffing, performance paths) for
  which the source repository's MIT terms apply.

This Go port reimplements the exposed TUI *semantics and API* in Go; it does
not embed Node.js, React, Yoga bindings or any JavaScript runtime, and it was
produced from the audited source tree rather than by translating application
code. See `SOURCE_AUDIT.md` and `PORT_STATUS.md` for the audited boundaries.

The Go implementation is a native port of the audited behavior and does not
embed the fork's Node.js, React, Yoga or JavaScript runtime. The upstream
attribution and MIT terms are retained in [`LICENSE`](LICENSE).


## Test-only PTY harness

The separate `test/pty` integration-test module uses
`github.com/aymanbagabas/go-pty` (MIT) to create Unix PTYs and Windows ConPTY
sessions. Its transitive Go-module dependencies are development/test tooling
only and are not imported by the published `github.com/frudas24/inkgo` module.
No `go-pty` or transitive dependency source is vendored into this repository;
their own upstream license terms apply when the test module is downloaded.

## Unicode data

`internal/textutil/emoji_properties.go` and `emoji_pictographic.go` contain
compacted property ranges derived from Unicode Emoji 17.0 `emoji-data.txt` for
`Emoji`, `Emoji_Presentation` and `Extended_Pictographic`.
`internal/textutil/emoji_variation.go` contains the compacted set of bases from
Unicode Emoji 17.0 `emoji-variation-sequences.txt` that have registered FE0E
and FE0F presentation sequences. Unicode data files
are Copyright © 1991-2026 Unicode, Inc. and are made available under the
Unicode License v3, which permits use, modification and redistribution subject
to its notice requirements and warranty disclaimer. The authoritative data and
license are available from:

- https://www.unicode.org/Public/17.0.0/ucd/emoji/emoji-data.txt
- https://www.unicode.org/Public/17.0.0/ucd/emoji/emoji-variation-sequences.txt
- https://www.unicode.org/license.txt

The property tables are data only; no Unicode library or runtime dependency is
linked into inkgo.

