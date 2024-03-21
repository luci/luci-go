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
	"time"

	"go.chromium.org/luci/gae/service/datastore"
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

// ExportSchedule stores the per-table timestamp to start exporting events from.
type ExportSchedule struct {
	_extra datastore.PropertyMap `gae:"-,extra"`
	_kind  string                `gae:"$kind,bq.ExportSchedule"`

	// ID is the BQ table name where events are exported to.
	//
	// It is one of the constants defined above, e.g. TaskRequests.
	ID string `gae:"$id"`

	// NextExport is a timestamp to start exporting events from.
	//
	// It is updated by scheduleExportTasks once it schedules TQ tasks that do
	// actual export.
	NextExport time.Time `gae:",noindex"`
}

// ExportState stores the name of the WriteStream associated with a particular
// export task.
type ExportState struct {
	_extra datastore.PropertyMap `gae:"-,extra"`
	_kind  string                `gae:"$kind,bq.ExportState"`

	// ID is taskspb.ExportInterval.OperationId of the corresponding export task.
	//
	// This ID is derived based on the destination table name and the time
	// interval being exported.
	ID string `gae:"$id"`

	// WriteStreamName is name of the BigQuery WriteStream being exported to.
	//
	// When empty, it means we have yet to start working on the export task.
	//
	// After the WriteStream has been committed, this can be checked using the
	// BigQuery managedwriter SDK.
	WriteStreamName string `gae:",noindex"`
}
