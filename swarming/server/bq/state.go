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
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
)

const (
	// TaskRequests exports model.TaskRequest to the `task_requests` table.
	TaskRequests = "task_requests"

	// BotEvents exports model.BotEvent to the `bot_events` table.
	BotEvents = "bot_events"

	// TaskRunResults exports model.TaskRunResults to the `task_results_run`
	// table.
	TaskRunResults = "task_results_run"

	// TaskResultSummaries exports model.TaskResultSummaries to the
	// `task_results_summary` table.
	TaskResultSummaries = "task_results_summary"
)

// How long to keep ExportState in datastore before deleting it.
const exportStateExpiry = 7 * 24 * time.Hour

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

// ExportState stores the BQ WriteStream associated with an export task.
//
// It exists only if the write stream is already finalize and either already
// committed or is about to be committed. The purpose of this entity is to
// make sure that if an export task operation is retried, it doesn't
// accidentally export the same data again.
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
	// Used when restarting the export task after transient errors to check if
	// the stream is committed.
	WriteStreamName string `gae:",noindex"`

	// ExpireAt is a timestamp when this entity can be deleted.
	//
	// Used in the Cloud Datastore TTL policy.
	ExpireAt time.Time `gae:",noindex"`
}

// MigrationState is used during Python => Go BQ export migration.
//
// It holds a timestamp when the migration must happen. It is respected by both
// Python and Go code bases: when this timestamp is reached, Python will stop
// exports (by doing nothing in the export TQ task) and Go will start them
// (by starting doing something in the export TQ task).
//
// The timestamp must be an exact minute e.g. "04:30:00.000 UTC".
//
// Python exports data in 1m intervals, aligned to 1m grid:
//
//	[04:30:00, 04:31:00)
//	[04:31:00, 04:32:00)
//	...
//
// Go exports data in 15s intervals, also aligned to 1m grid:
//
//	[04:30:00, 04:30:15)
//	[04:30:15, 04:30:30)
//	[04:30:30, 04:30:45)
//	[04:30:45, 04:31:00)
//	[04:31:00, 04:31:15)
//	...
//
// If we switch at an exact minute, there will be no gaps.
//
// If the entity is missing, assume Python is the active exporter.
type MigrationState struct {
	_extra datastore.PropertyMap `gae:"-,extra"`
	_kind  string                `gae:"$kind,BqMigrationState"`

	// ID is a table name being migrated, e.g. "task_requests".
	ID string `gae:"$id"`

	// PythonToGo is when to switch from Python to Go.
	PythonToGo time.Time `gae:"python_to_go,noindex"`
}

// SetMigrationState is manually called to populate MigrationState entity.
func SetMigrationState(ctx context.Context, table string, fromNow time.Duration) error {
	when := clock.Now(ctx).Add(fromNow).Truncate(time.Minute).UTC()
	logging.Infof(ctx, "Table %q will be switched at %s", table, when)
	return datastore.Put(ctx, &MigrationState{
		ID:         table,
		PythonToGo: when,
	})
}

// ClearMigrationState deletes MigrationState entity.
func ClearMigrationState(ctx context.Context, table string) error {
	return datastore.Delete(ctx, datastore.NewKey(ctx, "BqMigrationState", table, 0, nil))
}

// ShouldExport is true if Go BQ export tasks should do the export.
func ShouldExport(ctx context.Context, table string, ts time.Time) (bool, error) {
	state := &MigrationState{ID: table}
	switch err := datastore.Get(ctx, state); {
	case errors.Is(err, datastore.ErrNoSuchEntity):
		return false, nil
	case err == nil:
		// This checks `ts >= PythonToGo`.
		return !state.PythonToGo.After(ts), nil
	default:
		return false, errors.Annotate(err, "failed to check MigrationState").Tag(transient.Tag).Err()
	}
}
