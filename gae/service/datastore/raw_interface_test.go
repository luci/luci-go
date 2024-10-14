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

package datastore

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestMultiMetaGetter(t *testing.T) {
	t.Parallel()

	ftt.Run("Test MultiMetaGetter", t, func(t *ftt.Test) {
		t.Run("nil", func(t *ftt.Test) {
			mmg := NewMultiMetaGetter(nil)
			val, ok := mmg.GetMeta(7, "hi")
			assert.Loosely(t, ok, should.BeFalse)
			assert.Loosely(t, val, should.BeNil)

			assert.Loosely(t, GetMetaDefault(mmg.GetSingle(7), "hi", "value"), should.Equal("value"))

			m := mmg.GetSingle(10)
			val, ok = m.GetMeta("hi")
			assert.Loosely(t, ok, should.BeFalse)
			assert.Loosely(t, val, should.BeNil)

			assert.Loosely(t, GetMetaDefault(m, "hi", "value"), should.Equal("value"))
		})

		t.Run("stuff", func(t *ftt.Test) {
			pmaps := []PropertyMap{{}, nil, {}}
			assert.Loosely(t, pmaps[0].SetMeta("hi", "thing"), should.BeTrue)
			assert.Loosely(t, pmaps[2].SetMeta("key", 100), should.BeTrue)
			mmg := NewMultiMetaGetter(pmaps)

			// oob is OK
			assert.Loosely(t, GetMetaDefault(mmg.GetSingle(7), "hi", "value"), should.Equal("value"))

			// nil is OK
			assert.Loosely(t, GetMetaDefault(mmg.GetSingle(1), "key", true), should.Equal(true))

			val, ok := mmg.GetMeta(0, "hi")
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, val, should.Equal("thing"))

			assert.Loosely(t, GetMetaDefault(mmg.GetSingle(2), "key", 20), should.Equal(100))
		})
	})
}
