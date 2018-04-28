// Copyright 2017 The LUCI Authors.
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

package presentation

import (
	"golang.org/x/net/context"

	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/scheduler/appengine/catalog"
	"go.chromium.org/luci/scheduler/appengine/engine"
	"go.chromium.org/luci/scheduler/appengine/schedule"
	"go.chromium.org/luci/scheduler/appengine/task"
)

// PublicStateKind defines state of the job which is exposed in UI and API
// instead of internal states which are kept as an implementation detail.
type PublicStateKind string

// When a PublicStateKind is added/removed/updated, update scheduler api proto
// doc for `JobState`.
const (
	PublicStateDisabled  PublicStateKind = "DISABLED"
	PublicStatePaused    PublicStateKind = "PAUSED"
	PublicStateRunning   PublicStateKind = "RUNNING"
	PublicStateScheduled PublicStateKind = "SCHEDULED"
	PublicStateWaiting   PublicStateKind = "WAITING"
)

// GetPublicStateKind returns user-friendly state for a job.
func GetPublicStateKind(j *engine.Job, traits task.Traits) PublicStateKind {
	// TODO(vadimsh): Expose more states.
	cronTick := j.CronTickTime()
	switch {
	case len(j.ActiveInvocations) != 0:
		return PublicStateRunning
	case !j.Enabled:
		return PublicStateDisabled
	case j.Paused:
		return PublicStatePaused
	case !cronTick.IsZero() && cronTick != schedule.DistantFuture:
		return PublicStateScheduled
	default:
		return PublicStateWaiting
	}
}

// GetJobTraits asks the corresponding task manager for a traits struct.
func GetJobTraits(ctx context.Context, cat catalog.Catalog, j *engine.Job) (task.Traits, error) {
	taskDef, err := cat.UnmarshalTask(ctx, j.Task)
	if err != nil {
		logging.WithError(err).Warningf(ctx, "Failed to unmarshal task proto for %s", j.JobID)
		return task.Traits{}, err
	}
	if manager := cat.GetTaskManager(taskDef); manager != nil {
		return manager.Traits(), nil
	}
	return task.Traits{}, nil
}
