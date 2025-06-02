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
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"go.chromium.org/luci/common/errors"
)

type attr struct {
	jobMu sync.Mutex
	job   windows.Handle
}

func iterateChildThreads(pid uint32, f func(uint32) error) error {
	handle, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, pid)
	if err != nil {
		return errors.Fmt("failed to get snapshot: %w", err)
	}
	defer windows.CloseHandle(handle)

	var threadEntry windows.ThreadEntry32
	threadEntry.Size = uint32(unsafe.Sizeof(threadEntry))

	if err := windows.Thread32First(handle, &threadEntry); err != nil {
		if serr, ok := err.(syscall.Errno); !ok || serr != windows.ERROR_NO_MORE_FILES {
			return errors.Fmt("failed to call Thread32First: %w", err)
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
				return errors.Fmt("failed to call Thread32Next: %w", err)
			}
			return nil
		}
	}
}

func (c *Cmd) setupCmd() {
	c.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_SUSPENDED | windows.CREATE_NEW_PROCESS_GROUP,
	}
}

func createJobObject() (windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, errors.Fmt("failed to create job object: %w", err)
	}

	// TODO(tikuta): use SetInformationJobObject

	return job, nil
}

func (c *Cmd) start() error {
	if err := c.Cmd.Start(); err != nil {
		return errors.Fmt("failed to start process: %w", err)
	}

	pid := uint32(c.Process.Pid)

	success := false
	defer func() {
		if !success {
			c.Process.Kill()
			c.wait()
		}
	}()

	job, err := createJobObject()
	if err != nil {
		return errors.Fmt("failed to create job object: %w", err)
	}

	defer func() {
		if !success {
			windows.CloseHandle(job)
		}
	}()

	c.attr.jobMu.Lock()
	c.attr.job = job
	c.attr.jobMu.Unlock()

	// TODO: potential performance improvement https://crbug.com/974202
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, pid)
	if err != nil {
		return errors.Fmt("failed to open process handle: %w", err)
	}
	defer windows.CloseHandle(process)

	if err := windows.AssignProcessToJobObject(job, process); err != nil {
		return errors.Fmt("failed to assign process to job object: %w", err)
	}

	// TODO: potential performance improvement https://crbug.com/974202
	err = iterateChildThreads(pid, func(tid uint32) error {
		thread, err := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, tid)
		if err != nil {
			return errors.Fmt("failed to call OpenThread: %w", err)
		}
		defer windows.CloseHandle(thread)

		if _, err := windows.ResumeThread(thread); err != nil {
			return errors.Fmt("failed to call ResumeThread: %w", err)
		}
		return nil
	})
	if err != nil {
		return err
	}

	success = true
	return nil
}

func (c *Cmd) terminate() error {
	// Child process is created with CREATE_NEW_PROCESS_GROUP flag.
	// And we use CTRL_BREAK_EVENT here to send signal to all child process group instead of a child process.
	return windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, uint32(c.Process.Pid))
}

func (c *Cmd) wait() error {
	if err := c.Cmd.Wait(); err != nil {
		return err
	}

	c.attr.jobMu.Lock()
	if c.attr.job != windows.InvalidHandle {
		if err := windows.CloseHandle(c.attr.job); err != nil {
			return errors.Fmt("failed to close job object handle: %w", err)
		}
		c.attr.job = windows.InvalidHandle
	}
	c.attr.jobMu.Unlock()

	return nil
}

func (c *Cmd) kill() error {
	c.attr.jobMu.Lock()
	defer c.attr.jobMu.Unlock()

	if err := windows.TerminateJobObject(c.attr.job, 1); err != nil {
		return errors.Fmt("failed to terminate job object: %w", err)
	}

	if err := windows.CloseHandle(c.attr.job); err != nil {
		return errors.Fmt("failed to close job object handle: %w", err)
	}
	c.attr.job = windows.InvalidHandle

	return nil
}
