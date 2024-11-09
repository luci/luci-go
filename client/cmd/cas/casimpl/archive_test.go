// Copyright 2020 The LUCI Authors.
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

package casimpl

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestGetRoot(t *testing.T) {
	t.Parallel()

	ftt.Run(`Basic`, t, func(t *ftt.Test) {
		dirs := make(scatterGather)
		assert.Loosely(t, dirs.Add("wd1", "rel"), should.BeNil)
		assert.Loosely(t, dirs.Add("wd1", "rel2"), should.BeNil)

		wd, err := getRoot(dirs)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, wd, should.Equal("wd1"))

		assert.Loosely(t, dirs.Add("wd2", "rel3"), should.BeNil)

		_, err = getRoot(dirs)
		assert.Loosely(t, err, should.NotBeNil)
	})
}

func TestLoadPathsJSON(t *testing.T) {
	t.Parallel()

	ftt.Run(`Basic`, t, func(t *ftt.Test) {
		dir := t.TempDir()
		input := [][2]string{
			{dir, "foo.txt"},
			{dir, "bar.txt"},
		}
		b, err := json.Marshal(input)
		assert.Loosely(t, err, should.BeNil)

		pathsJSON := filepath.Join(dir, "paths.json")
		err = os.WriteFile(pathsJSON, b, 0o600)
		assert.Loosely(t, err, should.BeNil)

		res, err := loadPathsJSON(pathsJSON)
		assert.Loosely(t, err, should.BeNil)

		assert.Loosely(t, res, should.Resemble(scatterGather{
			"foo.txt": dir,
			"bar.txt": dir,
		}))
	})
}
