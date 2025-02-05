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

package filesystem

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"

	"go.chromium.org/luci/common/data/sortby"
	"go.chromium.org/luci/common/errors"
)

// IsNotExist calls os.IsNotExist on the unwrapped err.
func IsNotExist(err error) bool { return os.IsNotExist(errors.Unwrap(err)) }

// MakeDirs is a convenience wrapper around os.MkdirAll that applies a 0755
// mask to all created directories.
func MakeDirs(path string) error {
	if err := os.MkdirAll(path, 0755); err != nil {
		return errors.Annotate(err, "").Err()
	}
	return nil
}

// AbsPath is a convenience wrapper around filepath.Abs that accepts a string
// pointer, base, and updates it on successful resolution.
func AbsPath(base *string) error {
	v, err := filepath.Abs(*base)
	if err != nil {
		return errors.Annotate(err, "unable to resolve absolute path: %q", *base).Err()
	}
	*base = v
	return nil
}

// Touch creates a new, empty file at the specified path.
//
// If when is zero-value, time.Now will be used.
func Touch(path string, when time.Time, mode os.FileMode) error {
	// Try and create a file at the target path.
	fd, err := os.OpenFile(path, (os.O_CREATE | os.O_RDWR | os.O_EXCL), mode)
	if err == nil {
		if err := fd.Close(); err != nil {
			return errors.Annotate(err, "failed to close new file").Err()
		}
		if when.IsZero() {
			// If "now" was specified, and we created a new file, then its times will
			// be now by default.
			return nil
		}
	}

	// Couldn't create a new file. Either it exists already, it is a directory,
	// or there was an OS-level failure. Since we can't really distinguish
	// between these cases, try Chtimes (update timestamp) and error
	// if this fails.
	if when.IsZero() {
		when = time.Now()
	}
	if err := os.Chtimes(path, when, when); err != nil {
		return errors.Annotate(err, "failed to Chtimes: %q", path).Err()
	}

	return nil
}

// RemoveAll is a fork of os.RemoveAll that attempts to deal with read only
// files and directories by modifying permissions as necessary.
//
// If the specified path does not exist, RemoveAll will return nil.
//
// Note that RemoveAll will not modify permissions on parent directory of the
// provided path, even if it is read only and preventing deletion of the path on
// POSIX system.
//
// Copied from
// https://go.googlesource.com/go/+/b86e76681366447798c94abb959bb60875bcc856/src/os/path.go#63
func RemoveAll(path string) error {
	const isWin = runtime.GOOS == "windows"
	// Simple case: try removing as if it was a file or empty directory.
	var err error
	if isWin {
		// In theory this call should not be necessary. os.Remove() already
		// tries to remove the FILE_ATTRIBUTE_READONLY attribute at
		// https://go.googlesource.com/go/+blame/go1.13/src/os/file_windows.go#296.
		// In practice this doesn't work in one case, when it is a symlink that
		// points to a missing file. In this case, ErrNotExist is returned, but
		// the function call is still needed for the os.Remove() to work below.
		err = MakePathUserWritable(path, nil)
	}
	if err == nil || IsNotExist(err) {
		// On Windows, invalid symlink is treated as not exist error, but need to
		// remove that.
		err = os.Remove(path)
	}
	if err == nil || IsNotExist(err) {
		return nil
	}

	// Otherwise, is this a directory we need to recurse into?
	dir, serr := os.Lstat(path)
	if serr != nil {
		if serr, ok := serr.(*os.PathError); ok && (IsNotExist(serr.Err) || serr.Err == syscall.ENOTDIR) {
			return nil
		}
		return serr
	}
	if !dir.IsDir() {
		// Not a directory; return the error from Remove.
		return err
	}
	// Directory.
	if !isWin {
		// On POSIX systems, the directory must have write access for its files to
		// be deleted. Best effort attempt to make it writable.
		_ = MakePathUserWritable(path, dir)
	}
	fd, err := os.Open(path)
	if err != nil {
		if IsNotExist(err) {
			// Race. It was deleted between the Lstat and Open.
			// Return nil per RemoveAll's docs.
			return nil
		}
		return err
	}
	// Remove contents & return first error.
	err = nil
	for {
		if err == nil && (runtime.GOOS == "plan9" || runtime.GOOS == "nacl") {
			// Reset read offset after removing directory entries.
			// See golang.org/issue/22572.
			fd.Seek(0, 0)
		}
		names, err1 := fd.Readdirnames(100)
		for _, name := range names {
			err1 := RemoveAll(path + string(os.PathSeparator) + name)
			if err == nil {
				err = err1
			}
		}
		if err1 == io.EOF {
			break
		}
		// If Readdirnames returned an error, use it.
		if err == nil {
			err = err1
		}
		if len(names) == 0 {
			break
		}
	}
	// Close directory, because windows won't remove opened directory.
	fd.Close()
	// Remove directory.
	err1 := os.Remove(path)
	if err1 == nil || IsNotExist(err1) {
		return nil
	}
	if err == nil {
		err = err1
	}
	return err
}

