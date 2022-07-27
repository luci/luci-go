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

package execute

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"

	"go.chromium.org/luci/cv/internal/tryjob"
)

func (e *Executor) logRequirementChanged(ctx context.Context) {
	e.log(&tryjob.ExecutionLogEntry{
		Time: timestamppb.New(clock.Now(ctx).UTC()),
		Kind: &tryjob.ExecutionLogEntry_RequirementChanged_{
			RequirementChanged: &tryjob.ExecutionLogEntry_RequirementChanged{},
		},
	})
}

func (e *Executor) logTryjobDiscarded(ctx context.Context, def *tryjob.Definition, exec *tryjob.ExecutionState_Execution, reason string) {
	latestAttempt := tryjob.LatestAttempt(exec)
	if latestAttempt != nil {
		e.log(&tryjob.ExecutionLogEntry{
			Time: timestamppb.New(clock.Now(ctx).UTC()),
			Kind: &tryjob.ExecutionLogEntry_TryjobDiscarded_{
				TryjobDiscarded: &tryjob.ExecutionLogEntry_TryjobDiscarded{
					Snapshot: makeLogTryjobSnapshotFromAttempt(def, latestAttempt),
					Reason:   reason,
				},
			},
		})
	}
}

func (e *Executor) logRetryDenied(ctx context.Context, execState *tryjob.ExecutionState, failedIndices []int, reason string) {
	snapshots := make([]*tryjob.ExecutionLogEntry_TryjobSnapshot, len(failedIndices))
	for i, idx := range failedIndices {
		snapshots[i] = makeLogTryjobSnapshotFromAttempt(
			execState.GetRequirement().GetDefinitions()[idx],
			tryjob.LatestAttempt(execState.GetExecutions()[idx]))
	}
	e.log(&tryjob.ExecutionLogEntry{
		Time: timestamppb.New(clock.Now(ctx).UTC()),
		Kind: &tryjob.ExecutionLogEntry_RetryDenied_{
			RetryDenied: &tryjob.ExecutionLogEntry_RetryDenied{
				Tryjobs: snapshots,
				Reason:  reason,
			},
		},
	})
}

// log adds a new execution log entry.
func (e *Executor) log(entry *tryjob.ExecutionLogEntry) {
	if entry.GetTime() == nil {
		panic(fmt.Errorf("log entry must provide time; got %s", entry))
	}
	// add the entry at the end first and then move it to the right location to
	// ensure logEntries are ordered from oldest to newest.
	e.logEntries = append(e.logEntries, entry)
	for i := len(e.logEntries) - 1; i > 0; i-- {
		if !e.logEntries[i].GetTime().AsTime().Before(e.logEntries[i-1].GetTime().AsTime()) {
			return // found the right spot.
		}
		e.logEntries[i], e.logEntries[i-1] = e.logEntries[i-1], e.logEntries[i]
	}
}
