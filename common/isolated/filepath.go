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

package isolated

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"

	"go.chromium.org/luci/common/logging"
)

// SetReadOnly sets or resets the write bit on a file or directory.
// Zaps out access to 'group' and 'others'.
func SetReadOnly(path string, readOnly bool) error {
	fi, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("failed to get lstat for %s: %v", path, err)
	}

	origMode := fi.Mode()
	mode := origMode
	if readOnly {
		mode &= syscall.S_IRUSR | syscall.S_IXUSR
	} else {
		mode |= syscall.S_IRUSR | syscall.S_IWUSR
		if runtime.GOOS != "windows" && mode.IsDir() {
			mode |= syscall.S_IXUSR
		}
	}
	if origMode&os.ModeSymlink != 0 {
		// Skip symlink.
		return nil
	}

	return os.Chmod(path, mode)
}

// MakeTreeReadOnly makes all the files in the directories read only.
// Also makes the directories read only, only if it makes sense on the platform.
// This means no file can be created or deleted.
func MakeTreeReadOnly(ctx context.Context, root string) error {
	logging.Debugf(ctx, "MakeTreeReadOnly(%s)", root)
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if runtime.GOOS != "windows" || !info.IsDir() {
			serr := SetReadOnly(path, true)
			if err == nil {
				return serr
			}
		}
		return err
	})
}

// MakeTreeFilesReadOnly makes all the files in the directories read only but
// not the directories themselves.
// This means files can be created or deleted.
func MakeTreeFilesReadOnly(ctx context.Context, root string) error {
	logging.Debugf(ctx, "MakeTreeFilesReadOnly(%s)", root)
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() {
			serr := SetReadOnly(path, true)
			if err == nil {
				return serr
			}
		} else if runtime.GOOS != "windows" {
			serr := SetReadOnly(path, false)
			if err == nil {
				return serr
			}
		}
		return err
	})
}

// MakeTreeWritable makes all the files in the directories writeable.
// Also makes the directories writeable, only if it makes sense on the platform.
// It is different from make_tree_deleteable() because it unconditionally
// affects the files.
func MakeTreeWritable(ctx context.Context, root string) error {
	logging.Debugf(ctx, "MakeTreeReadOnly(%s)", root)
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if runtime.GOOS != "windows" || !info.IsDir() {
			serr := SetReadOnly(path, false)
			if err == nil {
				return serr
			}
		}
		return err
	})
}
