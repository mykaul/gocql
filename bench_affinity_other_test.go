// Copyright (c) 2012 The gocql Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build (all || unit) && !linux

package gocql

import "runtime"

// pinToSingleCore restricts the current goroutine to a single OS thread for
// reproducible benchmark results. On non-Linux platforms, CPU affinity pinning
// is not available, so only GOMAXPROCS and LockOSThread are used.
func pinToSingleCore() {
	runtime.GOMAXPROCS(1)
	runtime.LockOSThread()
}
