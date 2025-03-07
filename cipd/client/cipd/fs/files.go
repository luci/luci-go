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

package fs

import (
	"context"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/cipd/common/cipderr"
)

const (
	// SiteServiceDir is a name of the directory inside an installation root
	// reserved for cipd stuff.
	SiteServiceDir = ".cipd"
)

// File defines a single file to be added or extracted from a package.
//
// All paths are slash separated (including symlink targets).
//
// Errors are annotated with error codes.
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
//
// Errors are annotated with error codes.
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
//
// Errors are annotated with error codes.
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
	return "", errors.Reason("%q: not a symlink", f.Name()).Tag(cipderr.IO).Err()
}

func (f *fileSystemFile) Open() (io.ReadCloser, error) {
	if f.Symlink() {
		return nil, errors.Reason("%q: opening a symlink is not allowed", f.Name()).Tag(cipderr.IO).Err()
	}
	r, err := os.Open(f.absPath)
	if err != nil {
		return nil, errors.Annotate(err, "opening %q", f.Name()).Tag(cipderr.IO).Err()
	}
	return r, nil
}

// ScanFilter is predicate used by ScanFileSystem to decide what files to skip.
//
// It receives a path being judged and returns true if it should be skipped.
// The path is a filepath relative to the starting scanning directory (i.e.
// 'dir' in ScanFileSystem).
type ScanFilter func(rel string) bool

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
// alphabetical order.
//
// It scans 'dir' path, returning File objects that have paths relative to
// 'root'. Returns only files, skipping directory entries (i.e. empty
// directories are completely "invisible").
//
// It also skips files and directories for which 'exclude(path relative to dir)'
// returns true. It also always skips <root>/<SiteServiceDir>, but follows
// symlinks that point there.
//
// Symlinks are preserved (see Symlink() method of File interface), but the
// following rules apply:
//   - Relative symlinks pointing outside of the root are forbidden.
//   - An absolute symlink with the target outside the root is kept as such.
//   - An absolute symlink with the target within the root becomes relative.
//
// If 'dir' is a symlink itself, it is emitted as such (with all above rules
// applying), except when 'dir == root' (lexicographically, as cleaned absolute
// paths). In that case the symlink is followed and whatever it points to is
// used as new 'dir' and 'root'. This is completely transparent in the output
// though, since File uses only relative paths. Without this exception using
// a symlink as a package root leads to very surprising errors.
//
// Errors are annotated with error codes.
func ScanFileSystem(dir string, root string, exclude ScanFilter, scanOpts ScanOptions) ([]File, error) {
	dir, err := filepath.Abs(filepath.Clean(dir))
	if err != nil {
		return nil, errors.Annotate(err, "bad input path").Tag(cipderr.BadArgument).Err()
	}
	root, err = filepath.Abs(filepath.Clean(root))
	if err != nil {
		return nil, errors.Annotate(err, "bad root path").Tag(cipderr.BadArgument).Err()
	}

	// If we are scanning 'root' directly, it doesn't matter if it is itself
	// a symlink. We should never try to package it as such, it is useless.
	if dir == root {
		resolved, err := filepath.EvalSymlinks(dir)
		if err != nil {
			return nil, errors.Annotate(err, "resolving symlinks in the input dir path").Tag(cipderr.IO).Err()
		}
		dir = resolved
		root = resolved
	}

	if !IsSubpath(dir, root) {
		return nil, errors.Reason("the scanned directory must be under the root directory").Tag(cipderr.BadArgument).Err()
	}

	files := []File{}

	svcDir := filepath.Join(root, SiteServiceDir)
	err = filepath.WalkDir(dir, func(abs string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip the SiteServiceDir entirely.
		if abs == svcDir {
			return fs.SkipDir
		}

		// Apply the exclusion filter. Also skip files for which filepath.Rel
		// returns an error.
		if exclude != nil && abs != dir {
			if rel, err := filepath.Rel(dir, abs); err != nil || exclude(rel) {
				if entry.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}

		// Skip exotic file types, we deal only with regular files and symlinks.
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
		if cipderr.ToCode(err) != cipderr.Unknown {
			return nil, err // already annotated, do not clobber the code
		}
		return nil, errors.Annotate(err, "scanning input directory").Tag(cipderr.IO).Err()
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
// a native absolute path).
//
// Returned File object has path relative to 'root'. If fileInfo is given, it
// will be used to grab file mode and size, otherwise os.Lstat will be used to
// get it.
//
// Recognizes symlinks.
//
// Errors are annotated with error codes.
func WrapFile(abs string, root string, fileInfo *os.FileInfo, scanOpts ScanOptions) (File, error) {
	if !filepath.IsAbs(abs) {
		return nil, errors.Reason("expecting absolute file path, got %q", abs).Tag(cipderr.BadArgument).Err()
	}
	if !filepath.IsAbs(root) {
		return nil, errors.Reason("expecting absolute root path, got %q", root).Tag(cipderr.BadArgument).Err()
	}
	if !IsSubpath(abs, root) {
		return nil, errors.Reason("path %q is not under %q", abs, root).Tag(cipderr.BadArgument).Err()
	}

	var info os.FileInfo
	if fileInfo == nil {
		// Use Lstat to NOT follow symlinks (as os.Stat does).
		var err error
		info, err = os.Lstat(abs)
		if err != nil {
			return nil, errors.Annotate(err, "checking file info").Tag(cipderr.IO).Err()
		}
	} else {
		info = *fileInfo
	}

	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return nil, errors.Annotate(err, "getting relative path").Tag(cipderr.IO).Err()
	}

	// Recognize symlinks as such, convert target to relative path, if needed.
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(abs)
		if err != nil {
			return nil, errors.Annotate(err, "reading symlink target").Tag(cipderr.IO).Err()
		}
		targetAbs := ""
		if filepath.IsAbs(target) {
			targetAbs = target
			// Convert absolute path pointing somewhere in "root" into a path
			// relative to the symlink file itself. Store other absolute paths as
			// they are. For example, it allows to package virtual env directory
			// that symlinks python binary from /usr/bin.
			if IsSubpath(target, root) {
				target, err = filepath.Rel(filepath.Dir(abs), target)
				if err != nil {
					return nil, errors.Annotate(err, "converting symlink target path to be relative").Tag(cipderr.IO).Err()
				}
			}
		} else {
			// Only relative paths that do not go outside "root" are allowed.
			// A package must not depend on its installation path.
			targetAbs = filepath.Clean(filepath.Join(filepath.Dir(abs), target))
			if !IsSubpath(targetAbs, root) {
				return nil, errors.Reason(
					"invalid symlink %q: a relative symlink pointing to a file outside of the package root", rel).Tag(cipderr.BadArgument).Err()
			}
		}

		// Symlinks with targets within SiteServiceDir get resolved as normal files.
		if IsSubpath(targetAbs, filepath.Join(root, SiteServiceDir)) {
			abs = targetAbs
			info, err = os.Stat(abs)
			if err != nil {
				return nil, errors.Annotate(err, "checking symlink target").Tag(cipderr.IO).Err()
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
			return nil, errors.Annotate(err, "getting win file attributes").Tag(cipderr.IO).Err()
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

	return nil, errors.Reason("%q: not a regular file or symlink", abs).Tag(cipderr.BadArgument).Err()
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
	if !IsSubpath(path, d.dest) {
		return "", errors.Reason("%q: invalid relative file name", name).Tag(cipderr.BadArgument).Err()
	}
	if _, err := d.fs.EnsureDirectory(ctx, filepath.Dir(path)); err != nil {
		return "", errors.Annotate(err, "creating parent dir for a file").Tag(cipderr.IO).Err()
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
		return nil, errors.Reason("%q: already open", name).Tag(cipderr.IO).Err()
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
		return nil, errors.Annotate(err, "creating destination file").Tag(cipderr.IO).Err()
	}

	// Close and delete (if it was a temp) on failures. Best effort.
	disarm := false
	defer func() {
		if !disarm {
			if clErr := file.Close(); clErr != nil {
				logging.Warningf(ctx, "Failed to close %s when cleaning up: %s", writeTo, clErr)
			}
			if d.atomic {
				// Nuke the unfinished temp file. The error is logged inside.
				d.fs.EnsureFileGone(ctx, writeTo)
			}
		}
	}()

	if err := setWinFileAttributes(writeTo, opts.WinAttrs); err != nil {
		return nil, errors.Annotate(err, "setting win attributes").Tag(cipderr.IO).Err()
	}

	d.lock.Lock()
	defer d.lock.Unlock()

	if _, ok := d.openFiles[path]; ok {
		return nil, errors.Reason("race condition when creating %s", name).Tag(cipderr.IO).Err()
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
					logging.Warningf(ctx, "[data-loss] cannot set mtime for %s: %s", path, chErr)
				}
			}

			// In atomic mode need to rename the temp file into its final name.
			if writeTo != path {
				replErr := d.fs.Replace(ctx, writeTo, path)
				if err == nil { // prefer to return errors from Close()
					err = replErr
				}
			}

			if err != nil {
				err = errors.Annotate(err, "closing the destination file").Tag(cipderr.IO).Err()
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
		if !IsSubpath(targetAbs, d.dest) {
			return errors.Reason("%q: relative symlink is pointing outside of the destination dir", name).Tag(cipderr.BadArgument).Err()
		}
	}

	if err := d.fs.EnsureSymlink(ctx, path, target); err != nil {
		return errors.Annotate(err, "creating symlink").Tag(cipderr.IO).Err()
	}
	return nil
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
		return errors.Reason("destination is already open").Tag(cipderr.BadArgument).Err()
	}

	// Ensure the parent directory of the destination directory exists, to be able
	// to create a temp subdir there.
	var err error
	if d.dest, err = d.fs.CwdRelToAbs(d.dest); err != nil {
		return errors.Annotate(err, "bad destination path").Tag(cipderr.BadArgument).Err()
	}
	if _, err := d.fs.EnsureDirectory(ctx, filepath.Dir(d.dest)); err != nil {
		return errors.Annotate(err, "creating the destination path").Tag(cipderr.IO).Err()
	}

	// Create the staging directory, on the same level as the destination
	// directory, so it can just be renamed into d.dest on completion. Let umask
	// trim the permissions appropriately. Note that it is not really a "private"
	// temp dir, since it will be eventually renamed into the "public" destination
	// that should be readable to everyone (sans umask).
	tempDir, err := TempDir(filepath.Dir(d.dest), "", 0777)
	if err != nil {
		return errors.Annotate(err, "creating temporary destination dir").Tag(cipderr.IO).Err()
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
		return nil, errors.Reason("destination is not open").Tag(cipderr.BadArgument).Err()
	}
	return d.fsDest.CreateFile(ctx, name, opts)
}

func (d *txnFSDest) CreateSymlink(ctx context.Context, name string, target string) error {
	if d.fsDest == nil {
		return errors.Reason("destination is not open").Tag(cipderr.BadArgument).Err()
	}
	return d.fsDest.CreateSymlink(ctx, name, target)
}

func (d *txnFSDest) End(ctx context.Context, success bool) error {
	if d.fsDest == nil {
		return errors.Reason("destination is not open").Tag(cipderr.BadArgument).Err()
	}
	if leaking := d.fsDest.numOpenFiles(); leaking != 0 {
		return errors.Reason("not all files were closed (leaking %d files)", leaking).Tag(cipderr.IO).Err()
	}

	// Clean up the temp dir and the state no matter what. On success it is
	// already gone (renamed into d.dest).
	defer func() {
		d.fs.EnsureDirectoryGone(ctx, d.fsDest.dest)
		d.fsDest = nil
	}()

	if success {
		if err := d.fs.Replace(ctx, d.fsDest.dest, d.dest); err != nil {
			return errors.Annotate(err, "moving temp destination directory into its final location").Tag(cipderr.IO).Err()
		}
	}

	// Let the defer clean the garbage left in fsDest.
	return nil
}
