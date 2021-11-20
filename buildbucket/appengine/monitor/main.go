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
	"net/http"
	"net/url"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/monitor"
	"go.chromium.org/luci/common/tsmon/store"
	"go.chromium.org/luci/common/tsmon/target"

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

const prodXEndpoint = "https://prodxmon-pa.googleapis.com/v1:insert"

func newMonitor(ctx context.Context, acc string) (monitor.Monitor, error) {
	tr, err := auth.GetRPCTransport(
		ctx, auth.AsActor,
		auth.WithServiceAccount(acc),
		auth.WithScopes(monitor.ProdxmonScopes...),
	)
	if err != nil {
		return nil, nil
	}
	ep, err := url.Parse(prodXEndpoint)
	if err != nil {
		return nil, nil
	}
	return monitor.NewHTTPMonitor(ctx, &http.Client{Transport: tr}, ep)
}

func main() {
	mods := []module.Module{
		cron.NewModuleFromFlags(),
		gaeemulation.NewModuleFromFlags(),
		secrets.NewModuleFromFlags(),
	}

	server.Main(nil, mods, func(srv *server.Server) error {
		o := srv.Options

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
			if err := metrics.ReportBuilderMetrics(ctx, o.TsMonServiceName, o.TsMonJobName, o.Hostname); err != nil {
				return errors.Annotate(err, "computing builder metrics").Err()
			}
			if err := state.Flush(ctx, mon); err != nil {
				return errors.Annotate(err, "flushing builder metrics").Err()
			}
			return nil
		})

		return nil
	})
}
