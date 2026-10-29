//go:build windows

package system

import (
	"os"
	"syscall"
	"unsafe"
)

var getDiskFreeSpaceExW = syscall.NewLazyDLL("kernel32.dll").NewProc("GetDiskFreeSpaceExW")

// readDisk 通过 GetDiskFreeSpaceEx 读取系统盘容量（仅用于 Windows 本地调试）。
// 面板正式部署在 Linux 上，这里保证跨平台可编译。
func readDisk(m *Metrics) {
	root := os.Getenv("SystemDrive")
	if root == "" {
		root = "C:"
	}
	p, err := syscall.UTF16PtrFromString(root + `\`)
	if err != nil {
		return
	}
	var freeBytes, totalBytes, totalFreeBytes uint64
	ok, _, _ := getDiskFreeSpaceExW.Call(
		uintptr(unsafe.Pointer(p)),
		uintptr(unsafe.Pointer(&freeBytes)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&totalFreeBytes)),
	)
	if ok == 0 || totalBytes == 0 {
		return
	}
	m.DiskTotal = totalBytes
	m.DiskUsed = totalBytes - totalFreeBytes
	m.DiskPercent = round1(float64(m.DiskUsed) / float64(totalBytes) * 100)
}
