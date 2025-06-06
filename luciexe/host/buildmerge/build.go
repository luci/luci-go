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
	"google.golang.org/protobuf/types/known/structpb"
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
	// TODO(crbug.com/1450399): Only update build.Output.Status
	// after recipe_engine change is fully rolled out.
	build.Status = bbpb.Status_INFRA_FAILURE
	if build.Output == nil {
		build.Output = &bbpb.Build_Output{}
	}
	build.Output.Status = bbpb.Status_INFRA_FAILURE
	build.Output.SummaryMarkdown = build.SummaryMarkdown
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
	if !protoutil.IsEnded(build.Output.GetStatus()) {
		setErrorOnBuild(build, errors.Fmt("Expected a terminal build status, got %s, while top level status is %s.",
			build.Output.GetStatus(), build.Status))
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
	step.Status = build.Output.GetStatus()
	// TODO(crbug.com/1450399): remove after recipe_engine change is fully rolled out.
	if step.Status != build.Status {
		step.Status = build.Status
	}
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

func getStructIn(s *structpb.Struct, path []string) *structpb.Struct {
	for _, tok := range path {
		deeperS := s.Fields[tok].GetStructValue()
		if deeperS == nil {
			if s.Fields == nil {
				s.Fields = map[string]*structpb.Value{}
			}
			deeperS = &structpb.Struct{
				Fields: map[string]*structpb.Value{},
			}
			s.Fields[tok] = structpb.NewStructValue(deeperS)
		}
		s = deeperS
	}
	if s.Fields == nil {
		s.Fields = map[string]*structpb.Value{}
	}
	return s
}

// Implements MergeBuild.legacy_global_namespace and
// MergeBuild.merge_output_properties_to.
//
// Path describes the path within parent.Output.Properties to merge subBuild's
// Output.Properties.
func updateOutputProperties(parent, subBuild *bbpb.Build, path []string) {
	if len(path) == 1 && path[0] == "" {
		// a.k.a. "merge to top level"
		path = nil
	}

	subOut := subBuild.GetOutput()
	if subP := subOut.GetProperties(); subP != nil {
		if parent.Output == nil {
			parent.Output = &bbpb.Build_Output{}
		}
		if parent.Output.Properties == nil {
			parent.Output.Properties = &structpb.Struct{}
		}

		target := getStructIn(parent.Output.Properties, path)
		for key, item := range subP.Fields {
			target.Fields[key] = item
		}
	}
}
