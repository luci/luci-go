// Copyright 2022 The LUCI Authors.
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

	"go.chromium.org/luci/cv/internal/aggrmetrics"
	"go.chromium.org/luci/cv/internal/common"
)

const (
	// aggregateMetricsCronTimeout is the amount off time the Cron has to compute
	// and flush the aggregation metrics.
	aggregateMetricsCronTimeout = 2 * time.Minute
)

func main() {

	modules := []module.Module{
		cron.NewModuleFromFlags(),
		gaeemulation.NewModuleFromFlags(),
		redisconn.NewModuleFromFlags(),
		secrets.NewModuleFromFlags(),
	}

	server.Main(nil, modules, func(srv *server.Server) error {
		opts := srv.Options
		env := common.MakeEnv(opts)

		// Init a new tsmon.State with the default task target,
		// configured in luci/server.
		target := *tsmon.GetState(srv.Context).Store().DefaultTarget().(*target.Task)
		state := tsmon.NewState()
		state.SetStore(store.NewInMemory(&target))
		state.InhibitGlobalCallbacksOnFlush()

		mon, err := tsmonsrv.NewProdXMonitor(srv.Context, 1024, opts.TsMonAccount)
		if err != nil {
			return errors.Fmt("failed to initiate monitoring client: %w", err)
		}

		cron.RegisterHandler("report-aggregated-metrics", func(ctx context.Context) error {
			ctx, cancel := context.WithTimeout(ctx, aggregateMetricsCronTimeout)
			defer cancel()

			// Override the state to avoid using the default state from the server.
			ctx = tsmon.WithState(ctx, state)
			aggregator := aggrmetrics.New(env)
			start := clock.Now(ctx)
			if err := aggregator.Cron(ctx); err != nil {
				return errors.Fmt("failed to compute aggregation metrics: %w", err)
			}
			logging.Infof(ctx, "computing aggregation metrics took %s", clock.Since(ctx, start))
			start = clock.Now(ctx)
			if err := state.ParallelFlush(ctx, mon, 8); err != nil {
				return errors.Fmt("failed to flush aggregation metrics: %w", err)
			}
			logging.Infof(ctx, "flushing aggregation metrics took %s", clock.Since(ctx, start))
			return nil
		})

		return nil
	})
}
