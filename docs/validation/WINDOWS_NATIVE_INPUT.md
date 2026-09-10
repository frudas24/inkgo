# Windows native console input / resize validation

Baseline: post-v0.1.6 hardening checkpoint. This remains an **Unreleased** development tree until promoted to a new immutable version.

## Change

Windows `Runtime.Run` no longer samples terminal size every 60 ms. For a real console input handle, one exclusive `ReadConsoleInputW` pump now consumes the Windows `INPUT_RECORD` stream.

- `WINDOW_BUFFER_SIZE_EVENT` calls the existing thread-safe `queueResize`, retaining burst coalescing and bounded PTY-size retries;
- `KEY_EVENT` records are converted into parser-compatible UTF-8/CSI/CSI-u bytes, preserving repeat count and Shift/Alt/Ctrl, including Insert/Delete/Page/F1-F12;
- printable AltGr (`RightAlt+Ctrl`) remains ordinary text input;
- UTF-16 surrogate pairs are reassembled and malformed pairs recover without dropping the following BMP rune;
- `MOUSE_EVENT` records become SGR mouse packets, with screen-buffer coordinates normalized to the visible console window origin;
- `FOCUS_EVENT` becomes the same focus-in/focus-out response understood by the existing runtime;
- Windows raw mode enables `ENABLE_WINDOW_INPUT` and `ENABLE_MOUSE_INPUT`, disables Quick Edit while owned, and restores the exact original mode;
- `WaitForMultipleObjects` waits on the console input handle and a private stop event, so shutdown does not require polling and does not leave a blocked native read that can steal the next shell/application key.

Redirected stdin, pipes, PTYs and caller-owned wrapped readers intentionally retain the generic `io.Reader` path. `Start` does not consume input by contract; embedded applications that own their loop also own resize notification.

## Additional bugs fixed

The native conversion audit exposed two independent correctness gaps:

1. xterm modifier sequences for Insert/Delete/PageUp/PageDown and F1-F12 were not decoded with modifiers; the parser now preserves them.
2. Windows mouse coordinates are screen-buffer-relative, not viewport-relative; the native path now subtracts the console window origin before producing SGR coordinates.

## Validation

```text
go generate ./...                  PASS
go test ./...                      PASS
go vet ./...                       PASS
go test -race ./...                PASS
coverage                           78.3% (CI minimum 77.0%)
external Go dependencies           0

windows/amd64 go build ./...       PASS
windows/amd64 engine test compile  PASS
windows/amd64 parser test compile  PASS
windows/arm64 go build ./...       PASS
windows/arm64 engine test compile  PASS
linux/amd64 go build ./...         PASS
darwin/amd64 go build ./...        PASS
darwin/arm64 go build ./...        PASS
```

Fuzz pass on the validation host:

```text
FuzzParserNeverPanics              80,254 executions   PASS
FuzzScreenWideCellInvariants       75,151 executions   PASS
FuzzLayoutAndRenderInvariants     123,987 executions   PASS
```

The Windows syscall boundary also has Windows-only ABI/decode tests; the platform-neutral record encoder is tested on every runner for keys, modifiers, repeats, AltGr, UTF-16, mouse, wheel and focus. A real Windows Terminal smoke remains recommended before publishing the next tag because redirected CI cannot prove interactive console-host behavior.
