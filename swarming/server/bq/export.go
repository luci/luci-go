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

package bq

import (
	"context"
	"errors"
	"fmt"
	"time"

	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/swarming/server/bq/taskspb"
)

// exportDuration is the duration of the time interval to export to bigquery.
const exportDuration = 15 * time.Second

// maxTasksToSchedule is the maximum number of export tasks which may be
// scheduled per cron job.
const maxTasksToSchedule = 20

// latestAge represents the latest time in the past which can be scheduled for
// export by ScheduleExportTasks
const latestAge = 2 * time.Minute

// maxExportStateAge is the amount of time before an ExportState is garbage
// collected.
const maxExportStateAge = 24 * time.Hour

func RegisterTQTasks() {
	tq.RegisterTaskClass(tq.TaskClass{
		ID:        "bq-export-interval",
		Kind:      tq.NonTransactional,
		Prototype: &taskspb.CreateExportTask{},
		Queue:     "bq-export-interval",
		Handler: func(ctx context.Context, payload proto.Message) error {
			return exportTask(ctx, payload.(*taskspb.CreateExportTask))
		},
	})
}

func tableID(cloudProject, dataset, tableName string) string {
	return fmt.Sprintf("%s.%s.%s", cloudProject, dataset, tableName)
}

// CleanupExportState deletes export states which are older than
// maxExportStateAge.
func CleanupExportState(ctx context.Context) error {
	// ScheduleExportTasks runs every 1m
	// * schedules 4 exports per minute
	// * on 4 tables
	const batchSize = 4 * 4 * 10
	// Will need to tune this value
	const nWorkers = 64
	g := new(errgroup.Group)
	g.SetLimit(nWorkers)

	now := clock.Now(ctx).UTC()
	cutoff := now.Add(-maxExportStateAge)
	logging.Infof(ctx, "Deleting ExportState created earlier than %s", cutoff)
	q := datastore.NewQuery(exportStateKind).Lte("CreatedAt", cutoff)

	deleteBatch := func(batch []*datastore.Key) {
		g.Go(func() error {
			logging.Debugf(ctx, "Attempting delete of %d ExportStates", len(batch))
			return datastore.Delete(ctx, batch)
		})
	}

	// RunInBatch works sequentially, so we can use closure to store the
	// current batch.
	batch := make([]*datastore.Key, 0, batchSize)
	err := datastore.RunBatch(ctx, batchSize, q, func(key *datastore.Key) {
		batch = append(batch, key)
		if len(batch) == batchSize {
			deleteBatch(batch)
			batch = make([]*datastore.Key, 0, batchSize)
		}
	})
	// Whatever is left of batches gets deleted in this call.
	deleteBatch(batch)

	if err != nil {
		logging.Errorf(ctx, "ExportState cleanup query failed")
		// Useful work may still happen in g, in that case wait until its done
		return errors.Join(err, g.Wait())
	}
	return g.Wait()
}

// ScheduleExportTasks creates a series of tasks responsible for
// exporting a specific time interval to bigquery. All of the TQ tasks scheduled
// will cover the range [NextExport, cutoff). If exports fall behind schedule,
// the scheduler will try and catch up as much as possible by spawning as many
// tasks as possible. A `DuplicationKey` is used to ensure that no duplicate
// tasks are created if there are temporary failures to write to datastore. Will
// schedule a maxium of MaxTasksToSchedule export tasks.
func ScheduleExportTasks(ctx context.Context, cloudProject, dataset, tableName string) error {
	now := clock.Now(ctx).UTC()
	cutoff := now.Add(-latestAge)
	tableID := tableID(cloudProject, dataset, tableName)
	logging.Infof(ctx, "Scheduling export tasks: %s - %s", tableID, cutoff)
	sch := ExportSchedule{Key: exportScheduleKey(ctx, tableName)}
	err := datastore.Get(ctx, &sch)
	if err != nil {
		if errors.Is(err, datastore.ErrNoSuchEntity) {
			sch.NextExport = now.Truncate(time.Minute)
			logging.Infof(ctx, "Creating initial ExportSchedule - %+v", &sch)
			return datastore.Put(ctx, &sch)
		} else {
			return err
		}
	}
	i := 0
	for {
		// At this point, we have generated exports up until the cutoff point
		// Or we have reached maximum number of export tasks to schedule.
		if sch.NextExport.Add(exportDuration).After(cutoff) || i >= maxTasksToSchedule {
			logging.Infof(ctx, "Scheduling export tasks done: %s", sch.NextExport)
			break
		}
		payload := taskspb.CreateExportTask{
			Start:        timestamppb.New(sch.NextExport),
			Duration:     durationpb.New(exportDuration),
			CloudProject: cloudProject,
			Dataset:      dataset,
			TableName:    tableName,
		}
		ts := sch.NextExport.Unix()
		dedupKey := fmt.Sprintf("%s:%d:%d", tableID, ts, exportDuration/time.Second)
		task := tq.Task{
			Title:            dedupKey,
			DeduplicationKey: dedupKey,
			Payload:          &payload,
		}
		logging.Debugf(ctx, "Triggering %s: - %+v",
			dedupKey,
			&payload)
		err = tq.AddTask(ctx, &task)
		if err != nil {
			logging.Warningf(ctx, "Failed to trigger export task: %+v", &payload)
			break
		}
		sch.NextExport = sch.NextExport.Add(exportDuration)
		i += 1
	}
	logging.Infof(ctx, "Updating export schedule after %d iterations: %+v", i, sch)
	return errors.Join(err, datastore.Put(ctx, &sch))
}

func exportTask(ctx context.Context, t *taskspb.CreateExportTask) error {
	logging.Infof(ctx, "ExportTask started for %s:%s:%d",
		tableID(t.CloudProject, t.Dataset, t.TableName),
		t.Start.AsTime(),
		t.Duration)
	return nil
}
