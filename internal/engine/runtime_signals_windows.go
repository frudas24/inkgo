//go:build windows

package engine

// Runtime.Run receives WINDOW_BUFFER_SIZE_EVENT through its exclusive
// ReadConsoleInputW pump. Windows has no SIGWINCH and no periodic size polling
// is needed. Embedded callers that own their input loop can call RefreshSize
// when their host reports a resize.
func installRuntimeSignalHandlers(_ *Runtime) func() { return func() {} }

func suspendRuntime(_ *Runtime) bool { return false }
