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
)

func TestStringSliceFlag(t *testing.T) {
	t.Parallel()

	ftt.Run("one", t, func(t *ftt.Test) {
		var flag stringSliceFlag
		assert.Loosely(t, flag.Set("abc"), should.BeNil)
		assert.Loosely(t, flag.Get(), should.Match([]string{"abc"}))
		assert.Loosely(t, flag.String(), should.Equal("abc"))
	})

	ftt.Run("many", t, func(t *ftt.Test) {
		var flag stringSliceFlag
		assert.Loosely(t, flag.Set("abc"), should.BeNil)
		assert.Loosely(t, flag.Set("def"), should.BeNil)
		assert.Loosely(t, flag.Set("ghi"), should.BeNil)
		assert.Loosely(t, flag.Get(), should.Match([]string{"abc", "def", "ghi"}))
		assert.Loosely(t, flag.String(), should.Equal("abc, def, ghi"))
	})
}

func TestStringSlice(t *testing.T) {
	t.Parallel()

	ftt.Run("one", t, func(t *ftt.Test) {
		var s []string
		assert.Loosely(t, StringSlice(&s).Set("abc"), should.BeNil)
		assert.Loosely(t, s, should.Match([]string{"abc"}))
	})

	ftt.Run("many", t, func(t *ftt.Test) {
		var s []string
		assert.Loosely(t, StringSlice(&s).Set("abc"), should.BeNil)
		assert.Loosely(t, StringSlice(&s).Set("def"), should.BeNil)
		assert.Loosely(t, StringSlice(&s).Set("ghi"), should.BeNil)
		assert.Loosely(t, s, should.Match([]string{"abc", "def", "ghi"}))
	})
}
