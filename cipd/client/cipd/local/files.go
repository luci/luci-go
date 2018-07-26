// Copyright 2014 The LUCI Authors.
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

package local

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"go.chromium.org/luci/common/logging"

	"golang.org/x/net/context"
)

// File defines a single file to be added or extracted from a package.
//
// All paths are slash separated (including symlink targets).
type File interface {
	// Name returns slash separated file path relative to a package root
	//
	// For example "dir/dir/file".
	Name() string

	// Size returns size of the file. 0 for symlinks.
	Size() uint64

	// Executable returns true if the file is executable.
	//
	// Only used for Linux\Mac archives. false for symlinks.
	Executable() bool

	// Writable returns true if the file is user-writable.
	Writable() bool

	// ModTime returns modification time of the file. Zero value means no mtime is
	// recorded.
	ModTime() time.Time

	// Symlink returns true if the file is a symlink.
	Symlink() bool

	// SymlinkTarget return a path the symlink is pointing to.
	SymlinkTarget() (string, error)

	// WinAttrs returns the windows attributes, if any.
	WinAttrs() WinAttrs

	// Open opens the regular file for reading.
	//
	// Returns error for symlink files.
	Open() (io.ReadCloser, error)
}

// WinAttrs represents the extra file attributes for windows.
type WinAttrs uint32

// These are the valid WinAttrs values. They may be OR'd together to form
// a mask. These match the windows GetFileAttributes values.
const (
	WinAttrHidden WinAttrs = 0x2
	WinAttrSystem WinAttrs = 0x4

	WinAttrsAll WinAttrs = WinAttrHidden | WinAttrSystem
)

func (w WinAttrs) String() string {
	ret := ""
	if w&WinAttrHidden != 0 {
		ret += "H"
	}
	if w&WinAttrSystem != 0 {
		ret += "S"
	}
	return ret
}

// CreateFileOptions provides arguments to Destination.CreateFile().
type CreateFileOptions struct {
	// Executable makes the file executable.
	Executable bool
	// Writable makes the file user-writable.
	Writable bool
	// ModTime, when non-zero, sets the mtime of the file.
	ModTime time.Time
	// WinAttrs is used on Windows.
	WinAttrs WinAttrs
}

// Destination knows how to create files and symlink when extracting a package.
//
// All paths are slash separated and relative to the destination root.
//
// 'CreateFile' and 'CreateSymlink' can be called concurrently.
type Destination interface {
	// CreateFile opens a writer to extract some package file to.
	CreateFile(ctx context.Context, name string, opts CreateFileOptions) (io.WriteCloser, error)
	// CreateSymlink creates a symlink (with absolute or relative target).
	CreateSymlink(ctx context.Context, name string, target string) error
}

// TransactionalDestination is a destination that supports transactions.
//
// It provides 'Begin' and 'End' methods: all calls to 'CreateFile' and
// 'CreateSymlink' should happen between them. No changes are really applied
// until End(true) is called. A call to End(false) discards any pending changes.
type TransactionalDestination interface {
	Destination

	// Begin initiates a new write transaction.
	Begin(ctx context.Context) error
	// End finalizes package extraction (commit or rollback, based on success).
	End(ctx context.Context, success bool) error
}

////////////////////////////////////////////////////////////////////////////////
// File system source.

type fileSystemFile struct {
	absPath       string
	name          string
	size          uint64
	executable    bool
	writable      bool
	modtime       time.Time
	symlinkTarget string
	winAttrs      WinAttrs
}

func (f *fileSystemFile) Name() string       { return f.name }
func (f *fileSystemFile) Size() uint64       { return f.size }
func (f *fileSystemFile) Executable() bool   { return f.executable }
func (f *fileSystemFile) Writable() bool     { return f.writable }
func (f *fileSystemFile) ModTime() time.Time { return f.modtime }
func (f *fileSystemFile) Symlink() bool      { return f.symlinkTarget != "" }
func (f *fileSystemFile) WinAttrs() WinAttrs { return f.winAttrs }

func (f *fileSystemFile) SymlinkTarget() (string, error) {
	if f.symlinkTarget != "" {
		return f.symlinkTarget, nil
	}
	return "", fmt.Errorf("not a symlink: %s", f.Name())
}

