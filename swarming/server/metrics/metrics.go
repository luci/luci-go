// Copyright 2024 The LUCI Authors.
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

// Package metrics defines metrics used in Swarming.
package metrics

import (
	"math"
	"time"

	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/types"
)

const (
	buckets     = 100
	scaleFactor = 100.0
)

var (
	// Custom bucketer in the range of 100ms...100000s. Used for task scheduling
	// latency measurements.
	// Technically the maximum allowed expiration for a pending task is 7 days
	// (604800s) to accommodate some ChromeOS tasks, but it's rarely reached.
	// As of Aug 23, 2024, the 99 percentile pending time for chromeos-swarming
	// is 62447.690s, which is larger than chromium-swarm or chrome-swarming but
	// still less than 100000s.
	// Set the bucketer with scale of 100 because the lower bound is 100ms.
	// Without it the lower bound would be 1ms and we'll waste the first 25 buckets.
	schedulingLatencyBucketer = distribution.GeometricBucketerWithScale(
		math.Pow(float64((100000*time.Second).Milliseconds()/scaleFactor), 1.0/buckets),
		buckets,
		scaleFactor,
	)
)

var (
	TaskStatusChangePubsubLatency = metric.NewCumulativeDistribution(
		"swarming/tasks/state_change_pubsub_notify_latencies",
		"Latency (in ms) of PubSub notification when backend receives task_update",
		&types.MetricMetadata{Units: types.Milliseconds},
		// Custom bucketer in the range of 100ms...100s. Used for
		// pubsub latency measurements.
		// Roughly speaking measurements range between 150ms and 300ms. However timeout
		// for pubsub notification is 10s.
		distribution.GeometricBucketerWithScale(math.Pow(float64((100*time.Second).Milliseconds()/scaleFactor), 1.0/buckets), buckets, scaleFactor),
		field.String("pool"),          // e.g. "skia".
		field.String("status"),        // e.g. "User canceled"
		field.Int("http_status_code")) // e.g. 404

	TaskStatusChangeSchedulerLatency = metric.NewCumulativeDistribution(
		"swarming/tasks/state_change_scheduling_latencies",
		"Latency (in ms) of task scheduling request",
		&types.MetricMetadata{Units: types.Milliseconds},
		schedulingLatencyBucketer,
		field.String("pool"),        // e.g. "skia".
		field.String("spec_name"),   // e.g. "linux_chromium_tsan_rel_ng"
		field.String("status"),      // e.g. "No resource available"
		field.String("device_type")) // e.g. "walleye"

	TaskToRunConsumeLatency = metric.NewCumulativeDistribution(
		"swarming/tasks/ttr_consume_latencies",
		"Latency (in ms) between TaskToRun is created and consumed",
		&types.MetricMetadata{Units: types.Milliseconds},
		schedulingLatencyBucketer,
		field.String("spec_name"),     // name of a job specification.
		field.String("project_id"),    // e.g. "chromium".
		field.String("subproject_id"), // e.g. "blink". Set to empty string if not used.
		field.String("pool"),          // e.g. "Chrome".
		field.String("rbe"),           // RBE instance of the task or literal "none".
	)

	JobsActives = metric.NewInt(
		"jobs/active",
		"Number of running, pending or otherwise active jobs.",
		nil,
		field.String("spec_name"),     // name of a job specification.
		field.String("project_id"),    // e.g. "chromium".
		field.String("subproject_id"), // e.g. "blink". Set to empty string if not used.
		field.String("pool"),          // e.g. "Chrome".
		field.String("rbe"),           // RBE instance of the task or literal "none".
		field.String("status"),        // "pending", or "running".
	)

	JobsRequested = metric.NewCounter(
		"jobs/requested",
		"Number of requested jobs over time.",
		nil,
		field.String("spec_name"),     // name of a job specification.
		field.String("project_id"),    // e.g. "chromium".
		field.String("subproject_id"), // e.g. "blink". Set to empty string if not used.
		field.String("pool"),          // e.g. "Chrome".
		field.String("rbe"),           // RBE instance of the task or literal "none".
		field.Bool("deduped"),         // whether the job was deduped or not.
	)

	JobsCompleted = metric.NewCounter(
		"jobs/completed",
		"Number of completed jobs.",
		nil,
		field.String("spec_name"),     // name of a job specification.
		field.String("project_id"),    // e.g. "chromium".
		field.String("subproject_id"), // e.g. "blink". Set to empty string if not used.
		field.String("pool"),          // e.g. "Chrome".
		field.String("rbe"),           // RBE instance of the task or literal "none".
		field.String("result"),        // "success", "failure" or "infra_failure"
		field.String("status"),        // one of the end states, see model.TaskStateString().
	)

	JobsDuration = metric.NewCumulativeDistribution(
		"jobs/durations",
		"Cycle times of completed jobs, in seconds.",
		&types.MetricMetadata{Units: types.Milliseconds},
		schedulingLatencyBucketer,
		field.String("spec_name"),     // name of a job specification.
		field.String("project_id"),    // e.g. "chromium".
		field.String("subproject_id"), // e.g. "blink". Set to empty string if not used.
		field.String("pool"),          // e.g. "Chrome".
		field.String("rbe"),           // RBE instance of the task or literal "none".
		field.String("result"),        // "success", "failure" or "infra_failure"

	)

	BotsPerState = metric.NewInt(
		"swarming/rbe_migration/bots",
		"Number of Swarming bots per RBE migration state.",
		nil,
		field.String("pool"),  // e.g "luci.infra.ci"
		field.String("state"), // e.g. "RBE", "SWARMING", "HYBRID"
	)

	BotsStatus = metric.NewString(
		"executors/status",
		"Status of a job executor.",
		nil,
	)

	BotsDimensionsPool = metric.NewString(
		"executors/pool",
		"Pool name for a given job executor.",
		nil,
	)

	BotsRBEInstance = metric.NewString(
		"executors/rbe",
		"RBE instance of a job executor.",
		nil,
	)

	BotsVersion = metric.NewString(
		"executors/version",
		"Version of a job executor.",
		nil,
	)

	BotAuthSuccesses = metric.NewCounter(
		"swarming/bot_auth/success",
		"Number of successful bot authentication events",
		nil,
		field.String("auth_method"), // e.g. "luci_token", "service_account", ...
		field.String("condition"),   // depends on auth_method, e.g. service account email
	)
)
