// Copyright 2020 The LUCI Authors.
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

// +build windows

package invoke

import (
	"os/exec"
	"syscall"

	"go.chromium.org/luci/common/errors"
)

// setSysProcAttr sets flags which ensure the process starts detached in its own process group.
func setSysProcAttr(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}

func (s *Subprocess) terminiate() error {
	dll, err := syscall.LoadDLL("kernel32.dll")
	if err != nil {
		return errors.Annotate(err, "LoadDLL").Err()
	}
	proc, err := dll.FindProc("GenerateConsoleCtrlEvent")
	if err != nil {
		return errors.Annotate(err, "Find GenerateConsoleCtrlEvent function").Err()
	}

	if ret, _, err := proc.Call(syscall.CTRL_BREAK_EVENT, uintptr(s.cmd.Process.Pid)); ret == 0 {
		return errors.Annotate(err, "Send CTRL-BREAK event").Err()
	}
	return nil
}
