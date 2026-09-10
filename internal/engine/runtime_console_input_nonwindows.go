//go:build !windows

package engine

func startNativeConsoleInputPump(_ *Runtime, _ chan<- runtimeReadResult, _ <-chan struct{}, _ <-chan struct{}) bool {
	return false
}
