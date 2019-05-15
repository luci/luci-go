// Copyright 2019 The LUCI Authors.
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

package metrics

import (
	"context"
	"testing"
	"time"

	"go.chromium.org/gae/impl/memory"
	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/tsmon"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTasks(t *testing.T) {
	t.Parallel()

	Convey("Tasks", t, func() {
		c, _ := tsmon.WithDummyInMemory(memory.Use(context.Background()))
		datastore.GetTestable(c).AutoIndex(true)
		datastore.GetTestable(c).Consistent(true)
		s := tsmon.Store(c)

		Convey("TaskCount", func() {
			tc := &TaskCount{}
			So(tc.Executing, ShouldEqual, 0)
			So(tc.Total, ShouldEqual, 0)
			So(tc.Update(c, "queue", 1, 2), ShouldBeNil)

			tc = &TaskCount{
				ID: "queue",
			}
			So(datastore.Get(c, tc), ShouldBeNil)
			So(tc.Executing, ShouldEqual, 1)
			So(tc.Total, ShouldEqual, 2)
			So(tc.Queue, ShouldEqual, tc.ID)
		})

		Convey("updateTasks", func() {
			fields := []interface{}{"queue"}

			tc := &TaskCount{
				ID:        "queue",
				Executing: 1,
				Queue:     "queue",
				Total:     3,
			}

			tc.Computed = time.Time{}
			So(datastore.Put(c, tc), ShouldBeNil)
			updateTasks(c)
			So(s.Get(c, tasksExecuting, time.Time{}, fields), ShouldBeNil)
			So(s.Get(c, tasksPending, time.Time{}, fields), ShouldBeNil)
			So(s.Get(c, tasksTotal, time.Time{}, fields), ShouldBeNil)
			So(datastore.Get(c, &TaskCount{
				ID: tc.ID,
			}), ShouldEqual, datastore.ErrNoSuchEntity)

			tc.Computed = time.Now().UTC()
			So(datastore.Put(c, tc), ShouldBeNil)
			updateTasks(c)
			So(s.Get(c, tasksExecuting, time.Time{}, fields).(int64), ShouldEqual, 1)
			So(s.Get(c, tasksPending, time.Time{}, fields).(int64), ShouldEqual, 2)
			So(s.Get(c, tasksTotal, time.Time{}, fields).(int64), ShouldEqual, 3)
			So(datastore.Get(c, &TaskCount{
				ID: tc.ID,
			}), ShouldBeNil)
		})
	})
}
