// Copyright 2018 The LUCI Authors.
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
	"path"
	"path/filepath"
	"strings"
)

// IsSubpath returns true if 'path' is 'root' or is inside a subdirectory of
// 'root'. Both 'path' and 'root' should be given as a native paths. If any of
// paths can't be converted to an absolute path returns false.
func IsSubpath(path, root string) bool {
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

// IsCleanSlashPath returns true if path is a relative slash-separated path with
// no '..' or '.' entries and no '\\'. Basically "a/b/c/d".
func IsCleanSlashPath(p string) bool {
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
