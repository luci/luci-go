// Copyright 2022 The LUCI Authors.
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

package tryjob

import (
	"context"
	"fmt"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/cv/internal/common"
)

const CancelStaleTaskClass = "cancel-stale-tryjobs"
const UpdateTaskClass = "update-tryjob"

// TaskBindings allow us to assign handlers separately from task registration.
type TaskBindings struct {
	CancelStale tq.TaskClassRef
	Update      tq.TaskClassRef
	tqd         *tq.Dispatcher
}

// Notifier exports tryjob management methods.
type Notifier struct {
	Bindings TaskBindings
}

// NewNotifier creates a Notifier and registers tryjob management task classes.
func NewNotifier(tqd *tq.Dispatcher) *Notifier {
	return &Notifier{Bindings: TaskBindings{
		CancelStale: tqd.RegisterTaskClass(
			tq.TaskClass{
				ID:           CancelStaleTaskClass,
				Prototype:    &CancelStaleTryjobsTask{},
				Queue:        "cancel-stale-tryjobs",
				Kind:         tq.Transactional,
				Quiet:        true,
				QuietOnError: true,
			},
		),
		Update: tqd.RegisterTaskClass(
			tq.TaskClass{
				ID:           UpdateTaskClass,
				Prototype:    &UpdateTryjobTask{},
				Queue:        "update-tryjob",
				Kind:         tq.NonTransactional,
				Quiet:        true,
				QuietOnError: true,
			},
		),
		tqd: tqd,
	},
	}
}

// NotifyCancelStale schedules a task if tryjobs associated with the given CL
// may be stale and need to be cancelled.
func (n *Notifier) NotifyCancelStale(ctx context.Context, clid common.CLID, prevMinEquivalentPatchset, currentMinEquivalentPatchset int32) error {
	if prevMinEquivalentPatchset < currentMinEquivalentPatchset {
		err := n.Bindings.tqd.AddTask(ctx, &tq.Task{
			Payload: &CancelStaleTryjobsTask{
				Clid:                     int64(clid),
				PreviousMinEquivPatchset: prevMinEquivalentPatchset,
				CurrentMinEquivPatchset:  currentMinEquivalentPatchset,
			},
		})
		if err != nil {
			return errors.Annotate(err, "failed to schedule task to cancel stale tryjobs for CLID %d", clid).Tag(transient.Tag).Err()
		}
	}
	return nil
}

// ScheduleUpdate schedules a task to update the given tryjob.
// At least one ID must be given.
func (n *Notifier) ScheduleUpdate(ctx context.Context, id common.TryjobID, eid ExternalID) error {
	var taskTitle string
	switch {
	case id == 0 && eid == "":
		return errors.New("At least one of the tryjob's IDs must be given.")
	case id != 0 && eid != "":
		taskTitle = fmt.Sprintf("id-%d/eid-%s", id, eid)
	case id != 0:
		taskTitle = fmt.Sprintf("id-%d", id)
	case eid != "":
		taskTitle = fmt.Sprintf("eid-%s", eid)
	}
	// id will be set, but eid may not be. In such case, it's up to the task to
	// resolve it.
	return n.Bindings.tqd.AddTask(ctx, &tq.Task{
		Title:   taskTitle,
		Payload: &UpdateTryjobTask{ExternalId: string(eid), Id: int64(id)},
	})
}
