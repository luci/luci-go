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
	"time"

	"github.com/golang/protobuf/ptypes"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/data/strpair"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/mask"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/buildbucket/appengine/internal/perm"
	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

var emptyStruct, _ = structpb.NewStruct(map[string]interface{}{})

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
func validateUpdate(req *pb.UpdateBuildRequest, bs *model.BuildSteps) error {
	if req.GetBuild().GetId() == 0 {
		return errors.Reason("build.id: required").Err()
	}
	if len(req.UpdateMask.GetPaths()) == 0 {
		return errors.Reason("build.update_mask: required").Err()
	}

	buildStatus := pb.Status_STATUS_UNSPECIFIED
	hasStepsMask := false
	for _, p := range req.UpdateMask.GetPaths() {
		switch p {
		case "build.output":
			// TODO(crbug/1110990): validate properties and gitiles_commit
		case "build.output.properties":
			for k, v := range req.Build.Output.GetProperties().AsMap() {
				if v == nil {
					return errors.Reason("build.output.properties[%q]: value is not set; if necessary, use null_value instead", k).Err()
				}
			}
		case "build.output.gitiles_commit":
			if err := validateCommitWithRef(req.Build.Output.GetGitilesCommit()); err != nil {
				return errors.Annotate(err, "build.output.gitiles_commit").Err()
			}
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
		default:
			return errors.Reason("unsupported path %q", p).Err()
		}
	}

	if hasStepsMask {
		if err := validateSteps(bs, req.Build.Steps, buildStatus); err != nil {
			return errors.Annotate(err, "build.steps").Err()
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
	var err error
	if step.StartTime != nil {
		if st, err = ptypes.Timestamp(step.StartTime); err != nil {
			return errors.Annotate(err, "start_time").Err()
		}
	}
	if step.EndTime != nil {
		if et, err = ptypes.Timestamp(step.EndTime); err != nil {
			return errors.Annotate(err, "end_time").Err()
		}
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

func getBuildForUpdate(ctx context.Context, updateMask *mask.Mask, req *pb.UpdateBuildRequest) (*model.Build, error) {
	build, err := getBuild(ctx, req.Build.Id)
	if err != nil {
		if _, isAppStatusErr := appstatus.Get(err); isAppStatusErr {
			return nil, err
		}
		return nil, appstatus.Errorf(codes.Internal, "failed to get build %d: %s", req.Build.Id, err)
	}

	if protoutil.IsEnded(build.Status) {
		return nil, appstatus.Errorf(codes.FailedPrecondition, "cannot update an ended build")
	}

	finalStatus := build.Proto.Status
	if updateMask.MustIncludes("status") == mask.IncludeEntirely {
		finalStatus = req.Build.Status
	}

	// ensure that a SCHEDULED build does not have steps or output.
	if finalStatus == pb.Status_SCHEDULED {
		if updateMask.MustIncludes("steps") != mask.Exclude {
			return nil, appstatus.Errorf(codes.InvalidArgument, "cannot update steps of a SCHEDULED build; either set status to non-SCHEDULED or do not update steps")
		}

		if updateMask.MustIncludes("output") != mask.Exclude {
			return nil, appstatus.Errorf(codes.InvalidArgument, "cannot update build output fields of a SCHEDULED build; either set status to non-SCHEDULED or do not update build output")
		}
	}

	return build, nil
}

func updateEntities(ctx context.Context, req *pb.UpdateBuildRequest, updateMask *mask.Mask, steps *model.BuildSteps) error {
	var b *model.Build
	var origStatus pb.Status
	txErr := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		var err error
		b, err = getBuildForUpdate(ctx, updateMask, req)
		if err != nil {
			return err
		}
		toSave := []interface{}{b}
		bk := datastore.KeyForObj(ctx, b)

		now := timestamppb.New(clock.Now(ctx))

		// output.properties
		if updateMask.MustIncludes("output.properties") == mask.IncludeEntirely {
			prop := model.DSStruct{}
			if req.Build.Output.GetProperties() != nil {
				prop = model.DSStruct{*req.Build.Output.Properties}
			}
			toSave = append(toSave, &model.BuildOutputProperties{
				Build: bk,
				Proto: prop,
			})
		}

		// merge the tags of the build entity with the request.
		if len(req.Build.GetTags()) > 0 && updateMask.MustIncludes("tags") == mask.IncludeEntirely {
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
		updateMask.Merge(req.Build, &b.Proto)
		if b.Proto.Output != nil {
			b.Proto.Output.Properties = nil
		}
		isEndedStatus := protoutil.IsEnded(b.Proto.Status)
		switch {
		case origStatus == b.Proto.Status:
		case b.Proto.Status == pb.Status_STARTED:
			if b.Proto.StartTime == nil {
				b.Proto.StartTime = now
			}
			if err := buildStarting(ctx, b); err != nil {
				return nil
			}
		case isEndedStatus:
			b.Leasee = nil
			b.LeaseExpirationDate = time.Time{}
			b.LeaseKey = 0

			if b.Proto.EndTime == nil {
				b.Proto.EndTime = now
			}
			if err := buildCompleting(ctx, b); err != nil {
				return err
			}
		default:
			// var `updateBuildStatuses` should contain only STARTED and terminal
			// statuses. If this happens, it must be a bug.
			panic(fmt.Sprintf("invalid status %q for UpdateBuild", b.Proto.Status))
		}

		// steps
		if updateMask.MustIncludes("steps") == mask.IncludeEntirely {
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

		return datastore.Put(ctx, toSave)
	}, nil)

	// send pubsub notifications and update metrics.
	switch {
	case txErr != nil: // skip, if the txn failed.
	case origStatus == b.Status: // skip, if no status changed.
	case protoutil.IsEnded(b.Status):
		buildCompleted(ctx, b)
	default:
		buildStarted(ctx, b)
	}
	return txErr
}

// UpdateBuild handles a request to update a build. Implements pb.UpdateBuild.
func (*Builds) UpdateBuild(ctx context.Context, req *pb.UpdateBuildRequest) (*pb.Build, error) {
	switch can, err := perm.CanUpdateBuild(ctx); {
	case err != nil:
		return nil, appstatus.Errorf(codes.Internal, "failed to check membership of the updater group: %s", err)
	case !can:
		return nil, appstatus.Errorf(codes.PermissionDenied, "%q not permitted to update build", auth.CurrentIdentity(ctx))
	}
	logging.Infof(ctx, "Received an UpdateBuild request for build-%d", req.GetBuild().GetId())

	var bs model.BuildSteps
	if err := validateUpdate(req, &bs); err != nil {
		return nil, appstatus.Errorf(codes.InvalidArgument, "%s", err)
	}
	um, err := mask.FromFieldMask(req.UpdateMask, req, false, true)
	if err != nil {
		return nil, appstatus.Errorf(codes.InvalidArgument, "update_mask: %s", err)
	}
	updateMask := um.MustSubmask("build")

	// pre-check if the build can be updated before updating it with a transaction.
	b, err := getBuildForUpdate(ctx, updateMask, req)
	if err != nil {
		return nil, err
	}

	// TODO(crbug.com/1152628) - Use Cloud Secret Manager to validate build update tokens.
	if err := validateBuildToken(ctx, b); err != nil {
		return nil, err
	}

	if err := updateEntities(ctx, req, updateMask, &bs); err != nil {
		return nil, appstatus.Errorf(codes.Internal, "failed to update the build entity: %s", err)
	}

	// return an empty build. In practice, clients do not need the response, but just
	// want to provide the data.
	return &pb.Build{}, nil
}
