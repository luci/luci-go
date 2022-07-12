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

func (e *Executor) logTryjobEnded(ctx context.Context, def *tryjob.Definition, tryjobID int64) {
	e.log(&tryjob.ExecutionLogEntry{
		// TODO(yiwzhang): to be more precise, record end time in the backend
		// system for tryjob and use that time instead.
		Time: timestamppb.New(clock.Now(ctx).UTC()),
		Kind: &tryjob.ExecutionLogEntry_TryjobEnded_{
			TryjobEnded: &tryjob.ExecutionLogEntry_TryjobEnded{
				Definition: def,
				TryjobId:   tryjobID,
			},
		},
	})
}

func (e *Executor) logTryjobDiscarded(ctx context.Context, def *tryjob.Definition, exec *tryjob.ExecutionState_Execution, reason string) {
	var latestAttempt *tryjob.ExecutionState_Execution_Attempt
	if len(exec.GetAttempts()) > 0 {
		latestAttempt = exec.GetAttempts()[len(exec.GetAttempts())-1]
	}
	e.log(&tryjob.ExecutionLogEntry{
		Time: timestamppb.New(clock.Now(ctx).UTC()),
		Kind: &tryjob.ExecutionLogEntry_TryjobDiscarded_{
			TryjobDiscarded: &tryjob.ExecutionLogEntry_TryjobDiscarded{
				Definition: def,
				TryjobId:   latestAttempt.GetTryjobId(),
				ExternalId: latestAttempt.GetExternalId(),
				Reason:     reason,
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
