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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/store"
	"go.chromium.org/luci/common/tsmon/target"
	"go.chromium.org/luci/common/tsmon/types"

	"go.chromium.org/luci/server/router"
)

func TestDispatcher(t *testing.T) {
	t.Parallel()

	ftt.Run("With dispatcher", t, func(t *ftt.Test) {
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
				assert.Loosely(t, val, should.HaveType[*distribution.Distribution])
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

		t.Run("Handler IDs", func(t *ftt.Test) {
			d.RegisterHandler("h1", func(ctx context.Context) error { return nil })
			d.RegisterHandler("h2", func(ctx context.Context) error { return nil })
			assert.Loosely(t, d.handlerIDs(), should.Resemble([]string{"h1", "h2"}))
		})

		t.Run("Works", func(t *ftt.Test) {
			called := false
			d.RegisterHandler("ok", func(ctx context.Context) error {
				called = true
				return nil
			})
			assert.Loosely(t, call("/crons/ok"), should.Equal(200))
			assert.Loosely(t, called, should.BeTrue)
			assert.Loosely(t, metric(callsCounter, "ok", "OK"), should.Equal(1))
			assert.Loosely(t, metricDist(callsDurationMS, "ok", "OK"), should.Equal(1))
		})

		t.Run("Fatal error", func(t *ftt.Test) {
			d.RegisterHandler("boom", func(ctx context.Context) error {
				return errors.New("boom")
			})
			assert.Loosely(t, call("/crons/boom"), should.Equal(202))
			assert.Loosely(t, metric(callsCounter, "boom", "fatal"), should.Equal(1))
			assert.Loosely(t, metricDist(callsDurationMS, "boom", "fatal"), should.Equal(1))
		})

		t.Run("Transient error", func(t *ftt.Test) {
			d.RegisterHandler("smaller-boom", func(ctx context.Context) error {
				return transient.Tag.Apply(errors.New("smaller boom"))
			})
			assert.Loosely(t, call("/crons/smaller-boom"), should.Equal(500))
			assert.Loosely(t, metric(callsCounter, "smaller-boom", "transient"), should.Equal(1))
			assert.Loosely(t, metricDist(callsDurationMS, "smaller-boom", "transient"), should.Equal(1))
		})

		t.Run("Unknown handler", func(t *ftt.Test) {
			assert.Loosely(t, call("/crons/unknown"), should.Equal(202))
			assert.Loosely(t, metric(callsCounter, "unknown", "no_handler"), should.Equal(1))
		})

		t.Run("Panic", func(t *ftt.Test) {
			d.RegisterHandler("panic", func(ctx context.Context) error {
				panic("boom")
			})
			assert.Loosely(t, func() { call("/crons/panic") }, should.Panic)
			assert.Loosely(t, metric(callsCounter, "panic", "panic"), should.Equal(1))
			assert.Loosely(t, metricDist(callsDurationMS, "panic", "panic"), should.Equal(1))
		})
	})
}
