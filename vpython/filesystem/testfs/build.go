// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package testfs

import (
	"io/ioutil"
	"path/filepath"
	"sort"
	"strings"

	"github.com/luci/luci-go/vpython/filesystem"
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
			if err := ioutil.WriteFile(path, []byte(content), 0644); err != nil {
				return err
			}
		}
	}
	return nil
}
