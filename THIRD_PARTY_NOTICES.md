# Third-party notices

## Ink (upstream)

This library is a native Go port of the terminal-UI engine popularized by
**Ink** — https://github.com/vadimdemedes/ink — licensed under the **MIT**
License, Copyright (c) 2015-2019 Vadim Demedes and Ink contributors.

The MIT notice for the derived portions is reproduced in [`LICENSE`](LICENSE).

## Customized Ink fork (ported reference)

The behavior ported here was audited against a *customized Ink fork* supplied
as a `src/ink/` tree (reference snapshot 2026-09-09). That fork:

- contains the upstream Ink core (MIT) **without** carrying a `LICENSE` file,
  license field or copyright headers; and
- adds its own modifications (alternate-screen lifecycle, scroll
  virtualization, selection, mouse/hit testing, search overlays, terminal
  response parsing, ANSI internals, screen diffing, performance paths) for
  which **no license is declared**.

This Go port reimplements the exposed TUI *semantics and API* in Go; it does
not embed Node.js, React, Yoga bindings or any JavaScript runtime, and it was
produced from the audited source tree rather than by translating application
code. See `SOURCE_AUDIT.md` and `PORT_STATUS.md` for the audited boundaries.

### Outstanding clarification

The customizations contributed by that fork do not carry an explicit license.
Before any use that relies on those specific behaviors, obtain a license
declaration for the fork (or a written permission for this port). This notice
exists so downstream users can evaluate that chain explicitly.


## Test-only PTY harness

The separate `test/pty` integration-test module uses
`github.com/aymanbagabas/go-pty` (MIT) to create Unix PTYs and Windows ConPTY
sessions. Its transitive Go-module dependencies are development/test tooling
only and are not imported by the published `github.com/frudas24/inkgo` module.
No `go-pty` or transitive dependency source is vendored into this repository;
their own upstream license terms apply when the test module is downloaded.
