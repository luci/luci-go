// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package local

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/luci/luci-go/common/logging"
)

// FileSystem is a high-level interface for operations that touch single file
// system subpath. All functions operate in terms of native file paths. It
// exists mostly to hide differences between file system semantic on Windows and
// Linux\Mac.
type FileSystem interface {
	// Root returns absolute path to a directory FileSystem operates in. All FS
	// actions are restricted to this directory.
	Root() string

	// CwdRelToAbs converts a path relative to cwd to an absolute one and verifies
	// it is under the root path of the FileSystem object. If passed path is
	// already absolute, just checks that it's under the root.
	CwdRelToAbs(path string) (string, error)

	// RootRelToAbs converts a path relative to Root() to an absolute one and
	// verifies it is under the root path of the FileSystem object. If passed path
	// is already absolute, just checks that it's under the root.
	RootRelToAbs(path string) (string, error)

	// EnsureDirectory creates a directory at given native path if it doesn't
	// exist yet. It takes an absolute path or a path relative to the current
	// working directory and always returns absolute path.
	EnsureDirectory(path string) (string, error)

	// EnsureSymlink creates a symlink pointing to a target. It will create full
	// directory path to the symlink if necessary.
	EnsureSymlink(path string, target string) error

	// EnsureFile creates a file with given content. If will create full directory
	// path to the file if necessary.
	EnsureFile(path string, body []byte, perm os.FileMode) error

	// EnsureFileGone removes a file, logging the errors (if any). Missing file is
	// not an error.
	EnsureFileGone(path string) error

	// EnsureDirectoryGone recursively removes a directory.
	EnsureDirectoryGone(path string) error

	// Renames oldpath to newpath. If newpath already exists (be it a file or a
	// directory), removes it first. If oldpath is a symlink, it's moved as is
	// (e.g. as a symlink).
	Replace(oldpath, newpath string) error
}

// NewFileSystem returns default FileSystem implementation that operates with
// files under a given path. All methods accept absolute paths or paths relative
// to current working directory. FileSystem will ensure they are under 'root'
// directory.
func NewFileSystem(root string, logger logging.Logger) FileSystem {
	if logger == nil {
		logger = logging.Null()
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return &fsImplErr{err}
	}
	return &fsImpl{abs, logger}
}

// fsImplErr implements FileSystem by returning given error from all methods.
type fsImplErr struct {
	err error
}

func (f *fsImplErr) Root() string                                                { return "" }
func (f *fsImplErr) CwdRelToAbs(path string) (string, error)                     { return "", f.err }
func (f *fsImplErr) RootRelToAbs(path string) (string, error)                    { return "", f.err }
func (f *fsImplErr) EnsureDirectory(path string) (string, error)                 { return "", f.err }
func (f *fsImplErr) EnsureSymlink(path string, target string) error              { return f.err }
func (f *fsImplErr) EnsureFile(path string, body []byte, perm os.FileMode) error { return f.err }
func (f *fsImplErr) EnsureFileGone(path string) error                            { return f.err }
func (f *fsImplErr) EnsureDirectoryGone(path string) error                       { return f.err }
func (f *fsImplErr) Replace(oldpath, newpath string) error                       { return f.err }

/// Implementation.

