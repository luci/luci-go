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

// Package main is the main entry point for the app.
package main

import (
	"context"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/store"
	"go.chromium.org/luci/common/tsmon/target"

	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/cron"
	"go.chromium.org/luci/server/gaeemulation"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/redisconn"
	"go.chromium.org/luci/server/secrets"
	tsmonsrv "go.chromium.org/luci/server/tsmon"

	"go.chromium.org/luci/buildbucket/appengine/internal/metrics"
)

func main() {
	mods := []module.Module{
		cron.NewModuleFromFlags(),
		gaeemulation.NewModuleFromFlags(),
		secrets.NewModuleFromFlags(),
		redisconn.NewModuleFromFlags(),
	}

	server.Main(nil, mods, func(srv *server.Server) error {
		o := srv.Options
		srv.Context = metrics.WithServiceInfo(srv.Context, o.TsMonServiceName, o.TsMonJobName, o.Hostname)

		// Init a new tsmon.State with the default task target,
		// configured in luci/server. V1 metrics need it.
		target := *tsmon.GetState(srv.Context).Store().DefaultTarget().(*target.Task)
		state := tsmon.NewState()
		state.SetStore(store.NewInMemory(&target))
		state.InhibitGlobalCallbacksOnFlush()
		ctx := tsmon.WithState(srv.Context, state)

		mon, err := tsmonsrv.NewProdXMonitor(ctx, 1024, o.TsMonAccount)
		if err != nil {
			srv.Fatal(errors.Annotate(err, "initiating tsmon").Err())
		}

		cron.RegisterHandler("report_builder_metrics", func(_ context.Context) error {
			// allow at most 10 mins to run.
			ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
			defer cancel()

			start := clock.Now(ctx)
			if err := metrics.ReportBuilderMetrics(ctx); err != nil {
				return errors.Annotate(err, "computing builder metrics").Err()
			}
			logging.Infof(ctx, "computing builder metrics took %s", clock.Since(ctx, start))
			start = clock.Now(ctx)
			if err := state.ParallelFlush(ctx, mon, 8); err != nil {
				return errors.Annotate(err, "flushing builder metrics").Err()
			}
			logging.Infof(ctx, "flushing builder metrics took %s", clock.Since(ctx, start))
			return nil
		})

		return nil
	})
}
