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
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/tsmon"
	"github.com/luci/luci-go/common/tsmon/metric"
	"github.com/luci/luci-go/server/router"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

func flushNowWithMiddleware(c context.Context, state *State) {
	i := instance{
		ID:          instanceEntityID(c),
		TaskNum:     0,
		LastUpdated: clock.Now(c).Add(-2 * time.Minute).UTC(),
	}
	So(ds.Put(c, &i), ShouldBeNil)

	state.lastFlushed = clock.Now(c).Add(-2 * time.Minute)

	rec := httptest.NewRecorder()
	router.RunMiddleware(
		&router.Context{Context: c, Writer: rec, Request: &http.Request{}},
		router.NewMiddlewareChain(state.Middleware),
		nil,
	)
	So(rec.Code, ShouldEqual, http.StatusOK)
}

func TestGlobalCallbacks(t *testing.T) {
	Convey("Global callbacks", t, func() {
		c, _ := buildGAETestContext()
		state, mon := buildTestState()

		m := metric.NewCallbackStringIn(c, "foo", "", nil)

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
			housekeepingHandler(&router.Context{
				Context: c,
				Writer:  rec,
				Request: &http.Request{},
			})
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
