// Copyright 2020 The LUCI Authors.
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

// Executable tq-sweeper-spanner runs an inproc sweeping driver that scans
// Spanner database for TQ transactional tasks reminders.
//
// It should run as a single replica per Spanner DB. It is OK if it temporarily
// goes offline or runs on multiple replicas: this will just delay the task
// reminders sweeping.
package main

import (
	"context"
	"flag"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"

	// Sweep Spanner DB.
	_ "go.chromium.org/luci/server/tq/txn/spanner"
)

func main() {
	sweepInterval := flag.Duration("tq-sweep-interval", time.Minute, "How often to run full sweeps")

	opts := &tq.ModuleOptions{
		SweepMode:               "inproc",
		ServingPrefix:           "-", // not serving any tasks
		SweepInitiationEndpoint: "-", // sweeps are launched by the timer only
	}
	opts.Register(flag.CommandLine)

	modules := []module.Module{
		span.NewModuleFromFlags(nil),
		tq.NewModule(opts),
	}

	server.Main(nil, modules, func(srv *server.Server) error {
		if *sweepInterval < 0 {
			*sweepInterval = 0
		}

		srv.RunInBackground("", func(ctx context.Context) {
			count := 0

			for ctx.Err() == nil {
				count++
				start := clock.Now(ctx)

				logging.Infof(ctx, "--------------- Starting the sweep #%d ------------------", count)
				tq.Default.Sweep(ctx)
				logging.Infof(ctx, "--------------- Sweep #%d done in %s ---------------", count, clock.Now(ctx).Sub(start))

				if sleep := start.Add(*sweepInterval).Sub(clock.Now(ctx)); sleep > 0 && ctx.Err() == nil {
					logging.Infof(ctx, "Waiting %s until the next sweep", sleep)
					clock.Sleep(ctx, sleep)
				}
			}
		})

		return nil
	})
}
