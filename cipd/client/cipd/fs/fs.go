// Copyright 2015 The LUCI Authors.
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

package fs

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.chromium.org/luci/common/logging"
)

// FileSystem abstracts operations that touch single file system subpath.
//
// All functions operate in terms of native file paths. It exists mostly to hide
// differences between file system semantic on Windows and Linux\Mac.
type FileSystem interface {
	// Root returns absolute path to a directory FileSystem operates in.
	//
	// All FS actions are restricted to this directory.
	Root() string

	// CwdRelToAbs converts a path relative to cwd to an absolute one.
	//
	// If also verifies the path is under the root path of the FileSystem object.
	// If passed path is already absolute, just checks that it's under the root.
	CwdRelToAbs(path string) (string, error)

	// RootRelToAbs converts a path relative to Root() to an absolute one.
	//
	// It verifies the path is under the root path of the FileSystem object.
	// If passed path is already absolute, just checks that it's under the root.
	RootRelToAbs(path string) (string, error)

	// OpenFile opens a file and returns its file handle.
	//
	// Files opened with OpenFile can be safely manipulated by other
	// FileSystem functions.
	//
	// This differs from os.Open notably on Windows, where OpenFile ensures that
	// files are open with FILE_SHARE_DELETE permisson to enable them to be
	// atomically renamed without contention.
	OpenFile(path string) (*os.File, error)

	// Stat returns a FileInfo describing the named file, following symlinks.
	Stat(ctx context.Context, path string) (os.FileInfo, error)

	// Lstat returns a FileInfo describing the named file, not following symlinks.
	Lstat(ctx context.Context, path string) (os.FileInfo, error)

	// EnsureDirectory creates a directory at given native path.
	//
	// Does nothing it the path already exists and it is a directory. Replaces
	// existing file with a directory.
	//
	// It takes an absolute path or a path relative to the current working
	// directory and always returns absolute path.
	EnsureDirectory(ctx context.Context, path string) (string, error)

	// EnsureSymlink creates a symlink pointing to a target.
	//
	// It will create full directory path to the symlink if necessary.
	EnsureSymlink(ctx context.Context, path string, target string) error

	// EnsureFile creates a file and calls the function to write file content.
	//
	// It will create full directory path to the file if necessary.
	EnsureFile(ctx context.Context, path string, write func(*os.File) error) error

	// EnsureFileGone removes a file, logging the errors (if any).
	//
	// Missing file is not an error.
	EnsureFileGone(ctx context.Context, path string) error

	// EnsureDirectoryGone recursively removes a directory.
	EnsureDirectoryGone(ctx context.Context, path string) error

	// Renames oldpath to newpath.
	//
	// If newpath already exists (be it a file or a directory), removes it first.
	// If oldpath is a symlink, it's moved as is (e.g. as a symlink).
	Replace(ctx context.Context, oldpath, newpath string) error

	// CleanupTrash attempts to remove all files that ended up in the trash.
	//
	// This is a best effort operation. Errors are logged (either at Debug or
	// Warning level, depending on severity of the trash state).
	CleanupTrash(ctx context.Context)
}

// NewFileSystem returns default FileSystem implementation.
//
// It operates with files under a given path. All methods accept absolute paths
// or paths relative to current working directory. FileSystem will ensure they
// are under 'root' directory.
//
// It can also accept a path to a directory to put "trash" into: files that
// can't be removed because there are some processes keeping lock on them.
// This is useful on Windows when replacing running executables. The trash
// directory must be on the same disk as the root directory.
//
// It 'trash' is empty string, the trash directory will be created under
// 'root'.
func NewFileSystem(root, trash string) FileSystem {
	var err error
	if root, err = filepath.Abs(root); err != nil {
		return &fsImplErr{err}
	}
	if trash != "" {
		if trash, err = filepath.Abs(trash); err != nil {
			return &fsImplErr{err}
		}
	} else {
		trash = filepath.Join(root, ".cipd_trash")
	}
	return &fsImpl{root, trash}
}

// EnsureFile creates a file with the given content.
// It will create full directory path to the file if necessary.
func EnsureFile(ctx context.Context, fs FileSystem, path string, content io.Reader) error {
	return fs.EnsureFile(ctx, path, func(f *os.File) error {
		_, err := io.Copy(f, content)
		return err
	})
}

// fsImplErr implements FileSystem by returning given error from all methods.
type fsImplErr struct {
	err error
}

