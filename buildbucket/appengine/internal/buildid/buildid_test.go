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

package buildid

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestNewBuildIDs(t *testing.T) {
	t.Parallel()

	ftt.Run("NewBuildIDs", t, func(t *ftt.Test) {
		ctx := mathrand.Set(context.Background(), rand.New(rand.NewSource(0)))
		ts := time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC)
		assert.Loosely(t, 1<<16, should.Equal(65536))

		t.Run("zero", func(t *ftt.Test) {
			ids := NewBuildIDs(ctx, ts, 0)
			assert.Loosely(t, ids, should.BeEmpty)
		})

		t.Run("one", func(t *ftt.Test) {
			ids := NewBuildIDs(ctx, ts, 1)
			assert.Loosely(t, ids, should.Resemble([]int64{
				0x7DB4463C7FF2FFA1,
			}))
			assert.Loosely(t, ids[0]>>63, should.BeZero)
			assert.Loosely(t, ids[0]&0x0FFFFFFFFFFFFFFF>>buildIDTimeSuffixLen, should.Equal(941745227775))
			assert.Loosely(t, ids[0]&0x0000000000000001, should.Equal(buildIDVersion))
		})

		t.Run("two", func(t *ftt.Test) {
			ids := NewBuildIDs(ctx, ts, 2)
			assert.Loosely(t, ids, should.Resemble([]int64{
				0x7DB4463C7FFA8F71,
				0x7DB4463C7FFA8F61,
			}))
			assert.Loosely(t, ids[0]>>63, should.BeZero)
			assert.Loosely(t, ids[0]&0x0000000000000001, should.Equal(buildIDVersion))
			assert.Loosely(t, ids[0]&0x0FFFFFFFFFFFFFFF>>buildIDTimeSuffixLen, should.Equal(941745227775))
			assert.Loosely(t, ids[1]>>63, should.BeZero)
			assert.Loosely(t, ids[1]&0x0000000000000001, should.Equal(buildIDVersion))
			assert.Loosely(t, ids[1]&0x0FFFFFFFFFFFFFFF>>buildIDTimeSuffixLen, should.Equal(941745227775))
		})

		t.Run("many", func(t *ftt.Test) {
			for i := 0; i < 2^16; i++ {
				ids := NewBuildIDs(ctx, ts, i)
				assert.Loosely(t, ids, should.HaveLength(i))
				prev := BuildIDMax
				for _, id := range ids {
					// Ensure strictly decreasing.
					assert.Loosely(t, id, should.BeLessThan(prev))
					prev = id
					// Ensure positive.
					assert.Loosely(t, id>>63, should.BeZero)
					// Ensure time component.
					assert.Loosely(t, id&0x0FFFFFFFFFFFFFFF>>buildIDTimeSuffixLen, should.Equal(941745227775))
					// Ensure version.
					assert.Loosely(t, id&0x000000000000000F, should.Equal(buildIDVersion))
				}
			}
		})
	})
}

func TestIDRange(t *testing.T) {
	t.Parallel()

	ftt.Run("IDRange", t, func(t *ftt.Test) {
		t.Run("valid time", func(t *ftt.Test) {
			timeLow := time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC)
			timeHigh := timeLow.Add(timeResolution * 10000)
			idLow, idHigh := IDRange(timeLow, timeHigh)

			inRange := func(t time.Time, suffix int64) bool {
				buildID := idTimeSegment(t) | suffix
				return idLow <= buildID && buildID < idHigh
			}
			ones := (int64(1) << buildIDTimeSuffixLen) - 1

			// Ensure that min and max possible build IDs are within
			// the range up to the timeResolution.
			for _, suffix := range []int64{0, ones} {
				assert.Loosely(t, inRange(timeLow.Add(-timeResolution), suffix), should.BeFalse)
				assert.Loosely(t, inRange(timeLow, suffix), should.BeTrue)
				assert.Loosely(t, inRange(timeLow.Add(timeResolution), suffix), should.BeTrue)

				assert.Loosely(t, inRange(timeHigh.Add(-timeResolution), suffix), should.BeTrue)
				assert.Loosely(t, inRange(timeHigh, suffix), should.BeFalse)
				assert.Loosely(t, inRange(timeHigh.Add(timeResolution), suffix), should.BeFalse)
			}
		})

		t.Run("invalid time", func(t *ftt.Test) {
			timeLow := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
			timeHigh := timeLow.Add(timeResolution * 10000)
			idLow, idHigh := IDRange(timeLow, timeHigh)
			assert.Loosely(t, idLow, should.BeZero)
			assert.Loosely(t, idHigh, should.BeZero)
		})
	})
}

func TestIDTimeSegment(t *testing.T) {
	t.Parallel()

	ftt.Run("idTimeSegment", t, func(t *ftt.Test) {
		t.Run("after the start of the word time", func(t *ftt.Test) {
			id := idTimeSegment(beginningOfTheWorld.Add(timeResolution))
			assert.Loosely(t, id, should.Equal(0x7FFFFFFFFFE00000))
		})

		t.Run("at the start of the word time", func(t *ftt.Test) {
			id := idTimeSegment(beginningOfTheWorld)
			assert.Loosely(t, id, should.Equal(0x7FFFFFFFFFF00000))
		})

		t.Run("before the start of the word time", func(t *ftt.Test) {
			ts := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
			id := idTimeSegment(ts)
			assert.Loosely(t, id, should.BeZero)
		})
	})
}

func TestMayContainBuilds(t *testing.T) {
	t.Parallel()

	ftt.Run("normal", t, func(t *ftt.Test) {
		low := time.Date(2011, 1, 1, 0, 0, 0, 0, time.UTC)
		high := time.Date(2011, 2, 1, 0, 0, 0, 0, time.UTC)
		assert.Loosely(t, MayContainBuilds(low, high), should.BeTrue)
	})

	ftt.Run("low time is larger than high time", t, func(t *ftt.Test) {
		low := time.Date(2011, 2, 1, 0, 0, 0, 0, time.UTC)
		high := time.Date(2011, 1, 1, 0, 0, 0, 0, time.UTC)
		assert.Loosely(t, MayContainBuilds(low, high), should.BeFalse)
	})

	ftt.Run("low and high time are nil", t, func(t *ftt.Test) {
		low := time.Time{}
		high := time.Time{}
		assert.Loosely(t, MayContainBuilds(low, high), should.BeTrue)
	})

	ftt.Run("high time is less than beginningOfTheWorld", t, func(t *ftt.Test) {
		low := time.Date(2011, 2, 1, 0, 0, 0, 0, time.UTC)
		high := beginningOfTheWorld.Add(-1)
		assert.Loosely(t, MayContainBuilds(low, high), should.BeFalse)
	})
}
