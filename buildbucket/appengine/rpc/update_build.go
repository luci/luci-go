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

package rpc

import (
	"context"
	"fmt"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	cipdCommon "go.chromium.org/luci/cipd/common"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/data/strpair"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/mask"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/buildbucket/appengine/common"
	"go.chromium.org/luci/buildbucket/appengine/internal/buildstatus"
	"go.chromium.org/luci/buildbucket/appengine/internal/metrics"
	"go.chromium.org/luci/buildbucket/appengine/model"
	"go.chromium.org/luci/buildbucket/appengine/tasks"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

var emptyStruct, _ = structpb.NewStruct(map[string]any{})

func nilifyReqBuildDetails(b *pb.Build) func() {
	origTags, origSteps, origOutProp := b.Tags, b.Steps, emptyStruct
	if b.Output != nil {
		origOutProp = b.Output.Properties
		b.Output.Properties = emptyStruct
	}
	b.Tags, b.Steps = nil, nil

	return func() {
		if b.Output != nil {
			b.Output.Properties = origOutProp
		}
		b.Tags, b.Steps = origTags, origSteps
	}
}

var (
	// updateBuildStatuses is a set of build statuses supported by UpdateBuild RPC.
	updateBuildStatuses = map[pb.Status]struct{}{
		pb.Status_STARTED: {},
		// kitchen does not actually use SUCCESS. It relies on swarming pubsub
		// handler in Buildbucket because a task may fail after recipe succeeded.
		pb.Status_SUCCESS:       {},
		pb.Status_FAILURE:       {},
		pb.Status_INFRA_FAILURE: {},
		pb.Status_CANCELED:      {},
	}

	// statusesWithStartTime is a set of step statuses that requires the step to have
	// a start_time field set.
	statusesWithStartTime = map[pb.Status]struct{}{
		pb.Status_STARTED:       {},
		pb.Status_SUCCESS:       {},
		pb.Status_FAILURE:       {},
		pb.Status_INFRA_FAILURE: {},
	}
)

// validateUpdate validates the given request.
func validateUpdate(ctx context.Context, req *pb.UpdateBuildRequest, bs *model.BuildSteps) error {
	if req.GetBuild().GetId() == 0 {
		return errors.Reason("build.id: required").Err()
	}

	buildStatus := pb.Status_STATUS_UNSPECIFIED
	hasStepsMask := false
	hasOutputMask := false
	outputSubMasks := stringset.New(len(req.UpdateMask.GetPaths()))
	for _, p := range req.UpdateMask.GetPaths() {
		switch p {
		case "build.output":
			hasOutputMask = true
		case "build.output.status":
			if _, ok := updateBuildStatuses[req.Build.Output.GetStatus()]; !ok {
				return errors.Reason("build.output.status: invalid status %s for UpdateBuild", req.Build.Output.GetStatus()).Err()
			}
			buildStatus = req.Build.Output.Status
			outputSubMasks.Add("build.output.status")
		case "build.output.status_details":
			outputSubMasks.Add("build.output.status_details")
		case "build.output.summary_markdown":
			if err := validateSummaryMarkdown(req.Build.Output.GetSummaryMarkdown()); err != nil {
				return errors.Annotate(err, "build.output.summary_markdown").Err()
			}
			outputSubMasks.Add("build.output.summary_markdown")
		case "build.output.properties":
			if err := validateOutputProperties(req.Build.Output.GetProperties()); err != nil {
				return err
			}
			outputSubMasks.Add("build.output.properties")
		case "build.output.gitiles_commit":
			if err := validateCommitWithRef(req.Build.Output.GetGitilesCommit()); err != nil {
				return errors.Annotate(err, "build.output.gitiles_commit").Err()
			}
			outputSubMasks.Add("build.output.gitiles_commit")
		case "build.status":
			if _, ok := updateBuildStatuses[req.Build.Status]; !ok {
				return errors.Reason("build.status: invalid status %s for UpdateBuild", req.Build.Status).Err()
			}
			buildStatus = req.Build.Status
		case "build.status_details":
		case "build.steps":
			hasStepsMask = true
		case "build.summary_markdown":
			if err := validateSummaryMarkdown(req.Build.SummaryMarkdown); err != nil {
				return errors.Annotate(err, "build.summary_markdown").Err()
			}
		case "build.tags":
			if err := validateTags(req.Build.Tags, TagAppend); err != nil {
				return errors.Annotate(err, "build.tags").Err()
			}
		case "build.infra.buildbucket.agent.output":
			if err := validateAgentOutput(req); err != nil {
				return errors.Annotate(err, "build.infra.buildbucket.agent.output").Err()
			}
		case "build.infra.buildbucket.agent.purposes":
			if err := validateAgentDataPurposes(ctx, req); err != nil {
				return errors.Annotate(err, "build.infra.buildbucket.agent.purposes").Err()
			}
		case "build.cancel_time":
			if req.Build.CancelTime.AsTime().After(clock.Now(ctx)) {
				return errors.Reason("build.cancel_time cannot be in the future").Err()
			}
		case "build.cancellation_markdown":
			if err := validateSummaryMarkdown(req.Build.CancellationMarkdown); err != nil {
				return errors.Annotate(err, "build.cancellation_markdown").Err()
			}
		case "build.view_url":
		default:
			return errors.Reason("unsupported path %q", p).Err()
		}
	}

	if hasStepsMask {
		if err := validateSteps(bs, req.Build.Steps, buildStatus); err != nil {
			return errors.Annotate(err, "build.steps").Err()
		}
	}

	if hasOutputMask {
		if err := validateOutput(req.Build.Output, outputSubMasks); err != nil {
			return errors.Annotate(err, "build.output").Err()
		}
	}
	return nil
}

// validateOutput validates build.Output fields if "build.output" is part of
// the update mask, while their corresponding sub paths are not.
func validateOutput(output *pb.Build_Output, subMasks stringset.Set) error {
	// output status will only be updated if "build.output.status" is explicitly
	// included in update mask, so no need to validate here.
	if output.GetSummaryMarkdown() != "" && !subMasks.Has("build.output.summary_markdown") {
		if err := validateSummaryMarkdown(output.SummaryMarkdown); err != nil {
			return errors.Annotate(err, "summary_markdown").Err()
		}
	}
	if output.GetProperties() != nil && !subMasks.Has("build.output.properties") {
		if err := validateOutputProperties(output.GetProperties()); err != nil {
			return err
		}
	}
	if output.GetGitilesCommit() != nil && !subMasks.Has("build.output.gitiles_commit") {
		if err := validateCommitWithRef(output.GetGitilesCommit()); err != nil {
			return errors.Annotate(err, "gitiles_commit").Err()
		}
	}
	return nil
}

func validateOutputProperties(properties *structpb.Struct) error {
	for k, v := range properties.AsMap() {
		if v == nil {
			return errors.Reason("build.output.properties[%q]: value is not set; if necessary, use null_value instead", k).Err()
		}
	}
	return nil
}

// validateAgentDataPurposes validates the agent.purposes of the Build.
func validateAgentDataPurposes(ctx context.Context, req *pb.UpdateBuildRequest) error {
	if req.Build.Infra.GetBuildbucket().GetAgent().GetPurposes() == nil {
		return errors.New("not set")
	}
	agent := req.Build.Infra.Buildbucket.Agent
	if len(agent.Purposes) == 0 {
		return nil
	}

	infra := &model.BuildInfra{Build: datastore.KeyForObj(ctx, &model.Build{ID: req.Build.Id})}
	if err := datastore.Get(ctx, infra); err != nil {
		return err
	}

	inputDataRef := infra.Proto.Buildbucket.Agent.Input.Data
	outputDataRef := infra.Proto.Buildbucket.Agent.Output.GetResolvedData()
	if stringset.NewFromSlice(req.UpdateMask.GetPaths()...).Has("build.infra.buildbucket.agent.output") {
		outputDataRef = agent.Output.GetResolvedData()
	}
	if outputDataRef == nil {
		outputDataRef = map[string]*pb.ResolvedDataRef{}
	}

	for path := range agent.Purposes {
		if d1, d2 := inputDataRef[path], outputDataRef[path]; d1 == nil && d2 == nil {
			return errors.Reason("Invalid path %s - not in either input or output dataRef", path).Err()
		}
	}
	return nil
}

// validateAgentOutput validates the agent output of the Build.
func validateAgentOutput(req *pb.UpdateBuildRequest) error {
	if req.Build.Infra.GetBuildbucket().GetAgent().GetOutput() == nil {
		return errors.Reason("agent output is not set while its field path appears in update_mask").Err()
	}
	output := req.Build.Infra.Buildbucket.Agent.Output
	if protoutil.IsEnded(req.Build.Status) && !protoutil.IsEnded(output.Status) {
		return errors.Reason("build is in an ended status while agent output status is not ended").Err()
	}
	for _, resolved := range output.ResolvedData {
		for _, spec := range resolved.GetCipd().GetSpecs() {
			if err := cipdCommon.ValidatePackageName(spec.Package); err != nil {
				return errors.Annotate(err, "cipd.package").Err()
			}
			if err := cipdCommon.ValidateInstanceID(spec.Version, cipdCommon.AnyHash); err != nil {
				return errors.Annotate(err, "cipd.version").Err()
			}
		}
	}
	return nil
}

// validateSteps validates the steps of the Build.
func validateSteps(bs *model.BuildSteps, steps []*pb.Step, buildStatus pb.Status) error {
	if err := bs.FromProto(steps); err != nil {
		return err
	}
	if len(bs.Bytes) > model.BuildStepsMaxBytes {
		return errors.Reason("too big to accept").Err()
	}

	seen := make(map[string]*pb.Step, len(steps))
	for i, step := range steps {
		var parent *pb.Step
		var exist bool

		if err := protoutil.ValidateStepName(step.Name); err != nil {
			return errors.Annotate(err, "step[%d].name", i).Err()
		}
		if _, exist = seen[step.Name]; exist {
			return errors.Reason("step[%d]: duplicate: %q", i, step.Name).Err()
		}
		seen[step.Name] = step

		if pn := protoutil.ParentStepName(step.Name); pn != "" {
			if parent, exist = seen[pn]; !exist {
				return errors.Reason("step[%d]: parent of %q must precede", i, step.Name).Err()
			}
		}

		if err := validateStep(step, parent, buildStatus); err != nil {
			return errors.Annotate(err, "step[%d]", i).Err()
		}
	}
	return nil
}

func validateStep(step *pb.Step, parent *pb.Step, buildStatus pb.Status) error {
	var st, et time.Time
	if step.StartTime != nil {
		st = step.StartTime.AsTime()
	}
	if step.EndTime != nil {
		et = step.EndTime.AsTime()
	}

	_, stRequired := statusesWithStartTime[step.Status]
	_, isValidStatus := pb.Status_name[int32(step.Status)]
	switch {
	// This is the case, where the status field is set with an invalid int directly
	// inside the server code.
	case !isValidStatus:
		return errors.Reason("status: invalid status %d", int32(step.Status)).Err()
	// If a client sends a request with an invalid status num, then it is coerced to
	// the zero value. Thus, if the status is STATUS_UNSPECIFIED, it's either
	// unspecified or specified with an invalid value.
	case step.Status == pb.Status_STATUS_UNSPECIFIED:
		return errors.Reason("status: is unspecified or unknown").Err()
	case step.Status == pb.Status_ENDED_MASK:
		return errors.Reason("status: must not be ENDED_MASK").Err()
	case protoutil.IsEnded(buildStatus) && !protoutil.IsEnded(step.Status):
		return errors.Reason("status: cannot be %q because the build has a terminal status %q", step.Status, buildStatus).Err()
	case stRequired && st.IsZero():
		return errors.Reason("start_time: required by status %q", step.Status).Err()
	case step.Status < pb.Status_STARTED && !st.IsZero():
		return errors.Reason("start_time: must not be specified for status %q", step.Status).Err()
	case protoutil.IsEnded(step.Status) == et.IsZero():
		return errors.Reason("end_time: must have both or neither end_time and a terminal status").Err()
	case !et.IsZero() && et.Before(st):
		return errors.Reason("end_time: is before the start_time: %q < %q", et, st).Err()
	}

	seen := stringset.New(len(step.Logs))
	for i, log := range step.Logs {
		switch {
		case log.GetName() == "":
			return errors.Reason("logs[%d].name: required", i).Err()
		case log.Url == "":
			return errors.Reason("logs[%d].url: required", i).Err()
		case log.ViewUrl == "":
			return errors.Reason("logs[%d].view_url: required", i).Err()
		case !seen.Add(log.Name):
			return errors.Reason("logs[%d].name: duplicate: %q", i, log.Name).Err()
		}
	}

	for i, tag := range step.Tags {
		switch {
		case tag.Key == "":
			return errors.Reason("tags[%d].key: required", i).Err()
		case strings.HasPrefix(tag.Key, "luci."):
			return errors.Reason("tags[%d].key: reserved prefix 'luci.'", i).Err()
		case tag.Value == "":
			return errors.Reason("tags[%d].value: required", i).Err()
		case len(tag.Key) > 256:
			return errors.Reason("tags[%d].key: len > 256", i).Err()
		case len(tag.Value) > 1024:
			return errors.Reason("tags[%d].value: len > 1024", i).Err()
		}
	}

	// NOTE: We used to validate consistency of timestamps and status between
	// parent and child. However with client-side protocols such as luciexe, the
	// parent and child steps may actually belong to separate processes on the
	// machine, and can race each other in `bbagent`.
	//
	// Additionally, there's no way to guarantee that these two processes would
	// have a consistent monotonic clock state that's shared between them (this is
	// possible, but would take a fair amount of work) and events such as Daylight
	// Savings Time shifts could lead to up to an hour of inconsistency between
	// step timestamps.

	return nil
}

// mustIncludes checks the inclusiveness of a path.
// Unlike updateMask.MustIncludes, here we treat empty updateMask as it excludes
// every path.
func mustIncludes(updateMask *mask.Mask, req *pb.UpdateBuildRequest, path string) mask.Inclusiveness {
	if len(req.UpdateMask.GetPaths()) == 0 {
		return mask.Exclude
	}
	return updateMask.MustIncludes(path)
}

func explicitlyIncludesOutputStatus(req *pb.UpdateBuildRequest) bool {
	// mustIncludes(updateMask, req, "output.status") will return mask.IncludeEntirely
	// if output is part of the mask. Explicitly checking build.output.status in
	// req.UpdateMask.GetPaths().
	for _, p := range req.UpdateMask.GetPaths() {
		if p == "build.output.status" {
			return true
		}
	}
	return false
}

func checkBuildForUpdate(updateMask *mask.Mask, req *pb.UpdateBuildRequest, build *model.Build) error {
	if protoutil.IsEnded(build.Status) || protoutil.IsEnded(build.Proto.Output.GetStatus()) {
		return appstatus.Errorf(codes.FailedPrecondition, "cannot update an ended build")
	}

	finalStatus := build.Proto.Status
	if mustIncludes(updateMask, req, "status") == mask.IncludeEntirely {
		finalStatus = req.Build.Status
	} else if explicitlyIncludesOutputStatus(req) {
		finalStatus = req.Build.Output.GetStatus()
	}

	// ensure that a SCHEDULED build does not have steps, output or agent.output.
	if finalStatus == pb.Status_SCHEDULED {
		if mustIncludes(updateMask, req, "steps") != mask.Exclude {
			return appstatus.Errorf(codes.InvalidArgument, "cannot update steps of a SCHEDULED build; either set status to non-SCHEDULED or do not update steps")
		}

		if mustIncludes(updateMask, req, "output") != mask.Exclude {
			return appstatus.Errorf(codes.InvalidArgument, "cannot update build output fields of a SCHEDULED build; either set status to non-SCHEDULED or do not update build output")
		}

		if mustIncludes(updateMask, req, "infra.buildbucket.agent.output") != mask.Exclude {
			return appstatus.Errorf(codes.InvalidArgument, "cannot update agent output of a SCHEDULED build; either set status to non-SCHEDULED or do not update agent output")
		}
	}

	return nil
}

func updateEntities(ctx context.Context, req *pb.UpdateBuildRequest, parentID int64, updateMask *mask.Mask, steps *model.BuildSteps) (*model.Build, error) {
	var b *model.Build
	var origStatus pb.Status
	// Flag for should cancel this build's children or not.
	shouldCancelChildren := false

	// Get parent at the outside of the transaction.
	// Because if a build has too many children, we may hit "concurrent transaction"
	// for all the children reading the parent within transactions concurrently.
	var parent *model.Build
	if parentID != 0 {
		parent = &model.Build{ID: parentID}
		err := datastore.Get(ctx, parent)
		switch err {
		case nil:
		case datastore.ErrNoSuchEntity:
			// if parent is not found, we should cancel this build below.
			parent = nil
		default:
			return nil, errors.Annotate(err, "failed to get parent %d", parentID).Err()
		}
	}

	txErr := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		shouldCancelChildren = false
		var err error
		b, err = common.GetBuild(ctx, req.Build.Id)
		if err != nil {
			if _, isAppStatusErr := appstatus.Get(err); isAppStatusErr {
				return err
			}
			return appstatus.Errorf(codes.Internal, "failed to get build %d: %s", req.Build.Id, err)
		}
		if err = checkBuildForUpdate(updateMask, req, b); err != nil {
			return err
		}
		toSave := []any{b}
		var toSaveOutputProperties *model.BuildOutputProperties
		bk := datastore.KeyForObj(ctx, b)

		// output.properties
		if mustIncludes(updateMask, req, "output.properties") == mask.IncludeEntirely {
			var prop *structpb.Struct
			if req.Build.Output.GetProperties() != nil {
				prop = req.Build.Output.Properties
			}
			toSaveOutputProperties = &model.BuildOutputProperties{
				Build: bk,
				Proto: prop,
			}
		}

		now := clock.Now(ctx)

		// Always set UpdateTime.
		b.Proto.UpdateTime = timestamppb.New(now)

		// merge the tags of the build entity with the request.
		if len(req.Build.GetTags()) > 0 && mustIncludes(updateMask, req, "tags") == mask.IncludeEntirely {
			tags := stringset.NewFromSlice(b.Tags...)
			for _, tag := range req.Build.GetTags() {
				tags.Add(strpair.Format(tag.Key, tag.Value))
			}
			b.Tags = tags.ToSortedSlice()
		}

		// clear the request fields stored in other entities/fields.
		//
		// TODO(ddoman): if crbug.com/1154557 is done, remove nilifyReqBuildDetails().
		// Instead, remove the field paths from the mask and merge the protos with the mask.
		defer nilifyReqBuildDetails(req.Build)()
		origStatus = b.Proto.Status

		if mustIncludes(updateMask, req, "status") == mask.IncludeEntirely {
			if req.Build.Status == pb.Status_STARTED {
				logging.Debugf(ctx, "UpdateBuild is used to start build %d", b.ID)
			}
			statusUpdater := buildstatus.Updater{
				Build:       b,
				BuildStatus: &buildstatus.StatusWithDetails{Status: req.Build.Status},
				UpdateTime:  now,
				PostProcess: tasks.SendOnBuildStatusChange,
			}
			bs, err := statusUpdater.Do(ctx)
			if err != nil {
				return errors.Annotate(err, "updating build status").Err()
			}
			if bs != nil {
				toSave = append(toSave, bs)
			}
		} else if explicitlyIncludesOutputStatus(req) {
			if req.Build.Output.GetStatus() == pb.Status_STARTED {
				logging.Debugf(ctx, "UpdateBuild is used to start build %d", b.ID)
			}
			statusUpdater := buildstatus.Updater{
				Build:        b,
				OutputStatus: &buildstatus.StatusWithDetails{Status: req.Build.Output.GetStatus()},
				UpdateTime:   now,
				PostProcess:  tasks.SendOnBuildStatusChange,
			}
			bs, err := statusUpdater.Do(ctx)
			if err != nil {
				return errors.Annotate(err, "updating build status and output.status").Err()
			}
			if bs != nil {
				toSave = append(toSave, bs)
			}
		}

		// During build status transition, bbagent will try to update Build.Status
		// and Build.Output.Status at the same time, while only Build.Status takes
		// effect.
		//
		// Log the cases that Build.Status and Build.Output.Status are different so
		// we can fix them.
		//
		// TODO(crbug.com/1450399): Remove the log after build status refinement
		// is completed.
		if mustIncludes(updateMask, req, "status") == mask.IncludeEntirely && explicitlyIncludesOutputStatus(req) && req.Build.Status != req.Build.Output.GetStatus() {
			logging.Debugf(ctx, "Update Build %d to Status %s, while update Build.Output.Status to %s", req.Build.Id, req.Build.Status, req.Build.Output.GetStatus())
		}

		// Reset req.Build.Output.Status if the request does not intend to update
		// Build.Output.Status.
		if !explicitlyIncludesOutputStatus(req) && req.Build.GetOutput() != nil && req.Build.Output.Status != b.Proto.Output.GetStatus() {
			req.Build.Output.Status = b.Proto.Output.GetStatus()
		}
		if err := updateMask.Merge(req.Build, b.Proto); err != nil {
			return errors.Annotate(err, "attempting to merge masked build").Err()
		}
		if b.Proto.Output != nil {
			b.Proto.Output.Properties = nil
		}
		isEndedStatus := protoutil.IsEnded(b.Proto.Status)
		switch {
		case origStatus == b.Proto.Status:
		case b.Proto.Status == pb.Status_STARTED:
		case isEndedStatus:
			shouldCancelChildren = true
		case len(req.UpdateMask.GetPaths()) == 0:
			// empty request
		default:
			// var `updateBuildStatuses` should contain only STARTED and terminal
			// statuses. If this happens, it must be a bug.
			panic(fmt.Sprintf("invalid status %q for UpdateBuild", b.Proto.Status))
		}

		// Check cancel signal from backend, e.g. user kills the swarming task.
		if mustIncludes(updateMask, req, "cancel_time") == mask.IncludeEntirely {
			shouldCancelChildren = true
			b.Proto.CanceledBy = "backend"

			if err := tasks.ScheduleCancelBuildTask(ctx, b.ID, b.Proto.GracePeriod.AsDuration()); err != nil {
				return appstatus.Errorf(codes.Internal, "failed to schedule CancelBuildTask for build %d: %s", b.ID, err)
			}
		} else if !isEndedStatus && b.Proto.CancelTime == nil && parentID != 0 && !b.Proto.CanOutliveParent {
			// Check parent.
			if parent == nil || protoutil.IsEnded(parent.Status) {
				// Start the cancel process.
				b.Proto.CancelTime = timestamppb.New(now)
				// Buildbucket internal logic decides to cancel this build, so set
				// CanceledBy as "buildbucket".
				b.Proto.CanceledBy = "buildbucket"
				if parent == nil {
					b.Proto.CancellationMarkdown = fmt.Sprintf("canceled because its parent %d is missing", parentID)
				} else {
					b.Proto.CancellationMarkdown = fmt.Sprintf("canceled because its parent %d has terminated", parentID)
				}
				if err := tasks.ScheduleCancelBuildTask(ctx, b.ID, b.Proto.GracePeriod.AsDuration()); err != nil {
					return appstatus.Errorf(codes.Internal, "failed to schedule CancelBuildTask for build %d: %s", b.ID, err)
				}
				shouldCancelChildren = true
			}
		}

		// steps
		if mustIncludes(updateMask, req, "steps") == mask.IncludeEntirely {
			steps.Build = bk
			toSave = append(toSave, steps)
		} else if isEndedStatus {
			existingSteps := &model.BuildSteps{Build: bk}
			// If the build has no steps, ignore the ErrNoSuchEntity error.
			// CancelIncomplete will return false and existingSteps will be skipped.
			if err := model.GetIgnoreMissing(ctx, existingSteps); err != nil {
				return err
			}
			switch changed, err := existingSteps.CancelIncomplete(ctx, b.Proto.EndTime); {
			case err != nil:
				return err
			case changed:
				toSave = append(toSave, existingSteps)
			}
		}

		// infra.buildbucket.agent
		if mustIncludes(updateMask, req, "infra.buildbucket.agent") == mask.IncludePartially {
			infra := &model.BuildInfra{
				Build: bk,
			}
			if err := datastore.Get(ctx, infra); err != nil {
				return err
			}
			if mustIncludes(updateMask, req, "infra.buildbucket.agent.output") == mask.IncludeEntirely {
				infra.Proto.Buildbucket.Agent.Output = req.Build.Infra.Buildbucket.Agent.Output
			}
			if mustIncludes(updateMask, req, "infra.buildbucket.agent.purposes") == mask.IncludeEntirely {
				infra.Proto.Buildbucket.Agent.Purposes = req.Build.Infra.Buildbucket.Agent.Purposes
			}
			toSave = append(toSave, infra)
		}

		if toSaveOutputProperties != nil {
			if err := toSaveOutputProperties.Put(ctx); err != nil {
				return errors.Annotate(err, "failed to put BuildOutputProperties").Err()
			}
		}
		return datastore.Put(ctx, toSave)
	}, nil)

	if txErr != nil {
		return b, txErr
	}

	// send pubsub notifications and update metrics.
	switch {
	case origStatus == b.Status: // skip, if no status changed.
	case protoutil.IsEnded(b.Status):
		logging.Infof(ctx, "Build %d: completed by %q with status %q", b.ID, auth.CurrentIdentity(ctx), b.Status)
		metrics.BuildCompleted(ctx, b)
	default:
		logging.Infof(ctx, "Build %d: started", b.ID)
		metrics.BuildStarted(ctx, b)
	}

	// Cancel children.
	if shouldCancelChildren {
		if err := tasks.CancelChildren(ctx, b.ID); err != nil {
			// Failures of canceling children should not block updating parent.
			logging.Debugf(ctx, "failed to cancel children of %d: %s", b.ID, err)
		}
	}

	return b, nil
}

