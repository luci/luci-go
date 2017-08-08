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
	"fmt"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/scheduler/appengine/catalog"
	"go.chromium.org/luci/scheduler/appengine/engine"
	"go.chromium.org/luci/scheduler/appengine/task"
)

// PublicStateKind defines state of the job which is exposed in UI and API
// instead of internal engine.StateKind which is kept as an implementation
// detail.
type PublicStateKind string

// When a PublicStateKind is added/removed/updated, update scheduler api proto
// doc for `JobState`.
const (
	PublicStateDisabled  PublicStateKind = "DISABLED"
	PublicStateOverrun   PublicStateKind = "OVERRUN"
	PublicStatePaused    PublicStateKind = "PAUSED"
	PublicStateRetrying  PublicStateKind = "RETRYING"
	PublicStateRunning   PublicStateKind = "RUNNING"
	PublicStateScheduled PublicStateKind = "SCHEDULED"
	PublicStateStarting  PublicStateKind = "STARTING"
	PublicStateSuspended PublicStateKind = "SUSPENDED"
	PublicStateWaiting   PublicStateKind = "WAITING"
)

// jobStateInternalToPublic translates some internal states to public ones.
// However, this map is not sufficient, so see and use GetPublicStateKind
// function to handle the translation.
var jobStateInternalToPublic = map[engine.StateKind]PublicStateKind{
	engine.JobStateDisabled:  PublicStateDisabled,
	engine.JobStateScheduled: PublicStateScheduled,
	engine.JobStateSuspended: PublicStateSuspended,
	engine.JobStateRunning:   PublicStateRunning,
	engine.JobStateOverrun:   PublicStateOverrun,
}

// GetPublicStateKind returns user-friendly state and labelClass.
func GetPublicStateKind(j *engine.Job, traits task.Traits) PublicStateKind {
	switch {
	case j.State.IsRetrying():
		// Retries happen when invocation fails to launch (move from "STARTING" to
		// "RUNNING" state). Such invocation is retried (as new invocation). When
		// a retry is enqueued, we display the job state as "RETRYING" (even though
		// technically it is still "QUEUED").
		return PublicStateRetrying
	case !traits.Multistage && j.State.InvocationID != 0:
		// The job has an active invocation and the engine has called LaunchTask for
		// it already. Jobs with Multistage == false trait do all their work in
		// LaunchTask, so we display them as "RUNNING" (instead of "STARTING").
		return PublicStateRunning
	case j.State.State == engine.JobStateQueued:
		// An invocation has been added to the task queue, and the engine hasn't
		// attempted to launch it yet.
		return PublicStateStarting
	case j.State.State == engine.JobStateSlowQueue:
		// Job invocation is still in the task queue, but new invocation should be
		// starting now (so the queue is lagging for some reason).
		return PublicStateStarting
	case j.Paused && j.State.State == engine.JobStateSuspended:
		// Paused jobs don't have a schedule, so they are always in "SUSPENDED"
		// state. Make it clearer that they are just paused. This applies to both
		// triggered and periodic jobs.
		return PublicStatePaused
	case j.State.State == engine.JobStateSuspended && j.Flavor == catalog.JobFlavorTriggered:
		// Triggered jobs don't run on a schedule. They are in "SUSPENDED" state
		// between triggering events, rename it to "WAITING" for clarity.
		return PublicStateWaiting
	default:
		if r, ok := jobStateInternalToPublic[j.State.State]; !ok {
			panic(fmt.Errorf("unknown state: %q", j.State.State))
		} else {
			return r
		}
	}
}

func GetJobTraits(ctx context.Context, cat catalog.Catalog, j *engine.Job) (task.Traits, error) {
	// trais = task.Traits{}
	taskDef, err := cat.UnmarshalTask(j.Task)
	if err != nil {
		logging.WithError(err).Warningf(ctx, "Failed to unmarshal task proto for %s", j.JobID)
		return task.Traits{}, err
	}
	if manager := cat.GetTaskManager(taskDef); manager != nil {
		return manager.Traits(), nil
	}
	return task.Traits{}, nil
}
