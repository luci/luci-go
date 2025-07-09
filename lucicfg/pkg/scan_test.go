// Copyright 2025 The LUCI Authors.
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

package pkg

import (
	"path/filepath"
	"testing"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestScanForRoots(t *testing.T) {
	t.Parallel()

	tmp := prepDisk(t, map[string]string{
		"repo a/.git/config":                "",
		"repo a/file.star":                  "",
		"repo a/deeper/file.star":           "",
		"repo a/inner/.git/config":          "",
		"repo a/inner/file.star":            "",
		"repo b/pkg 1/PACKAGE.star":         "",
		"repo b/pkg 1/deeper/.lucicfgfmtrc": "",
		"repo b/pkg 1/deeper/file.star":     "",
		"repo b/pkg 1/inner/PACKAGE.star":   "",
		"repo b/pkg 2/.lucicfgfmtrc":        "",
		"repo b/pkg 2/file.star":            "",
		"repo b/pkg 2/deeper/.lucicfgfmtrc": "",
		"repo b/pkg 2/deeper/file.star":     "",
		"repo c/.git/config":                "",
		"repo c/ignored/.lucicfgfmtrc":      "",
		"repo c/ignored/file":               "",
	})

	abs := func(p string) string {
		return filepath.Join(tmp, filepath.FromSlash(p))
	}

	rebasePath := func(p string) string {
		var err error
		p, err = filepath.Rel(tmp, p)
		if err != nil {
			panic(err)
		}
		return filepath.ToSlash(p)
	}

	rebaseRes := func(res []*ScanResult) []*ScanResult {
		for _, r := range res {
			r.Root = rebasePath(r.Root)
			for i, f := range r.Files {
				r.Files[i] = rebasePath(f)
			}
		}
		return res
	}

	expectedAll := []*ScanResult{
		{
			Root:  "repo a",
			Files: []string{"repo a/deeper/file.star", "repo a/file.star"},
		},
		{
			Root:  "repo a/inner",
			Files: []string{"repo a/inner/file.star"},
		},
		{
			Root:      "repo b/pkg 1",
			IsPackage: true,
			Files:     []string{"repo b/pkg 1/PACKAGE.star", "repo b/pkg 1/deeper/file.star"},
		},
		{
			Root:      "repo b/pkg 1/inner",
			IsPackage: true,
			Files:     []string{"repo b/pkg 1/inner/PACKAGE.star"},
		},
		{
			Root:  "repo b/pkg 2",
			Files: []string{"repo b/pkg 2/file.star"},
		},
		{
			Root:  "repo b/pkg 2/deeper",
			Files: []string{"repo b/pkg 2/deeper/file.star"},
		},
	}

	t.Run("Recursive all", func(t *testing.T) {
		res, err := ScanForRoots([]string{tmp})
		assert.NoErr(t, err)
		assert.That(t, rebaseRes(res), should.Match(expectedAll))
	})

	t.Run("Individual files", func(t *testing.T) {
		res, err := ScanForRoots([]string{
			abs("repo a/file.star"),
			abs("repo a/deeper/file.star"),
			abs("repo a/inner/file.star"),
			abs("repo b/pkg 1/PACKAGE.star"),
			abs("repo b/pkg 1/deeper/file.star"),
			abs("repo b/pkg 1/inner/PACKAGE.star"),
			abs("repo b/pkg 2/file.star"),
			abs("repo b/pkg 2/deeper/file.star"),
		})
		assert.NoErr(t, err)
		assert.That(t, rebaseRes(res), should.Match(expectedAll))
	})

	t.Run("Individual dirs", func(t *testing.T) {
		res, err := ScanForRoots([]string{
			abs("repo a"),
			abs("repo a/deeper"),
			abs("repo b/pkg 1"),
			abs("repo b/pkg 1/deeper"),
			abs("repo b/pkg 1/inner"),
			abs("repo b/pkg 2"),
			abs("repo b/pkg 2/deeper"),
		})
		assert.NoErr(t, err)
		assert.That(t, rebaseRes(res), should.Match(expectedAll))
	})

	t.Run("Few leaves", func(t *testing.T) {
		res, err := ScanForRoots([]string{
			abs("repo a/deeper/file.star"),
			abs("repo b/pkg 1/deeper/file.star"),
			abs("repo b/pkg 2/deeper/file.star"),
		})
		assert.NoErr(t, err)
		assert.That(t, rebaseRes(res), should.Match([]*ScanResult{
			{
				Root: "repo a", Files: []string{"repo a/deeper/file.star"},
			},
			{
				Root:      "repo b/pkg 1",
				IsPackage: true,
				Files:     []string{"repo b/pkg 1/deeper/file.star"},
			},
			{
				Root:  "repo b/pkg 2/deeper",
				Files: []string{"repo b/pkg 2/deeper/file.star"},
			},
		}))
	})

	t.Run("RelFiles", func(t *testing.T) {
		res, err := ScanForRoots([]string{
			abs("repo a"),
		})
		assert.NoErr(t, err)
		assert.That(t, res[0].Root, should.Equal(abs("repo a")))
		assert.That(t, res[0].RelFiles(), should.Match([]string{
			"deeper/file.star",
			"file.star",
		}))
	})

	t.Run("FindRootForFile", func(t *testing.T) {
		for _, tc := range []struct {
			path string
			root string
		}{
			{
				path: abs("repo a/deeper/file.star"),
				root: "repo a",
			},
			{
				path: abs("repo a/inner/file.star"),
				root: "repo a/inner",
			},
			{
				path: abs("repo a/nonexistent.star"),
				root: "repo a",
			},
			{
				path: abs("repo a/inner/path/to/nonexistent.star"),
				root: "repo a/inner",
			},
		} {
			res, err := FindRootForFile(tc.path, "")
			assert.NoErr(t, err)
			assert.That(t, rebasePath(res), should.Match(tc.root))
		}
		res, err := FindRootForFile("/not/in/a/repo/nonexistent.star", "")
		assert.NoErr(t, err)
		// On unix, this should equal /
		// On windows, this should equal C:\, or whatever the drive name is.
		assert.That(t, res, should.Match(filepath.Dir(res)))
	})
}
