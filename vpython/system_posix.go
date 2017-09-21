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

// +build darwin linux freebsd netbsd openbsd android

package vpython

import (
	"os"
	"syscall"

	"go.chromium.org/luci/vpython/venv"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/system/environ"

	"golang.org/x/net/context"
)

// systemSpecificLaunch launches the process described by "cmd" while ensuring
// that the VirtualEnv lock is held throughout its duration (best effort).
//
// On Linux/Mac, we use "execve" to *become* the target process. We need to
// continue to hold the lock for that process, though. We do this by passing it
// as an open file handle to the subprocess.
//
// This can be error-prone, as it places the burden on the subprocess to
// manage the file descriptor.
func systemSpecificLaunch(c context.Context, ve *venv.Env, argv []string, env environ.Env, dir string) error {
	// Change directory.
	if dir != "" {
		if err := os.Chdir(dir); err != nil {
			return errors.Annotate(err, "failed to chdir to %q", dir).Err()
		}
	}

	// Store our lock file descriptor as FD #3 (after #2, STDERR).
	lockFD := ve.LockHandle.LockFile().Fd()
	if lockFD == 3 {
		// "dup2" doesn't change flags if the source and destination file
		// descriptors are the same. Explicitly remove the close-on-exec flag, which
		// Go enables by default.
		if _, _, err := syscall.RawSyscall(syscall.SYS_FCNTL, lockFD, syscall.F_SETFD, 0); err != 0 {
			return errors.Annotate(err, "could not remove close-on-exec for lock file").Err()
		}
	} else {
		// Use "dup2" to copy the file descriptor to #3 slot. This will also clear
		// its flags, including close-on-exec.
		if _, _, err := syscall.RawSyscall(syscall.SYS_DUP2, lockFD, 3, 0); err != 0 {
			return errors.Annotate(err, "could not dup2 lock file").Err()
		}
	}

	// This is the original process. Become Python.
	if err := syscall.Exec(argv[0], argv, env.Sorted()); err != nil {
		return errors.Annotate(err, "failed to execve %q", argv[0]).Err()
	}
	return nil
}
