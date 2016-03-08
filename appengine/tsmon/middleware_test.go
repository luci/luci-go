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
	"github.com/luci/luci-go/common/tsmon/store"
	"github.com/luci/luci-go/common/tsmon/target"
	"github.com/luci/luci-go/common/tsmon/types"
	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

type fakeMonitor struct {
	chunkSize int
	cells     [][]types.Cell
}

func (m *fakeMonitor) ChunkSize() int {
	return m.chunkSize
}

func (m *fakeMonitor) Send(ctx context.Context, cells []types.Cell) error {
	m.cells = append(m.cells, cells)
	return nil
}

func TestMiddleware(t *testing.T) {
	monitor := &fakeMonitor{}
	metric := &store.FakeMetric{"m", []field.Field{}, types.CumulativeIntType}

	f := func(c context.Context, rw http.ResponseWriter, r *http.Request, p httprouter.Params) {
		So(store.IsNilStore(tsmon.Store()), ShouldBeFalse)

		monitor.cells = nil
		tsmon.Initialize(monitor, tsmon.Store())

		tsmon.Unregister(metric)
		tsmon.Register(metric)
		So(tsmon.Store().Incr(c, metric, time.Time{}, []interface{}{}, int64(1)), ShouldBeNil)
	}

	Convey("Creates instance entity", t, func() {
		ctx, _ := buildGAETestContext()
		ds := datastore.Get(ctx)

		exists, err := ds.Exists(ds.NewKey("Instance", instanceEntityID(ctx), 0, nil))
		So(err, ShouldBeNil)
		So(exists, ShouldBeFalse)

		rec := httptest.NewRecorder()
		Middleware(f)(ctx, rec, &http.Request{}, nil)
		So(rec.Code, ShouldEqual, http.StatusOK)

		exists, err = ds.Exists(ds.NewKey("Instance", instanceEntityID(ctx), 0, nil))
		So(err, ShouldBeNil)
		So(exists, ShouldBeTrue)

		// Shouldn't flush since the instance entity doesn't have a task number yet.
		So(len(monitor.cells), ShouldEqual, 0)
	})

	Convey("Flushes after 2 minutes", t, func() {
		ctx, clock := buildGAETestContext()
		ds := datastore.Get(ctx)

		i := instance{
			ID:          instanceEntityID(ctx),
			TaskNum:     0,
			LastUpdated: clock.Now().Add(-2 * time.Minute),
		}
		So(ds.Put(&i), ShouldBeNil)

		lastFlushed.Time = clock.Now().Add(-2 * time.Minute)

		rec := httptest.NewRecorder()
		Middleware(f)(ctx, rec, &http.Request{}, nil)
		So(rec.Code, ShouldEqual, http.StatusOK)

		So(len(monitor.cells), ShouldEqual, 1)
		So(monitor.cells[0][0].Name, ShouldEqual, "m")
		So(monitor.cells[0][0].Value, ShouldEqual, int64(1))

		// Flushing should update the LastUpdated time.
		i = *getOrCreateInstanceEntity(ctx)
		So(i.LastUpdated, ShouldResemble, clock.Now().Round(time.Second))

		// The value should still be set.
		value, err := tsmon.Store().Get(ctx, metric, time.Time{}, []interface{}{})
		So(err, ShouldBeNil)
		So(value, ShouldEqual, int64(1))
	})

	Convey("Resets cumulative metrics", t, func() {
		ctx, clock := buildGAETestContext()

		tsmon.Store().DefaultTarget().(*target.Task).TaskNum = proto.Int32(int32(0))
		lastFlushed.Time = clock.Now().Add(-2 * time.Minute)

		rec := httptest.NewRecorder()
		Middleware(f)(ctx, rec, &http.Request{}, nil)
		So(rec.Code, ShouldEqual, http.StatusOK)

		So(len(monitor.cells), ShouldEqual, 0)

		// Value should be reset.
		value, err := tsmon.Store().Get(ctx, metric, time.Time{}, []interface{}{})
		So(err, ShouldBeNil)
		So(value, ShouldBeNil)
	})
}