func (f *fsImplErr) Root() string                                                   { return "" }
func (f *fsImplErr) CwdRelToAbs(string) (string, error)                             { return "", f.err }
func (f *fsImplErr) RootRelToAbs(string) (string, error)                            { return "", f.err }
func (f *fsImplErr) OpenFile(string) (*os.File, error)                              { return nil, f.err }
func (f *fsImplErr) Stat(context.Context, string) (os.FileInfo, error)              { return nil, f.err }
func (f *fsImplErr) Lstat(context.Context, string) (os.FileInfo, error)             { return nil, f.err }
func (f *fsImplErr) EnsureDirectory(context.Context, string) (string, error)        { return "", f.err }
func (f *fsImplErr) EnsureSymlink(context.Context, string, string) error            { return f.err }
func (f *fsImplErr) EnsureFile(context.Context, string, func(*os.File) error) error { return f.err }
func (f *fsImplErr) EnsureFileGone(context.Context, string) error                   { return f.err }
func (f *fsImplErr) EnsureDirectoryGone(context.Context, string) error              { return f.err }
func (f *fsImplErr) Replace(context.Context, string, string) error                  { return f.err }
func (f *fsImplErr) CleanupTrash(context.Context)                                   {}

/// Implementation.

type fsImpl struct {
	root  string
	trash string
}

func (f *fsImpl) Root() string {
	return f.root
}

func (f *fsImpl) CwdRelToAbs(p string) (string, error) {
	p, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(f.root, p)
	if err != nil {
		return "", err
	}
	rel = filepath.ToSlash(rel)
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return "", fmt.Errorf("fs: path %s is outside of %s", p, f.root)
	}
	return p, nil
}

func (f *fsImpl) RootRelToAbs(p string) (string, error) {
	if filepath.IsAbs(p) {
		return f.CwdRelToAbs(p)
	}
	return f.CwdRelToAbs(filepath.Join(f.root, p))
}

func (f *fsImpl) OpenFile(p string) (*os.File, error) {
	p, err := f.CwdRelToAbs(p)
	if err != nil {
		return nil, err
	}
	return openFile(p)
}

func (f *fsImpl) Stat(ctx context.Context, p string) (os.FileInfo, error) {
	p, err := f.CwdRelToAbs(p)
	if err != nil {
		return nil, err
	}
	return os.Stat(p)
}

func (f *fsImpl) Lstat(ctx context.Context, p string) (os.FileInfo, error) {
	p, err := f.CwdRelToAbs(p)
	if err != nil {
		return nil, err
	}
	return os.Lstat(p)
}

func (f *fsImpl) EnsureDirectory(ctx context.Context, path string) (string, error) {
	path, err := f.CwdRelToAbs(path)
	if err != nil {
		return "", err
	}

	err = os.MkdirAll(path, 0777)

	// ENOTDIR/ERROR_DIRECTORY happens if 'path' or some of its parents is a not a
	// directory. We want to delete such file so 'path' can be built up to be
	// a directory.
	//
	// In this scenario IsNotExist is specifically for Windows, which for whatever
	// reason returns it sometimes instead of ERROR_DIRECTORY.
	//
	// See fs_(posix|windows).go for implementation of isNotDir.
	if os.IsNotExist(err) || isNotDir(err) {
		cur := path
		for {
			fi, err := os.Lstat(cur)

			// If 'cur' doesn't exist yet or some of its parent is not a directory, go
			// up until we find this non-directory.
			if os.IsNotExist(err) || isNotDir(err) {
				dir := filepath.Dir(cur)
				if dir == cur {
					break // reached the root, MkdirAll must succeed then
				}
				cur = dir
				continue
			}

			// Some fatal error in Lstat (most likely permissions)?
			if err != nil {
				return "", err
			}

			// Found no non-directories in 'path', MkdirAll mush succeed then.
			if fi.IsDir() {
				break
			}

			// Found a non-directory element! Delete it and try MkdirAll again, which
			// should succeed now.
			if err := f.EnsureFileGone(ctx, cur); err != nil {
				return "", err
			}
			break
		}
		// Try again. Once.
		err = os.MkdirAll(path, 0777)
	}

	if err != nil {
		return "", err
	}
	return path, nil
}

