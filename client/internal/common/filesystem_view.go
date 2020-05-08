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
	"strings"

	"go.chromium.org/luci/common/errors"
)

// FilesystemView provides a filtered "view" of a filesystem.
// It translates absolute paths to relative paths based on its configured root path.
// It also hides any paths which match a blacklist entry.
type FilesystemView struct {
	root           string
	ignoredPathsRe []*regexp.Regexp
	// TODO(crbug/1080471): Deprecate blacklist
	blacklist []string
	// The path prefix representing the relative path in another view which has this one
	// obtained through a sequence of symlinked nodes.
	sourcePrefix string
}

// NewFilesystemView returns a FilesystemView based on the supplied root and
// blacklist, or an error if blacklist contains a bad pattern.
//
// root is the the base path used by RelativePath to calculate relative paths.
//
// blacklist is a list of globs of files to ignore. See RelativePath for more
// information.
//
// ignoredPathsRe is a list of regular expressions. Compared to blacklist, it's
// not limited by how filepath.Match() works
// (https://godoc.org/path/filepath#Match), hence offers more flexibility. For
// example, instead of writing blacklist=["foo/*", "foo/a/*", "foo/a/b/*", ...]
// to completely skip "foo/", you can just use ignoredPathsRe=["foo/.*"]
func NewFilesystemView(root string, blacklist []string, ignoredPathsRe []string) (FilesystemView, error) {
	for _, b := range blacklist {
		if _, err := filepath.Match(b, b); err != nil {
			return FilesystemView{}, fmt.Errorf("bad blacklist pattern \"%s\"", b)
		}
	}
	var compiledRe []*regexp.Regexp
	for _, r := range ignoredPathsRe {
		// Matches the whole string
		if !strings.HasPrefix(r, "^") {
			r = "^" + r
		}
		if !strings.HasSuffix(r, "$") {
			r += "$"
		}
		cr, err := regexp.Compile(r)
		if err != nil {
			return FilesystemView{}, errors.Annotate(err, "bad ignoredPathsRe regexp \"%s\"", r).Err()
		}
		compiledRe = append(compiledRe, cr)
	}
	return FilesystemView{root: root, blacklist: blacklist, ignoredPathsRe: compiledRe}, nil
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

	for _, glob := range ff.blacklist {
		if match(glob, relPath) || match(glob, filepath.Base(relPath)) {
			return true
		}
	}

	for _, re := range ff.ignoredPathsRe {
		if re.MatchString(relPath) {
			return true
		}
	}

	return false
}

// NewSymlinkedView returns a filesystem view from a symlinked directory within itself.
func (ff FilesystemView) NewSymlinkedView(source, linkname string) FilesystemView {
	prefix := source
	if ff.sourcePrefix != "" {
		prefix = filepath.Join(ff.sourcePrefix, source)
	}
	return FilesystemView{root: linkname, blacklist: ff.blacklist, sourcePrefix: prefix}
}

// match is equivalent to filepath.Match, but assumes that pattern is valid.
func match(pattern, name string) bool {
	matched, _ := filepath.Match(pattern, name)
	return matched
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
