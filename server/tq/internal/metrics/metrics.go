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

// Package metrics contains definition of metrics exposed by server/tq.
package metrics

import (
	"math"

	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/types"
)

// MaxRetryFieldValue is the number to cap the value of retry count field at,
// for metrics that include it, thus indicating a value greater or equal to it.
// This makes the metric field have a reasonable number of distinct values.
const MaxRetryFieldValue = 10

var (
	// bucketer1msTo5min covers range of 1..300k.
	bucketer1msTo5min = distribution.GeometricBucketer(math.Pow(10, 0.055), 100)

	// TQ guts metrics, primary useful to debug TQ.

	InprocSweepDurationMS = metric.NewCumulativeDistribution(
		"tq/sweep/inproc/duration",
		"Duration of a full inproc sweep cycle across all DBs (ms)",
		&types.MetricMetadata{Units: types.Milliseconds},
		bucketer1msTo5min,
	)

	SweepFetchMetaDurationsMS = metric.NewCumulativeDistribution(
		"tq/sweep/fetch/meta/durations",
		"Duration of FetchRemindersMeta operation (ms)",
		&types.MetricMetadata{Units: types.Milliseconds},
		bucketer1msTo5min,
		field.String("status"), // OK | limit | timeout | failures
		field.Int("level"),     // 0 means the primary shard task, 1+ are its children
		field.String("db"),
	)

	SweepFetchMetaReminders = metric.NewCounter(
		"tq/sweep/fetch/meta/reminders",
		"Count of Reminders fetched by FetchRemindersMeta",
		nil,
		field.String("status"), // OK | limit | timeout | failures
		field.Int("level"),     // 0 means the primary shard task, 1+ are its children
		field.String("db"),
	)

	ReminderStalenessMS = metric.NewCumulativeDistribution(
		"tq/reminders/staleness",
		("Distribution of staleness of scanned Reminders during the sweep. " +
			"May be incomplete if keyspace wasn't scanned completely"),
		&types.MetricMetadata{Units: types.Milliseconds},
		distribution.DefaultBucketer,
		field.Int("level"),
		field.String("db"),
	)

	RemindersCreated = metric.NewCounter(
		"tq/reminders/created",
		"Count of reminders created and if they are still fresh in the post-txn defer",
		nil,
		field.String("task_class"), // matches TaskClass.ID
		field.String("staleness"),  // fresh | stale
		field.String("db"),
	)

	RemindersDeleted = metric.NewCounter(
		"tq/reminders/processed",
		"Count of reminders processed (i.e. deleted)",
		nil,
		field.String("task_class"), // matches TaskClass.ID
		field.String("txn_path"),   // happy | sweep
		field.String("db"),
	)

	RemindersLatencyMS = metric.NewCumulativeDistribution(
		"tq/reminders/latency",
		"Time between AddTask call and the deletion of the reminder",
		&types.MetricMetadata{Units: types.Milliseconds},
		bucketer1msTo5min,
		field.String("task_class"), // matches TaskClass.ID
		field.String("txn_path"),   // happy | sweep
		field.String("db"),
	)

	// TQ metrics that might be useful for TQ clients as well.

	SubmitCount = metric.NewCounter(
		"tq/submit/count",
		"Count of submitted tasks",
		nil,
		field.String("task_class"), // matches TaskClass.ID
		field.String("queue"),      // the task queue submitted into (short form)
		field.String("txn_path"),   // none | happy | sweep
		field.String("grpc_code"),  // gRPC canonical code
	)

	SubmitDurationMS = metric.NewCumulativeDistribution(
		"tq/submit/duration",
		"Duration of submit calls",
		&types.MetricMetadata{Units: types.Milliseconds},
		distribution.DefaultBucketer,
		field.String("task_class"), // matches TaskClass.ID
		field.String("queue"),      // the task queue submitted into (short form)
		field.String("txn_path"),   // none | happy | sweep
		field.String("grpc_code"),  // gRPC canonical code
	)

	ServerRejectedCount = metric.NewCounter(
		"tq/server/rejected",
		"Count of rejected (e.g. malformed) task pushes",
		nil,
		field.String("queue"),  // the task queue that delivered the task or "" if unknown
		field.String("reason"), // auth | bad_request | unknown_class | no_handler | bad_payload
	)

	ServerHandledCount = metric.NewCounter(
		"tq/server/handled",
		"Count of handled non-rejected tasks",
		nil,
		field.String("task_class"), // matches TaskClass.ID
		field.String("queue"),      // the task queue that delivered the task
		field.String("result"),     // OK | retry | transient | fatal
		field.Int("retry"),         // 0 for first try, incrementing until cap.
	)

	ServerDurationMS = metric.NewCumulativeDistribution(
		"tq/server/duration",
		"Duration of handling of non-rejected tasks",
		&types.MetricMetadata{Units: types.Milliseconds},
		distribution.DefaultBucketer,
		field.String("task_class"), // matches TaskClass.ID
		field.String("queue"),      // the task queue that delivered the task
		field.String("result"),     // OK | retry | transient | fatal
	)

	ServerTaskLatency = metric.NewCumulativeDistribution(
		"tq/server/latency",
		"Time between task's expected ETA and actual completion",
		&types.MetricMetadata{Units: types.Milliseconds},
		distribution.DefaultBucketer,
		field.String("task_class"), // matches TaskClass.ID
		field.String("queue"),      // the task queue that delivered the task
		field.String("result"),     // OK | retry | transient | fatal
		field.Int("retry"),         // 0 for first try, incrementing until cap.
	)

	ServerRunning = metric.NewInt(
		"tq/server/running",
		"Number of task handlers currently running",
		nil,
		field.String("task_class"), // matches TaskClass.ID
	)
)
