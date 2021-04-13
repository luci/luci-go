// Copyright 2021 The LUCI Authors.
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

// Package fs implements a context-based filesystem abstraction layer.
//
// This uses `github.com/spf13/afero` under the hood.
package fs

import (
	"context"
	"os"
	"time"

	"github.com/spf13/afero"
)

var ctxKey = "holds an Fs"

// Fs is the filesystem interface.
type Fs afero.Fs

// File represents a file in the filesystem.
type File afero.File

// GetFs retrieves the filesystem currently set in this context.
//
// If there is no Fs set in `ctx`, this returns a new OsFs (i.e. directly
// passes through to "os" stdlib functions).
func GetFs(ctx context.Context) Fs {
	ret, ok := ctx.Value(&ctxKey).(Fs)
	if ok && ret != nil {
		return ret
	}
	return afero.NewOsFs()
}

// SetFs installs the given Fs into the context.
//
// If `fs` is nil, installs a new OsFs (i.e. directly passes through
// to "os" stdlib functions).
func SetFs(ctx context.Context, fs Fs) context.Context {
	if fs == nil {
		fs = afero.NewOsFs()
	}
	return context.WithValue(ctx, &ctxKey, fs)
}

// Create creates a file in the filesystem, returning the file and an
// error, if any happens.
func Create(ctx context.Context, name string) (File, error) {
	return GetFs(ctx).Create(name)
}

// Mkdir creates a directory in the filesystem, return an error if any
// happens.
func Mkdir(ctx context.Context, name string, perm os.FileMode) error {
	return GetFs(ctx).Mkdir(name, perm)
}

// MkdirAll creates a directory path and all parents that does not exist
// yet.
func MkdirAll(ctx context.Context, path string, perm os.FileMode) error {
	return GetFs(ctx).MkdirAll(path, perm)
}

// Open opens a file, returning it or an error, if any happens.
func Open(ctx context.Context, name string) (File, error) {
	return GetFs(ctx).Open(name)
}

// OpenFile opens a file using the given flags and the given mode.
func OpenFile(ctx context.Context, name string, flag int, perm os.FileMode) (File, error) {
	return GetFs(ctx).OpenFile(name, flag, perm)
}

// Remove removes a file identified by name, returning an error, if any
// happens.
func Remove(ctx context.Context, name string) error {
	return GetFs(ctx).Remove(name)
}

// RemoveAll removes a directory path and any children it contains. It
// does not fail if the path does not exist (return nil).
func RemoveAll(ctx context.Context, path string) error {
	return GetFs(ctx).RemoveAll(path)
}

// Rename renames a file.
func Rename(ctx context.Context, oldname, newname string) error {
	return GetFs(ctx).Rename(oldname, newname)
}

// Stat returns a FileInfo describing the named file, or an error, if any
// happens.
func Stat(ctx context.Context, name string) (os.FileInfo, error) {
	return GetFs(ctx).Stat(name)
}

// Chmod changes the mode of the named file to mode.
func Chmod(ctx context.Context, name string, mode os.FileMode) error {
	return GetFs(ctx).Chmod(name, mode)
}

// Chown changes the uid and gid of the named file.
func Chown(ctx context.Context, name string, uid, gid int) error {
	return GetFs(ctx).Chown(name, uid, gid)
}

//Chtimes changes the access and modification times of the named file
func Chtimes(ctx context.Context, name string, atime time.Time, mtime time.Time) error {
	return GetFs(ctx).Chtimes(name, atime, mtime)
}