// UpdateBuild handles a request to update a build. Implements pb.UpdateBuild.
func (*Builds) UpdateBuild(ctx context.Context, req *pb.UpdateBuildRequest) (*pb.Build, error) {
	_, err := validateToken(ctx, req.Build.Id, pb.TokenBody_BUILD)
	if err != nil {
		return nil, err
	}
	logging.Infof(ctx, "Received an UpdateBuild request for build %d", req.GetBuild().GetId())

	var bs model.BuildSteps
	if err := validateUpdate(ctx, req, &bs); err != nil {
		return nil, appstatus.Errorf(codes.InvalidArgument, "%s", err)
	}
	um, err := mask.FromFieldMask(req.UpdateMask, req, false, true)
	if err != nil {
		return nil, appstatus.Errorf(codes.InvalidArgument, "update_mask: %s", err)
	}
	updateMask := um.MustSubmask("build")

	readMask, err := model.NewBuildMask("", req.Fields, req.Mask)
	if err != nil {
		return nil, appstatus.BadRequest(errors.Annotate(err, "invalid mask").Err())
	}

	build, err := common.GetBuild(ctx, req.Build.Id)
	if err != nil {
		return nil, err
	}

	// pre-check if the build can be updated before updating it with a transaction.
	if err := checkBuildForUpdate(updateMask, req, build); err != nil {
		return nil, err
	}

	build, err = updateEntities(ctx, req, build.GetParentID(), updateMask, &bs)
	if err != nil {
		if _, isAppStatusErr := appstatus.Get(err); isAppStatusErr {
			return nil, err
		}
		return nil, appstatus.Errorf(codes.Internal, "failed to update the build entity: %s", err)
	}
	// We don't need to redact the build details here, because this can only be called
	// by the specific machine that has the update token for this build.
	return build.ToProto(ctx, readMask, nil)
}
