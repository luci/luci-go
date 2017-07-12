// Copyright 2015 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
