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

package exec2

import (
	"os"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/system/exec2/internal"
)

var devnull *os.File
var devnullonce sync.Once

func iterateChildThreads(pid uint32, f func(uint32) error) error {
	handle, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, pid)
	if err != nil {
		return errors.Annotate(err, "failed to get snapshot").Err()
	}
	defer windows.CloseHandle(handle)

	var threadEntry windows.ThreadEntry32
	threadEntry.Size = uint32(unsafe.Sizeof(threadEntry))

	if err := windows.Thread32First(handle, &threadEntry); err != nil {
		if serr, ok := err.(syscall.Errno); !ok || serr != windows.ERROR_NO_MORE_FILES {
			return errors.Annotate(err, "failed to call Thread32First").Err()
		}
		return nil
	}

	for {
		if pid == threadEntry.OwnerProcessID {
			if err := f(threadEntry.ThreadID); err != nil {
				return err
			}
		}

		if err := windows.Thread32Next(handle, &threadEntry); err != nil {
			if serr, ok := err.(syscall.Errno); !ok || serr != windows.ERROR_NO_MORE_FILES {
				return errors.Annotate(err, "failed to call Thread32First").Err()
			}
			return nil
		}
	}
}

type attr struct {
	jobMu sync.Mutex
	job   windows.Handle

	pid      uint32
	process  windows.Handle
	exitCode int
}

func (c *Cmd) setupCmd() {
	c.cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_SUSPENDED | windows.CREATE_NEW_CONSOLE,
	}
	c.attr.exitCode = -1
}

func createJobObject() (windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, errors.Annotate(err, "failed to create job object").Err()
	}

	// TODO(tikuta): use SetInformationJobObject

	return job, nil
}

func (c *Cmd) start() error {
	// TODO(tikuta): use os/exec package if https://github.com/golang/go/issues/32404 is fixed.
	sysattr := &syscall.ProcAttr{
		Dir: c.cmd.Dir,
		Env: c.cmd.Env,
		Sys: c.cmd.SysProcAttr,
	}

	if sysattr.Env == nil {
		sysattr.Env = os.Environ()
	}

	devnullonce.Do(func() {
		devnull, _ = os.Open(os.DevNull)
	})

	sysattr.Files = append(sysattr.Files, devnull.Fd(), os.Stdout.Fd(), os.Stderr.Fd())

	lp, err := internal.LookExtensions(c.cmd.Path, c.cmd.Dir)
	if err != nil {
		return errors.Annotate(err, "failed to call lookExtensions").Err()
	}
	c.cmd.Path = lp
	pid, process, thread, err := internal.StartProcess(c.cmd.Path, c.cmd.Args, sysattr)
	if err != nil {
		return errors.Annotate(err, "failed to call startProcess").Err()
	}
	defer windows.CloseHandle(thread)
	c.attr.pid = pid
	c.attr.process = process

	success := false

	defer func() {
		if !success {
			c.kill()
			c.wait()
		}
	}()

	job, err := createJobObject()
	if err != nil {
		return errors.Annotate(err, "failed to create job object").Err()
	}

	defer func() {
		if !success {
			windows.CloseHandle(job)
		}
	}()

	c.attr.jobMu.Lock()
	c.attr.job = job
	c.attr.jobMu.Unlock()

	if err := windows.AssignProcessToJobObject(job, process); err != nil {
		return errors.Annotate(err, "failed to assign process to job object").Err()
	}

	if _, err := windows.ResumeThread(thread); err != nil {
		return errors.Annotate(err, "failed to resume thread").Err()
	}

	success = true
	return nil
}

func (c *Cmd) terminate() error {
	// Child process is created with CREATE_NEW_CONSOLE flag.
	// And we use CTRL_C_EVENT here to send signal only to the child process instead of whole process group.
	return windows.GenerateConsoleCtrlEvent(windows.CTRL_C_EVENT, c.attr.pid)
}

func (c *Cmd) wait() error {
	e, err := windows.WaitForSingleObject(c.attr.process, windows.INFINITE)
	if err != nil {
		return errors.Annotate(err, "failed to call WaitForSingleObject").Err()
	}

	if e != windows.WAIT_OBJECT_0 {
		return errors.Reason("unknown return value from WaitForSingleObject: %d", e).Err()
	}

	var ec uint32
	if err := windows.GetExitCodeProcess(c.attr.process, &ec); err != nil {
		return errors.Annotate(err, "failed to call GetExitCodeProcess").Err()
	}
	c.attr.exitCode = int(ec)

	if err := windows.CloseHandle(c.attr.process); err != nil {
		return errors.Annotate(err, "failed to close process handle").Err()
	}
	c.attr.process = windows.InvalidHandle

	c.attr.jobMu.Lock()
	if c.attr.job != windows.InvalidHandle {
		if err := windows.CloseHandle(c.attr.job); err != nil {
			return errors.Annotate(err, "failed to close job object handle").Err()
		}
		c.attr.job = windows.InvalidHandle
	}
	c.attr.jobMu.Unlock()

	if ec != 0 {
		return errors.Reason("exit status %d", ec).Err()
	}

	return nil
}

func (c *Cmd) kill() error {
	c.attr.jobMu.Lock()
	defer c.attr.jobMu.Unlock()

	if err := windows.TerminateJobObject(c.attr.job, 1); err != nil {
		return errors.Annotate(err, "failed to terminate job object").Err()
	}

	if err := windows.CloseHandle(c.attr.job); err != nil {
		return errors.Annotate(err, "failed to close job object handle").Err()
	}
	c.attr.job = windows.InvalidHandle

	return nil
}

func (c *Cmd) exitCode() int {
	return c.attr.exitCode
}
