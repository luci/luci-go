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

package tsmon

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/target"
	"go.chromium.org/luci/gae/service/datastore"
	srvtsmon "go.chromium.org/luci/server/tsmon"
)

func TestFindGaps(t *testing.T) {
	t.Parallel()

	tests := []struct {
		numbers        []int
		wantFirst5Gaps []int
	}{
		{[]int{1}, []int{0, 2, 3, 4, 5}},
		{[]int{-1, 1}, []int{0, 2, 3, 4, 5}},
		{[]int{1, 3, 5}, []int{0, 2, 4, 6, 7}},
		{[]int{5, 3, 1}, []int{0, 2, 4, 6, 7}},
		{[]int{3, 1, 5}, []int{0, 2, 4, 6, 7}},
		{[]int{4}, []int{0, 1, 2, 3, 5}},
	}

	for i, test := range tests {
		ftt.Run(fmt.Sprintf("%d. %v", i, test.numbers), t, func(t *ftt.Test) {
			numbers := map[int]struct{}{}
			for _, n := range test.numbers {
				numbers[n] = struct{}{}
			}

			nextNum := gapFinder(numbers)

			// Read 5 numbers from the channel.
			var got []int
			for i := 0; i < 5; i++ {
				got = append(got, nextNum())
			}

			assert.Loosely(t, got, should.Resemble(test.wantFirst5Gaps))
		})
	}
}

func buildGAETestContext() (context.Context, testclock.TestClock) {
	c := gaetesting.TestingContext()
	c, clock := testclock.UseTime(c, testclock.TestTimeUTC)
	datastore.GetTestable(c).Consistent(true)
	c, _, _ = tsmon.WithFakes(c)
	return c, clock
}

func TestAssignTaskNumbers(t *testing.T) {
	t.Parallel()

	task1 := target.Task{HostName: "1"}
	task2 := target.Task{HostName: "2"}

	allocator := DatastoreTaskNumAllocator{}

	ftt.Run("Assigns task numbers to unassigned instances", t, func(t *ftt.Test) {
		c, _ := buildGAETestContext()

		// Two new processes belonging to different targets.
		_, err := allocator.NotifyTaskIsAlive(c, &task1, "some.id")
		assert.Loosely(t, err, should.Equal(srvtsmon.ErrNoTaskNumber))
		_, err = allocator.NotifyTaskIsAlive(c, &task2, "some.id")
		assert.Loosely(t, err, should.Equal(srvtsmon.ErrNoTaskNumber))

		assert.Loosely(t, AssignTaskNumbers(c), should.BeNil)

		// Both get 0, since they belong to different targets.
		i, err := allocator.NotifyTaskIsAlive(c, &task1, "some.id")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, i, should.BeZero)
		i, err = allocator.NotifyTaskIsAlive(c, &task2, "some.id")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, i, should.BeZero)

		// Once numbers are assigned, AssignTaskNumbers is noop.
		assert.Loosely(t, AssignTaskNumbers(c), should.BeNil)
	})

	ftt.Run("Doesn't assign the same task number", t, func(t *ftt.Test) {
		c, _ := buildGAETestContext()

		// Two processes belonging to a single target.
		allocator.NotifyTaskIsAlive(c, &task1, "some.task.0")
		allocator.NotifyTaskIsAlive(c, &task1, "some.task.1")

		assert.Loosely(t, AssignTaskNumbers(c), should.BeNil)

		// Get different task numbers.
		i, err := allocator.NotifyTaskIsAlive(c, &task1, "some.task.0")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, i, should.BeZero)
		i, err = allocator.NotifyTaskIsAlive(c, &task1, "some.task.1")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, i, should.Equal(1))
	})

	ftt.Run("Expires old instances", t, func(t *ftt.Test) {
		c, clock := buildGAETestContext()

		for _, count := range []int{1, taskQueryBatchSize + 1} {
			t.Run(fmt.Sprintf("Count: %d", count), func(t *ftt.Test) {
				// Request a bunch of task numbers.
				for i := 0; i < count; i++ {
					_, err := allocator.NotifyTaskIsAlive(c, &task1, fmt.Sprintf("%d", i))
					assert.Loosely(t, err, should.Equal(srvtsmon.ErrNoTaskNumber))
				}

				// Get all the numbers assigned.
				assert.Loosely(t, AssignTaskNumbers(c), should.BeNil)

				// Yep. Assigned.
				numbers := map[int]struct{}{}
				for i := 0; i < count; i++ {
					num, err := allocator.NotifyTaskIsAlive(c, &task1, fmt.Sprintf("%d", i))
					assert.Loosely(t, err, should.BeNil)
					numbers[num] = struct{}{}
				}
				assert.Loosely(t, len(numbers), should.Equal(count))

				// Move time to make number assignments expire.
				clock.Add(instanceExpirationTimeout + time.Second)
				assert.Loosely(t, AssignTaskNumbers(c), should.BeNil)

				// Yep. Expired.
				for i := 0; i < count; i++ {
					_, err := allocator.NotifyTaskIsAlive(c, &task1, fmt.Sprintf("%d", i))
					assert.Loosely(t, err, should.Equal(srvtsmon.ErrNoTaskNumber))
				}
			})
		}
	})
}
