package shellui

import (
	"syscall"
	"unsafe"
)

var (
	user32          = syscall.NewLazyDLL("user32.dll")
	procMessageBoxW = user32.NewProc("MessageBoxW")
)

const (
	MBOK              = 0x00000000
	MBIconInformation = 0x00000040
	MBIconWarning     = 0x00000030
	MBIconError       = 0x00000010
)

func MessageBox(title, text string, style uintptr) {
	titlePtr, _ := syscall.UTF16PtrFromString(title)
	textPtr, _ := syscall.UTF16PtrFromString(text)
	_, _, _ = procMessageBoxW.Call(0, uintptr(unsafe.Pointer(textPtr)), uintptr(unsafe.Pointer(titlePtr)), style)
}

func Info(title, text string)  { MessageBox(title, text, MBOK|MBIconInformation) }
func Warn(title, text string)  { MessageBox(title, text, MBOK|MBIconWarning) }
func Error(title, text string) { MessageBox(title, text, MBOK|MBIconError) }
