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

package buildmerge

import (
	"fmt"

	"github.com/golang/protobuf/ptypes/timestamp"
	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/errors"
)

// setErrorOnBuild modifies `build` so that it's SummaryMarkdown contains `err`
// and its Status is INFRA_FAILURE.
func setErrorOnBuild(build *bbpb.Build, err error) {
	addon := fmt.Sprintf("\n\nError in build protocol: %s", err)
	// TODO(iannucci): extract this magical 4KB constant and trimming function
	// into bbpb.
	// TODO(iannucci): 4KB is pretty small.
	if over := (len(build.SummaryMarkdown) + len(addon)) - 4093; over > 0 {
		build.SummaryMarkdown = build.SummaryMarkdown[:len(build.SummaryMarkdown)-over] + "..."
	}
	build.SummaryMarkdown += addon
	build.Status = bbpb.Status_INFRA_FAILURE
}

// processFinalBuild adjusts the final state of the build if needed.
//
// This ensures that `build` has a final `Status` code (or marks the build as in
// an error'd state).
//
// Sets EndTime and UpdateTime for the build, and EndTime for all steps (if
// they're missing one). Any incomplete steps will be marked as "CANCELED".
func processFinalBuild(now *timestamp.Timestamp, build *bbpb.Build) {
	if !protoutil.IsEnded(build.Status) {
		setErrorOnBuild(build, errors.Reason(
			"Expected a terminal build status, got %s.",
			build.Status).Err())
	}

	// Mark incomplete steps as canceled and set EndTime for all steps missing it.
	for _, s := range build.Steps {
		if !protoutil.IsEnded(s.Status) {
			s.Status = bbpb.Status_CANCELED
			if s.SummaryMarkdown != "" {
				s.SummaryMarkdown += "\n"
			}
			s.SummaryMarkdown += "step was never finalized; did the build crash?"
		}
		if s.EndTime == nil {
			s.EndTime = now
		}
	}
	build.UpdateTime = now
	build.EndTime = now
}

// updateStepFromBuild adjusts `step` (presumably a "merge step") from the given
// build; this overwrites the step's SummaryMarkdown, Status and EndTime to
// match that of `build`, and appends any Output.Logs from `build` to step.Log.
func updateStepFromBuild(step *bbpb.Step, build *bbpb.Build) {
	step.SummaryMarkdown = build.SummaryMarkdown
	step.Status = build.Status
	step.EndTime = build.EndTime
	for _, log := range build.Output.GetLogs() {
		step.Logs = append(step.Logs, log)
	}
}

// updateBaseFromUserBuild updates a 'template' base build with all the
// output-able data from `build`.
//
// In this scenario, `base` would be the input build to a task, and `build`
// would be the outputs from the running luciexe.
func updateBaseFromUserBuild(base, build *bbpb.Build) {
	if build == nil {
		return
	}
	base.SummaryMarkdown = build.SummaryMarkdown
	base.Status = build.Status
	base.StatusDetails = build.StatusDetails
	base.UpdateTime = build.UpdateTime
	base.EndTime = build.EndTime
	base.Tags = build.Tags
	base.Output = build.Output
}
