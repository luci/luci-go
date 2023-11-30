// Copyright 2023 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package bugupdater

import (
	"context"

	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/types"
)

var (
	// runsCounter is the metric that counts the number of bug filing
	// runs by LUCI Analysis, by project and outcome.
	runsCounter = metric.NewCounter("analysis/bug_updater/runs",
		"The number of auto-bug filing runs completed, "+
			"by LUCI Project and status.",
		&types.MetricMetadata{
			Units: "runs",
		},
		// The LUCI project.
		field.String("project"),
		// The run status.
		field.String("status"), // success | failure
	)

	// statusGauge reports the most recent status of the bug updater job.
	// Reports either "success" or "failure".
	statusGauge = metric.NewString("analysis/bug_updater/status",
		"Whether automatic bug updates are succeeding, by LUCI Project.",
		nil,
		// The LUCI project.
		field.String("project"),
	)

	durationGauge = metric.NewFloat("analysis/bug_updater/duration",
		"How long it is taking to update bugs, by LUCI Project.",
		&types.MetricMetadata{
			Units: types.Seconds,
		},
		// The LUCI project.
		field.String("project"),
	)
)

func init() {
	// Register metrics as global metrics, which has the effort of
	// resetting them after every flush.
	tsmon.RegisterGlobalCallback(func(ctx context.Context) {
		// Do nothing -- the metrics will be populated by the cron
		// job itself and does not need to be triggered externally.
	}, statusGauge, durationGauge)
}
