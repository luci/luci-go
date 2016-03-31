// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tsmon

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/julienschmidt/httprouter"
	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/common/tsmon"
	"github.com/luci/luci-go/common/tsmon/field"
	"github.com/luci/luci-go/common/tsmon/monitor"
	"github.com/luci/luci-go/common/tsmon/store"
	"github.com/luci/luci-go/common/tsmon/target"
	"github.com/luci/luci-go/common/tsmon/types"
	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMiddleware(t *testing.T) {
	metric := &store.FakeMetric{"m", "", []field.Field{}, types.CumulativeIntType}

	f := func(c context.Context, rw http.ResponseWriter, r *http.Request, p httprouter.Params) {
		So(store.IsNilStore(tsmon.Store(c)), ShouldBeFalse)

		// Replace the monitor with a fake so the flush goes there instead.
		tsmon.GetState(c).M = &monitor.Fake{}

		tsmon.Register(c, metric)
		So(tsmon.Store(c).Incr(c, metric, time.Time{}, []interface{}{}, int64(1)), ShouldBeNil)
	}

	lastFlushed.Time = time.Time{}

	Convey("Creates instance entity", t, func() {
		c, _ := buildGAETestContext()
		ds := datastore.Get(c)

		exists, err := ds.Exists(ds.NewKey("Instance", instanceEntityID(c), 0, nil))
		So(err, ShouldBeNil)
		So(exists, ShouldBeFalse)

		rec := httptest.NewRecorder()
		Middleware(f)(c, rec, &http.Request{}, nil)
		So(rec.Code, ShouldEqual, http.StatusOK)

		exists, err = ds.Exists(ds.NewKey("Instance", instanceEntityID(c), 0, nil))
		So(err, ShouldBeNil)
		So(exists, ShouldBeTrue)

		// Shouldn't flush since the instance entity doesn't have a task number yet.
		monitor := tsmon.GetState(c).M.(*monitor.Fake)
		So(len(monitor.Cells), ShouldEqual, 0)
	})

	Convey("Flushes after 2 minutes", t, func() {
		c, clock := buildGAETestContext()

		ds := datastore.Get(c)

		i := instance{
			ID:          instanceEntityID(c),
			TaskNum:     0,
			LastUpdated: clock.Now().Add(-2 * time.Minute),
		}
		So(ds.Put(&i), ShouldBeNil)

		lastFlushed.Time = clock.Now().Add(-2 * time.Minute)

		rec := httptest.NewRecorder()
		Middleware(f)(c, rec, &http.Request{}, nil)
		So(rec.Code, ShouldEqual, http.StatusOK)

		monitor := tsmon.GetState(c).M.(*monitor.Fake)
		So(len(monitor.Cells), ShouldEqual, 1)
		So(monitor.Cells[0][0].Name, ShouldEqual, "m")
		So(monitor.Cells[0][0].Value, ShouldEqual, int64(1))

		// Flushing should update the LastUpdated time.
		i = *getOrCreateInstanceEntity(c)
		So(i.LastUpdated, ShouldResemble, clock.Now().Round(time.Second))

		// The value should still be set.
		value, err := tsmon.Store(c).Get(c, metric, time.Time{}, []interface{}{})
		So(err, ShouldBeNil)
		So(value, ShouldEqual, int64(1))
	})

	Convey("Resets cumulative metrics", t, func() {
		c, clock := buildGAETestContext()

		lastFlushed.Time = clock.Now().Add(-2 * time.Minute)

		rec := httptest.NewRecorder()
		Middleware(func(c context.Context, rw http.ResponseWriter, r *http.Request, p httprouter.Params) {
			f(c, rw, r, p)

			// Override the TaskNum here - it's created just before this handler runs
			// and used just after.
			tar := tsmon.Store(c).DefaultTarget().(*target.Task)
			tar.TaskNum = proto.Int32(int32(0))
			tsmon.Store(c).SetDefaultTarget(tar)
		})(c, rec, &http.Request{}, nil)
		So(rec.Code, ShouldEqual, http.StatusOK)

		So(len(tsmon.GetState(c).RegisteredMetrics), ShouldEqual, 1)

		monitor := tsmon.GetState(c).M.(*monitor.Fake)
		So(len(monitor.Cells), ShouldEqual, 0)

		// Value should be reset.
		value, err := tsmon.Store(c).Get(c, metric, time.Time{}, []interface{}{})
		So(err, ShouldBeNil)
		So(value, ShouldBeNil)
	})
}
