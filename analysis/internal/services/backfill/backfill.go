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

// Package backfill implements tasks to backfill data in BigQuery.
package backfill

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/iterator"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/analysis/internal/bqutil"
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
)

const (
	taskClass = "backfill"
	queue     = "backfill"
)

// The last test_results partition that may be backfilled by this process.
// Data after this day needs to be manually backfilled with a different query
// that only backfills data not already in the table.
var lastAllowedDay = time.Date(2024, 5, 8, 0, 0, 0, 0, time.UTC)

var tc = tq.RegisterTaskClass(tq.TaskClass{
	ID:        taskClass,
	Prototype: &taskspb.Backfill{},
	Queue:     queue,
	Kind:      tq.NonTransactional,
})

// RegisterTaskHandler registers the handler for backfill tasks.
func RegisterTaskHandler(srv *server.Server) error {
	client, err := bqutil.Client(srv.Context, srv.Options.CloudProject)
	if err != nil {
		return err
	}

	handler := func(ctx context.Context, payload proto.Message) error {
		task := payload.(*taskspb.Backfill)
		return backfill(ctx, client, task)
	}
	tc.AttachHandler(handler)
	return nil
}

// Schedule enqueues a task to backfill a day of data.
func Schedule(ctx context.Context, task *taskspb.Backfill) error {
	title := fmt.Sprintf("backfill-%v", task.Day.AsTime().Format("20060102"))

	dedupKey, err := randomDeduplicationKey()
	if err != nil {
		return errors.Annotate(err, "obtain deduplication key").Err()
	}
	taskProto := &tq.Task{
		Title: title,
		// Copy the task to avoid the caller retaining an alias to
		// the task proto passed to tq.AddTask.
		Payload: proto.Clone(task).(*taskspb.Backfill),
		// Use a deduplication key to avoid retried task creations
		// accidentally resulting in two tasks being created, in case
		// of failure to receive CreateTask response.
		// Note that this is only a best-effort deduplication, the
		// task should still assume the possibility of multiple
		// tasks being created and avoid data correctness issues
		// in this case.
		DeduplicationKey: dedupKey,
	}

	// Manually retry transient errors. The Cloud Tasks client
	// does not automatically retry CreateTask RPCs, presumably
	// as the RPC does not offer strong guarantees against multiple
	// task creation in case of retry.
	err = retry.Retry(ctx, transient.Only(retry.Default), func() error {
		err := tq.AddTask(ctx, taskProto)
		if err != nil {
			return errors.Annotate(err, "create task").Err()
		}
		return nil
	}, nil)
	return err
}

func randomDeduplicationKey() (string, error) {
	var b [16]byte
	_, err := rand.Read(b[:])
	if err != nil {
		return "", errors.Annotate(err, "read random bytes").Err()
	}
	return hex.EncodeToString(b[:]), nil
}

func backfill(ctx context.Context, client *bigquery.Client, task *taskspb.Backfill) error {
	if task.Day.AsTime().After(lastAllowedDay) {
		return tq.Fatal.Apply(errors.Reason("cannot backfill after %v (got %v)", lastAllowedDay, task.Day.AsTime()).Err())
	}

	// Check if we have previously backfilled this date. This is to make the
	// task resilient to retries and avoid backfill mistakes.
	query := client.Query(`SELECT TRUE FROM internal.test_results WHERE TIMESTAMP_TRUNC(partition_time, DAY) = @partitionDay LIMIT 1`)
	query.Parameters = []bigquery.QueryParameter{
		{Name: "partitionDay", Value: task.Day.AsTime()},
	}
	it, err := query.Read(ctx)
	if err != nil {
		return errors.Annotate(err, "check partition is empty").Err()
	}
	hasRow := false
	for {
		var row []bigquery.Value
		err := it.Next(&row)
		if errors.Is(err, iterator.Done) {
			break
		}
		hasRow = true
	}

	if hasRow {
		logging.Infof(ctx, "Existing backfill task for %v, partition already populated.", task.Day.AsTime())
		return nil
	}

	logging.Infof(ctx, "Continuing with backfill of day %v...", task.Day.AsTime())
	query = client.Query(`
		INSERT INTO internal.test_results (project, test_id, variant, variant_hash, invocation, partition_time, parent, name, result_id, expected, status, summary_html, start_time, duration_secs, tags, failure_reason, skip_reason, properties, sources, source_ref, source_ref_hash, test_metadata, insert_time)
		SELECT
			v.project,
			v.test_id,
			v.variant,
			v.variant_hash,
			STRUCT(v.invocation.id as id, v.invocation.realm as realm) as invocation,
			v.partition_time,
			STRUCT(r.parent.id as id, CAST([] AS ARRAY<STRUCT<key STRING, value STRING>>) as tags, CAST(NULL as STRING) as realm, CAST(NULL AS JSON) as properties) as parent,
			r.name,
			r.result_id,
			r.expected,
			r.status,
			r.summary_html,
			r.start_time,
			r.duration as duration_secs,
			r.tags,
			r.failure_reason,
			r.skip_reason,
			r.properties,
			STRUCT(v.sources.gitiles_commit, ARRAY(SELECT STRUCT(cl.host, cl.project, cl.change, cl.patchset, cl.owner_kind) FROM UNNEST(v.sources.changelists) cl), v.sources.is_dirty) as sources,
			v.source_ref,
			v.source_ref_hash,
			v.test_metadata,
			v.insert_time
		FROM internal.test_verdicts v, UNNEST(results) r
		WHERE TIMESTAMP_TRUNC(v.partition_time, DAY) = @partitionDay
	`)
	query.Parameters = []bigquery.QueryParameter{
		{Name: "partitionDay", Value: task.Day.AsTime()},
	}
	job, err := query.Run(ctx)
	if err != nil {
		return errors.Annotate(err, "start partition backfill").Err()
	}
	status, err := bqutil.WaitForJob(ctx, job)
	if err != nil {
		return errors.Annotate(err, "wait for partition backfill").Err()
	}
	if err := status.Err(); err != nil {
		return errors.Annotate(err, "partition backfill").Err()
	}
	return nil
}
