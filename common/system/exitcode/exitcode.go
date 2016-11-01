// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package exitcode provides common methods to extract exit codes from errors
// returned by exec.Cmd.
package exitcode

import (
	"os/exec"
	"syscall"

	"github.com/luci/luci-go/common/errors"
)

// Get returns the process process exit return code given an error returned by
// exec.Cmd's Wait or Run methods. If no exit code is present, Get will return
// false.
func Get(err error) (int, bool) {
	err = errors.Unwrap(err)
	if err == nil {
		return 0, true
	}

	if ee, ok := err.(*exec.ExitError); ok {
		return ee.Sys().(syscall.WaitStatus).ExitStatus(), true
	}
	return 0, false
}