func (f *fsImpl) EnsureFile(ctx context.Context, path string, write func(*os.File) error) error {
	path, err := f.CwdRelToAbs(path)
	if err != nil {
		return err
	}
	if _, err := f.EnsureDirectory(ctx, filepath.Dir(path)); err != nil {
		return err
	}

	temp := tempFileName(path)

	// Make sure to cleanup garbage on errors or panics.
	ok := false
	defer func() {
		if !ok {
			if err := os.Remove(temp); err != nil && !os.IsNotExist(err) {
				logging.Warningf(ctx, "fs: failed to remove %s - %s", temp, err)
			}
		}
	}()

	// Create a temp file with new content.
	if err := createFile(temp, write); err != nil {
		return err
	}

	// Replace the current file (if there's one) with a new one. Use nuclear
	// version (f.Replace) instead of simple atomicReplace to handle various edge
	// cases handled by the nuclear version (e.g replacing a non-empty directory).
	if err := f.Replace(ctx, temp, path); err != nil {
		return err
	}

	ok = true
	return nil
}

func (f *fsImpl) EnsureSymlink(ctx context.Context, path string, target string) error {
	path, err := f.CwdRelToAbs(path)
	if err != nil {
		return err
	}
	if existing, _ := os.Readlink(path); existing == target {
		return nil
	}
	if _, err := f.EnsureDirectory(ctx, filepath.Dir(path)); err != nil {
		return err
	}

	// Create a new symlink file, can't modify existing one in place.
	temp := tempFileName(path)
	if err := os.Symlink(target, temp); err != nil {
		return err
	}

	// Replace the current symlink with a new one. Use nuclear version (f.Replace)
	// instead of simple atomicReplace to handle various edge cases handled by
	// the nuclear version (e.g replacing a non-empty directory).
	if err := f.Replace(ctx, temp, path); err != nil {
		if err2 := os.Remove(temp); err2 != nil && !os.IsNotExist(err2) {
			logging.Warningf(ctx, "fs: failed to remove %s - %s", temp, err2)
		}
		return err
	}

	return nil
}

func (f *fsImpl) EnsureFileGone(ctx context.Context, path string) error {
	path, err := f.CwdRelToAbs(path)
	if err != nil {
		return err
	}
	if err = os.Remove(path); err != nil && !os.IsNotExist(err) {
		logging.Warningf(ctx, "fs: failed to remove %s - %s", path, err)
		if f.moveToTrash(ctx, path) == "" {
			return err
		}
	}
	return nil
}

func (f *fsImpl) EnsureDirectoryGone(ctx context.Context, path string) error {
	path, err := f.CwdRelToAbs(path)
	if err != nil {
		return err
	}
	// Make directory "disappear" instantly by renaming it first.
	temp := tempFileName(path)
	if err = atomicRename(path, temp); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		logging.Warningf(ctx, "fs: failed to rename directory %s - %s", path, err)
		return err
	}
	if err = os.RemoveAll(temp); err != nil {
		logging.Warningf(ctx, "fs: failed to remove directory %s - %s", temp, err)
		return err
	}
	return nil
}

func (f *fsImpl) Replace(ctx context.Context, oldpath, newpath string) error {
	oldpath, err := f.CwdRelToAbs(oldpath)
	if err != nil {
		return err
	}
	newpath, err = f.CwdRelToAbs(newpath)
	if err != nil {
		return err
	}
	if oldpath == newpath {
		return nil
	}

	// Make sure oldpath exists before doing heavy stuff.
	if _, err = os.Lstat(oldpath); err != nil {
		return err
	}

	// Make parent directory of newpath.
	if _, err = f.EnsureDirectory(ctx, filepath.Dir(newpath)); err != nil {
		return err
	}

	// Try a regular move first. Replaces files and empty directories.
	if err = atomicRename(oldpath, newpath); err == nil {
		return nil
	}

	// This code path is hit it two cases:
	//
	// 1. 'newpath' is non-empty directory.
	// 2. 'newpath' is locked running executable on Windows.
	//
	// We try to move existing path away into the trash directory. This
	// returns "" if the path is no longer there or the file can't be moved
	// for some reason. In later case the rename below will most probably
	// also fail, and the error will be properly propagated.
	//
	// Note that this fails for files open for exclusive write on Windows,
	// they are unmovable.
	trash := f.moveToTrash(ctx, newpath)

	// 'newpath' now should be available.
	if err := atomicRename(oldpath, newpath); err != nil {
		logging.Warningf(ctx, "fs: failed to rename(%q, %q) - %s", oldpath, newpath, err)
		// Try to return the path back... May be too late already.
		if trash != "" {
			if err := atomicRename(trash, newpath); err != nil {
				logging.Warningf(ctx, "fs: failed to rename(%q, %q) after unsuccessful move - %s", trash, newpath, err)
			}
		}
		return err
	}

	// If 'newpath' was a directory, we can actually completely delete it now.
	// This will fail for locked files though. They are opportunistically cleaned
	// up in CleanupTrash.
	if trash != "" {
		f.cleanupTrashedFile(ctx, trash)
	}

	return nil
}

