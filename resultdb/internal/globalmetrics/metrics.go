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

	oldestTaskMetric = metric.NewInt(
		"resultdb/task/oldest_create_time",
		"The creation Unix timestamp of the oldest task.",
		&types.MetricMetadata{Units: types.Seconds},
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

	srv.RunInBackground("resultdb.oldest_task", func(ctx context.Context) {
		cron.Run(ctx, interval, updateOldestTask)
	})

	srv.RunInBackground("resultdb.oldest_expired_result", func(ctx context.Context) {
		cron.Run(ctx, interval, updateExpiredResultsDelayMetric)
	})
}

func updateOldestTask(ctx context.Context) error {
	st := spanner.NewStatement(`
		SELECT TaskType, MIN(CreateTime)
		FROM InvocationTasks
		GROUP BY TaskType
	`)

	var b span.Buffer
	return span.Query(ctx, span.Client(ctx).Single(), st, func(row *spanner.Row) error {
		var taskType string
		var minCreateTime time.Time
		if err := b.FromSpanner(row, &taskType, &minCreateTime); err != nil {
			return err
		}
		oldestTaskMetric.Set(ctx, minCreateTime.Unix(), taskType)
		return nil
	})
}

func updateExpiredResultsDelayMetric(ctx context.Context) error {
	switch val, err := oldestExpiredResult(ctx); {
	case err == span.ErrNoResults:
		return nil
	case err != nil:
		return err
	default:
		oldestExpiredResultMetric.Set(ctx, val.Unix())
		return nil
	}
}

// oldestExpiredResult computes the creation time of the oldest invocation
// pending to be purged in seconds.
func oldestExpiredResult(ctx context.Context) (time.Time, error) {
	st := spanner.NewStatement(`
		SELECT MIN(EarliestExpiration)
		FROM (
			SELECT (
				SELECT MIN(ExpectedTestResultsExpirationTime)
				FROM Invocations@{FORCE_INDEX=InvocationsByExpectedTestResultsExpiration}
				WHERE ShardId = TargetShard
				AND ExpectedTestResultsExpirationTime IS NOT NULL
			) AS EarliestExpiration
			FROM UNNEST(GENERATE_ARRAY(0, (
				SELECT MAX(ShardId)
				FROM Invocations@{FORCE_INDEX=InvocationsByExpectedTestResultsExpiration}
				WHERE ExpectedTestResultsExpirationTime IS NOT NULL
			))) AS TargetShard
		)
	`)
	var ret time.Time
	err := span.QueryFirstRow(ctx, span.Client(ctx).Single(), st, &ret)
	return ret, err
}
