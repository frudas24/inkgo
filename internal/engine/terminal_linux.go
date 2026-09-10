//go:build linux

package engine

import (
	"os"
	"syscall"
	"unsafe"
)

type linuxRawState struct{ termios syscall.Termios }
type winsize struct{ Row, Col, Xpixel, Ypixel uint16 }

func ioctl(fd uintptr, req uintptr, arg unsafe.Pointer) error {
	_, _, e := syscall.Syscall(syscall.SYS_IOCTL, fd, req, uintptr(arg))
	if e != 0 {
		return e
	}
	return nil
}
func terminalSize(f *os.File) (Size, error) {
	var ws winsize
	if err := ioctl(f.Fd(), syscall.TIOCGWINSZ, unsafe.Pointer(&ws)); err != nil {
		return Size{}, err
	}
	return Size{Width: int(ws.Col), Height: int(ws.Row)}, nil
}
func makeRaw(f *os.File) (any, error) {
	var old syscall.Termios
	if err := ioctl(f.Fd(), syscall.TCGETS, unsafe.Pointer(&old)); err != nil {
		return nil, err
	}
	raw := old
	raw.Iflag &^= syscall.BRKINT | syscall.ICRNL | syscall.INPCK | syscall.ISTRIP | syscall.IXON
	raw.Oflag &^= syscall.OPOST
	raw.Cflag |= syscall.CS8
	raw.Lflag &^= syscall.ECHO | syscall.ICANON | syscall.IEXTEN | syscall.ISIG
	raw.Cc[syscall.VMIN] = 1
	raw.Cc[syscall.VTIME] = 0
	if err := ioctl(f.Fd(), syscall.TCSETS, unsafe.Pointer(&raw)); err != nil {
		return nil, err
	}
	return &linuxRawState{old}, nil
}
func restoreRaw(f *os.File, state any) error {
	s, ok := state.(*linuxRawState)
	if !ok {
		return syscall.EINVAL
	}
	return ioctl(f.Fd(), syscall.TCSETS, unsafe.Pointer(&s.termios))
}

func prepareOutput(f *os.File) (func() error, error) { return func() error { return nil }, nil }
