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

package certconfig

import (
	"testing"
	"time"

	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	ds "go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/caching"
)

func TestListCAs(t *testing.T) {
	ftt.Run("ListCAs works", t, func(t *ftt.Test) {
		ctx := gaetesting.TestingContext()

		// Empty.
		cas, err := ListCAs(ctx)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, len(cas), should.BeZero)

		// Add some.
		err = ds.Put(ctx, &CA{CN: "abc", Removed: true}, &CA{CN: "def"})
		assert.Loosely(t, err, should.BeNil)
		ds.GetTestable(ctx).CatchupIndexes()

		cas, err = ListCAs(ctx)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, cas, should.Resemble([]string{"def"}))
	})
}

func TestCAUniqueIDToCNMapLoadStore(t *testing.T) {
	ftt.Run("CAUniqueIDToCNMap Load and Store works", t, func(t *ftt.Test) {
		ctx := gaetesting.TestingContext()

		// Empty.
		mapping, err := LoadCAUniqueIDToCNMap(ctx)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, len(mapping), should.BeZero)

		// Store some.
		toStore := map[int64]string{
			1: "abc",
			2: "def",
		}
		err = StoreCAUniqueIDToCNMap(ctx, toStore)
		assert.Loosely(t, err, should.BeNil)

		// Not empty now.
		mapping, err = LoadCAUniqueIDToCNMap(ctx)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, mapping, should.Resemble(toStore))
	})
}

func TestGetCAByUniqueID(t *testing.T) {
	ftt.Run("GetCAByUniqueID works", t, func(t *ftt.Test) {
		ctx := gaetesting.TestingContext()
		ctx = caching.WithEmptyProcessCache(ctx)
		ctx, clk := testclock.UseTime(ctx, testclock.TestTimeUTC)

		// Empty now.
		val, err := GetCAByUniqueID(ctx, 1)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, val, should.BeEmpty)

		// Add some.
		err = StoreCAUniqueIDToCNMap(ctx, map[int64]string{
			1: "abc",
			2: "def",
		})
		assert.Loosely(t, err, should.BeNil)

		// Still empty (cached old value).
		val, err = GetCAByUniqueID(ctx, 1)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, val, should.BeEmpty)

		// Updated after cache expires.
		clk.Add(2 * time.Minute)
		val, err = GetCAByUniqueID(ctx, 1)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, val, should.Equal("abc"))
	})
}
