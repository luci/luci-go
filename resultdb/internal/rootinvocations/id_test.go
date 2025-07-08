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

package rootinvocations

import (
	"testing"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/resultdb/internal/spanutil"
)

func TestSpannerConversion(t *testing.T) {
	t.Parallel()
	ftt.Run(`ID`, t, func(t *ftt.Test) {
		var b spanutil.Buffer

		t.Run(`ID`, func(t *ftt.Test) {
			id := ID("a")
			assert.Loosely(t, id.ToSpanner(), should.Equal("ca978112:a"))

			row, err := spanner.NewRow([]string{"a"}, []any{id.ToSpanner()})
			assert.Loosely(t, err, should.BeNil)
			var actual ID
			err = b.FromSpanner(row, &actual)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actual, should.Equal(id))
		})

		t.Run(`IDSet`, func(t *ftt.Test) {
			ids := NewIDSet("a", "b")
			assert.Loosely(t, ids.ToSpanner(), should.Match([]string{"3e23e816:b", "ca978112:a"}))

			row, err := spanner.NewRow([]string{"a"}, []any{ids.ToSpanner()})
			assert.Loosely(t, err, should.BeNil)
			var actual IDSet
			err = b.FromSpanner(row, &actual)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actual, should.Match(ids))
		})

		t.Run(`ShardID`, func(t *ftt.Test) {
			id := ID("a")
			assert.Loosely(t, id.shardID(10), should.Equal(0))
			id = ID("b")
			assert.Loosely(t, id.shardID(10), should.Equal(6))
			id = ID("c")
			assert.Loosely(t, id.shardID(10), should.Equal(3))
			id = ID("d")
			assert.Loosely(t, id.shardID(10), should.Equal(3))
			id = ID("e")
			assert.Loosely(t, id.shardID(10), should.Equal(9))
		})
	})
}
