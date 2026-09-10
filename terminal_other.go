//go:build !linux && !windows

package inkgo

import (
	"errors"
	"os"
)

func terminalSize(f *os.File) (Size, error) {
	return Size{}, errors.New("terminal size unsupported on this platform")
}
func makeRaw(f *os.File) (any, error) {
	return nil, errors.New("raw terminal unsupported on this platform")
}
func restoreRaw(f *os.File, state any) error { return nil }

func prepareOutput(f *os.File) (func() error, error) { return func() error { return nil }, nil }
