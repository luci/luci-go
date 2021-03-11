// Copyright 2020 The LUCI Authors.
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

package prjpb

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// PMTaskInterval is target frequency of executions of ManageProjectTask.
//
// See Dispatch() for details.
const PMTaskInterval = time.Second

// MaxAcceptableDelay prevents TQ tasks which arrive too late from invoking PM.
//
// MaxAcceptableDelay / PMTaskInterval effectively limits # concurrent
// invocations of PM on the same project that may happen due to task retries,
// delays, and queue throttling.
//
// Do not set too low, as this may prevent actual PM invoking from happening at
// all if the TQ is overloaded.
const MaxAcceptableDelay = 60 * time.Second

// ManageProjectTaskRef is used by PM implementation to add its handler.
var ManageProjectTaskRef tq.TaskClassRef

const (
	ManageProjectTaskClass = "manage-project"
	PurgeCLTaskClass       = "purge-project-cl"
)

func init() {
	ManageProjectTaskRef = tq.RegisterTaskClass(tq.TaskClass{
		ID:        ManageProjectTaskClass,
		Prototype: &ManageProjectTask{},
		Queue:     "manage-project",
	})

	tq.RegisterTaskClass(tq.TaskClass{
		ID:        "kick-" + ManageProjectTaskClass,
		Prototype: &KickManageProjectTask{},
		Queue:     "kick-manage-project",
		Quiet:     true,
		Handler: func(ctx context.Context, payload proto.Message) error {
			task := payload.(*KickManageProjectTask)
			var eta time.Time
			if t := task.GetEta(); t != nil {
				eta = t.AsTime()
			}
			err := Dispatch(ctx, task.GetLuciProject(), eta)
			return common.TQifyError(ctx, err)
		},
	})
}

// Dispatch ensures invocation of ProjectManager via ManageProjectTask.
//
// ProjectManager will be invoked at approximately no earlier than both:
// * eta time
// * next possible.
func Dispatch(ctx context.Context, luciProject string, eta time.Time) error {
	if datastore.CurrentTransaction(ctx) != nil {
		// TODO(tandrii): use txndefer to immediately trigger a ManageProjectTask after
		// transaction completes to reduce latency in *most* circumstances.
		// The KickManageProjectTask is still required for correctness.
		payload := &KickManageProjectTask{LuciProject: luciProject}
		if !eta.IsZero() {
			payload.Eta = timestamppb.New(eta)
		}
		return tq.AddTask(ctx, &tq.Task{
			Title:            luciProject,
			DeduplicationKey: "", // not allowed in a transaction
			Payload:          payload,
		})
	}

	// If actual local clock is more than `clockDrift` behind, the "next" computed
	// ManageProjectTask moment might be already executing, meaning task dedup
	// will ensure no new task will be scheduled AND the already executing run
	// might not have read the Event that was just written.
	// Thus, this should be large for safety. However, large value leads to higher
	// latency of event processing of non-busy ProjectManagers.
	// TODO(tandrii): this can be reduced significantly once safety "ping" events
	// are originated from Config import cron tasks.
	const clockDrift = 100 * time.Millisecond
	now := clock.Now(ctx).Add(clockDrift) // Use the worst possible time.
	if eta.IsZero() || eta.Before(now) {
		eta = now
	}
	eta = eta.Truncate(PMTaskInterval).Add(PMTaskInterval)
	return tq.AddTask(ctx, &tq.Task{
		Title:            luciProject,
		DeduplicationKey: fmt.Sprintf("%s\n%d", luciProject, eta.UnixNano()),
		ETA:              eta,
		Payload:          &ManageProjectTask{LuciProject: luciProject, Eta: timestamppb.New(eta)},
	})
}
