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

// Package testfs implements a test filesystem.
package testfs

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/system/filesystem"
)

// Build constructs a filesystem hierarchy given a layout.
//
// The layouts keys should be ToSlash-style file paths. Its values should be the
// content that is written at those paths. Intermediate directories will be
// automatically created.
//
// To create a directory, end its path with a "/". In this case, the content
// will be ignored.
func Build(base string, layout map[string]string) error {
	keys := make([]string, 0, len(layout))
	for k := range layout {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, path := range keys {
		makeDir := strings.HasSuffix(path, "/")
		content := layout[path]

		// Normalize "path" to the current OS.
		path = filepath.Join(base, filepath.FromSlash(path))

		if makeDir {
			// Make a directory.
			if err := filesystem.MakeDirs(path); err != nil {
				return err
			}
		} else {
			// Make a file.

			if err := filesystem.MakeDirs(filepath.Dir(path)); err != nil {
				return err
			}
			if err := os.WriteFile(path, []byte(content), 0644); err != nil {
				return err
			}
		}
	}
	return nil
}

// Collect constructs layout from a given directory.
//
// This function does reverse of Build.
// Content of empty directory is represented as "" with "/" suffix.
// But this does not work if there are symlink entries under |base| dir.
func Collect(base string) (map[string]string, error) {
	layout := make(map[string]string)

	base = strings.TrimSuffix(base, "/")

	if err := filepath.Walk(base, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if path == base {
			return nil
		}
		path = path[len(base)+1:]

		// Remove non-empty directory.
		delete(layout, filepath.ToSlash(filepath.Dir(path))+"/")

		if info.IsDir() {
			layout[filepath.ToSlash(path)+"/"] = ""
			return nil
		}

		if !info.Mode().IsRegular() {
			return errors.Fmt("unknown file info is detected for %s: %v", path, info)
		}

		buf, err := os.ReadFile(filepath.Join(base, path))
		if err != nil {
			return errors.Fmt("failed to read: %s: %w", filepath.Join(base, path), err)
		}

		layout[filepath.ToSlash(path)] = string(buf)
		return nil
	}); err != nil {
		return nil, err
	}

	return layout, nil
}
