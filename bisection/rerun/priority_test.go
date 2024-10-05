// Copyright 2022 The LUCI Authors.
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

package rerun

import (
	"context"
	"testing"
	"time"

	"go.chromium.org/luci/bisection/model"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
)

func TestPriority(t *testing.T) {
	t.Parallel()

	ftt.Run("CapPriority", t, func(t *ftt.Test) {
		assert.Loosely(t, CapPriority(40), should.Equal(40))
		assert.Loosely(t, CapPriority(260), should.Equal(255))
		assert.Loosely(t, CapPriority(10), should.Equal(20))
	})
}

func TestOffsetDuration(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())
	cl := testclock.New(testclock.TestTimeUTC)
	c = clock.Set(c, cl)

	ftt.Run("OffsetDuration", t, func(t *ftt.Test) {
		now := clock.Now(c)
		fb := &model.LuciFailedBuild{
			Id: 123,
			LuciBuild: model.LuciBuild{
				StartTime: now,
				EndTime:   now.Add(9 * time.Minute),
			},
		}
		assert.Loosely(t, datastore.Put(c, fb), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		cf := &model.CompileFailure{
			Build: datastore.KeyForObj(c, fb),
		}
		assert.Loosely(t, datastore.Put(c, cf), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		cfa := &model.CompileFailureAnalysis{
			CompileFailure: datastore.KeyForObj(c, cf),
		}
		assert.Loosely(t, datastore.Put(c, cfa), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		pri, err := OffsetPriorityBasedOnRunDuration(c, 100, cfa)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, pri, should.Equal(80))

		fb.LuciBuild.EndTime = now.Add(20 * time.Minute)
		assert.Loosely(t, datastore.Put(c, fb), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()
		pri, err = OffsetPriorityBasedOnRunDuration(c, 100, cfa)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, pri, should.Equal(90))

		fb.LuciBuild.EndTime = now.Add(50 * time.Minute)
		assert.Loosely(t, datastore.Put(c, fb), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()
		pri, err = OffsetPriorityBasedOnRunDuration(c, 100, cfa)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, pri, should.Equal(100))

		fb.LuciBuild.EndTime = now.Add(90 * time.Minute)
		assert.Loosely(t, datastore.Put(c, fb), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()
		pri, err = OffsetPriorityBasedOnRunDuration(c, 100, cfa)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, pri, should.Equal(120))

		fb.LuciBuild.EndTime = now.Add(300 * time.Minute)
		assert.Loosely(t, datastore.Put(c, fb), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()
		pri, err = OffsetPriorityBasedOnRunDuration(c, 100, cfa)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, pri, should.Equal(140))
	})
}
