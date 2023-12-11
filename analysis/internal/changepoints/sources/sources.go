// Copyright 2023 The LUCI Authors.
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

// Package sources handles sources information.
package sources

import (
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"

	controlpb "go.chromium.org/luci/analysis/internal/ingestion/control/proto"
	analysispb "go.chromium.org/luci/analysis/proto/v1"
)

// FromUnsubmittedCode Return true if sources is from unsubmitted code, i.e.
// from try run that did not result in submitted code.
func FromUnsubmittedCode(sources *rdbpb.Sources, presubmit *controlpb.PresubmitResult) bool {
	hasCL := len(sources.GetChangelists()) > 0
	submittedPresubmit := presubmit != nil &&
		presubmit.Status == analysispb.PresubmitRunStatus_PRESUBMIT_RUN_STATUS_SUCCEEDED &&
		presubmit.Mode == analysispb.PresubmitRunMode_FULL_RUN
	return hasCL && !submittedPresubmit
}

// SourcesMapHasCommitData checks if sourcesMap has commit data.
// It returns true if at least one sources in the map has commit position data.
func SourcesMapHasCommitData(sourcesMap map[string]*rdbpb.Sources) bool {
	for _, sources := range sourcesMap {
		if HasCommitData(sources) {
			return true
		}
	}
	return false
}

func HasCommitData(sources *rdbpb.Sources) bool {
	if sources.IsDirty {
		return false
	}
	commit := sources.GitilesCommit
	if commit == nil {
		return false
	}
	return commit.GetHost() != "" && commit.GetProject() != "" && commit.GetRef() != "" && commit.GetPosition() != 0
}

func CommitPosition(sources *rdbpb.Sources) int {
	return int(sources.GitilesCommit.Position)
}
