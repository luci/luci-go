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
	"context"
	"os"
	"syscall"

	"golang.org/x/sys/unix"

	"go.chromium.org/luci/vpython/python"
	"go.chromium.org/luci/vpython/venv"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/system/environ"
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
func systemSpecificLaunch(c context.Context, ve *venv.Env, cl *python.CommandLine, env environ.Env, dir string) error {
	return Exec(c, ve.Interpreter(), cl, env, dir, func() error {
		// Store our lock file descriptor as FD #3 (after #2, STDERR).
		lockFD := ve.LockHandle.LockFile().Fd()
		if lockFD == 3 {
			// "dup2" doesn't change flags if the source and destination file
			// descriptors are the same. Explicitly remove the close-on-exec flag, which
			// Go enables by default.
			if _, _, err := unix.Syscall(unix.SYS_FCNTL, lockFD, unix.F_SETFD, 0); err != 0 {
				return errors.Annotate(err, "could not remove close-on-exec for lock file").Err()
			}
		} else {
			// Use "dup2" to copy the file descriptor to #3 slot. This will also clear
			// its flags, including close-on-exec.
			if err := unix.Dup2(int(lockFD), 3); err != nil {
				return errors.Annotate(err, "could not dup2 lock file").Err()
			}
		}
		return nil
	})
}

func execImpl(c context.Context, argv []string, env environ.Env, dir string, setupFn func() error) error {
	// Change directory.
	if dir != "" {
		if err := os.Chdir(dir); err != nil {
			return errors.Annotate(err, "failed to chdir to %q", dir).Err()
		}
	}

	// At this point, ANY ERROR will be fatal (panic). We assume that each
	// operation may permanently alter our runtime environment.
	if setupFn != nil {
		if err := setupFn(); err != nil {
			panic(err)
		}
	}

	if err := syscall.Exec(argv[0], argv, env.Sorted()); err != nil {
		panic(errors.Annotate(err, "failed to execve %q", argv[0]).Err())
	}
	panic("must not return")
}
