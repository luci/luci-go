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
	"math/rand/v2"
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
			return errors.Fmt("failed to create custom metrics: %w", err)
		}

		var mon monitor.Monitor
		switch {
		case o.Prod && o.TsMonAccount != "":
			var err error
			mon, err = tsmonsrv.NewProdXMonitor(srv.Context, 1024, o.TsMonAccount)
			if err != nil {
				srv.Fatal(errors.Fmt("initiating tsmon: %w", err))
			}
		case !o.Prod:
			mon = monitor.NewDebugMonitor("")
		default:
			mon = monitor.NewNilMonitor()
		}

		cron.RegisterHandler("report_builder_metrics", func(_ context.Context) error {
			// Note we specifically use `ctx` from the outer scope, not the one passed
			// to the handler. That way we pick up custom tsmon state associated with
			// it, while still allowing server.Server report its own built-in metrics
			// normally (since we didn't touch tsmon state in srv.Context). As
			// a downside, logs from this handler are attributed to the server as
			// a whole, not individual cron request invocation. As a workaround, we
			// attach a custom logging field that would allow to manually correlate
			// logs belonging to the same cron invocation. Additionally we need to
			// manually maintain the request timeout, since the server sets it only in
			// the per-request context which we ignore.
			ctx := logging.SetField(ctx, "rid", rand.Int32())

			// An error from ReportBuilderMetrics + two tsmon flushes.
			merr := make(errors.MultiError, 3)

			// Have two separate timeouts for querying and flushing.
			start := clock.Now(ctx)
			queryCtx, queryCtxCancel := context.WithTimeout(ctx, 20*time.Minute)
			defer queryCtxCancel()

			// Collect all metrics. This involves a ton of datastore queries. Some of
			// them can fail. We'll still report as many metrics as we could fetch.
			logging.Infof(queryCtx, "Computing building metrics")
			merr[0] = metrics.ReportBuilderMetrics(queryCtx)
			if merr[0] == nil {
				logging.Infof(queryCtx, "Builder metrics computed successfully in %s", clock.Since(queryCtx, start))
			} else {
				logging.Warningf(queryCtx, "Some builder metrics could not be computed, see logs, took %s", clock.Since(queryCtx, start))
			}

			// Have two separate timeouts for querying and flushing.
			start = clock.Now(ctx)
			flushCtx, flushCtxCancel := context.WithTimeout(ctx, 5*time.Minute)
			defer flushCtxCancel()

			var wg sync.WaitGroup
			wg.Add(2)

			// Flush built-in metrics.
			go func() {
				defer wg.Done()
				merr[1] = state.ParallelFlush(flushCtx, mon, 8)
			}()

			// Flush custom metrics
			go func() {
				defer wg.Done()
				globalCfg, err := config.GetSettingsCfg(flushCtx)
				if err != nil {
					merr[2] = err
					return
				}
				cms := metrics.GetCustomMetrics(flushCtx)
				merr[2] = cms.Flush(flushCtx, globalCfg, mon)
			}()

			wg.Wait()

			logging.Infof(flushCtx, "Flushing took %s", clock.Since(flushCtx, start))
			if merr[1] != nil {
				logging.Errorf(flushCtx, "Built-in metrics flush failed: %s", merr[1])
			}
			if merr[2] != nil {
				logging.Errorf(flushCtx, "Custom metrics flush failed: %s", merr[2])
			}
			switch count, first := merr.Summary(); {
			case count == 0:
				return nil
			case count == 1:
				return first
			default:
				return merr
			}
		})

		return nil
	})
}
