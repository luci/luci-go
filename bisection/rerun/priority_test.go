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

	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/bisection/model"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
)

func TestPriority(t *testing.T) {
	t.Parallel()

	Convey("CapPriority", t, func() {
		So(CapPriority(40), ShouldEqual, 40)
		So(CapPriority(260), ShouldEqual, 255)
		So(CapPriority(10), ShouldEqual, 20)
	})
}

func TestOffsetDuration(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())
	cl := testclock.New(testclock.TestTimeUTC)
	c = clock.Set(c, cl)

	Convey("OffsetDuration", t, func() {
		now := clock.Now(c)
		fb := &model.LuciFailedBuild{
			Id: 123,
			LuciBuild: model.LuciBuild{
				StartTime: now,
				EndTime:   now.Add(9 * time.Minute),
			},
		}
		So(datastore.Put(c, fb), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		cf := &model.CompileFailure{
			Build: datastore.KeyForObj(c, fb),
		}
		So(datastore.Put(c, cf), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		cfa := &model.CompileFailureAnalysis{
			CompileFailure: datastore.KeyForObj(c, cf),
		}
		So(datastore.Put(c, cfa), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		pri, err := OffsetPriorityBasedOnRunDuration(c, 100, cfa)
		So(err, ShouldBeNil)
		So(pri, ShouldEqual, 80)

		fb.LuciBuild.EndTime = now.Add(20 * time.Minute)
		So(datastore.Put(c, fb), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()
		pri, err = OffsetPriorityBasedOnRunDuration(c, 100, cfa)
		So(err, ShouldBeNil)
		So(pri, ShouldEqual, 90)

		fb.LuciBuild.EndTime = now.Add(50 * time.Minute)
		So(datastore.Put(c, fb), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()
		pri, err = OffsetPriorityBasedOnRunDuration(c, 100, cfa)
		So(err, ShouldBeNil)
		So(pri, ShouldEqual, 100)

		fb.LuciBuild.EndTime = now.Add(90 * time.Minute)
		So(datastore.Put(c, fb), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()
		pri, err = OffsetPriorityBasedOnRunDuration(c, 100, cfa)
		So(err, ShouldBeNil)
		So(pri, ShouldEqual, 120)

		fb.LuciBuild.EndTime = now.Add(300 * time.Minute)
		So(datastore.Put(c, fb), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()
		pri, err = OffsetPriorityBasedOnRunDuration(c, 100, cfa)
		So(err, ShouldBeNil)
		So(pri, ShouldEqual, 140)
	})
}
