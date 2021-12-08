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
	"go.chromium.org/luci/server/secrets"

	// TODO(crbug/1242998): Remove once safe get becomes datastore default.
	_ "go.chromium.org/luci/gae/service/datastore/crbug1242998safeget"

	"go.chromium.org/luci/buildbucket/appengine/internal/metrics"
)

// prodXGRPCTarget is the dial target for prodx grpc service.
const prodXGRPCTarget = "prodxmon-pa.googleapis.com:443"

func newMonitor(ctx context.Context, acc string) (monitor.Monitor, error) {
	cred, err := auth.GetPerRPCCredentials(
		ctx, auth.AsActor,
		auth.WithServiceAccount(acc),
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
	return monitor.NewGRPCMonitor(ctx, conn), nil
}

func main() {
	mods := []module.Module{
		cron.NewModuleFromFlags(),
		gaeemulation.NewModuleFromFlags(),
		secrets.NewModuleFromFlags(),
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

		mon, err := newMonitor(ctx, o.TsMonAccount)
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
			if err := state.Flush(ctx, mon); err != nil {
				return errors.Annotate(err, "flushing builder metrics").Err()
			}
			logging.Infof(ctx, "flushing builder metrics took %s", clock.Since(ctx, start))
			return nil
		})

		return nil
	})
}