// RenamingRemoveAll opportunistically renames a path first, and then removes it.
//
// The advantage over RemoveAll is, if renaming succeeds, lower chance of
// interference from other writers/readers of the filesystem.
// If renaming fails, removes the original path via RemoveAll.
//
// If renameToDir is given, a new temp directory will be created in it.
// Else, a new temp directory is placed within the path's parent dir.
// After this, a file/dir represented by the path is moved into the temp dir.
//
// In case of any failures during the temp dir creation or the move,
// default to RemoveAll of path in place.
//
// Returned renamedToPath is the renamed path if renaming succeeded and ""
// otherwise.
// Returned error is the one from RemoveAll call.
func RenamingRemoveAll(path, renameToDir string) (renamedToPath string, err error) {
	pathParentDir, pathFileOrDir := filepath.Split(filepath.Clean(path))
	if renameToDir == "" {
		renameToDir = pathParentDir
	}
	renameToDir, err = os.MkdirTemp(renameToDir, ".trash-")
	if err != nil {
		err = RemoveAll(path)
		return
	}

	renamedToPath = filepath.Join(renameToDir, pathFileOrDir)
	if err = os.Rename(path, renamedToPath); err != nil {
		// delete temp dir we just created and ignore errors -- there is not much we can do.
		_ = os.Remove(renameToDir)
		renamedToPath = ""
		err = RemoveAll(path)
		return
	}
	err = RemoveAll(renamedToPath)
	return
}

// MakeReadOnly recursively iterates through all of the files and directories
// starting at path and marks them read-only.
func MakeReadOnly(path string, filter func(string) bool) error {
	return recursiveChmod(path, filter, func(mode os.FileMode) os.FileMode {
		return mode & (^os.FileMode(0222))
	})
}

// MakePathUserWritable updates the filesystem metadata on a single file or
// directory to make it user-writable.
//
// fi is optional. If nil, os.Stat will be called on path. Otherwise, fi will
// be regarded as the results of calling os.Stat on path. This is provided as
// an optimization, since some filesystem operations automatically yield a
// FileInfo.
func MakePathUserWritable(path string, fi os.FileInfo) error {
	if fi == nil {
		var err error
		if fi, err = os.Stat(path); err != nil {
			return errors.Annotate(err, "failed to Stat path: %q", path).Err()
		}
	}

	// Make user-writable, if it's not already.
	mode := fi.Mode()
	if (mode & 0200) == 0 {
		mode |= 0200
		if err := os.Chmod(path, mode); err != nil {
			return errors.Annotate(err, "could not Chmod path: mode=%#o: %q", mode, path).Err()
		}
	}
	return nil
}

func recursiveChmod(path string, filter func(string) bool, chmod func(mode os.FileMode) os.FileMode) error {
	if filter == nil {
		filter = func(string) bool { return true }
	}

	err := filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return errors.Annotate(err, "").Err()
		}

		mode := info.Mode()
		if (mode.IsRegular() || mode.IsDir()) && filter(path) {
			if newMode := chmod(mode); newMode != mode {
				if err := os.Chmod(path, newMode); err != nil {
					return errors.Annotate(err, "failed to Chmod: %q", path).Err()
				}
			}
		}
		return nil
	})
	if err != nil {
		return errors.Annotate(err, "").Err()
	}
	return nil
}

