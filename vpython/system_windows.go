// Copyright 2017 The LUCI Authors.
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

package vpython

import (
	"context"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/environ"
)

// Copied from https://github.com/golang/go/blob/go1.16.15/src/syscall/exec_windows.go
// createEnvBlock converts an array of environment strings into
// the representation required by CreateProcess: a sequence of NUL
// terminated strings followed by a nil.
// Last bytes are two UCS-2 NULs, or four NUL bytes.
func createEnvBlock(envv []string) *uint16 {
	if len(envv) == 0 {
		return &utf16.Encode([]rune("\x00\x00"))[0]
	}
	length := 0
	for _, s := range envv {
		length += len(s) + 1
	}
	length += 1

	b := make([]byte, length)
	i := 0
	for _, s := range envv {
		l := len(s)
		copy(b[i:i+l], []byte(s))
		copy(b[i+l:i+l+1], []byte{0})
		i = i + l + 1
	}
	copy(b[i:i+1], []byte{0})

	return &utf16.Encode([]rune(string(b)))[0]
}

func execImpl(c context.Context, argv []string, env environ.Env, dir string, setupFn func() error) error {
	// As of go 1.17, handles to be passed to subprocesses via Cmd.Run must be explicitly
	// specified. To keep the expected behavior of letting Python inherit all inheritable
	// file handles, we instead use syscall directly, based on the Go 1.16 implementation
	// of cmd.Run and syscall.StartProcess.
	// Tracked in https://github.com/golang/go/issues/53652
	resolvedPath, err := exec.LookPath(argv[0])
	if err != nil {
		return errors.Annotate(err, "Could not locate executable for %v", argv[0]).Err()
	}
	resolvedPath, err = filepath.Abs(resolvedPath)
	if err != nil {
		return err
	}

	sys := new(syscall.SysProcAttr)
	procAttr := syscall.ProcAttr{
		Dir:   dir,
		Env:   env.Sorted(),
		Files: []uintptr{os.Stdin.Fd(), os.Stdout.Fd(), os.Stderr.Fd()},
		Sys:   sys,
	}

	argv0p, err := syscall.UTF16PtrFromString(resolvedPath)
	if err != nil {
		return err
	}

	var cmdline string
	for _, arg := range argv {
		if len(cmdline) > 0 {
			cmdline += " "
		}
		cmdline += syscall.EscapeArg(arg)
	}

	var argvp *uint16
	if len(cmdline) != 0 {
		argvp, err = syscall.UTF16PtrFromString(cmdline)
		if err != nil {
			return err
		}
	}

	var dirp *uint16
	if len(procAttr.Dir) != 0 {
		dirp, err = syscall.UTF16PtrFromString(procAttr.Dir)
		if err != nil {
			return err
		}
	}

	// At this point, ANY ERROR will be fatal (panic). We assume that each
	// operation may permanently alter our runtime environment.
	if setupFn != nil {
		if err := setupFn(); err != nil {
			panic(err)
		}
	}

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	go func() {
		<-ch
		logging.Debugf(c, "os.Interrupt recieved, restoring signal handler.")
		signal.Stop(ch)
		// Due to the nature of os.Interrupt (either CTRL_C_EVENT or
		// CTRL_BREAK_EVENT), they're sent to the entire process group. Since we
		// haven't created a separate group for `cmd`, we don't need to relay the
		// signal (since `cmd` would have gotten it as well).
	}()

	// Acquire the fork lock so that no other threads
	// create new fds that are not yet close-on-exec
	// before we fork.
	syscall.ForkLock.Lock()
	defer syscall.ForkLock.Unlock()

	p, _ := syscall.GetCurrentProcess()
	fd := make([]syscall.Handle, len(procAttr.Files))
	for i := range procAttr.Files {
		if procAttr.Files[i] > 0 {
			err := syscall.DuplicateHandle(p, syscall.Handle(procAttr.Files[i]), p, &fd[i], 0, true, syscall.DUPLICATE_SAME_ACCESS)
			if err != nil {
				panic(err)
			}
			defer syscall.CloseHandle(syscall.Handle(fd[i]))
		}
	}
	si := new(syscall.StartupInfo)
	si.Cb = uint32(unsafe.Sizeof(*si))
	si.Flags = syscall.STARTF_USESTDHANDLES
	if sys.HideWindow {
		si.Flags |= syscall.STARTF_USESHOWWINDOW
		si.ShowWindow = syscall.SW_HIDE
	}
	si.StdInput = fd[0]
	si.StdOutput = fd[1]
	si.StdErr = fd[2]

	pi := new(syscall.ProcessInformation)

	flags := sys.CreationFlags | syscall.CREATE_UNICODE_ENVIRONMENT
	err = syscall.CreateProcess(argv0p, argvp, sys.ProcessAttributes, sys.ThreadAttributes, !sys.NoInheritHandles, flags, createEnvBlock(procAttr.Env), dirp, si, pi)
	if err != nil {
		panic(err)
	}
	defer syscall.CloseHandle(syscall.Handle(pi.Thread))

	handle := uintptr(pi.Process)
	s, err := syscall.WaitForSingleObject(syscall.Handle(handle), syscall.INFINITE)
	switch s {
	case syscall.WAIT_OBJECT_0:
		break
	case syscall.WAIT_FAILED:
		panic("WaitForSingleObject failed")
	default:
		panic("Unexpected result from WaitForSingleObject")
	}

	var rc uint32
	if err = syscall.GetExitCodeProcess(syscall.Handle(handle), &rc); err != nil {
		panic(err)
	}

	// The process had an exit code (includes err==nil, 0).
	logging.Debugf(c, "Python subprocess has terminated: %v", err)
	os.Exit(int(rc))
	panic("must not return")
}
