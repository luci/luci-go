// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tsmon

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/julienschmidt/httprouter"
	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/tsmon"
	"github.com/luci/luci-go/common/tsmon/metric"
	"github.com/luci/luci-go/common/tsmon/monitor"
	"github.com/luci/luci-go/common/tsmon/store"
	"github.com/luci/luci-go/common/tsmon/target"
	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

func flushNowWithMiddleware(c context.Context, clock testclock.TestClock) {
	ds := datastore.Get(c)

	i := instance{
		ID:          instanceEntityID(c),
		TaskNum:     0,
		LastUpdated: clock.Now().Add(-2 * time.Minute),
	}
	So(ds.Put(&i), ShouldBeNil)

	lastFlushed.Time = clock.Now().Add(-2 * time.Minute)

	rec := httptest.NewRecorder()
	Middleware(func(c context.Context, rw http.ResponseWriter, r *http.Request, p httprouter.Params) {})(c, rec, &http.Request{}, nil)
	So(rec.Code, ShouldEqual, http.StatusOK)
}

func TestGlobalCallbacks(t *testing.T) {
	c := context.Background()

	Convey("Register callback without metrics panics", t, func() {
		So(func() {
			RegisterGlobalCallbackIn(c, func(context.Context) {})
		}, ShouldPanic)
	})

	Convey("Global callbacks", t, func() {
		c, clock := buildGAETestContext()
		s := store.NewInMemory(&target.Task{ServiceName: "default target"})
		mon := &monitor.Fake{}
		state := tsmon.GetState(c)
		state.S = s
		state.M = mon
		m := metric.NewCallbackStringIn(c, "foo", "")

		RegisterGlobalCallbackIn(c, func(c context.Context) {
			m.Set(c, "bar")
		}, m)

		Convey("are not run on flush", func() {
			flushNowWithMiddleware(c, clock)
			val, err := s.Get(c, m, time.Time{}, []interface{}{})
			So(err, ShouldBeNil)
			So(val, ShouldBeNil)
		})

		Convey("but are run by housekeeping", func() {
			rec := httptest.NewRecorder()
			HousekeepingHandler(c, rec, &http.Request{}, nil)
			So(rec.Code, ShouldEqual, http.StatusOK)

			val, err := s.Get(c, m, time.Time{}, []interface{}{})
			So(err, ShouldBeNil)
			So(val, ShouldEqual, "bar")

			Convey("and are reset on flush", func() {
				flushNowWithMiddleware(c, clock)

				val, err = s.Get(c, m, time.Time{}, []interface{}{})
				So(err, ShouldBeNil)
				So(val, ShouldBeNil)

				So(len(mon.Cells), ShouldEqual, 1)
				So(len(mon.Cells[0]), ShouldEqual, 1)
				So(mon.Cells[0][0].Value, ShouldEqual, "bar")
			})
		})
	})
}
