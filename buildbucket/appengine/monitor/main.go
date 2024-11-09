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
	"sync"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/monitor"
	"go.chromium.org/luci/common/tsmon/store"
	"go.chromium.org/luci/common/tsmon/target"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/cron"
	"go.chromium.org/luci/server/gaeemulation"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/redisconn"
	"go.chromium.org/luci/server/secrets"
	tsmonsrv "go.chromium.org/luci/server/tsmon"

	"go.chromium.org/luci/buildbucket/appengine/internal/config"
	"go.chromium.org/luci/buildbucket/appengine/internal/internalcontext"
	"go.chromium.org/luci/buildbucket/appengine/internal/metrics"
	pb "go.chromium.org/luci/buildbucket/proto"
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

		// Register custom metrics
		// Directly return error when fail to create custom metrics, since the
		// sole purpose of this service is monitoring. Failing of setting up
		// custom metrics would fail the cron jobs.
		ctx, err := internalcontext.WithCustomMetrics(ctx)
		if err != nil {
			return errors.Annotate(err, "failed to create custom metrics").Err()
		}

		var mon monitor.Monitor
		switch {
		case o.Prod && o.TsMonAccount != "":
			var err error
			mon, err = tsmonsrv.NewProdXMonitor(srv.Context, 1024, o.TsMonAccount)
			if err != nil {
				srv.Fatal(errors.Annotate(err, "initiating tsmon").Err())
			}
		case !o.Prod:
			mon = monitor.NewDebugMonitor("")
		default:
			mon = monitor.NewNilMonitor()
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

			var wg sync.WaitGroup
			wg.Add(2)

			var err1, err2 error
			go func() {
				defer wg.Done()
				err1 = state.ParallelFlush(ctx, mon, 8)
			}()

			// Flush custom metrics
			go func() {
				defer wg.Done()
				var globalCfg *pb.SettingsCfg
				globalCfg, err2 = config.GetSettingsCfg(ctx)
				if err2 != nil {
					return
				}

				cms := metrics.GetCustomMetrics(ctx)
				err2 = cms.Flush(ctx, globalCfg, mon)
			}()

			wg.Wait()

			switch {
			case err1 != nil:
				return err1
			case err2 != nil:
				return err2
			default:
				logging.Infof(ctx, "flushing builder metrics took %s", clock.Since(ctx, start))
				return nil
			}
		})

		return nil
	})
}
