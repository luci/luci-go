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

package tsmon

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/monitor"
	"go.chromium.org/luci/common/tsmon/store"
	"go.chromium.org/luci/common/tsmon/target"

	"go.chromium.org/luci/server/router"
)

var testMetric = metric.NewCounter("test_metric", "", nil)

func TestMiddleware(t *testing.T) {
	t.Parallel()

	runMiddlware := func(c context.Context, state *State, cb func(*router.Context)) {
		rec := httptest.NewRecorder()
		router.RunMiddleware(
			&router.Context{Writer: rec, Request: (&http.Request{}).WithContext(c)},
			router.NewMiddlewareChain(state.Middleware),
			cb,
		)
		assert.Loosely(t, rec.Code, should.Equal(http.StatusOK))
	}

	incrMetric := func(c *router.Context) {
		assert.Loosely(t, store.IsNilStore(tsmon.Store(c.Request.Context())), should.BeFalse)
		tsmon.Store(c.Request.Context()).Incr(c.Request.Context(), testMetric, []any{}, int64(1))
	}

	readMetric := func(c context.Context) any {
		return tsmon.Store(c).Get(c, testMetric, []any{})
	}

	ftt.Run("With fakes", t, func(t *ftt.Test) {
		c, clock := buildTestContext()
		state, monitor, allocator := buildTestState()

		t.Run("Pings TaskNumAllocator and waits for number", func(t *ftt.Test) {
			assert.Loosely(t, allocator.instanceIDs, should.HaveLength(0))
			allocator.taskNum = -1 // no number yet

			// Do the flush.
			state.nextFlush = clock.Now()
			runMiddlware(c, state, incrMetric)

			// Called the allocator.
			assert.Loosely(t, allocator.instanceIDs, should.HaveLength(1))
			// Shouldn't flush since the instance entity doesn't have a task number yet.
			assert.Loosely(t, monitor.Cells, should.HaveLength(0))

			// Wait until next expected flush.
			clock.Add(time.Minute)
			allocator.taskNum = 0 // got the number!

			// Do the flush again.
			state.nextFlush = clock.Now()
			runMiddlware(c, state, incrMetric)

			// Flushed stuff this time.
			assert.Loosely(t, monitor.Cells, should.HaveLength(1))

			// The value should still be set.
			assert.Loosely(t, readMetric(c), should.Equal(int64(2)))
		})

		t.Run("Resets cumulative metrics", func(t *ftt.Test) {
			// Do the flush.
			state.nextFlush = clock.Now()
			runMiddlware(c, state, incrMetric)

			// Flushed stuff.
			assert.Loosely(t, monitor.Cells, should.HaveLength(1))
			monitor.Cells = nil

			// The value is set.
			assert.Loosely(t, readMetric(c), should.Equal(int64(1)))

			// Lost the task number.
			allocator.taskNum = -1

			// Do the flush again.
			state.nextFlush = clock.Now()
			runMiddlware(c, state, incrMetric)

			// No stuff is sent, and cumulative metrics are reset.
			assert.Loosely(t, monitor.Cells, should.HaveLength(0))
			assert.Loosely(t, readMetric(c), should.BeNil)
		})

		t.Run("Dynamic enable and disable works", func(t *ftt.Test) {
			// Enabled. Store is not nil.
			runMiddlware(c, state, func(c *router.Context) {
				assert.Loosely(t, store.IsNilStore(tsmon.Store(c.Request.Context())), should.BeFalse)
			})

			// Disabled. Store is nil.
			state.Settings.Enabled = false
			runMiddlware(c, state, func(c *router.Context) {
				assert.Loosely(t, store.IsNilStore(tsmon.Store(c.Request.Context())), should.BeTrue)
			})
		})

		t.Run("Check error backoff", func(t *ftt.Test) {
			// Flush now.
			state.nextFlush = clock.Now()
			runMiddlware(c, state, incrMetric)
			// Break the monitor.
			monitor.SendErr = http.ErrServerClosed
			clock.Add(time.Minute)
			flushTime := clock.Now()
			lastRetry := state.flushRetry
			state.nextFlush = flushTime
			runMiddlware(c, state, incrMetric)
			// lastFlush should not have changed.
			assert.Loosely(t, state.lastFlush, should.NotMatch(flushTime))
			assert.Loosely(t, state.flushRetry, should.Equal(lastRetry*2))
			clock.Add(time.Minute)
			runMiddlware(c, state, incrMetric)
			// Check retry is doubling.
			assert.Loosely(t, state.flushRetry, should.Equal(lastRetry*4))
		})
	})
}

////////////////////////////////////////////////////////////////////////////////

func buildTestContext() (context.Context, testclock.TestClock) {
	c := context.Background()
	c, clock := testclock.UseTime(c, testclock.TestTimeUTC)
	c, _, _ = tsmon.WithFakes(c)
	return c, clock
}

func buildTestState() (*State, *monitor.Fake, *fakeNumAllocator) {
	mon := &monitor.Fake{}
	allocator := &fakeNumAllocator{}
	return &State{
		Target: func(c context.Context) target.Task {
			return target.Task{
				DataCenter:  "datacenter",
				ServiceName: "app-id",
				JobName:     "service-name",
				HostName:    "12345-version",
			}
		},
		InstanceID:        func(c context.Context) string { return "some.id" },
		TaskNumAllocator:  allocator,
		FlushInMiddleware: true,
		CustomMonitor:     mon,
		Settings: &Settings{
			Enabled:          true,
			FlushIntervalSec: 60,
		},
	}, mon, allocator
}

type fakeNumAllocator struct {
	taskNum     int
	instanceIDs []string
}

func (a *fakeNumAllocator) NotifyTaskIsAlive(c context.Context, task *target.Task, instanceID string) (int, error) {
	a.instanceIDs = append(a.instanceIDs, instanceID)
	if a.taskNum == -1 {
		return 0, ErrNoTaskNumber
	}
	return a.taskNum, nil
}
