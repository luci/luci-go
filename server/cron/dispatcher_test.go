// Copyright 2021 The LUCI Authors.
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

package cron

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/store"
	"go.chromium.org/luci/common/tsmon/target"
	"go.chromium.org/luci/common/tsmon/types"

	"go.chromium.org/luci/server/router"

	. "github.com/smartystreets/goconvey/convey"
)

func TestDispatcher(t *testing.T) {
	t.Parallel()

	Convey("With dispatcher", t, func() {
		ctx := context.Background()
		ctx = gologger.StdConfig.Use(ctx)
		ctx, _, _ = tsmon.WithFakes(ctx)
		tsmon.GetState(ctx).SetStore(store.NewInMemory(&target.Task{}))

		metric := func(m types.Metric, fieldVals ...any) any {
			return tsmon.GetState(ctx).Store().Get(ctx, m, time.Time{}, fieldVals)
		}

		metricDist := func(m types.Metric, fieldVals ...any) (count int64) {
			val := metric(m, fieldVals...)
			if val != nil {
				So(val, ShouldHaveSameTypeAs, &distribution.Distribution{})
				count = val.(*distribution.Distribution).Count()
			}
			return
		}

		d := &Dispatcher{DisableAuth: true}

		srv := router.New()
		d.InstallCronRoutes(srv, "/crons")

		call := func(path string) int {
			req := httptest.NewRequest("GET", path, nil).WithContext(ctx)
			rec := httptest.NewRecorder()
			srv.ServeHTTP(rec, req)
			return rec.Result().StatusCode
		}

		Convey("Handler IDs", func() {
			d.RegisterHandler("h1", func(ctx context.Context) error { return nil })
			d.RegisterHandler("h2", func(ctx context.Context) error { return nil })
			So(d.handlerIDs(), ShouldResemble, []string{"h1", "h2"})
		})

		Convey("Works", func() {
			called := false
			d.RegisterHandler("ok", func(ctx context.Context) error {
				called = true
				return nil
			})
			So(call("/crons/ok"), ShouldEqual, 200)
			So(called, ShouldBeTrue)
			So(metric(callsCounter, "ok", "OK"), ShouldEqual, 1)
			So(metricDist(callsDurationMS, "ok", "OK"), ShouldEqual, 1)
		})

		Convey("Fatal error", func() {
			d.RegisterHandler("boom", func(ctx context.Context) error {
				return errors.New("boom")
			})
			So(call("/crons/boom"), ShouldEqual, 202)
			So(metric(callsCounter, "boom", "fatal"), ShouldEqual, 1)
			So(metricDist(callsDurationMS, "boom", "fatal"), ShouldEqual, 1)
		})

		Convey("Transient error", func() {
			d.RegisterHandler("smaller-boom", func(ctx context.Context) error {
				return transient.Tag.Apply(errors.New("smaller boom"))
			})
			So(call("/crons/smaller-boom"), ShouldEqual, 500)
			So(metric(callsCounter, "smaller-boom", "transient"), ShouldEqual, 1)
			So(metricDist(callsDurationMS, "smaller-boom", "transient"), ShouldEqual, 1)
		})

		Convey("Unknown handler", func() {
			So(call("/crons/unknown"), ShouldEqual, 202)
			So(metric(callsCounter, "unknown", "no_handler"), ShouldEqual, 1)
		})

		Convey("Panic", func() {
			d.RegisterHandler("panic", func(ctx context.Context) error {
				panic("boom")
			})
			So(func() { call("/crons/panic") }, ShouldPanic)
			So(metric(callsCounter, "panic", "panic"), ShouldEqual, 1)
			So(metricDist(callsDurationMS, "panic", "panic"), ShouldEqual, 1)
		})
	})
}
