package diag

import (
	"syscall"
	"unsafe"
)

var procGetProcessMemoryInfo = syscall.NewLazyDLL("kernel32.dll").NewProc("K32GetProcessMemoryInfo")

// memoryCounters is PROCESS_MEMORY_COUNTERS_EX2, of Windows 10 1809 on.
type memoryCounters struct {
	cb                         uint32
	pageFaultCount             uint32
	peakWorkingSetSize         uintptr
	workingSetSize             uintptr
	quotaPeakPagedPoolUsage    uintptr
	quotaPagedPoolUsage        uintptr
	quotaPeakNonPagedPoolUsage uintptr
	quotaNonPagedPoolUsage     uintptr
	pagefileUsage              uintptr
	peakPagefileUsage          uintptr
	privateUsage               uintptr
	privateWorkingSetSize      uintptr
	sharedCommitUsage          uintptr
}

// processMemory returns the private working set of the process, which
// Task Manager shows as its memory (0 before Windows 10 1809), and its
// commit size.
func processMemory() (privateWS, commit uint64) {
	var m memoryCounters
	h, _ := syscall.GetCurrentProcess()
	m.cb = uint32(unsafe.Sizeof(m))
	if r, _, _ := procGetProcessMemoryInfo.Call(uintptr(h), uintptr(unsafe.Pointer(&m)), uintptr(m.cb)); r == 0 {
		m = memoryCounters{cb: uint32(unsafe.Offsetof(m.privateWorkingSetSize))}
		procGetProcessMemoryInfo.Call(uintptr(h), uintptr(unsafe.Pointer(&m)), uintptr(m.cb))
	}
	return uint64(m.privateWorkingSetSize), uint64(m.privateUsage)
}