// Copy makes a copy of the file.
func Copy(outfile, infile string, mode os.FileMode) (err error) {
	in, err := os.Open(infile)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := in.Close(); err == nil {
			err = cerr
		}
	}()

	out, err := os.OpenFile(outfile, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := out.Close(); err == nil {
			err = cerr
		}
	}()

	_, err = io.Copy(out, in)
	return err
}

// ReadableCopy makes a copy of the file that is readable by everyone.
func ReadableCopy(outfile, infile string) error {
	istat, err := os.Stat(infile)
	if err != nil {
		return err
	}

	return Copy(outfile, infile, addReadMode(istat.Mode()))
}

func hardlinkWithFallback(outfile, infile string) error {
	if err := os.Link(infile, outfile); err == nil {
		return nil
	}

	return ReadableCopy(outfile, infile)
}

// HardlinkRecursively efficiently copies a file or directory from src to dst.
//
// `src` may be a file, directory, or a symlink to a file or directory.
// All symlinks are replaced with their targets, so the resulting
// directory structure in `dst` will never have any symlinks.
//
// To increase speed, HardlinkRecursively hardlinks individual files into the
// (newly created) directory structure if possible.
func HardlinkRecursively(src, dst string) error {
	src, stat, err := ResolveSymlink(src)
	if err != nil {
		return errors.Annotate(err, "failed to call ResolveSymlink(%s)", src).Err()
	}

	if stat.Mode().IsRegular() {
		return hardlinkWithFallback(dst, src)
	}

	if !stat.Mode().IsDir() {
		return errors.Reason("%s is not a directory: %v", src, stat).Err()
	}

	if err := os.MkdirAll(dst, 0775); err != nil {
		return errors.Annotate(err, "failed to call MkdirAll for %s", dst).Err()
	}

	file, err := os.Open(src)
	if err != nil {
		return errors.Annotate(err, "failed to Open %s", src).Err()
	}
	defer file.Close()

	for {
		names, err := file.Readdirnames(100)
		if err == io.EOF {
			break
		}
		if err != nil {
			return errors.Annotate(err, "failed to call Readdirnames for %s", src).Err()
		}

		for _, name := range names {
			if err := HardlinkRecursively(filepath.Join(src, name), filepath.Join(dst, name)); err != nil {
				return errors.Annotate(err, "failed to call HardlinkRecursively(%s, %s)", filepath.Join(src, name), filepath.Join(dst, name)).Err()
			}

		}
	}

	return nil
}

// CreateDirectories creates the directory structure needed by the given list of files.
func CreateDirectories(baseDirectory string, files []string) error {
	dirs := make([]string, len(files))
	for i, file := range files {
		if filepath.IsAbs(file) {
			return errors.Reason("file should be relative path: %s", file).Err()
		}
		dirs[i] = filepath.Dir(file)
	}

	sort.Strings(dirs)

	for i, dir := range dirs {
		if dir == "" {
			continue
		}
		if i+1 < len(dirs) && filepath.HasPrefix(dirs[i+1], dir) {
			continue
		}
		dir = filepath.Join(baseDirectory, dir)

		if err := os.MkdirAll(dir, 0755); err != nil {
			return errors.Annotate(err, "failed to create directory for %s", dir).Err()
		}
	}

	return nil
}

// IsEmptyDir returns whether |dir| is empty or not.
// This returns error if |dir| is not directory, or find some error during checking.
func IsEmptyDir(dir string) (bool, error) {
	d, err := os.Open(dir)
	if err != nil {
		return false, errors.Annotate(err, "failed to Open(%s)", dir).Err()
	}
	defer d.Close()

	names, err := d.Readdirnames(1)
	if len(names) > 0 || err == io.EOF {
		return len(names) == 0, nil
	}

	return false, errors.Annotate(err, "failed to call Readdirnames(1) for %s", dir).Err()
}

// IsDir to see whether |path| is a directory.
// This is just a thin wrapper around os.Stat(...).
// If this returns True, |path| is a directory.
// If this returns False with nil err, |path| is not a directory.
// If this returns non-nil error, failed to determine |path| is a drectory.
func IsDir(path string) (bool, error) {
	stat, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return stat.IsDir(), nil
}

