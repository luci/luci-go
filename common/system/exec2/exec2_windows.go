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

	"golang.org/x/sys/windows"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/system/exec2/internal"
)

var devnull *os.File
var devnullonce sync.Once

type attr struct {
	job      windows.Handle
	process  windows.Handle
	exitCode int
}

func (c *Cmd) setupCmd() {
	c.Cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_SUSPENDED,
	}
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
		Dir: c.Dir,
		Env: c.Env,
		Sys: c.SysProcAttr,
	}

	if sysattr.Env == nil {
		sysattr.Env = os.Environ()
	}

	devnullonce.Do(func() {
		devnull, _ = os.Open(os.DevNull)
	})

	sysattr.Files = append(sysattr.Files, devnull.Fd(), os.Stdout.Fd(), os.Stderr.Fd())

	lp, err := internal.LookExtensions(c.Path, c.Dir)
	if err != nil {
		return errors.Annotate(err, "failed to call lookExtensions").Err()
	}
	c.Path = lp
	process, thread, err := internal.StartProcess(c.Path, c.Args, sysattr)
	if err != nil {
		return errors.Annotate(err, "failed to call startProcess").Err()
	}
	defer windows.CloseHandle(thread)
	c.attr.process = process

	success := false

	defer func() {
		if !success {
			c.kill()
			c.wait()
		}
	}()

	// need for ResumeThread
	// https://docs.microsoft.com/en-us/windows/desktop/api/processthreadsapi/nf-processthreadsapi-resumethread
	const PROCESS_SUSPEND_RESUME = 0x0800

	// need for AssignProcessToJobObject
	// https://docs.microsoft.com/en-us/windows/desktop/api/jobapi2/nf-jobapi2-assignprocesstojobobject
	const PROCESS_SET_QUOTA = 0x0100

	job, err := createJobObject()
	if err != nil {
		return errors.Annotate(err, "failed to create job object").Err()
	}

	defer func() {
		if !success {
			windows.CloseHandle(job)
		}
	}()
	c.attr.job = job

	if err := windows.AssignProcessToJobObject(job, process); err != nil {
		return errors.Annotate(err, "failed to assing process to job object").Err()
	}

	if _, err := windows.ResumeThread(thread); err != nil {
		return errors.Annotate(err, "failed to resume thread").Err()
	}

	success = true
	return nil
}

func (c *Cmd) terminate() error {
	// TODO(tikuta): use GenerateConsoleCtrlEvent
	return c.kill()
}

func (c *Cmd) killprocess() error {
	return windows.TerminateProcess(c.attr.process, 1)
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
	return nil
}

func (c *Cmd) kill() error {
	if err := c.killprocess(); err != nil {
		return errors.Annotate(err, "failed to call killprocess").Err()
	}

	return windows.TerminateJobObject(c.attr.job, 0)
}

func (c *Cmd) exitCode() int {
	return c.attr.exitCode
}
