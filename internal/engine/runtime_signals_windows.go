//go:build windows

package engine

func installRuntimeSignalHandlers(_ *Runtime) func() { return func() {} }
func suspendRuntime(_ *Runtime) bool                 { return false }
