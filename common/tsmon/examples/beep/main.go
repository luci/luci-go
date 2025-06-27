// Copyright 2019 The LUCI Authors.
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

// package dummy_project implements a demo application that populates monitoring
// data for a dummy_project.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/examples/beep/dummy_project"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/target"
)

var presence = metric.NewBoolWithOptions(
	"test/tsmon/examples/beep",
	&metric.Options{
		TargetType: (*dummy_project.DummyProject)(nil).Type(),
	},
	"A always-true heart-beating metric.",
	nil,
	field.Int("num"),
)

// initialize initializes flags and tsmon, and returns the context.
func initialize() context.Context {
	fs := flag.NewFlagSet("", flag.ExitOnError)
	tsmonFlags := tsmon.NewFlags()
	tsmonFlags.Flush = "manual"

	// The generated proto message will be displayed in stderr.
	tsmonFlags.Endpoint = "file://"
	tsmonFlags.Register(fs)

	loggingConfig := logging.Config{Level: logging.Info}
	loggingConfig.AddFlags(fs)
	fs.Parse(os.Args[1:])

	c := context.Background()
	c = gologger.StdConfig.Use(c)
	c = loggingConfig.Set(c)

	if err := tsmon.InitializeFromFlags(c, &tsmonFlags); err != nil {
		panic(fmt.Sprintf("failed to initialize tsmon: %s", err))
	}
	return c
}

func main() {
	c := initialize()
	for i := range 4 {
		// Create a context with a dummy project target.
		tc := target.Set(c, &dummy_project.DummyProject{
			Project:   fmt.Sprintf("MyProject-%d", i),
			Location:  "MyComputer",
			IsStaging: false,
		})
		presence.Set(tc, true, i)
	}

	// The output should contain 4 MetricCollections for the 4 project targets.
	tsmon.Flush(c)
}
