// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tsmon

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/info"
	"github.com/luci/luci-go/appengine/gaetesting"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/logging/gologger"
	"github.com/luci/luci-go/common/tsmon"
	"github.com/luci/luci-go/common/tsmon/store"
	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

type fakeInfo struct {
	info.Interface
}

func (i *fakeInfo) InstanceID() string { return "instance" }
func (i *fakeInfo) ModuleName() string { return "module" }

func TestFindGaps(t *testing.T) {
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
	datastore.Get(c).Testable().Consistent(true)
	c = info.Get(c).MustNamespace(instanceNamespace)
	c = gologger.StdConfig.Use(c)

	c = info.AddFilters(c, func(c context.Context, base info.Interface) info.Interface {
		return &fakeInfo{base}
	})

	c, _, _ = tsmon.WithFakes(c)
	tsmon.GetState(c).S = store.NewNilStore() // So it's recreated by initialize

	return c, clock
}

func TestHousekeepingHandler(t *testing.T) {
	Convey("Assigns task numbers to unassigned instances", t, func() {
		c, _ := buildGAETestContext()

		i := getOrCreateInstanceEntity(c)
		So(i.TaskNum, ShouldEqual, -1)

		rec := httptest.NewRecorder()
		HousekeepingHandler(c, rec, &http.Request{}, nil)
		So(rec.Code, ShouldEqual, http.StatusOK)

		i = getOrCreateInstanceEntity(c)
		So(i.TaskNum, ShouldEqual, 0)
	})

	Convey("Doesn't reassign the same task number", t, func() {
		c, clock := buildGAETestContext()

		otherInstance := instance{
			ID:          "foobar",
			TaskNum:     0,
			LastUpdated: clock.Now(),
		}
		So(datastore.Get(c).Put(&otherInstance), ShouldBeNil)

		getOrCreateInstanceEntity(c)

		rec := httptest.NewRecorder()
		HousekeepingHandler(c, rec, &http.Request{}, nil)
		So(rec.Code, ShouldEqual, http.StatusOK)

		i := getOrCreateInstanceEntity(c)
		So(i.TaskNum, ShouldEqual, 1)
	})

	Convey("Expires old instances", t, func() {
		c, clock := buildGAETestContext()
		ds := datastore.Get(c)

		oldInstance := instance{
			ID:          "foobar",
			TaskNum:     0,
			LastUpdated: clock.Now(),
		}
		So(ds.Put(&oldInstance), ShouldBeNil)
		exists, err := ds.Exists(ds.NewKey("Instance", "foobar", 0, nil))
		So(err, ShouldBeNil)
		So(exists, ShouldBeTrue)

		clock.Add(instanceExpirationTimeout + time.Second)

		rec := httptest.NewRecorder()
		HousekeepingHandler(c, rec, &http.Request{}, nil)
		So(rec.Code, ShouldEqual, http.StatusOK)

		exists, err = ds.Exists(ds.NewKey("Instance", "foobar", 0, nil))
		So(err, ShouldBeNil)
		So(exists, ShouldBeFalse)
	})
}
