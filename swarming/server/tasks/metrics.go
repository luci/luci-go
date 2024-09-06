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

	"go.chromium.org/luci/common/clock"

	"go.chromium.org/luci/swarming/server/metrics"
	"go.chromium.org/luci/swarming/server/model"
)

// onTaskStatusChangeSchedulerLatency reports to TaskStatusChangeSchedulerLatency
// for the task.
func onTaskStatusChangeSchedulerLatency(ctx context.Context, trs *model.TaskResultSummary) {
	tags := model.TagListToMap(trs.Tags)
	latency, deduped := trs.PendingNow(ctx, clock.Now(ctx))
	if deduped {
		// Don't report deduped tasks, they have no state changes.
		return
	}
	metrics.TaskStatusChangeSchedulerLatency.Add(ctx, float64(latency.Milliseconds()), tags["pool"], model.SpecName(tags), trs.State.String(), tags["device_type"])
}
