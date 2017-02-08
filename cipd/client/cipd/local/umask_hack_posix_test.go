// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// +build !windows

package local

import (
	"syscall"
)

func init() {
	// We explicitly set the Umask for tests so that we can deterministically
	// check the file modes of created files and directories.
	syscall.Umask(022)
}
