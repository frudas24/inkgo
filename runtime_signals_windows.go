//go:build windows

package inkgo

func installRuntimeSignalHandlers(_ *Runtime) func() { return func() {} }
func suspendRuntime(_ *Runtime) bool                 { return false }
