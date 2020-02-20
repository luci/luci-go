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

// Package globalmetrics reports metrics that are computationally heavy.
// There must be a single replica of globalmetrics server.
package globalmetrics

import (
	"context"
	"time"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/types"
	"go.chromium.org/luci/server"

	"go.chromium.org/luci/resultdb/internal/cron"
	"go.chromium.org/luci/resultdb/internal/span"
)

var (
	oldestExpiredResultMetric = metric.NewInt(
		"resultdb/oldest_expired_result",
		"Unix timestamp of the earliest result not yet purged",
		nil)
	expiredResultsPendingInvocationCount = metric.NewInt(
		"resultdb/expired_results/pending_invocations",
		"Number of pending invocations where expired results were not yet purged",
		nil)

	oldestTaskMetric = metric.NewInt(
		"resultdb/task/oldest_create_time",
		"The creation Unix timestamp of the oldest task.",
		&types.MetricMetadata{Units: types.Seconds},
		field.String("type")) // tasks.Type

	taskCountMetric = metric.NewInt(
		"resultdb/task/count",
		"Current number of tasks.",
		nil,
		field.String("type")) // tasks.Type
)

// Options is global metrics server configuration.
type Options struct {
	// UpdateInterval is how often to update metrics.
	UpdateInterval time.Duration
}

// InitServer initializes a backend server.
func InitServer(srv *server.Server, opts Options) {
	interval := opts.UpdateInterval
	if interval == 0 {
		interval = 5 * time.Minute
	}

	srv.RunInBackground("resultdb.tasks", func(ctx context.Context) {
		cron.Run(ctx, interval, updateTaskMetrics)
	})

	srv.RunInBackground("resultdb.oldest_expired_result", func(ctx context.Context) {
		cron.Run(ctx, interval, updateExpiredResultsMetrics)
	})
}

func updateTaskMetrics(ctx context.Context) error {
	st := spanner.NewStatement(`
		SELECT TaskType, MIN(CreateTime), COUNT(*)
		FROM InvocationTasks
		GROUP BY TaskType
	`)

	var b span.Buffer
	return span.Query(ctx, span.Client(ctx).Single(), st, func(row *spanner.Row) error {
		var taskType string
		var minCreateTime time.Time
		var count int64
		if err := b.FromSpanner(row, &taskType, &minCreateTime, &count); err != nil {
			return err
		}
		oldestTaskMetric.Set(ctx, minCreateTime.Unix(), taskType)
		taskCountMetric.Set(ctx, count, taskType)
		return nil
	})
}

func updateExpiredResultsMetrics(ctx context.Context) error {
	switch oldest, count, err := expiredResultStats(ctx); {
	case err == span.ErrNoResults:
		return nil
	case err != nil:
		return err
	default:
		oldestExpiredResultMetric.Set(ctx, oldest.Unix())
		expiredResultsPendingInvocationCount.Set(ctx, count)
		return nil
	}
}

// expiredResultStats computes the creation time of the oldest invocation
// pending to be purged in seconds.
func expiredResultStats(ctx context.Context) (oldestResult time.Time, pendingInvocationsCount int64, err error) {
	st := spanner.NewStatement(`
		SELECT
			MIN(ExpectedTestResultsExpirationTime) as EarliestExpiration,
			COUNT(*) as pending_count
		FROM UNNEST(GENERATE_ARRAY(0, (
			SELECT MAX(ShardId)
			FROM Invocations@{FORCE_INDEX=InvocationsByExpectedTestResultsExpiration}
			WHERE ExpectedTestResultsExpirationTime IS NOT NULL
		))) AS TargetShard
		JOIN Invocations@{FORCE_INDEX=InvocationsByExpectedTestResultsExpiration}
			ON ShardId = TargetShard
		WHERE ExpectedTestResultsExpirationTime IS NOT NULL
			AND ExpectedTestResultsExpirationTime < CURRENT_TIMESTAMP()
	`)
	err = span.QueryFirstRow(ctx, span.Client(ctx).Single(), st, &oldestResult, &pendingInvocationsCount)
	return
}
