// Copyright 2015 The LUCI Authors.
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

package version

import (
	"fmt"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"io/ioutil"
	"os"
	"runtime"
	"testing"
)

func TestRecoverSymlinkPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping test on Windows")
	}

	ftt.Run("recoverSymlinkPath works", t, func(c *ftt.Test) {
		assert.Loosely(c, recoverSymlinkPath("/a/b/.cipd/pkgs/cipd_mac-amd64_L8/08c6146/cipd"), should.Equal("/a/b/cipd"))
		assert.Loosely(c, recoverSymlinkPath("/a/b/.cipd/pkgs/cipd_mac-amd64_L8/08c6146/c/d/cipd"), should.Equal("/a/b/c/d/cipd"))
		assert.Loosely(c, recoverSymlinkPath("/a/b/.cipd/pkgs/0/_current/c/d/cipd"), should.Equal("/a/b/c/d/cipd"))
		assert.Loosely(c, recoverSymlinkPath(".cipd/pkgs/cipd_mac-amd64_L8/08c6146/a"), should.Equal("a"))
	})

	ftt.Run("recoverSymlinkPath handles bad paths", t, func(c *ftt.Test) {
		assert.Loosely(c, recoverSymlinkPath(""), should.BeEmpty)
		assert.Loosely(c, recoverSymlinkPath("/a/b/c"), should.BeEmpty)
		assert.Loosely(c, recoverSymlinkPath("/a/b/c/d/e/f"), should.BeEmpty)
		assert.Loosely(c, recoverSymlinkPath("/a/b/c/.cipd/pkgs/d"), should.BeEmpty)
		assert.Loosely(c, recoverSymlinkPath("/a/b/c/.cipd/pkgs/abc/d"), should.BeEmpty)
	})
}

func TestEvalSymlinksAndAbs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping test on Windows")
	}

	ftt.Run(`evalSymlinksAndAbs`, t, func(t *ftt.Test) {
		t.Run(`works`, func(t *ftt.Test) {
			dir, err := ioutil.TempDir("", "")
			assert.Loosely(t, err, should.BeNil)
			defer os.RemoveAll(dir)

			p := func(path string) string {
				return fmt.Sprintf("%s/%s", dir, path)
			}

			err = os.MkdirAll(p(`.cipd/pkgs/0/deadbeef`), 0755)
			assert.Loosely(t, err, should.BeNil)
			err = os.MkdirAll(p(`a`), 0755)
			assert.Loosely(t, err, should.BeNil)

			f, err := os.Create(p(`.cipd/pkgs/0/deadbeef/foo`))
			assert.Loosely(t, err, should.BeNil)
			f.Close()

			err = os.Symlink(p(`.cipd/pkgs/0/deadbeef/foo`), p(`.cipd/pkgs/0/deadbeef/bar`))
			assert.Loosely(t, err, should.BeNil)

			err = os.Symlink(p(`.cipd/pkgs/0/deadbeef`), p(`.cipd/pkgs/0/_current`))
			assert.Loosely(t, err, should.BeNil)

			err = os.Symlink(p(`.cipd/pkgs/0/_current/foo`), p(`a/foo`))
			assert.Loosely(t, err, should.BeNil)

			pth, err := evalSymlinksAndAbs(p(`a/foo`), nil)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pth, should.HaveSuffix(`.cipd/pkgs/0/deadbeef/foo`))
		})
	})
}
