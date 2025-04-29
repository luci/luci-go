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

// Package groupscheduler schedules group changepoints tasks.
package groupscheduler

import (
	"context"
	"time"

	"cloud.google.com/go/bigquery"

	"go.chromium.org/luci/common/bq"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/analysis/internal/bqutil"
	"go.chromium.org/luci/analysis/internal/changepoints"
	"go.chromium.org/luci/analysis/internal/services/changepointgrouper"
)

// The number of weeks to schedule grouping task for in each run.
const weeksPerRun = 8

func CronHandler(ctx context.Context, gcpProject string) (retErr error) {
	if err := scheduleGroupingTasks(ctx); err != nil {
		return errors.Annotate(err, "schedule group changepoint tasks").Err()
	}
	client, err := bq.NewClient(ctx, gcpProject)
	if err != nil {
		return errors.Annotate(err, "create bq client").Err()
	}
	defer func() {
		if err := client.Close(); err != nil && retErr == nil {
			retErr = errors.Annotate(err, "closing bq client").Err()
		}
	}()
	if err := purgeStaleRows(ctx, client); err != nil {
		return errors.Annotate(err, "purge stale rows").Err()
	}
	return nil
}

func scheduleGroupingTasks(ctx context.Context) error {
	currentWeek := changepoints.StartOfWeek(clock.Now(ctx))
	for i := 0; i < weeksPerRun; i++ {
		week := currentWeek.Add(-time.Duration(i) * 7 * 24 * time.Hour)
		if err := changepointgrouper.Schedule(ctx, week); err != nil {
			return errors.Annotate(err, "schedule group changepoint task week %s", week).Err()
		}
	}
	return nil
}

// purgeStaleRows purges stales rows from grouped_changepoints table.
func purgeStaleRows(ctx context.Context, client *bigquery.Client) error {
	q := client.Query(`
	DELETE FROM grouped_changepoints cps1
	WHERE
		-- Not the latest (version, start_hour_week) entry.
		cps1.version < (
			SELECT MAX(version)
			FROM grouped_changepoints cps2
			WHERE cps1.start_hour_week = cps2.start_hour_week
		)
		-- Keep rows that is inserted within the last 60 minutes.
		-- This is to support querying a particular version of the table
		-- within the last 60 minutes for pagination.
		AND cps1.version < TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 60 MINUTE)
	`)
	q.DefaultDatasetID = bqutil.InternalDatasetID
	job, err := q.Run(ctx)
	if err != nil {
		return errors.Annotate(err, "purge stale rows").Err()
	}
	waitCtx, cancel := context.WithTimeout(ctx, time.Minute*9)
	defer cancel()
	js, err := job.Wait(waitCtx)
	if err != nil {
		return errors.Annotate(err, "waiting for query to complete").Err()
	}
	if err := js.Err(); err != nil {
		return errors.Annotate(err, "DDL query failed").Err()
	}
	return nil
}
