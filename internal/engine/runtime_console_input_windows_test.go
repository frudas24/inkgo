//go:build windows

package engine

import (
	"encoding/binary"
	"syscall"
	"testing"
	"unsafe"
)

func TestConsoleInputRecordABI(t *testing.T) {
	if got := unsafe.Sizeof(consoleInputRecord{}); got != 20 {
		t.Fatalf("INPUT_RECORD size = %d, want 20", got)
	}
}

func TestDecodeConsoleInputRecordPayloads(t *testing.T) {
	var keyData [16]byte
	binary.LittleEndian.PutUint32(keyData[0:4], 1)
	binary.LittleEndian.PutUint16(keyData[4:6], 2)
	binary.LittleEndian.PutUint16(keyData[6:8], 'A')
	binary.LittleEndian.PutUint16(keyData[10:12], 'a')
	binary.LittleEndian.PutUint32(keyData[12:16], consoleShiftPressed)
	key := decodeConsoleKey(keyData)
	if !key.Down || key.Repeat != 2 || key.VirtualKey != 'A' || key.Unicode != 'a' || key.Control != consoleShiftPressed {
		t.Fatalf("decoded key = %#v", key)
	}

	var mouseData [16]byte
	binary.LittleEndian.PutUint16(mouseData[0:2], 11)
	binary.LittleEndian.PutUint16(mouseData[2:4], 7)
	binary.LittleEndian.PutUint32(mouseData[4:8], 1)
	binary.LittleEndian.PutUint32(mouseData[12:16], consoleMouseMoved)
	mouse := decodeConsoleMouse(mouseData)
	if mouse.X != 11 || mouse.Y != 7 || mouse.Buttons != 1 || mouse.EventFlags != consoleMouseMoved {
		t.Fatalf("decoded mouse = %#v", mouse)
	}
}

func TestWaitConsoleOrStopPrioritizesStop(t *testing.T) {
	console, err := createWindowsEvent()
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.CloseHandle(console)
	stop, err := createWindowsEvent()
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.CloseHandle(stop)

	// When both handles are signaled, stop must win. WaitForMultipleObjects
	// resolves ties by the lowest array index, so this protects unread console
	// input from being consumed during shutdown.
	signalWindowsEvent(console)
	signalWindowsEvent(stop)
	ready, err := waitConsoleOrStop(console, stop)
	if err != nil {
		t.Fatal(err)
	}
	if ready {
		t.Fatal("console input won over an already-signaled stop event")
	}
}

func TestWaitConsoleOrStopDistinguishesConsoleAndStop(t *testing.T) {
	console, err := createWindowsEvent()
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.CloseHandle(console)
	stop, err := createWindowsEvent()
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.CloseHandle(stop)

	signalWindowsEvent(console)
	ready, err := waitConsoleOrStop(console, stop)
	if err != nil || !ready {
		t.Fatalf("console-only wait: ready=%v err=%v", ready, err)
	}
}