func (f *fileSystemFile) Open() (io.ReadCloser, error) {
	if f.Symlink() {
		return nil, fmt.Errorf("opening a symlink is not allowed: %s", f.Name())
	}
	return os.Open(f.absPath)
}

// ScanFilter is predicate used by ScanFileSystem to decide what files to skip.
type ScanFilter func(abs string) bool

// ScanOptions specify which file properties to preserve in the archive.
type ScanOptions struct {
	// PreserveModTime when true saves the file's modification time.
	PreserveModTime bool
	// PreserveWritable when true saves the file's writable bit for either user,
	// group or world (mask 0222), and converts it to user-only writable bit (mask
	// 0200).
	PreserveWritable bool
}

// ScanFileSystem returns all files in some file system directory in
// an alphabetical order. It returns only files, skipping directory entries
// (i.e. empty directories are completely invisible).
//
// ScanFileSystem follows symbolic links which have a target in
// <root>/<SiteServiceDir> (i.e. the .cipd folder). Other symlinks will
// will be preserved as symlinks (see Symlink() method of File interface).
// Symlinks with absolute targets inside of the root will be converted to
// relative targets inside of the root.
//
// It scans "dir" path, returning File objects that have paths relative to
// "root". It skips files and directories for which 'exclude(absolute path)'
// returns true. It also will always skip <root>/<SiteServiceDir>.
func ScanFileSystem(dir string, root string, exclude ScanFilter, scanOpts ScanOptions) ([]File, error) {
	root, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return nil, err
	}
	dir, err = filepath.Abs(filepath.Clean(dir))
	if err != nil {
		return nil, err
	}
	if !isSubpath(dir, root) {
		return nil, fmt.Errorf("scanned directory must be under root directory")
	}

	files := []File{}

	svcDir := filepath.Join(root, SiteServiceDir)
	err = filepath.Walk(dir, func(abs string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// skip the SiteServiceDir entirely
		if abs == svcDir {
			return filepath.SkipDir
		}

		if exclude != nil && abs != dir && exclude(abs) {
			if info.Mode().IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			f, err := WrapFile(abs, root, &info, scanOpts)
			if err != nil {
				return err
			}
			files = append(files, f)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}
	return files, nil
}

func getWritable(info os.FileInfo, opts ScanOptions) bool {
	return opts.PreserveWritable && (info.Mode().Perm()&0222) != 0
}

func getModTime(info os.FileInfo, opts ScanOptions) time.Time {
	if !opts.PreserveModTime {
		return time.Time{}
	}
	return info.ModTime()
}

// WrapFile constructs File object for some file in the file system, specified
// by its native absolute path 'abs' (subpath of 'root', also specified as
// a native absolute path). Returned File object has path relative to 'root'.
// If fileInfo is given, it will be used to grab file mode and size, otherwise
// os.Lstat will be used to get it. Recognizes symlinks.
func WrapFile(abs string, root string, fileInfo *os.FileInfo, scanOpts ScanOptions) (File, error) {
	if !filepath.IsAbs(abs) {
		return nil, fmt.Errorf("expecting absolute path, got this: %q", abs)
	}
	if !filepath.IsAbs(root) {
		return nil, fmt.Errorf("expecting absolute path, got this: %q", root)
	}
	if !isSubpath(abs, root) {
		return nil, fmt.Errorf("path %q is not under %q", abs, root)
	}

	var info os.FileInfo
	if fileInfo == nil {
		// Use Lstat to NOT follow symlinks (as os.Stat does).
		var err error
		info, err = os.Lstat(abs)
		if err != nil {
			return nil, err
		}
	} else {
		info = *fileInfo
	}

	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return nil, err
	}

	// Recognize symlinks as such, convert target to relative path, if needed.
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(abs)
		if err != nil {
			return nil, err
		}
		targetAbs := ""
		if filepath.IsAbs(target) {
			targetAbs = target
			// Convert absolute path pointing somewhere in "root" into a path
			// relative to the symlink file itself. Store other absolute paths as
			// they are. For example, it allows to package virtual env directory
			// that symlinks python binary from /usr/bin.
			if isSubpath(target, root) {
				target, err = filepath.Rel(filepath.Dir(abs), target)
				if err != nil {
					return nil, err
				}
			}
		} else {
			// Only relative paths that do not go outside "root" are allowed.
			// A package must not depend on its installation path.
			targetAbs = filepath.Clean(filepath.Join(filepath.Dir(abs), target))
			if !isSubpath(targetAbs, root) {
				return nil, fmt.Errorf(
					"Invalid symlink %s: a relative symlink pointing to a file outside of the package root", rel)
			}
		}

		// Symlinks with targets within SiteServiceDir get resolved as normal files.
		if isSubpath(targetAbs, filepath.Join(root, SiteServiceDir)) {
			abs = targetAbs
			info, err = os.Stat(abs)
			if err != nil {
				return nil, err
			}
		} else {
			return &fileSystemFile{
				absPath:       abs,
				name:          filepath.ToSlash(rel),
				writable:      getWritable(info, scanOpts),
				modtime:       getModTime(info, scanOpts),
				symlinkTarget: filepath.ToSlash(target),
			}, nil
		}
	}

	// Regular file.
	if info.Mode().IsRegular() {
		attrs, err := getWinFileAttributes(info)
		if err != nil {
			return nil, err
		}

		return &fileSystemFile{
			absPath:    abs,
			name:       filepath.ToSlash(rel),
			size:       uint64(info.Size()),
			executable: (info.Mode().Perm() & 0111) != 0,
			writable:   getWritable(info, scanOpts),
			modtime:    getModTime(info, scanOpts),
			winAttrs:   attrs,
		}, nil
	}

	return nil, fmt.Errorf("not a regular file or symlink: %s", abs)
}

