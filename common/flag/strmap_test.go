// Copyright 2016 The LUCI Authors.
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

func TestStringMap(t *testing.T) {
	ftt.Run(`StringMap`, t, func(t *ftt.Test) {
		m := map[string]string{}
		f := StringMap(m)

		t.Run(`Set`, func(t *ftt.Test) {
			assert.Loosely(t, f.Set("a:1"), should.BeNil)
			assert.Loosely(t, m, should.Match(map[string]string{"a": "1"}))

			assert.Loosely(t, f.Set("b:2"), should.BeNil)
			assert.Loosely(t, m, should.Match(map[string]string{"a": "1", "b": "2"}))

			assert.Loosely(t, f.Set("b:3"), should.ErrLike(`key "b" is already specified`))

			assert.Loosely(t, f.Set("c:something:with:colon"), should.BeNil)
			assert.Loosely(t, m, should.Match(map[string]string{"a": "1", "b": "2", "c": "something:with:colon"}))
		})

		t.Run(`String`, func(t *ftt.Test) {
			m["a"] = "1"
			assert.Loosely(t, f.String(), should.Equal("a:1"))

			m["b"] = "2"
			assert.Loosely(t, f.String(), should.Equal("a:1 b:2"))
		})

		t.Run(`Value`, func(t *ftt.Test) {
			assert.Loosely(t, f.Get(), should.Match(m))
		})
	})
}
