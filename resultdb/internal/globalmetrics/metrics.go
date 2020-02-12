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

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/types"
	"go.chromium.org/luci/server"

	"go.chromium.org/luci/resultdb/internal/span"
)

var (
	expiredResultsDelayMetric = metric.NewInt(
		"resultdb/oldest_expired_result",
		"Unix timestamp of the earliest result not yet purged",
		nil)

	oldestTaskMetric = metric.NewInt(
		"resultdb/task/oldest_create_time",
		"The creation UNIX timestamp of the oldest task.",
		&types.MetricMetadata{Units: types.Seconds},
		field.String("type")) // tasks.Type
)

// InitServer initializes a backend server.
func InitServer(srv *server.Server) {
	srv.RunInBackground("resultdb.global_metrics", reportGlobalMetricsContinuously)
}

// reportGlobalMetricsContinuously reports metrics every 5m until ctx is done.
func reportGlobalMetricsContinuously(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return

		case <-time.After(5 * time.Minute):
			if err := reportGlobalMetrics(ctx); err != nil && err != ctx.Err() {
				logging.Errorf(ctx, "Failed to report metrics: %s", err)
			}
		}
	}
}

func reportGlobalMetrics(ctx context.Context) error {
	return parallel.FanOutIn(func(work chan<- func() error) {
		work <- func() error {
			return reportOldestTask(ctx)
		}
		work <- func() error {
			return reportExpiredResultsDelayMetric(ctx)
		}
	})
}

func reportOldestTask(ctx context.Context) error {
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

func reportExpiredResultsDelayMetric(ctx context.Context) error {
	val, err := expiredResultsDelaySeconds(ctx)
	if err != nil {
		return err
	}
	expiredResultsDelayMetric.Set(ctx, val)
	return nil
}

// expiredResultsDelaySeconds computes the age of the oldest invocation
// pending to be purged in seconds.
func expiredResultsDelaySeconds(ctx context.Context) (int64, error) {
	st := spanner.NewStatement(`
		SELECT GREATEST(0, TIMESTAMP_DIFF(CURRENT_TIMESTAMP(), MIN(EarliestExpiration), SECOND)) AS BacklogSeconds
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
	var ret int64
	if err := span.QueryFirstRow(ctx, span.Client(ctx).Single(), st, &ret); err != nil {
		return 0, err
	}
	return ret, nil
}