func (f *fsImpl) CleanupTrash(ctx context.Context) {
	trashed, err := ioutil.ReadDir(f.trash)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		logging.Warningf(ctx, "fs: cannot read the trash dir - %s", err)
		return
	}

	if len(trashed) > 0 {
		logging.Debugf(ctx, "fs: cleaning up trash (%d items)...", len(trashed))
	}
	undead := 0
	for _, file := range trashed {
		if f.cleanupTrashedFile(ctx, filepath.Join(f.trash, file.Name())) != nil {
			undead++
		}
	}

	switch {
	case undead > 100:
		logging.Warningf(ctx, "fs: too many undeletable files (%d) in the trash dir %s", undead, f.trash)
	case undead == 0:
		// Remove the empty directory too. Not a big deal if fails.
		os.Remove(f.trash)
	}
}

// moveToTrash is best-effort function to move file or dir to trash.
//
// It returns path to a moved file in trash, or empty string if it can't
// be done.
func (f *fsImpl) moveToTrash(ctx context.Context, path string) string {
	if err := os.MkdirAll(f.trash, 0777); err != nil {
		logging.Warningf(ctx, "fs: can't create trash directory %q - %s", f.trash, err)
		return ""
	}
	trashed := filepath.Join(f.trash, pseudoRand())
	if err := atomicRename(path, trashed); err != nil {
		if !os.IsNotExist(err) {
			logging.Warningf(ctx, "fs: failed to rename(%q, %q) - %s", path, trashed, err)
		}
		return ""
	}
	return trashed
}

// cleanupTrashedFile is best-effort function to remove a trashed file or dir.
//
// Logs errors.
func (f *fsImpl) cleanupTrashedFile(ctx context.Context, path string) error {
	if filepath.Dir(path) != f.trash {
		return fmt.Errorf("not in the trash - %q", path)
	}
	err := os.RemoveAll(path)
	if err != nil {
		logging.Debugf(ctx, "fs: failed to cleanup trashed file - %s", err)
	}
	return err
}

/// Internal stuff.

var (
	lastUsedTime     int64
	lastUsedTimeLock sync.Mutex
)

// tempFileName returns "random enough" path in the same directory as a given
// path. It's not actively trying to be secure. Assumes that 'path' is not world
// writable (i.e. not /tmp).
//
// Doesn't check for existence of the file at the given path (e.g. there may be
// conflicts, but the probability should be small).
//
// TODO(vadimsh): Maybe we should change that? This is dangerous assumption.
// Simple Exists(...) check would reduce the likelihood of a conflict
// significantly in exchange for some modest performance impact.
func tempFileName(path string) string {
	return filepath.Join(filepath.Dir(path), pseudoRand())
}

// pseudoRand returns "random enough" string that can be used in file system
// paths of temp files.
func pseudoRand() string {
	ts := time.Now().UnixNano()
	lastUsedTimeLock.Lock()
	if ts <= lastUsedTime {
		ts = lastUsedTime + 1
	}
	lastUsedTime = ts
	lastUsedTimeLock.Unlock()

	// Hash the state to get a smaller pseudorandom string.
	h := sha256.New()
	fmt.Fprintf(h, "%v_%v", os.Getpid(), ts)
	sum := h.Sum(nil)
	digest := base64.RawURLEncoding.EncodeToString(sum)
	return digest[:12]
}

// TempDir is like ioutil.TempDir(dir, ""), but uses shorter path suffixes.
//
// Path length is constraint resource of Windows.
//
// Supposed to be used only in cases when the probability of a conflict is low
// (e.g. when 'dir' is some "private" directory, not global /tmp or something
// like that).
//
// Additionally, this allows you to pass mode (which will respect the process
// umask). To get ioutils.TempDir behavior, pass 0700 for the mode.
func TempDir(dir string, prefix string, mode os.FileMode) (name string, err error) {
	for i := 0; i < 1000; i++ {
		try := filepath.Join(dir, prefix+pseudoRand())
		err = os.Mkdir(try, mode)
		if os.IsExist(err) {
			continue
		}
		if err == nil {
			name = try
		}
		break
	}
	return
}

// createFile creates a file and calls the function to write file content.
//
// Does NOT cleanup the file if something fails midway.
func createFile(path string, write func(*os.File) error) (err error) {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() {
		closeErr := file.Close()
		if err == nil {
			err = closeErr
		}
	}()
	return write(file)
}
