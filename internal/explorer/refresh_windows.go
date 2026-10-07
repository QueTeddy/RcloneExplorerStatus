//go:build windows

package explorer

import (
	"syscall"
	"unsafe"
)

var shell32 = syscall.NewLazyDLL("shell32.dll")

func NotifyPathChanged(path string) {
	procedure := shell32.NewProc("SHChangeNotify")
	widePath, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return
	}
	procedure.Call(0x00002000, 0x0005, uintptr(unsafe.Pointer(widePath)), 0)
}
