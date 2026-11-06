//go:build windows

package app

import (
	"io"
	"os"
	"syscall"
	"unsafe"
)

const enableVirtualTerminalProcessing = 0x0004

// enableVirtualTerminal turns on ANSI escape processing for a Windows
// console. It returns false when the console does not support it (legacy
// conhost), so the caller prints plain text instead of "?[32m" garbage.
func enableVirtualTerminal(output io.Writer) bool {
	file, ok := output.(*os.File)
	if !ok {
		return false
	}
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getMode := kernel32.NewProc("GetConsoleMode")
	setMode := kernel32.NewProc("SetConsoleMode")
	handle := file.Fd()
	var mode uint32
	if r, _, _ := getMode.Call(handle, uintptr(unsafe.Pointer(&mode))); r == 0 {
		return false
	}
	if mode&enableVirtualTerminalProcessing != 0 {
		return true
	}
	r, _, _ := setMode.Call(handle, uintptr(mode|enableVirtualTerminalProcessing))
	return r != 0
}
