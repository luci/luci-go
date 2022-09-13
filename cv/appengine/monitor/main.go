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

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/monitor"
	"go.chromium.org/luci/common/tsmon/store"
	"go.chromium.org/luci/common/tsmon/target"
	"go.chromium.org/luci/grpc/grpcmon"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/cron"
	"go.chromium.org/luci/server/gaeemulation"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/redisconn"
	"go.chromium.org/luci/server/secrets"

	"go.chromium.org/luci/cv/internal/aggrmetrics"
	"go.chromium.org/luci/cv/internal/common"
)

const (
	// prodXGRPCTarget is the dial target for prodx grpc service.
	prodXGRPCTarget = "prodxmon-pa.googleapis.com:443"

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
		ctx := tsmon.WithState(srv.Context, state)
		mon, err := newMonitor(ctx, opts.TsMonAccount)
		if err != nil {
			return errors.Annotate(err, "failed to initiate monitoring client").Err()
		}
		aggregator := aggrmetrics.New(env)

		cron.RegisterHandler("report-aggregated-metrics", func(ctx context.Context) error {
			ctx, cancel := context.WithTimeout(ctx, aggregateMetricsCronTimeout)
			defer cancel()

			start := clock.Now(ctx)
			if err := aggregator.Cron(ctx); err != nil {
				return errors.Annotate(err, "failed to compute aggregation metrics").Err()
			}
			logging.Infof(ctx, "computing aggregation metrics took %s", clock.Since(ctx, start))
			start = clock.Now(ctx)
			if err := state.ParallelFlush(ctx, mon, 8); err != nil {
				return errors.Annotate(err, "failed to flush aggregation metrics").Err()
			}
			logging.Infof(ctx, "flushing aggregation metrics took %s", clock.Since(ctx, start))
			return nil
		})

		return nil
	})
}

func newMonitor(ctx context.Context, account string) (monitor.Monitor, error) {
	cred, err := auth.GetPerRPCCredentials(
		ctx, auth.AsActor,
		auth.WithServiceAccount(account),
		auth.WithScopes(monitor.ProdxmonScopes...),
	)
	if err != nil {
		return nil, errors.Annotate(err, "failed to get per RPC credentials").Err()
	}
	conn, err := grpc.Dial(
		prodXGRPCTarget,
		grpc.WithTransportCredentials(credentials.NewTLS(nil)),
		grpc.WithPerRPCCredentials(cred),
		grpcmon.WithClientRPCStatsMonitor(),
	)
	if err != nil {
		return nil, errors.Annotate(err, "failed to dial ProdX service(%s)", prodXGRPCTarget).Err()
	}
	return monitor.NewGRPCMonitorWithChunkSize(ctx, 1024, conn), nil
}
