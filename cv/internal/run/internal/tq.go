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

package internal

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"
)

// PokeInterval is target frequency of executions of PokeRunTask.
//
// See Dispatch() for details.
const PokeInterval = time.Second

// PokeRunTaskRef is used by RunManager implementation to add its handler.
var PokeRunTaskRef tq.TaskClassRef

func init() {
	PokeRunTaskRef = tq.RegisterTaskClass(tq.TaskClass{
		ID:        "poke-run-task",
		Prototype: &PokeRunTask{},
		Queue:     "manage-run",
	})

	tq.RegisterTaskClass(tq.TaskClass{
		ID:        "kick-poke-run-task",
		Prototype: &KickPokeRunTask{},
		Queue:     "manage-run",
		Quiet:     true,
		Handler: func(ctx context.Context, payload proto.Message) error {
			task := payload.(*KickPokeRunTask)
			var eta time.Time
			if t := task.GetEta(); t != nil {
				eta = t.AsTime()
			}
			return Dispatch(ctx, task.GetRunId(), eta)
		},
	})
}

// Dispatch ensures invocation of RunManager via PokeRunTask.
//
// RunManager will be invoked at approximately no earlier than both:
//  * eta time (if given)
//  * next possible.
func Dispatch(ctx context.Context, runID string, eta time.Time) error {
	if datastore.CurrentTransaction(ctx) != nil {
		payload := &KickPokeRunTask{RunId: runID}
		if !eta.IsZero() {
			payload.Eta = timestamppb.New(eta)
		}
		return tq.AddTask(ctx, &tq.Task{
			DeduplicationKey: "", // not allowed in a transaction
			Payload:          payload,
		})
	}

	// If actual local clock is more than `clockDrift` behind, the "next" computed
	// PokeRunTask moment might be already executing, meaning task dedup will
	// ensure no new task will be scheduled AND the already executing run
	// might not have read the Event that was just written.
	// Thus, for safety, this should be large, however, will also leads to higher
	// latency of event processing of non-busy RunManager.
	// TODO(tandrii/yiwzhang): this can be reduced significantly once safety
	// "ping" events are originated from Config import cron tasks.
	const clockDrift = 100 * time.Millisecond
	now := clock.Now(ctx).Add(clockDrift) // Use the worst possible time.
	if eta.IsZero() || eta.Before(now) {
		eta = now
	}
	eta = eta.Truncate(PokeInterval).Add(PokeInterval)
	return tq.AddTask(ctx, &tq.Task{
		DeduplicationKey: fmt.Sprintf("%s\n%d", runID, eta.UnixNano()),
		ETA:              eta,
		Payload:          &PokeRunTask{RunId: runID},
	})
}
