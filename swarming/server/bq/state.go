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
	"fmt"
	"time"

	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/swarming/server/bq/taskspb"
)

const (
	// TaskRequests exports model.TaskRequest to the `task_requests` table.
	TaskRequests = "task_requests"

	// Bots exports model.BotInfo and model.BotEvent to the `bot_events` table.
	Bots = "bot_events"

	// TaskRunResults exports model.TaskRunResults to the `task_results_run`
	// table.
	TaskRunResults = "task_results_run"

	// TaskResultSummaries exports model.TaskResultSummaries to the
	// `task_results_summary` table.
	TaskResultSummaries = "task_results_summary"
)

// ExportSchedule stores the highest timestamp which has been exported to
// bigquery for a specific type of data.
// Tasks which are earlier than NextExport may have already been triggered.
// Deduplication is done at the tq.Tasks level using DeduplicationKey.
type ExportSchedule struct {
	// Key is derived from `ExportType`. See exportScheduleKey.
	Key *datastore.Key `gae:"$key"`

	// NextExport is a timestamp which represents the newest known export state
	// time which has been created.
	NextExport time.Time `gae:",noindex"`
}

func exportScheduleKey(ctx context.Context, tableName string) *datastore.Key {
	return datastore.NewKey(ctx, "bq.ExportSchedule", tableName, 0, nil)
}

const exportStateKind = "bq.ExportState"

// ExportState stores the name of the WriteStream associated with exporting
// datastore data to a specific table for a time interval.
type ExportState struct {
	// Extra are entity properties that didn't match any declared ones below.
	//
	// Should normally be empty.
	Extra datastore.PropertyMap `gae:"-,extra"`

	// Key is derived using exportStateKey(...).
	// It designed to ensure that the same taskpb.CreateExportTask will always
	// point to the same ExportState object.
	// The key does not protect against duplicate values for duration changes
	// or changes of the tableName.
	Key *datastore.Key `gae:"$key"`

	// WriteStreamName is name of the writestream being exported to.
	// When empty, it means we have yet to start exporting this time interval.
	// After the WriteStream has been committed, this can be checked using the
	// bigquery managedwriter SDK.
	WriteStreamName string `gae:",noindex"`

	// CreatedAt is the time when the exportState was created. Used to determine
	// when to "garbage collect" the ExportState.
	CreatedAt time.Time `gae:""`
}

// exportStateKey derives a key based a hash derived from the string of the task
func exportStateKey(ctx context.Context, task *taskspb.CreateExportTask) *datastore.Key {
	tableID := tableID(task.CloudProject, task.Dataset, task.TableName)
	text := fmt.Sprintf("%s:%d:%d", tableID, task.Start.AsTime().Unix(), task.Duration.AsDuration())
	return datastore.NewKey(ctx, exportStateKind, text, 0, nil)
}