// isSubpath returns true if 'path' is 'root' or is inside a subdirectory of
// 'root'. Both 'path' and 'root' should be given as a native paths. If any of
// paths can't be converted to an absolute path returns false.
func isSubpath(path, root string) bool {
	path, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return false
	}
	root, err = filepath.Abs(filepath.Clean(root))
	if err != nil {
		return false
	}
	if root == path {
		return true
	}
	if root[len(root)-1] != filepath.Separator {
		root += string(filepath.Separator)
	}
	return strings.HasPrefix(path, root)
}

// isCleanSlashPath returns true if path is a relative slash-separated path with
// no '..' or '.' entries and no '\\'. Basically "a/b/c/d".
func isCleanSlashPath(p string) bool {
	if p == "" {
		return false
	}
	if strings.ContainsRune(p, '\\') {
		return false
	}
	if p != path.Clean(p) {
		return false
	}
	if p[0] == '/' || p == ".." || strings.HasPrefix(p, "../") {
		return false
	}
	return true
}

////////////////////////////////////////////////////////////////////////////////
// Destination implementation based on FileSystem.

// fsDest implements Destination on top of some existing file system directory.
type fsDest struct {
	// FileSystem implementation to use for manipulating files.
	fs FileSystem
	// Where to create all files in, must be under 'fs' root.
	dest string
	// If true, CreateFile will use atomic rename trick to drop files.
	atomic bool

	// Currently open files.
	lock      sync.RWMutex
	openFiles map[string]*os.File
}

// ExistingDestination returns an object that knows how to create files in an
// existing directory in the file system.
//
// Will use a provided FileSystem object to operate on files if given, otherwise
// uses a default one. If FileSystem is provided, dest must be in a subdirectory
// of the given FileSystem root.
//
// Note that the returned object doesn't support transactional semantics, since
// transactionally writing a bunch of files into an existing directory is hard.
//
// See NewDestination for writing files into a completely new directory. This
// method supports transactions.
func ExistingDestination(dest string, fs FileSystem) Destination {
	if fs == nil {
		fs = NewFileSystem(filepath.Dir(dest), "")
	}
	return &fsDest{
		fs:        fs,
		dest:      dest,
		atomic:    true, // note: txnFSDest unsets this, see Begin
		openFiles: map[string]*os.File{},
	}
}

// numOpenFiles returns a number of currently open files.
//
// Used by txnFSDest to make sure all files are closed before finalizing the
// transaction.
func (d *fsDest) numOpenFiles() (n int) {
	d.lock.RLock()
	n = len(d.openFiles)
	d.lock.RUnlock()
	return
}

