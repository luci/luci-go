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

package filesystem

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
)

func setReadOnly(path string, fi os.FileInfo, readOnly bool) error {
	mode := fi.Mode()
	if mode&os.ModeSymlink != 0 {
		// Skip symlink.
		return nil
	}

	if readOnly {
		mode &= syscall.S_IRUSR | syscall.S_IXUSR
	} else {
		mode |= syscall.S_IRUSR | syscall.S_IWUSR
		if runtime.GOOS != "windows" && mode.IsDir() {
			mode |= syscall.S_IXUSR
		}
	}
	return os.Chmod(path, mode)
}

// SetReadOnly sets or resets the write bit on a file or directory.
// Zaps out access to 'group' and 'others'.
func SetReadOnly(path string, readOnly bool) error {
	fi, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("failed to get lstat for %s: %v", path, err)
	}
	return setReadOnly(path, fi, readOnly)
}

// MakeTreeReadOnly makes all the files in the directories read only.
// Also makes the directories read only, only if it makes sense on the platform.
// This means no file can be created or deleted.
func MakeTreeReadOnly(ctx context.Context, root string) error {
	logging.Debugf(ctx, "MakeTreeReadOnly(%s)", root)
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if runtime.GOOS != "windows" || !info.IsDir() {
			return setReadOnly(path, info, true)
		}
		return nil
	})
}

// MakeTreeFilesReadOnly makes all the files in the directories read only but
// not the directories themselves.
// This means files can be created or deleted.
func MakeTreeFilesReadOnly(ctx context.Context, root string) error {
	logging.Debugf(ctx, "MakeTreeFilesReadOnly(%s)", root)
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			return setReadOnly(path, info, true)
		} else if runtime.GOOS != "windows" {
			return setReadOnly(path, info, false)
		}
		return nil
	})
}

// MakeTreeWritable makes all the files in the directories writeable.
// Also makes the directories writeable, only if it makes sense on the platform.
func MakeTreeWritable(ctx context.Context, root string) error {
	logging.Debugf(ctx, "MakeTreeReadOnly(%s)", root)
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if runtime.GOOS != "windows" || !info.IsDir() {
			return setReadOnly(path, info, false)
		}
		return nil
	})
}

// ResolveSymlink recursively resolves simlink and returns absolute path that is
// not symlink with stat.
func ResolveSymlink(path string) (string, os.FileInfo, error) {
	var stat os.FileInfo
	for {
		var err error
		stat, err = os.Lstat(path)
		if err != nil {
			return "", stat, errors.Fmt("failed to call Lstat(%s): %w", path, err)
		}
		if (stat.Mode() & os.ModeSymlink) == 0 {
			break
		}

		link, err := os.Readlink(path)
		if err != nil {
			return "", stat, errors.Fmt("failed to call Readlink(%s): %w", path, err)
		}

		if filepath.IsAbs(link) {
			path = link
		} else {
			path = filepath.Join(filepath.Dir(path), link)
		}
	}

	return path, stat, nil
}

// GetFilenameNoExt returns the base file name without the extension.
func GetFilenameNoExt(path string) string {
	name := filepath.Base(path)
	if i := strings.LastIndexByte(name, '.'); i != -1 {
		name = name[:i]
	}
	return name
}