// GetFreeSpace returns the number of free bytes.
//
// On POSIX platforms, this returns the free space as visible by the current
// user. The returned value is what is usable, and it can be lower than the
// actual free disk space. For example on linux there's by default a 5% that is
// reserved to the root user.
func GetFreeSpace(path string) (uint64, error) {
	return getFreeSpace(path)
}

// findPathSeparators finds the index of all PathSeparators in `path` which
// don't split the Volume.
//
// This function is only defined for clean, absolute, paths which end with
// a path separator.
//
// For unix paths, the first returned index will always be 0.
func findPathSeparators(path string) []int {
	offset := len(filepath.VolumeName(path))
	path = path[offset:]

	ret := make([]int, 0, 10) // 10 is a guess, could be more or less.
	for {
		idx := strings.IndexByte(path, os.PathSeparator)
		if idx == -1 {
			break
		}
		ret = append(ret, offset+idx)
		offset += idx + 1
		path = path[idx+1:]
	}

	return ret
}

// ErrRootSentinel is wrapped and then returned from GetCommonAncestor when it
// encounters one of the provided rootSentinels.
var ErrRootSentinel = errors.New("hit root sentinel")

// GetCommonAncestor returns the smallest path which is the ancestor of all
// provided paths (which must actually exist on the filesystem).
//
// All paths here are converted to absolute paths before calculating the
// ancestor, and the returned path will also be an absolute path. Note that this
// doesn't do anything special with symlinks; the caller can resolve them if
// necessary.
//
// This function works correctly on case-insensitive filesystems, or on
// filesystems with a mix of case-sensitive and case-insensitive paths, but you
// can get some wild filesystems out there, so this will probably break on
// exotic setups. Note that the case of the path you get back will be derived
// from the shortest input path (after making them absolute); this function
// makes no attempt to "canonicalize" the case of any paths (but may do so
// accidentally, depending on the operating system). This function does not
// attempt to resolve symlinks.
//
// If a given path points to a file, the file's containing directory will be
// considered instead (i.e. GetCommonAncestor("a/b.ext") will return the
// absolute path of "a").
//
// `rootSentinels` is a list of sub paths to look for to stop walking up the
// directory hierarchy. A typical value would be something like
// []string{".git"}. If one of these is found, this function returns "" with
// a wrapped ErrRootSentinel. Use errors.Is to identify this.
//
// Returns an error if any of the provided paths does not exist.
// If successful, will return a path ending with PathSeparator.
//
// If no paths are prodvided, returns ("", nil)
func GetCommonAncestor(paths []string, rootSentinels []string) (string, error) {
	if len(paths) == 0 {
		return "", nil
	}

	const sep = string(os.PathSeparator)

	type cleanPath struct {
		// The cleaned, absolute path to a directory which exists.
		path string
		// Indexes in `path` for each os.PathSeparator which is a valid split point.
		// Note that UNC roots on windows may 'skip' PathSeparators (i.e. slashes[0]
		// may contain multiple PathSeparators).
		slashes []int
	}
	cleanPaths := make([]cleanPath, len(paths))

	var commonVolume *string

	// Clean all the paths, make them absolute.
	// Find all the slashes in the paths.
	//
	// Note that a UNC path like '\\host\share\something' would have its first
	// slash at index 12.
	// A Unix path like '/host/share/something' would have its first
	// slash at index 0.
	for i, path := range paths {
		if err := AbsPath(&path); err != nil {
			return "", err
		}
		vol := filepath.VolumeName(path)

		if commonVolume != nil {
			// note: we check this first to allow testing; otherwise we would need to
			// run tests on a machine with multiple volumes.
			if vol != *commonVolume {
				return "", errors.Reason("provided paths originate on different volumes: path[0]:%q, path[%d]:%q", *commonVolume, i, vol).Err()
			}
		} else {
			commonVolume = &vol
		}

		fi, err := os.Lstat(path)
		if err != nil {
			return "", errors.Annotate(err, "reading path[%d]: %q", i, path).Err()
		}
		if !fi.IsDir() {
			path = filepath.Dir(path)
			fi, err := os.Lstat(path)
			if err != nil {
				// given that we know that the original `path` exists, this SHOULD be
				// impossible, but FUSE exists so... idk.
				return "", errors.Annotate(err, "reading Dir(path[%d]): %q", i, path).Err()
			}
			if !fi.IsDir() {
				// this SHOULD ALSO be impossible...
				return "", errors.Annotate(err, "path %q could not be resolved to parent dir", path).Err()
			}
		}

		if !strings.HasSuffix(path, sep) {
			path = path + sep
		}
		cleanPaths[i] = cleanPath{
			path:    path,
			slashes: findPathSeparators(path),
		}
	}

	// sort by length and then alphabetically
	sort.Slice(cleanPaths, sortby.Chain{
		func(i, j int) bool { return len(cleanPaths[i].path) < len(cleanPaths[j].path) },
		func(i, j int) bool { return cleanPaths[i].path < cleanPaths[j].path },
	}.Use)

	candidate := cleanPaths[0]

	// We want to see if all the slashes in all other candidates line up with the
	// slashes in `candidate`.
	//
	// We are already making some lexical assumptions about the paths here; If we
	// wanted to discard lexical assumptions we would need to do all permutations
	// of SameFile checks for every directory combination in all paths, and pick
	// the lowest one which matched (due to the possibility of e.g. bind mounts).
	//
	// However, ain't nobody got time for that.
	slashesMatch := func(whichSlash int) bool {
		for _, other := range cleanPaths[1:] {
			// whichSlash+1 because we want to include everything up to, and
			// including, whichSlash.
			for i, slashIdx := range candidate.slashes[:whichSlash+1] {
				if other.slashes[i] != slashIdx {
					return false
				}
			}
		}
		return true
	}

	// for each slash in the candidate, see if all cleanPaths at this slash return
	// true from os.SameFile vs candidate.
	//
	// Calling this function implies that all other paths have already been
	// verified with slashesMatch.
	//
	// The first check would avoid comparing "/long/f" vs "/s/long"; Although they
	// both have a slash at 7, the prefix leading to that doesn't match. See
	// comment on slashesMatch.
	trySlash := func(curPath string) (bool, error) {
		var curFi os.FileInfo
		for _, other := range cleanPaths[1:] {
			otherPath := other.path[:len(curPath)]
			if otherPath == curPath {
				// exact match, try the next one
				continue
			}

			// ok, try SameFile
			if curFi == nil {
				var err error
				if curFi, err = os.Lstat(curPath); err != nil {
					return false, err
				}
			}
			otherFi, err := os.Lstat(otherPath)
			if err != nil {
				return false, err
			}
			if !os.SameFile(curFi, otherFi) {
				return false, nil
			}
		}
		return true, nil
	}

	for whichSlash := len(candidate.slashes) - 1; whichSlash >= 0; whichSlash-- {
		// curPath includes trailing slash
		curPath := candidate.path[:candidate.slashes[whichSlash]+1]

		// check if it's out of bounds
		for _, sentinel := range rootSentinels {
			sentinelPath := filepath.Join(curPath, sentinel)
			if _, err := os.Lstat(sentinelPath); err == nil {
				return "", errors.Annotate(ErrRootSentinel, "%q", sentinelPath).Err()
			} else if !os.IsNotExist(err) {
				return "", errors.Annotate(err, "failed to read root sentinel %q", sentinelPath).Err()
			}
		}

		// Check to see if all other candidates have the same slash structure; i.e.
		// the first `whichSlash` number of slashes line up across all paths. This
		// avoids doing stats to see if "/a/path/" and "/path/a/" are the same file.
		if !slashesMatch(whichSlash) {
			continue
		}

		// all slashes match, let's see if the targeted files are the same (either
		// lexically equivalent or are os.SameFile)
		ok, err := trySlash(curPath)
		if err != nil {
			return "", err
		}
		if ok {
			return curPath, nil
		}
	}

	// Should never get here:
	//   * We are on unix and so whichSlash==0 is always the path "/". Either we
	//     found the root sentinel above, or returned "/" successfully.
	//   * We are on !unix and so whichSlash==0 is always a Volume (e.g. "C:\\").
	//     However we already checked when resolving `paths` at the top that all
	//     the cleanPaths share the SAME volume. Thus we should have either found
	//     the root sentinel or returned this same volume (i.e. `commonVolume`).
	panic("impossible")
}
