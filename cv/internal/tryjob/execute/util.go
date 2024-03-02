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
	"go.chromium.org/luci/cv/internal/tryjob"
)

func makeLogTryjobSnapshot(def *tryjob.Definition, tj *tryjob.Tryjob, reused bool) *tryjob.ExecutionLogEntry_TryjobSnapshot {
	return &tryjob.ExecutionLogEntry_TryjobSnapshot{
		Definition: def,
		Id:         int64(tj.ID),
		ExternalId: string(tj.ExternalID),
		Status:     tj.Status,
		Result:     tj.Result,
		Reused:     reused,
	}
}

func makeLogTryjobSnapshotFromAttempt(def *tryjob.Definition, attempt *tryjob.ExecutionState_Execution_Attempt) *tryjob.ExecutionLogEntry_TryjobSnapshot {
	return &tryjob.ExecutionLogEntry_TryjobSnapshot{
		Definition: def,
		Id:         attempt.GetTryjobId(),
		ExternalId: attempt.GetExternalId(),
		Status:     attempt.GetStatus(),
		Result:     attempt.GetResult(),
		Reused:     attempt.GetReused(),
	}
}
