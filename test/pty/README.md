# inkgo PTY integration tests

This directory is a **separate Go module** used only for end-to-end terminal
integration tests. It intentionally does not add dependencies to the published
`github.com/frudas24/inkgo` module.

The harness uses [`github.com/aymanbagabas/go-pty`](https://github.com/aymanbagabas/go-pty)
to drive a real Unix PTY or Windows ConPTY from outside the application, in the
same spirit as Ink's `node-pty` integration tests.

## Why this go-pty revision?

The module is pinned to commit `bef704d47d9767893438efacb3c7a11b805dc82d`
(`v0.2.3-0.20260305173209-bef704d47d97`). It is the last upstream revision
before a dependency-only bump raised go-pty's module directive from Go 1.20 to
Go 1.24. At that revision the dependency set includes `x/crypto v0.31.0` and
`x/sys v0.28.0` and remains compatible with inkgo's Go 1.23 floor.

Compared with current go-pty PTY/ConPTY code, the only Go source change after
that compatible point is a three-line Windows `Wait` improvement that returns
`*exec.ExitError` on non-zero exit. The harness normalizes that behavior by
checking `ProcessState`, so the integration assertions are independent of that
small API difference.

## Run

```sh
cd test/pty
go test ./...
go test -race ./...   # Unix / supported Go race platforms
```

The tests cover real-process startup and terminal entry, keyboard and Unicode
input, bracketed paste across multiple reads, focus input on platforms where it
can be injected as VT, mouse input on platforms where it can be injected as VT,
resize without a keyboard wakeup, resize bursts, Escape timeout, Ctrl+C in raw
mode, panic unwinding, repeated sessions, and terminal-mode restoration.

The main module remains stdlib-only; verify from the repository root with:

```sh
./scripts/check-no-external-deps.sh
```
