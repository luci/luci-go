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

package common

import (
	"fmt"
	"os"
	"path/filepath"
)

// FilesystemView provides a filtered "view" of a filesystem.
// It translates absolute paths to relative paths based on its configured root path.
// It also hides any paths which match a blacklist entry.
type FilesystemView struct {
	root      string
	blacklist []string
}

// NewFilesystemView returns a FilesystemView based on the supplied root and blacklist, or
// an error if blacklist contains a bad pattern.
// root is the the base path used by RelativePath to calulate relative paths.
// blacklist is a list of globs of files to ignore.  See RelativePath for more information.
func NewFilesystemView(root string, blacklist []string) (FilesystemView, error) {
	for _, b := range blacklist {
		if _, err := filepath.Match(b, b); err != nil {
			return FilesystemView{}, fmt.Errorf("bad blacklist pattern \"%s\"", b)
		}
	}
	return FilesystemView{root: root, blacklist: blacklist}, nil
}

// RelativePath returns a version of path which is relative to the FilesystemView root,
// or an empty string if path matches a blacklist entry.
//
// Blacklist globs are matched against the entirety of each of:
//  * the path relative to the FilesystemView root.
//  * the basename return by filepath.Base(path).
// See filepath.Match for details about the format of blacklist globs.
func (ff FilesystemView) RelativePath(path string) (string, error) {
	relPath, err := filepath.Rel(ff.root, path)
	if err != nil {
		return "", fmt.Errorf("calculating relative path(%q): %v", path, err)
	}
	if ff.skipRelPath(relPath) {
		relPath = ""
	}
	return relPath, nil
}

func (ff FilesystemView) skipRelPath(relPath string) bool {
	// filepath.Rel is documented to call filepath.Clean on its result before returning it,
	// which results in "." for an empty relative path.
	if relPath == "." { // Root directory.
		return false
	}

	for _, glob := range ff.blacklist {
		if match(glob, relPath) || match(glob, filepath.Base(relPath)) {
			return true
		}
	}

	return false
}

// WithNewRoot returns a FilesystemView with an identical blacklist and new root.
func (ff FilesystemView) WithNewRoot(root string) FilesystemView {
	return FilesystemView{root: root, blacklist: ff.blacklist}
}

// match is equivalent to filepath.Match, but assumes that pattern is valid.
func match(pattern, name string) bool {
	matched, _ := filepath.Match(pattern, name)
	return matched
}

// WalkFuncSkipFile is a helper for implemenations of filepath.WalkFunc. The
// value that it returns may in turn be returned by the WalkFunc implementaiton
// to indicate that file should be skipped.
func WalkFuncSkipFile(file os.FileInfo) error {
	if file.IsDir() {
		return filepath.SkipDir
	}
	// If we were to return SkipDir for a file, it would cause
	// filepath.Walk to skip the file's containing directory, which we do
	// not want (see https://golang.org/pkg/path/filepath/#WalkFunc).
	return nil
}
