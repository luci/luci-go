// Copyright 2017 The LUCI Authors.
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

// goat_teleporter is minimal executable that configures tsmon and sense some
// metrics to it.
//
// It records one metric point and immediately flushes it. To see what exactly
// the program will send to tsmon backend, run it like so:
//
// ./goat_teleporter -ts-mon-endpoint file://
//
// To attempt to send the metrics for real:
//
// ./goat_teleporter -ts-mon-endpoint https://prodxmon-pa.googleapis.com/v1:insert
//
// Though it will most likely fail due to absence of necessary credentials.
package main

import (
	"flag"
	"os"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/metric"
)

var (
	goatsTeleported = metric.NewCounter(
		"goats/teleported",
		"Total number of teleported goats.",
		nil)
)

func run() int {
	ctx := gologger.StdConfig.Use(context.Background())
	ctx = logging.SetLevel(ctx, logging.Debug)

	tsmonFlags := tsmon.NewFlags()
	tsmonFlags.Target.TargetType = "task"
	tsmonFlags.Target.TaskServiceName = "goat_teleporter"
	tsmonFlags.Target.TaskJobName = "default"
	tsmonFlags.Flush = "manual"
	tsmonFlags.Register(flag.CommandLine)

	flag.Parse()

	// Note: this error may be ignored, in which case tsmon will just stay
	// disabled.
	if err := tsmon.InitializeFromFlags(ctx, &tsmonFlags); err != nil {
		logging.Errorf(ctx, "Failed to initialize tsmon - %s", err)
		return 1
	}

	// Bump the metric value.
	goatsTeleported.Add(ctx, 1)

	// We flush manually because -ts-mon-flush is set to 'manual'. For long
	// running processes it makes more sense to use default 'auto' mode.
	if err := tsmon.Flush(ctx); err != nil {
		logging.Errorf(ctx, "Failed to flush metrics - %s", err)
		return 2
	}

	return 0
}

func main() {
	mathrand.SeedRandomly()
	os.Exit(run())
}
