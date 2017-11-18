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
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/net/context"

	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/gae/service/info"
	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/server/router"
	srvtsmon "go.chromium.org/luci/server/tsmon"

	. "github.com/smartystreets/goconvey/convey"
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
		Convey(fmt.Sprintf("%d. %v", i, test.numbers), t, func() {
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

			So(got, ShouldResemble, test.wantFirst5Gaps)
		})
	}
}

func buildGAETestContext() (context.Context, testclock.TestClock) {
	c := gaetesting.TestingContext()
	c, clock := testclock.UseTime(c, testclock.TestTimeUTC)
	ds.GetTestable(c).Consistent(true)
	c = gologger.StdConfig.Use(c)

	c, _, _ = tsmon.WithFakes(c)
	return c, clock
}

func TestHousekeepingHandler(t *testing.T) {
	t.Parallel()

	allocator := DatastoreTaskNumAllocator{}

	Convey("Assigns task numbers to unassigned instances", t, func() {
		c, _ := buildGAETestContext()

		_, err := allocator.NotifyTaskIsAlive(c, "some.task.id")
		So(err, ShouldEqual, srvtsmon.ErrNoTaskNumber)

		rec := httptest.NewRecorder()
		housekeepingHandler(&router.Context{
			Context: c,
			Writer:  rec,
			Request: &http.Request{},
		})
		So(rec.Code, ShouldEqual, http.StatusOK)

		i, err := allocator.NotifyTaskIsAlive(c, "some.task.id")
		So(err, ShouldBeNil)
		So(i, ShouldEqual, 0)
	})

	Convey("Doesn't reassign the same task number", t, func() {
		c, _ := buildGAETestContext()

		allocator.NotifyTaskIsAlive(c, "some.task.0")
		allocator.NotifyTaskIsAlive(c, "some.task.1")

		rec := httptest.NewRecorder()
		housekeepingHandler(&router.Context{
			Context: c,
			Writer:  rec,
			Request: &http.Request{},
		})
		So(rec.Code, ShouldEqual, http.StatusOK)

		i, err := allocator.NotifyTaskIsAlive(c, "some.task.0")
		So(err, ShouldBeNil)
		So(i, ShouldEqual, 0)

		i, err = allocator.NotifyTaskIsAlive(c, "some.task.1")
		So(err, ShouldBeNil)
		So(i, ShouldEqual, 1)
	})

	Convey("Expires old instances", t, func() {
		c, clock := buildGAETestContext()
		c = info.MustNamespace(c, instanceNamespace)

		for _, count := range []int{1, int(taskQueryBatchSize) + 1} {
			Convey(fmt.Sprintf("Count: %d", count), func() {
				insts := make([]*instance, count)
				keys := make([]*ds.Key, count)
				for i := 0; i < count; i++ {
					insts[i] = &instance{
						ID:          fmt.Sprintf("foobar_%d", i),
						TaskNum:     i,
						LastUpdated: clock.Now(),
					}
					keys[i] = ds.KeyForObj(c, insts[i])
				}
				So(ds.Put(c, insts), ShouldBeNil)

				exists, err := ds.Exists(c, keys)
				So(err, ShouldBeNil)
				So(exists.All(), ShouldBeTrue)

				clock.Add(instanceExpirationTimeout + time.Second)

				rec := httptest.NewRecorder()
				housekeepingHandler(&router.Context{
					Context: c,
					Writer:  rec,
					Request: &http.Request{},
				})
				So(rec.Code, ShouldEqual, http.StatusOK)

				exists, err = ds.Exists(c, keys)
				So(err, ShouldBeNil)
				So(exists.Any(), ShouldBeFalse)
			})
		}
	})
}
