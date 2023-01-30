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

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/reflectutil"
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
//
// If this needs to make adjustments to `build.Steps`, it will make shallow
// copies of Steps, as well as the individual steps which need modification.
func processFinalBuild(now *timestamppb.Timestamp, build *bbpb.Build) {
	if !protoutil.IsEnded(build.Status) {
		setErrorOnBuild(build, errors.Reason(
			"Expected a terminal build status, got %s.",
			build.Status).Err())
	}

	// Mark incomplete steps as canceled and set EndTime for all steps missing it.
	newSteps := build.Steps
	copiedSteps := false
	for i, s := range build.Steps {
		terminalStatus := protoutil.IsEnded(s.Status)
		if terminalStatus && s.EndTime != nil {
			continue
		}

		if s.GetMergeBuild().GetFromLogdogStream() != "" {
			// It is okay merge step has a non-terminal status at this moment.
			// The final merge will make sure this merge step is updated with
			// the final state of sub build in the final build.
			continue
		}

		// maybe make a copy of the Steps slice
		if !copiedSteps {
			copiedSteps = true
			newSteps = make([]*bbpb.Step, len(build.Steps))
			copy(newSteps, build.Steps)
		}

		s = reflectutil.ShallowCopy(s).(*bbpb.Step)
		if !terminalStatus {
			s.Status = bbpb.Status_CANCELED
			if s.SummaryMarkdown != "" {
				s.SummaryMarkdown += "\n"
			}
			s.SummaryMarkdown += "step was never finalized; did the build crash?"
		}
		if s.EndTime == nil {
			s.EndTime = now
		}
		newSteps[i] = s
	}
	build.Steps = newSteps
	build.UpdateTime = now
	build.EndTime = now
}

// updateStepFromBuild adjusts `step` (presumably a "merge step") from the given
// build if the step status is not terminal; this overwrites the step's
// SummaryMarkdown, Status and EndTime to match that of `build`, and appends
// any Output.Logs from `build` to step.Log.
func updateStepFromBuild(step *bbpb.Step, build *bbpb.Build) {
	if protoutil.IsEnded(step.Status) {
		return
	}
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
//
// As a special case, Output is proto.Merge'd.
//
// NOTE: This function assumes exclusive read/write access of `base` only
// (not its subfields), and assumes read access to subfields. This function must
// therefore only write to the immediate subfields of `base`; It may READ
// some fields deeply (such as `.Output`), but will not modify any subfields
// therein.
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

	if build.Output != nil {
		var output *bbpb.Build_Output
		if base.Output != nil {
			output = proto.Clone(base.Output).(*bbpb.Build_Output)
		} else {
			output = &bbpb.Build_Output{}
		}
		proto.Merge(output, build.GetOutput())
		base.Output = output
	}
}

// Implements MergeBuild.legacy_global_namespace.
//
// Only used for legacy CrOS builders, see crbug.com/1310155.
//
// Merges:
//   - properties will be 'merged' by replacing top-level keys. If the
//     parent build and this sub-build both write in the same top-level keys
//     the sub-build value wins.
//
// The following fields COULD be merged, but aren't, because this
// functionality is legacy, and we're not trying to make it do
// anything more than is necessary:
//   - gitiles_commit will NOT be merged
//   - the sub-build's output.logs will NOT be merged.
//   - the sub-build's summary_markdown will NOT be merged.
func updateBuildFromGlobalSubBuild(parent, subBuild *bbpb.Build) {
	subOut := subBuild.GetOutput()
	if subP := subOut.GetProperties(); subP != nil {
		if parent.Output == nil {
			parent.Output = &bbpb.Build_Output{Properties: subP}
		} else if parent.Output.Properties == nil {
			parent.Output.Properties = subP
		} else {
			for key, item := range subP.GetFields() {
				parent.Output.Properties.Fields[key] = item
			}
		}
	}
}