// prepareFilePath performs steps common to CreateFile and CreateSymlink.
//
// It does some validation, expands "name" to an absolute path and creates
// parent directories for a future file. Returns absolute path where the file
// should be put.
func (d *fsDest) prepareFilePath(ctx context.Context, name string) (string, error) {
	path := filepath.Clean(filepath.Join(d.dest, filepath.FromSlash(name)))
	if !isSubpath(path, d.dest) {
		return "", fmt.Errorf("invalid relative file name: %s", name)
	}
	if _, err := d.fs.EnsureDirectory(ctx, filepath.Dir(path)); err != nil {
		return "", err
	}
	return path, nil
}

func (d *fsDest) CreateFile(ctx context.Context, name string, opts CreateFileOptions) (io.WriteCloser, error) {
	path, err := d.prepareFilePath(ctx, name)
	if err != nil {
		return nil, err
	}

	d.lock.RLock()
	_, ok := d.openFiles[path]
	d.lock.RUnlock()
	if ok {
		return nil, fmt.Errorf("file %s is already open", name)
	}

	// Let the umask trim the file mode.
	var mode os.FileMode
	if opts.Executable {
		if runtime.GOOS == "windows" {
			logging.Warningf(ctx, "[data-loss] ignoring +x on %q", name)
		}
		mode = 0555
	} else {
		mode = 0444
	}
	if opts.Writable {
		mode |= 0222
	}
	if opts.WinAttrs != 0 && runtime.GOOS != "windows" {
		logging.Warningf(ctx, "[data-loss] ignoring +%s on %q", opts.WinAttrs, name)
	}

	// In atomic mode we write to a temp file and then rename it into 'path',
	// in non-atomic mode we just directly write into 'path' (perhaps overwriting
	// it).
	writeTo := path
	flags := os.O_CREATE | os.O_WRONLY
	if d.atomic {
		writeTo = tempFileName(path)
		flags |= os.O_EXCL // for improbable case of collision on temp file name
	}
	file, err := os.OpenFile(writeTo, flags, mode)
	if !d.atomic && os.IsPermission(err) {
		// Attempting to open an existing read-only file in non-atomic mode. Delete
		// it and try again. We are overriding it anyway.
		d.fs.EnsureFileGone(ctx, writeTo)
		file, err = os.OpenFile(writeTo, flags, mode)
	}
	if err != nil {
		return nil, err
	}

	// Close and delete (if it was a temp) on failures. Best effort.
	disarm := false
	defer func() {
		if !disarm {
			if clErr := file.Close(); clErr != nil {
				logging.Warningf(ctx, "Failed to close %s when cleaning up - %s", writeTo, clErr)
			}
			if d.atomic {
				// Nuke the unfinished temp file. The error is logged inside.
				d.fs.EnsureFileGone(ctx, writeTo)
			}
		}
	}()

	if err := setWinFileAttributes(writeTo, opts.WinAttrs); err != nil {
		return nil, err
	}

	d.lock.Lock()
	defer d.lock.Unlock()

	if _, ok := d.openFiles[path]; ok {
		return nil, fmt.Errorf("race condition when creating %s", name)
	}
	d.openFiles[path] = file
	disarm = true // skip the defer, we want to keep the file open

	return &hookedWriter{
		Writer: file,

		closeCb: func() (err error) {
			d.lock.Lock()
			delete(d.openFiles, path)
			d.lock.Unlock()

			err = file.Close()

			if !opts.ModTime.IsZero() {
				if chErr := os.Chtimes(writeTo, opts.ModTime, opts.ModTime); chErr != nil {
					logging.Warningf(ctx, "[data-loss] cannot set mtime for %s - %s", path, chErr)
				}
			}

			// In atomic mode need to rename the temp file into its final name.
			if writeTo != path {
				replErr := d.fs.Replace(ctx, writeTo, path)
				if err == nil { // prefer to return errors from Close()
					err = replErr
				}
			}
			return
		},
	}, nil
}

type hookedWriter struct {
	io.Writer
	closeCb func() error
}

func (w *hookedWriter) Close() error { return w.closeCb() }

