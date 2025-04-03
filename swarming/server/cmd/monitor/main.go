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
	"flag"
	"time"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/tsmon/monitor"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/cron"
	"go.chromium.org/luci/server/gaeemulation"
	"go.chromium.org/luci/server/gaememcache"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/tq"
	tsmonsrv "go.chromium.org/luci/server/tsmon"

	"go.chromium.org/luci/swarming/server/cfg"
	"go.chromium.org/luci/swarming/server/scan"
	"go.chromium.org/luci/swarming/server/tasks"
)

func main() {
	modules := []module.Module{
		cron.NewModuleFromFlags(),
		gaeemulation.NewModuleFromFlags(),
		gaememcache.NewModuleFromFlags(),
		tq.NewModuleFromFlags(),
	}

	allowAbandoningTasks := flag.Bool(
		"allow-abandoning-tasks",
		false,
		"If set, enable new path for abandoning tasks in reaction to BotInfo events.",
	)

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

		cfg, err := cfg.NewProvider(srv.Context)
		if err != nil {
			return err
		}

		taskLifeCycle := &tasks.LifecycleTasksViaTQ{
			Dispatcher:           &tq.Default,
			AllowAbandoningTasks: *allowAbandoningTasks,
		}
		taskLifeCycle.RegisterTQTasks()

		cron.RegisterHandler("report-bots", func(ctx context.Context) error {
			conf, err := cfg.Latest(ctx)
			if err != nil {
				return errors.Annotate(err, "failed to fetch the service config").Err()
			}
			return scan.Bots(ctx, []scan.BotVisitor{
				&scan.BotsMetricsReporter{
					ServiceName: srv.Options.TsMonServiceName,
					JobName:     srv.Options.TsMonJobName,
					Monitor:     mon,
				},
				&scan.DeadBotDetector{
					BotDeathTimeout: time.Duration(conf.Settings().BotDeathTimeoutSecs) * time.Second,
					LifecycleTasks:  taskLifeCycle,
					ServerVersion:   srv.Options.ImageVersion(),
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
