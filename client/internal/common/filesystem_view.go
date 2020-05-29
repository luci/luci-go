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
	"regexp"

	"go.chromium.org/luci/common/errors"
)

// FilesystemView provides a filtered "view" of a filesystem.
// It translates absolute paths to relative paths based on its configured root path.
type FilesystemView struct {
	root          string
	ignoredPathRe *regexp.Regexp

	// The path prefix representing the relative path in another view which has this one
	// obtained through a sequence of symlinked nodes.
	sourcePrefix string
}

// NewFilesystemView returns a FilesystemView based on the supplied root, or an
// error if ignoredPathRe contains a bad pattern.
//
// root is the the base path used by RelativePath to calculate relative paths.
//
// ignoredPathRe is a regular expression string. Note that this is NOT a full
// string match, so "foo/.*" may match "bar/foo/xyz". Prepend ^ explicitly if
// you need to match a path that starts with the pattern. Similarly, append $ if
// necessary.
func NewFilesystemView(root string, ignoredPathRe string) (FilesystemView, error) {
	var compiledRe *regexp.Regexp
	if ignoredPathRe != "" {
		cr, err := regexp.Compile(ignoredPathRe)
		if err != nil {
			return FilesystemView{}, errors.Annotate(err, "bad ignoredPathRe regexp \"%s\"", ignoredPathRe).Err()
		}
		compiledRe = cr
	}
	return FilesystemView{root: root, ignoredPathRe: compiledRe}, nil
}

// RelativePath returns a version of path which is relative to the FilesystemView root
// or an empty string if path matches a ignored path filter.
func (ff FilesystemView) RelativePath(path string) (string, error) {
	relPath, err := filepath.Rel(ff.root, path)
	if err != nil {
		return "", fmt.Errorf("calculating relative path(%q): %v", path, err)
	}
	if ff.skipRelPath(relPath) {
		relPath = ""
	} else if ff.sourcePrefix != "" {
		relPath = filepath.Join(ff.sourcePrefix, relPath)
	}
	return relPath, nil
}

func (ff FilesystemView) skipRelPath(relPath string) bool {
	// filepath.Rel is documented to call filepath.Clean on its result before returning it,
	// which results in "." for an empty relative path.
	if relPath == "." { // Root directory.
		return false
	}

	if ff.ignoredPathRe != nil && ff.ignoredPathRe.MatchString(relPath) {
		return true
	}

	return false
}

// NewSymlinkedView returns a filesystem view from a symlinked directory within itself.
func (ff FilesystemView) NewSymlinkedView(source, linkname string) FilesystemView {
	prefix := source
	if ff.sourcePrefix != "" {
		prefix = filepath.Join(ff.sourcePrefix, source)
	}
	return FilesystemView{root: linkname, sourcePrefix: prefix}
}

// WalkFuncSkipFile is a helper for implementations of filepath.WalkFunc. The
// value that it returns may in turn be returned by the WalkFunc implementatiton
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
