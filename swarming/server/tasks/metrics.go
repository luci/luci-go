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

package tasks

import (
	"context"
	"time"

	"go.chromium.org/luci/common/clock"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/metrics"
	"go.chromium.org/luci/swarming/server/model"
)

// onTaskStatusChangeSchedulerLatency reports to TaskStatusChangeSchedulerLatency
// for the task.
func onTaskStatusChangeSchedulerLatency(ctx context.Context, trs *model.TaskResultSummary) {
	latency, deduped := trs.PendingNow(ctx, clock.Now(ctx))
	if deduped {
		// Don't report deduped tasks, they have no state changes.
		return
	}
	fields := trs.MetricFields(true)
	metrics.TaskStatusChangeSchedulerLatency.Add(
		ctx, float64(latency.Milliseconds()),
		fields.Pool,
		fields.SpecName,
		model.TaskStateString(trs.State),
		fields.DeviceType,
	)
}

// onTaskRequested reports to JobsRequested for the newly created task.
func onTaskRequested(ctx context.Context, trs *model.TaskResultSummary, deduped bool) {
	fields := trs.MetricFields(false)
	metrics.JobsRequested.Add(
		ctx, 1,
		fields.SpecName,
		fields.ProjectID,
		fields.SubprojectID,
		fields.Pool,
		fields.RBE,
		deduped,
	)
}

// onTaskToRunConsumed reports how long TaskToRun was pending.
func onTaskToRunConsumed(ctx context.Context, ttr *model.TaskToRun, trs *model.TaskResultSummary, consumedAt time.Time) {
	fields := trs.MetricFields(false)
	metrics.TaskToRunConsumeLatency.Add(
		ctx, max(consumedAt.Sub(ttr.Created), 0).Seconds()*1000.0,
		fields.SpecName,
		fields.ProjectID,
		fields.SubprojectID,
		fields.Pool,
		fields.RBE,
	)
}

// reportOnTaskCompleted reports to JobsCompleted and JobsDuration on the
// completed task.
func reportOnTaskCompleted(ctx context.Context, trs *model.TaskResultSummary) {
	fields := trs.MetricFields(false)
	status := model.TaskStateString(trs.State)
	var result string
	switch {
	case trs.InternalFailure:
		result = "infra_failure"
	case trs.Failure:
		result = "failure"
	default:
		result = "success"
	}

	metrics.JobsCompleted.Add(
		ctx, 1,
		fields.SpecName,
		fields.ProjectID,
		fields.SubprojectID,
		fields.Pool,
		fields.RBE,
		result,
		status,
	)

	if trs.DurationSecs.IsSet() {
		metrics.JobsDuration.Add(
			ctx, trs.DurationSecs.Get(),
			fields.SpecName,
			fields.ProjectID,
			fields.SubprojectID,
			fields.Pool,
			fields.RBE,
			result,
		)
	}
}

// onTaskExpired reports to task expiration metrics.
func onTaskExpired(ctx context.Context, trs *model.TaskResultSummary, ttr *model.TaskToRun, reason string) {
	fields := trs.MetricFields(false)

	// Slice expiration.
	if ttr.ExpirationDelay.IsSet() {
		metrics.TaskSliceExpirationDelay.Add(
			ctx, ttr.ExpirationDelay.Get(),
			fields.ProjectID,
			fields.RBE,
			ttr.TaskSliceIndex(),
			reason,
		)
	}

	// Task expiration.
	if trs.State == apipb.TaskState_EXPIRED {
		metrics.TasksExpired.Add(
			ctx, 1,
			fields.SpecName,
			fields.ProjectID,
			fields.SubprojectID,
			fields.Pool,
			fields.RBE,
			trs.RequestPriority,
		)
	}

	if trs.ExpirationDelay.IsSet() {
		metrics.TaskExpirationDelay.Add(
			ctx, trs.ExpirationDelay.Get(),
			fields.ProjectID,
			fields.RBE,
		)
	}
}
