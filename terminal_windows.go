//go:build windows

package ink

import (
	"os"
	"syscall"
	"unsafe"
)

var kernel32 = syscall.NewLazyDLL("kernel32.dll")
var procGetConsoleMode = kernel32.NewProc("GetConsoleMode")
var procSetConsoleMode = kernel32.NewProc("SetConsoleMode")
var procGetConsoleScreenBufferInfo = kernel32.NewProc("GetConsoleScreenBufferInfo")

type windowsRawState struct{ mode uint32 }
type coord struct{ X, Y int16 }
type smallRect struct{ Left, Top, Right, Bottom int16 }
type consoleScreenBufferInfo struct {
	Size              coord
	CursorPosition    coord
	Attributes        uint16
	Window            smallRect
	MaximumWindowSize coord
}

func terminalSize(f *os.File) (Size, error) {
	var i consoleScreenBufferInfo
	r, _, e := procGetConsoleScreenBufferInfo.Call(f.Fd(), uintptr(unsafe.Pointer(&i)))
	if r == 0 {
		return Size{}, e
	}
	return Size{Width: int(i.Window.Right - i.Window.Left + 1), Height: int(i.Window.Bottom - i.Window.Top + 1)}, nil
}
func makeRaw(f *os.File) (any, error) {
	var old uint32
	r, _, e := procGetConsoleMode.Call(f.Fd(), uintptr(unsafe.Pointer(&old)))
	if r == 0 {
		return nil, e
	}
	const enableProcessedInput = 0x0001
	const enableLineInput = 0x0002
	const enableEchoInput = 0x0004
	const enableVirtualTerminalInput = 0x0200
	mode := (old &^ (enableProcessedInput | enableLineInput | enableEchoInput)) | enableVirtualTerminalInput
	r, _, e = procSetConsoleMode.Call(f.Fd(), uintptr(mode))
	if r == 0 {
		return nil, e
	}
	return &windowsRawState{old}, nil
}
func restoreRaw(f *os.File, state any) error {
	s, ok := state.(*windowsRawState)
	if !ok {
		return syscall.EINVAL
	}
	r, _, e := procSetConsoleMode.Call(f.Fd(), uintptr(s.mode))
	if r == 0 {
		return e
	}
	return nil
}

func prepareOutput(f *os.File) (func() error, error) {
	var old uint32
	r, _, e := procGetConsoleMode.Call(f.Fd(), uintptr(unsafe.Pointer(&old)))
	if r == 0 {
		return nil, e
	}
	const enableProcessedOutput = 0x0001
	const enableVirtualTerminalProcessing = 0x0004
	mode := old | enableProcessedOutput | enableVirtualTerminalProcessing
	r, _, e = procSetConsoleMode.Call(f.Fd(), uintptr(mode))
	if r == 0 {
		return nil, e
	}
	return func() error {
		rr, _, ee := procSetConsoleMode.Call(f.Fd(), uintptr(old))
		if rr == 0 {
			return ee
		}
		return nil
	}, nil
}
