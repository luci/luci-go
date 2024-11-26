// Copyright 2023 The LUCI Authors.
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

package main

import (
	"context"

	"go.chromium.org/luci/common/tsmon/monitor"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/cron"
	"go.chromium.org/luci/server/gaeemulation"
	"go.chromium.org/luci/server/module"
	tsmonsrv "go.chromium.org/luci/server/tsmon"

	"go.chromium.org/luci/swarming/server/scan"
)

func main() {
	modules := []module.Module{
		cron.NewModuleFromFlags(),
		gaeemulation.NewModuleFromFlags(),
	}

	server.Main(nil, modules, func(srv *server.Server) error {
		var mon monitor.Monitor
		switch {
		case srv.Options.Prod && srv.Options.TsMonAccount != "":
			var err error
			mon, err = tsmonsrv.NewProdXMonitor(srv.Context, 4096, srv.Options.TsMonAccount)
			if err != nil {
				return err
			}
		case !srv.Options.Prod:
			mon = monitor.NewDebugMonitor("")
		default:
			mon = monitor.NewNilMonitor()
		}

		cron.RegisterHandler("report-bots", func(ctx context.Context) error {
			return scan.Bots(ctx, []scan.BotVisitor{
				&scan.BotsMetricsReporter{
					ServiceName: srv.Options.TsMonServiceName,
					JobName:     srv.Options.TsMonJobName,
					Monitor:     mon,
				},
				&scan.BotsDimensionsAggregator{},
				&scan.NamedCachesAggregator{},
			})
		})

		cron.RegisterHandler("report-tasks", func(ctx context.Context) error {
			return scan.ActiveTasks(ctx, []scan.TaskVisitor{
				&scan.ActiveJobsReporter{
					ServiceName: srv.Options.TsMonServiceName,
					JobName:     srv.Options.TsMonJobName,
					Monitor:     mon,
				},
			})
		})

		return nil
	})
}
