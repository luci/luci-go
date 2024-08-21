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

package gs

import (
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"testing"
)

func TestValidatePath(t *testing.T) {
	t.Parallel()

	ftt.Run("ValidatePath works", t, func(t *ftt.Test) {
		good := []string{
			"/z/z",
			"/z/z/z/z/z/z/z",
		}
		for _, p := range good {
			assert.Loosely(t, ValidatePath(p), should.BeNil)
		}
		bad := []string{
			"",
			"z/z/z",
			"/zzz",
			"//zz",
			"/z//z",
			"/z/z/",
			"/z/z\n",
			"/z/🤦",
		}
		for _, p := range bad {
			assert.Loosely(t, ValidatePath(p), should.NotBeNil)
		}
	})
}

func TestSplitPath(t *testing.T) {
	t.Parallel()

	ftt.Run("SplitPath works", t, func(t *ftt.Test) {
		bucket, path := SplitPath("/a/b/c/d")
		assert.Loosely(t, bucket, should.Equal("a"))
		assert.Loosely(t, path, should.Equal("b/c/d"))

		assert.Loosely(t, func() { SplitPath("") }, should.Panic)
		assert.Loosely(t, func() { SplitPath("/zzz") }, should.Panic)
	})
}