type fsImpl struct {
	root   string
	logger logging.Logger
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

func (f *fsImpl) EnsureDirectory(path string) (string, error) {
	path, err := f.CwdRelToAbs(path)
	if err != nil {
		return "", err
	}
	// MkdirAll returns nil if path already exists.
	if err = os.MkdirAll(path, 0777); err != nil {
		return "", err
	}
	// TODO(vadimsh): Do not fail if path already exists and is a regular file?
	return path, nil
}

func (f *fsImpl) EnsureFile(path string, body []byte, perm os.FileMode) error {
	path, err := f.CwdRelToAbs(path)
	if err != nil {
		return err
	}
	if _, err := f.EnsureDirectory(filepath.Dir(path)); err != nil {
		return err
	}

	// Create a temp file with new content.
	temp := tempFileName(path)
	if err := ioutil.WriteFile(temp, body, perm); err != nil {
		return err
	}

	// Replace the current file (if there's one) with a new one. Use nuclear
	// version (f.Replace) instead of simple atomicReplace to handle various edge
	// cases handled by the nuclear version (e.g replacing a non-empty directory).
	if err := f.Replace(temp, path); err != nil {
		if err2 := os.Remove(temp); err2 != nil && !os.IsNotExist(err2) {
			f.logger.Warningf("fs: failed to remove %s - %s", temp, err2)
		}
		return err
	}

	return nil
}

func (f *fsImpl) EnsureSymlink(path string, target string) error {
	path, err := f.CwdRelToAbs(path)
	if err != nil {
		return err
	}
	if existing, _ := os.Readlink(path); existing == target {
		return nil
	}
	if _, err := f.EnsureDirectory(filepath.Dir(path)); err != nil {
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
	if err := f.Replace(temp, path); err != nil {
		if err2 := os.Remove(temp); err2 != nil && !os.IsNotExist(err2) {
			f.logger.Warningf("fs: failed to remove %s - %s", temp, err2)
		}
		return err
	}

	return nil
}

func (f *fsImpl) EnsureFileGone(path string) error {
	path, err := f.CwdRelToAbs(path)
	if err != nil {
		return err
	}
	if err = os.Remove(path); err != nil && !os.IsNotExist(err) {
		f.logger.Warningf("fs: failed to remove %s - %s", path, err)
		return err
	}
	return nil
}

func (f *fsImpl) EnsureDirectoryGone(path string) error {
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
		f.logger.Warningf("fs: failed to rename directory %s - %s", path, err)
		return err
	}
	if err = os.RemoveAll(temp); err != nil {
		f.logger.Warningf("fs: failed to remove directory %s - %s", temp, err)
		return err
	}
	return nil
}

func (f *fsImpl) Replace(oldpath, newpath string) error {
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
	if _, err = f.EnsureDirectory(filepath.Dir(newpath)); err != nil {
		return err
	}

	// Try a regular move first. Replaces files and empty directories.
	if err = atomicRename(oldpath, newpath); err == nil {
		return nil
	}

	// Move existing path away, if it is there.
	temp := tempFileName(newpath)
	if err = atomicRename(newpath, temp); err != nil {
		if !os.IsNotExist(err) {
			f.logger.Warningf("fs: failed to rename(%v, %v) - %s", newpath, temp, err)
			return err
		}
		temp = ""
	}

	// 'newpath' now should be available.
	if err := atomicRename(oldpath, newpath); err != nil {
		f.logger.Warningf("fs: failed to rename(%v, %v) - %s", oldpath, newpath, err)
		// Try to return the path back... May be too late already.
		if temp != "" {
			if err := atomicRename(temp, newpath); err != nil {
				f.logger.Errorf("fs: failed to rename(%v, %v) after unsuccessful move - %s", temp, newpath, err)
			}
		}
		return err
	}

	// Cleanup the garbage left. Not a error if fails.
	if temp != "" {
		if err := f.EnsureDirectoryGone(temp); err != nil {
			f.logger.Warningf("fs: failed to cleanup garbage after file replace - %s", err)
		}
	}
	return nil
}

/// Internal stuff.

var (
	lastUsedTime     int64
	lastUsedTimeLock sync.Mutex
)

// tempFileName returns "random enough" path in the same directory as a given
// path. It's not actively trying to be secure. Assumes that 'path' is not world
// writable (i.e. not /tmp).
func tempFileName(path string) string {
	return filepath.Join(filepath.Dir(path), "tmp"+pseudoRand())
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
	return fmt.Sprintf("%v_%v", os.Getpid(), ts)
}
