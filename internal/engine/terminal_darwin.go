//go:build darwin

package engine

import (
	"os"
	"syscall"
	"unsafe"
)

type darwinRawState struct{ termios syscall.Termios }
type darwinWinsize struct{ Row, Col, Xpixel, Ypixel uint16 }

func darwinIoctl(fd uintptr, req uintptr, arg unsafe.Pointer) error {
	_, _, e := syscall.Syscall(syscall.SYS_IOCTL, fd, req, uintptr(arg))
	if e != 0 {
		return e
	}
	return nil
}

func terminalSize(f *os.File) (Size, error) {
	var ws darwinWinsize
	if err := darwinIoctl(f.Fd(), syscall.TIOCGWINSZ, unsafe.Pointer(&ws)); err != nil {
		return Size{}, err
	}
	return Size{Width: int(ws.Col), Height: int(ws.Row)}, nil
}

func makeRaw(f *os.File) (any, error) {
	var old syscall.Termios
	if err := darwinIoctl(f.Fd(), syscall.TIOCGETA, unsafe.Pointer(&old)); err != nil {
		return nil, err
	}
	raw := old
	raw.Iflag &^= syscall.BRKINT | syscall.ICRNL | syscall.INPCK | syscall.ISTRIP | syscall.IXON
	raw.Oflag &^= syscall.OPOST
	raw.Cflag |= syscall.CS8
	raw.Lflag &^= syscall.ECHO | syscall.ICANON | syscall.IEXTEN | syscall.ISIG
	raw.Cc[syscall.VMIN] = 1
	raw.Cc[syscall.VTIME] = 0
	if err := darwinIoctl(f.Fd(), syscall.TIOCSETA, unsafe.Pointer(&raw)); err != nil {
		return nil, err
	}
	return &darwinRawState{termios: old}, nil
}

func restoreRaw(f *os.File, state any) error {
	s, ok := state.(*darwinRawState)
	if !ok {
		return syscall.EINVAL
	}
	return darwinIoctl(f.Fd(), syscall.TIOCSETA, unsafe.Pointer(&s.termios))
}

func prepareOutput(_ *os.File) (func() error, error) {
	return func() error { return nil }, nil
}
