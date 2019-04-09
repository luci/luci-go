// Copyright 2019 The LUCI Authors.
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

package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"

	isatty "github.com/mattn/go-isatty"
	"go.chromium.org/luci/common/system/exitcode"
)

// less implements terminal paging using Unix less command.
// Implements io.Writer.
type less struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
}

// Write writes to the paged output.
func (l *less) Write(data []byte) (int, error) {
	return l.stdin.Write(data)
}

// Alive returns true if less subprocess is alive.
func (l *less) Alive() bool {
	// http://man7.org/linux/man-pages/man2/kill.2.html
	//   If sig is 0, then no signal is sent, but existence and permission
	//   checks are still performed; this can be used to check for the
	//   existence of a process ID or process group ID that the caller is
	//   permitted to signal.
	//
	// Note: this code does not run on Windows.
	return l.cmd.Process.Signal(syscall.Signal(0)) == nil
}

// Wait waits for the less to exit and returns the exit code.
func (l *less) Wait() (exitCode int, err error) {
	err = l.cmd.Wait()
	if exitCode, ok := exitcode.Get(err); ok {
		return exitCode, nil
	}
	return 0, err
}

var errLessUnavailable = fmt.Errorf("less is unavailable")

// startPager starts less and returns.
// startPager does not accept context because killing less breaks the
// terminal. Instead, user is expected to hit q key.
func startLess(out *os.File, disableColor bool) (*less, error) {
	lessPath, err := exec.LookPath("less")
	if err != nil {
		// Example in https://godoc.org/os/exec#LookPath
		// implies that an err is returned if the file is not found.
		return nil, errLessUnavailable
	}

	if !isatty.IsTerminal(out.Fd()) {
		disableColor = true
	}

	flags := "-FX"
	if !disableColor {
		flags += "r"
	}

	cmd := exec.Command(lessPath, flags)
	cmd.Stdout = out
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &less{cmd: cmd, stdin: stdin}, nil
}