func (d *fsDest) CreateSymlink(ctx context.Context, name string, target string) error {
	path, err := d.prepareFilePath(ctx, name)
	if err != nil {
		return err
	}

	// Forbid relative symlinks to files outside of the destination root.
	target = filepath.FromSlash(target)
	if !filepath.IsAbs(target) {
		targetAbs := filepath.Clean(filepath.Join(filepath.Dir(path), target))
		if !isSubpath(targetAbs, d.dest) {
			return fmt.Errorf("relative symlink is pointing outside of the destination dir: %s", name)
		}
	}

	return d.fs.EnsureSymlink(ctx, path, target)
}

////////////////////////////////////////////////////////////////////////////////
// TransactionalDestination implementation based on FileSystem.

// txnFSDest implements TransactionalDestination on top of a file system using
// "atomically rename a directory" trick for transactionality.
type txnFSDest struct {
	// FileSystem implementation to use for manipulating files.
	fs FileSystem
	// The final destination directory to be created when End(true) is called.
	dest string

	// The underlying FS destination which will be renamed into 'dest' in End().
	//
	// Setup to be a temp directory in Begin().
	fsDest *fsDest
}

// NewDestination returns a destination in the file system (a new directory)
// to extract a package to.
//
// Will use a provided FileSystem object to operate on files if given, otherwise
// use a default one. If FileSystem is provided, dir must be in a subdirectory
// of the given FileSystem root.
func NewDestination(dest string, fs FileSystem) TransactionalDestination {
	if fs == nil {
		fs = NewFileSystem(filepath.Dir(dest), "")
	}
	return &txnFSDest{fs: fs, dest: dest}
}

func (d *txnFSDest) Begin(ctx context.Context) error {
	if d.fsDest != nil {
		return fmt.Errorf("destination is already open")
	}

	// Ensure the parent directory of the destination directory exists, to be able
	// to create a temp subdir there.
	var err error
	if d.dest, err = d.fs.CwdRelToAbs(d.dest); err != nil {
		return err
	}
	if _, err := d.fs.EnsureDirectory(ctx, filepath.Dir(d.dest)); err != nil {
		return err
	}

	// Create the staging directory, on the same level as the destination
	// directory, so it can just be renamed into d.dest on completion. Let umask
	// trim the permissions appropriately. Note that it is not really a "private"
	// temp dir, since it will be eventually renamed into the "public" destination
	// that should be readable to everyone (sans umask).
	tempDir, err := tempDir(filepath.Dir(d.dest), "", 0777)
	if err != nil {
		return err
	}

	// Setup a non-txn destination that extracts into the staging directory. Note
	// that tempDir is totally owned by txnFSDest, and we ensure there are no
	// concurrent writes to the same destination file, so we disable 'atomic' mode
	// as it is unnecessary (and costs a bunch of syscalls per file).
	d.fsDest = ExistingDestination(tempDir, d.fs).(*fsDest)
	d.fsDest.atomic = false
	return nil
}

func (d *txnFSDest) CreateFile(ctx context.Context, name string, opts CreateFileOptions) (io.WriteCloser, error) {
	if d.fsDest == nil {
		return nil, fmt.Errorf("destination is not open")
	}
	return d.fsDest.CreateFile(ctx, name, opts)
}

func (d *txnFSDest) CreateSymlink(ctx context.Context, name string, target string) error {
	if d.fsDest == nil {
		return fmt.Errorf("destination is not open")
	}
	return d.fsDest.CreateSymlink(ctx, name, target)
}

func (d *txnFSDest) End(ctx context.Context, success bool) error {
	if d.fsDest == nil {
		return fmt.Errorf("destination is not open")
	}
	if leaking := d.fsDest.numOpenFiles(); leaking != 0 {
		return fmt.Errorf("not all files were closed (leaking %d files)", leaking)
	}

	// Clean up the temp dir and the state no matter what. On success it is
	// already gone (renamed into d.dest).
	defer func() {
		d.fs.EnsureDirectoryGone(ctx, d.fsDest.dest)
		d.fsDest = nil
	}()

	if success {
		return d.fs.Replace(ctx, d.fsDest.dest, d.dest)
	}
	// Let the defer clean the garbage left in fsDest.
	return nil
}
