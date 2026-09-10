//go:build windows

package engine

import (
	"encoding/binary"
	"os"
	"syscall"
	"unsafe"
)

const (
	consoleKeyEventType              = 0x0001
	consoleMouseEventType            = 0x0002
	consoleWindowBufferSizeEventType = 0x0004
	consoleFocusEventType            = 0x0010
)

var procReadConsoleInputW = kernel32.NewProc("ReadConsoleInputW")
var procCreateEventW = kernel32.NewProc("CreateEventW")
var procSetEvent = kernel32.NewProc("SetEvent")
var procWaitForMultipleObjects = kernel32.NewProc("WaitForMultipleObjects")

const (
	waitObject0  = 0x00000000
	waitFailed   = 0xffffffff
	infiniteWait = 0xffffffff
)

func createWindowsEvent() (syscall.Handle, error) {
	h, _, err := procCreateEventW.Call(0, 1, 0, 0) // manual-reset, initially nonsignaled
	if h == 0 {
		if err == syscall.Errno(0) {
			err = syscall.EINVAL
		}
		return 0, err
	}
	return syscall.Handle(h), nil
}

func signalWindowsEvent(handle syscall.Handle) {
	if handle != 0 {
		procSetEvent.Call(uintptr(handle))
	}
}

func waitConsoleOrStop(console, stop syscall.Handle) (bool, error) {
	// Stop must be first. WaitForMultipleObjects returns the lowest-index
	// signaled object when several are ready; prioritizing stop prevents a
	// buffered console event from being consumed after shutdown was requested.
	handles := [...]syscall.Handle{stop, console}
	r, _, err := procWaitForMultipleObjects.Call(
		uintptr(len(handles)),
		uintptr(unsafe.Pointer(&handles[0])),
		0,
		infiniteWait,
	)
	switch r {
	case waitObject0:
		return false, nil
	case waitObject0 + 1:
		return true, nil
	case waitFailed:
		if err == syscall.Errno(0) {
			err = syscall.EINVAL
		}
		return false, err
	default:
		return false, syscall.EINVAL
	}
}

type consoleInputRecord struct {
	EventType uint16
	Padding   uint16
	Data      [16]byte
}

func readConsoleInput(handle syscall.Handle, records []consoleInputRecord) (uint32, error) {
	if len(records) == 0 {
		return 0, nil
	}
	var read uint32
	r, _, err := procReadConsoleInputW.Call(
		uintptr(handle),
		uintptr(unsafe.Pointer(&records[0])),
		uintptr(uint32(len(records))),
		uintptr(unsafe.Pointer(&read)),
	)
	if r == 0 {
		if err == syscall.Errno(0) {
			err = syscall.EINVAL
		}
		return 0, err
	}
	return read, nil
}

func decodeConsoleKey(data [16]byte) consoleKeyEvent {
	return consoleKeyEvent{
		Down:       binary.LittleEndian.Uint32(data[0:4]) != 0,
		Repeat:     binary.LittleEndian.Uint16(data[4:6]),
		VirtualKey: binary.LittleEndian.Uint16(data[6:8]),
		Unicode:    binary.LittleEndian.Uint16(data[10:12]),
		Control:    binary.LittleEndian.Uint32(data[12:16]),
	}
}

func decodeConsoleMouse(data [16]byte) consoleMouseEvent {
	return consoleMouseEvent{
		X:          int16(binary.LittleEndian.Uint16(data[0:2])),
		Y:          int16(binary.LittleEndian.Uint16(data[2:4])),
		Buttons:    binary.LittleEndian.Uint32(data[4:8]),
		Control:    binary.LittleEndian.Uint32(data[8:12]),
		EventFlags: binary.LittleEndian.Uint32(data[12:16]),
	}
}

func consoleViewportOrigin(rt *Runtime) (int16, int16) {
	if rt == nil || rt.Terminal == nil || rt.Terminal.Out == nil {
		return 0, 0
	}
	var info consoleScreenBufferInfo
	r, _, _ := procGetConsoleScreenBufferInfo.Call(rt.Terminal.Out.Fd(), uintptr(unsafe.Pointer(&info)))
	if r == 0 {
		return 0, 0
	}
	return info.Window.Left, info.Window.Top
}

func consoleInputHandle(rt *Runtime) (*os.File, bool) {
	if rt == nil || rt.Terminal == nil || rt.Terminal.In == nil {
		return nil, false
	}
	file, ok := rt.In.(*os.File)
	if !ok || file != rt.Terminal.In {
		return nil, false
	}
	var mode uint32
	r, _, _ := procGetConsoleMode.Call(file.Fd(), uintptr(unsafe.Pointer(&mode)))
	return file, r != 0
}

// startNativeConsoleInputPump takes exclusive ownership of a real Windows
// console input buffer while Runtime.Run owns the loop. ReadConsoleInputW is
// required here rather than a resize-only watcher: every read consumes an
// INPUT_RECORD, so competing with io.Reader would lose/reorder key events.
// Pipes, redirected stdin and caller-owned wrapped readers return false and use
// the generic reader path instead.
func startNativeConsoleInputPump(rt *Runtime, reads chan<- runtimeReadResult, requests <-chan struct{}, done <-chan struct{}) bool {
	file, ok := consoleInputHandle(rt)
	if !ok {
		return false
	}
	stopEvent, err := createWindowsEvent()
	if err != nil {
		return false
	}
	bridgeStop := make(chan struct{})
	bridgeDone := make(chan struct{})
	go func() {
		defer close(bridgeDone)
		select {
		case <-done:
			signalWindowsEvent(stopEvent)
		case <-bridgeStop:
		}
	}()
	go func() {
		defer func() {
			close(bridgeStop)
			<-bridgeDone
			_ = syscall.CloseHandle(stopEvent)
		}()
		records := make([]consoleInputRecord, 64)
		encoder := consoleInputEncoder{}
		console := syscall.Handle(file.Fd())
		for {
			select {
			case <-done:
				return
			case <-requests:
			}
			ready, waitErr := waitConsoleOrStop(console, stopEvent)
			if waitErr != nil {
				select {
				case <-done:
					return
				case reads <- runtimeReadResult{err: waitErr}:
				}
				return
			}
			if !ready {
				return
			}

			n, err := readConsoleInput(console, records)
			if err != nil {
				select {
				case <-done:
					return
				case reads <- runtimeReadResult{err: err}:
				}
				return
			}
			var data []byte
			for i := uint32(0); i < n; i++ {
				record := records[i]
				switch record.EventType {
				case consoleKeyEventType:
					data = append(data, encoder.key(decodeConsoleKey(record.Data))...)
				case consoleMouseEventType:
					mouse := decodeConsoleMouse(record.Data)
					left, top := consoleViewportOrigin(rt)
					mouse.X -= left
					mouse.Y -= top
					data = append(data, encoder.mouse(mouse)...)
				case consoleWindowBufferSizeEventType:
					rt.queueResize()
				case consoleFocusEventType:
					if binary.LittleEndian.Uint32(record.Data[0:4]) != 0 {
						data = append(data, FocusIn...)
					} else {
						data = append(data, FocusOut...)
					}
				}
			}
			select {
			case <-done:
				return
			case reads <- runtimeReadResult{data: data}:
			}
		}
	}()
	return true
}
