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

package flag

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"google.golang.org/api/googleapi"
)

func TestFieldSliceFlag(t *testing.T) {
	t.Parallel()

	ftt.Run("one", t, func(t *ftt.Test) {
		var flag fieldSliceFlag
		assert.Loosely(t, flag.Set("abc"), should.BeNil)
		assert.Loosely(t, flag.Get(), should.Resemble([]googleapi.Field{"abc"}))
		assert.Loosely(t, flag.String(), should.Equal("abc"))
	})

	ftt.Run("many", t, func(t *ftt.Test) {
		var flag fieldSliceFlag
		assert.Loosely(t, flag.Set("abc"), should.BeNil)
		assert.Loosely(t, flag.Set("def"), should.BeNil)
		assert.Loosely(t, flag.Set("ghi"), should.BeNil)
		assert.Loosely(t, flag.Get(), should.Resemble([]googleapi.Field{"abc", "def", "ghi"}))
		assert.Loosely(t, flag.String(), should.Equal("abc, def, ghi"))
	})
}

func TestFieldSlice(t *testing.T) {
	t.Parallel()

	ftt.Run("one", t, func(t *ftt.Test) {
		var f []googleapi.Field
		assert.Loosely(t, FieldSlice(&f).Set("abc"), should.BeNil)
		assert.Loosely(t, f, should.Resemble([]googleapi.Field{"abc"}))
	})

	ftt.Run("many", t, func(t *ftt.Test) {
		var f []googleapi.Field
		assert.Loosely(t, FieldSlice(&f).Set("abc"), should.BeNil)
		assert.Loosely(t, FieldSlice(&f).Set("def"), should.BeNil)
		assert.Loosely(t, FieldSlice(&f).Set("ghi"), should.BeNil)
		assert.Loosely(t, f, should.Resemble([]googleapi.Field{"abc", "def", "ghi"}))
	})
}
