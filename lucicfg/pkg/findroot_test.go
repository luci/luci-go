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
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestFindRoot(t *testing.T) {
	t.Parallel()

	t.Run("Finds git", func(t *testing.T) {
		// This should find luci-go.git repository root.
		luciRoot, foundMarker, err := findRoot(".", "", nil)
		assert.NoErr(t, err)
		assert.That(t, filepath.IsAbs(luciRoot), should.BeTrue)
		assert.That(t, foundMarker, should.BeFalse)

		// Adding back our know path should get us back to ".".
		absFromRoot := filepath.Join(luciRoot, "lucicfg", "pkg")
		absFromCwd, _ := filepath.Abs(".")
		assert.That(t, absFromCwd, should.Equal(absFromRoot))
	})

	t.Run("Finds volume root", func(t *testing.T) {
		// Assume the temp dir is outside of any repositories.
		volumeRoot, foundMarker, err := findRoot(os.TempDir(), "", nil)
		assert.NoErr(t, err)
		assert.That(t, foundMarker, should.BeFalse)
		if runtime.GOOS == "windows" {
			// Assume it is a normal volume path, not a share.
			assert.That(t, volumeRoot, should.MatchRegexp(`[A-Z]\:\\`))
		} else {
			assert.That(t, volumeRoot, should.Equal("/"))
		}
	})

	t.Run("Find the marker file", func(t *testing.T) {
		tmp := t.TempDir()
		deep := filepath.Join(tmp, "a/b/c")
		assert.NoErr(t, os.MkdirAll(deep, 0750))
		assert.NoErr(t, os.MkdirAll(filepath.Join(tmp, ".git"), 0750))
		assert.NoErr(t, os.WriteFile(filepath.Join(tmp, "marker"), nil, 0666))

		root, foundMarker, err := findRoot(deep, "marker", nil)
		assert.NoErr(t, err)
		assert.That(t, foundMarker, should.BeTrue)
		assert.That(t, root, should.Equal(tmp))

		root, foundMarker, err = findRoot(deep, "another_marker", nil)
		assert.NoErr(t, err)
		assert.That(t, foundMarker, should.BeFalse)
		assert.That(t, root, should.Equal(tmp))
	})
}
