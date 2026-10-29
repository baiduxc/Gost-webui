//go:build !windows

package system

import "syscall"

// readDisk 通过 statfs 读取根分区容量（Linux/Unix）。
func readDisk(m *Metrics) {
	var st syscall.Statfs_t
	if err := syscall.Statfs("/", &st); err != nil {
		return
	}
	total := st.Blocks * uint64(st.Bsize)
	free := st.Bavail * uint64(st.Bsize)
	m.DiskTotal = total
	if total > 0 {
		m.DiskUsed = total - free
		m.DiskPercent = round1(float64(m.DiskUsed) / float64(total) * 100)
	}
}
