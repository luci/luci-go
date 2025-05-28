// Copyright 2019 The LUCI Authors.
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

package metrics

import (
	"context"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/gae/service/datastore"
)

var (
	tasksExecuting = metric.NewInt(
		"gce/tasks/executing",
		"The number of task queue tasks currently executing.",
		nil,
		field.String("queue"),
	)

	tasksPending = metric.NewInt(
		"gce/tasks/pending",
		"The number of task queue tasks currently pending.",
		nil,
		field.String("queue"),
	)

	tasksTotal = metric.NewInt(
		"gce/tasks/total",
		"The total number of task queue tasks in the queue.",
		nil,
		field.String("queue"),
	)
)

// TaskCount is a root entity representing a count of task queue tasks.
type TaskCount struct {
	// _extra is where unknown properties are put into memory.
	// Extra properties are not written to the datastore.
	_extra datastore.PropertyMap `gae:"-,extra"`
	// _kind is the entity's kind in the datastore.
	_kind string `gae:"$kind,TaskCount"`
	// ID is the unique identifier for this count.
	ID string `gae:"$id"`
	// Queue is the task queue for this count.
	Queue string `gae:"queue"`
	// Computed is the time this count was computed.
	Computed time.Time `gae:"computed"`
	// Executing is a count of currently executing tasks.
	Executing int `gae:"executing,noindex"`
	// Total is a count of the total number of tasks in the queue.
	Total int `gae:"pending,noindex"`
}

// Update updates metrics for counts of tasks for the given queue.
func (tc *TaskCount) Update(c context.Context, queue string, exec, tot int) error {
	// Queue names are globally unique, so we can use them as IDs.
	tc.ID = queue
	tc.Computed = clock.Now(c).UTC()
	tc.Queue = queue
	tc.Executing = exec
	tc.Total = tot
	if err := datastore.Put(c, tc); err != nil {
		return errors.Fmt("failed to store count: %w", err)
	}
	return nil
}

// updateTasks sets task queue task metrics.
func updateTasks(c context.Context) {
	now := clock.Now(c)
	q := datastore.NewQuery("TaskCount").Order("computed")
	if err := datastore.Run(c, q, func(tc *TaskCount) {
		if now.Sub(tc.Computed) > 5*time.Minute {
			logging.Debugf(c, "deleting outdated count %q", tc.Queue)
			if err := datastore.Delete(c, tc); err != nil {
				logging.Errorf(c, "%s", err)
			}
			return
		}
		tasksExecuting.Set(c, int64(tc.Executing), tc.Queue)
		tasksPending.Set(c, int64(tc.Total-tc.Executing), tc.Queue)
		tasksTotal.Set(c, int64(tc.Total), tc.Queue)
	}); err != nil {
		errors.Log(c, errors.Fmt("failed to fetch counts: %w", err))
	}
}

func init() {
	tsmon.RegisterGlobalCallback(updateTasks, tasksExecuting, tasksPending, tasksTotal)
}
