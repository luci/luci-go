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
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/tsmon"
	"github.com/luci/luci-go/common/tsmon/metric"
	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

func flushNowWithMiddleware(c context.Context, state *State) {
	ds := datastore.Get(c)

	i := instance{
		ID:          instanceEntityID(c),
		TaskNum:     0,
		LastUpdated: clock.Now(c).Add(-2 * time.Minute).UTC(),
	}
	So(ds.Put(&i), ShouldBeNil)

	state.lastFlushed = clock.Now(c).Add(-2 * time.Minute)

	rec := httptest.NewRecorder()
	state.Middleware(func(c context.Context, rw http.ResponseWriter, r *http.Request, p httprouter.Params) {})(c, rec, &http.Request{}, nil)
	So(rec.Code, ShouldEqual, http.StatusOK)
}

func TestGlobalCallbacks(t *testing.T) {
	Convey("Global callbacks", t, func() {
		c, _ := buildGAETestContext()
		state, mon := buildTestState()

		m := metric.NewCallbackStringIn(c, "foo", "")

		tsmon.RegisterGlobalCallbackIn(c, func(c context.Context) {
			m.Set(c, "bar")
		}, m)

		Convey("are not run on flush", func() {
			flushNowWithMiddleware(c, state)
			val, err := tsmon.Store(c).Get(c, m, time.Time{}, []interface{}{})
			So(err, ShouldBeNil)
			So(val, ShouldBeNil)
		})

		Convey("but are run by housekeeping", func() {
			state.checkSettings(c) // initialize the in-memory store
			s := tsmon.Store(c)

			rec := httptest.NewRecorder()
			housekeepingHandler(c, rec, &http.Request{}, nil)
			So(rec.Code, ShouldEqual, http.StatusOK)

			val, err := s.Get(c, m, time.Time{}, []interface{}{})
			So(err, ShouldBeNil)
			So(val, ShouldEqual, "bar")

			Convey("and are reset on flush", func() {
				flushNowWithMiddleware(c, state)

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
