// Copyright (c) 2012 The gocql Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build (all || unit) && linux

package gocql

import (
	"runtime"
	"syscall"
	"unsafe"
)

// pinToSingleCore restricts the current goroutine (and OS thread) to CPU core 0
// for reproducible benchmark results. It sets GOMAXPROCS=1, locks the goroutine
// to its OS thread, and uses sched_setaffinity to pin that thread to core 0.
func pinToSingleCore() {
	runtime.GOMAXPROCS(1)
	runtime.LockOSThread()

	// CPU affinity mask: bit 0 = core 0
	var mask [1024 / 64]uint64
	mask[0] = 1 // pin to core 0

	// SYS_SCHED_SETAFFINITY(pid=0 means current thread, cpusetsize, mask)
	_, _, errno := syscall.RawSyscall(
		syscall.SYS_SCHED_SETAFFINITY,
		0, // pid 0 = current thread
		unsafe.Sizeof(mask),
		uintptr(unsafe.Pointer(&mask[0])),
	)
	if errno != 0 {
		// Best-effort: if affinity fails (e.g., in containers with restricted
		// CPU sets), we still have GOMAXPROCS=1 + LockOSThread.
		_ = errno
	}
}
