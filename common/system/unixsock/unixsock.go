// Copyright 2025 The LUCI Authors.
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

// Package unixsock helps in creating symlinks for Unix domain socket paths.
//
// Unix domain socket address (which is a file system path) is limited to 108
// characters on Linux, which is often not enough. This package aids in creating
// short symlinks to such sockets.
//
// Does absolutely nothing on Windows.
package unixsock

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"

	"go.chromium.org/luci/common/errors"
)

// Shorten takes an absolute path to a Unix socket and tries to create a short
// path to it (using directory symlinks).
//
// Returns the short path that will fit into Unix domain socket address limits
// and a function that must be called to clean up after the socket is no longer
// needed.
//
// Intended to be used right before either binding listening unix domain socket
// or dialing it. The given file path doesn't need to exist yet, but the
// directory it is in must be present.
//
// Note the returned path is not safe to share across processes since it may be
// relying the '/proc/self' symlink. Share the full "long" path and make the
// subprocess call Shorten(...) itself on it.
func Shorten(long string) (short string, cleanup func() error, err error) {
	return shortenImpl(long, true)
}

// shortenImpl exists to allow unit testing tmpSymlinkPath code path on Linux.
func shortenImpl(long string, allowSelfProc bool) (short string, cleanup func() error, err error) {
	if !filepath.IsAbs(long) {
		return "", nil, errors.Fmt("not an absolute path %q", long)
	}

	if runtime.GOOS == "windows" {
		return long, func() error { return nil }, nil
	}

	maxLen := len(syscall.RawSockaddrUnix{}.Path)
	if len(long) < maxLen {
		return long, func() error { return nil }, nil
	}

	// On Linux we can abuse existing /self/proc/fd/... symlinks to get a short
	// path without creating any additional garbage in the file system or making
	// assumptions.
	if allowSelfProc && runtime.GOOS == "linux" {
		short, cleanup, err = selfProcPath(long)
		if err == nil && len(short) < maxLen {
			return
		}
		if cleanup != nil {
			_ = cleanup()
		}
	}

	// Otherwise create a symlink in `/tmp/...` specifically (not os.TempDir(),
	// since $TMPDIR is often overridden to be a long path).
	short, cleanup, err = tmpSymlinkPath(long)
	if err != nil || len(short) < maxLen {
		return
	}
	if cleanup != nil {
		_ = cleanup()
	}
	return "", nil, errors.Fmt("failed to shorten unix socket path %q to be less than %d bytes", long, maxLen)
}

func selfProcPath(long string) (short string, cleanup func() error, err error) {
	d, err := os.Open(filepath.Dir(long))
	if err != nil {
		return "", nil, err
	}
	// Verify "/proc/self" is actually present.
	symlink := fmt.Sprintf("/proc/self/fd/%d", d.Fd())
	if _, err := os.Lstat(symlink); err != nil {
		_ = d.Close()
		return "", nil, err
	}
	return filepath.Join(symlink, filepath.Base(long)), d.Close, nil
}

func tmpSymlinkPath(long string) (short string, cleanup func() error, err error) {
	// Note we want the leaf element of the path be a unix socket. Thus we can't
	// create a symlink that points directly to the socket (then the leaf element
	// of the path will be a symlink!). We instead need to create a symlink for
	// the directory that contains the socket.
	tmp, err := os.MkdirTemp("/tmp", "usock")
	if err != nil {
		return "", nil, errors.Fmt("creating a temp directory to hold the socket directory symlink: %w", err)
	}
	err = os.Symlink(filepath.Dir(long), filepath.Join(tmp, "d"))
	if err != nil {
		_ = os.RemoveAll(tmp)
		return "", nil, errors.Fmt("creating a symlink to the socket directory: %w", err)
	}
	return filepath.Join(tmp, "d", filepath.Base(long)), func() error { return os.RemoveAll(tmp) }, nil
}
