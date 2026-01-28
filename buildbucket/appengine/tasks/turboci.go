// Copyright 2025 The LUCI Authors.
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

package tasks

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"
	"go.chromium.org/luci/turboci/id"
	stagepb "go.chromium.org/turboci/proto/go/data/stage/v1"

	"go.chromium.org/luci/buildbucket/appengine/internal/turboci"
	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

// writeStageAttemptOnBuildCompletion updates a stage attempt to TurboCI when
// its underlying build completes.
func writeStageAttemptOnBuildCompletion(ctx context.Context, bID int64, oldStatus pb.Status) error {
	// Get build, no need to be in a transaction since the build should have
	// ended.
	// Check build status and call different turboci client code accordingly.
	bld := &model.Build{ID: bID}
	toGet := []any{bld}
	var infra *model.BuildInfra
	// The build failed to start, it's possible that TurboCI has not got the
	// details yet.
	// Also get build infra to populate details.
	if oldStatus == pb.Status_SCHEDULED {
		bk := datastore.KeyForObj(ctx, bld)
		infra = &model.BuildInfra{
			Build: bk,
		}
		toGet = append(toGet, infra)
	}

	switch err := datastore.Get(ctx, toGet...); {
	case errors.Contains(err, datastore.ErrNoSuchEntity):
		return tq.Fatal.Apply(errors.Fmt("entities for build %d not found: %w", bID, err))
	case err != nil:
		return transient.Tag.Apply(errors.Fmt("error fetching build %d: %w", bID, err))
	}

	if !protoutil.IsEnded(bld.Proto.Status) {
		// Should never happen.
		return tq.Fatal.Apply(errors.Fmt("build %d has not ended yet, current status: %s", bID, bld.Status))
	}

	if bld.StageAttemptToken == "" {
		// Should never happen.
		return tq.Fatal.Apply(errors.Fmt("build %d doesn't have a stage attempt token, cannot update TurboCI", bID))
	}

	creds, err := turboci.ProjectRPCCredentials(ctx, bld.Project)
	if err != nil {
		return err
	}
	cl := &turboci.Client{
		Creds: creds,
		Token: bld.StageAttemptToken,
	}

	bp := bld.Proto
	if infra != nil {
		bp.Infra = infra.Proto
	}

	switch bld.Status {
	case pb.Status_SUCCESS, pb.Status_FAILURE:
		err = cl.CompleteCurrentAttempt(ctx, bld.StageAttemptID)
	case pb.Status_CANCELED:
		err = cancelBuildStageAttempt(ctx, cl, bld.Proto, bld.StageAttemptID, oldStatus)
	case pb.Status_INFRA_FAILURE:
		err = failBuildStageAttemptWithDetails(ctx, cl, bld.Proto, bld.StageAttemptID, oldStatus)
	default:
		tq.Fatal.Apply(errors.Fmt("invalid status %s", bld.Status))
	}
	return nil
}

func cancelBuildStageAttempt(ctx context.Context, cl *turboci.Client, bp *pb.Build, attemptID string, oldStatus pb.Status) error {
	var details []proto.Message
	// The build failed to start, needs to populate details.
	if oldStatus == pb.Status_SCHEDULED {
		details = PopulateBuildDetails(bp)
	}

	return cl.AbortCurrentAttempt(ctx, attemptID, details...)
}

func failBuildStageAttemptWithDetails(ctx context.Context, cl *turboci.Client, bp *pb.Build, attemptID string, oldStatus pb.Status) error {
	aID, aErr := id.FromString(attemptID)
	if aErr != nil {
		logging.Errorf(ctx, "Build %d has a malformed StageAttemptID: %s", bp.Id, aErr)
		return nil
	}

	var errMsgs []string
	if bp.GetStatusDetails().GetResourceExhaustion() != nil {
		errMsgs = append(errMsgs, "Resource exhausted")
	}
	if bp.GetStatusDetails().GetTimeout() != nil {
		errMsgs = append(errMsgs, "Timed out")
	}

	var errMsg string
	if len(errMsgs) > 0 {
		errMsg = strings.Join(errMsgs, "\n")
	} else {
		errMsg = "Infra failure"
	}

	var details []proto.Message
	// The build failed to start, needs to populate details.
	if oldStatus == pb.Status_SCHEDULED {
		details = PopulateBuildDetails(bp)
	}

	return cl.FailCurrentAttempt(ctx, aID.GetStageAttempt(), &turboci.AttemptFailure{Err: errors.New(errMsg), Details: bp.GetStatusDetails()}, details...)
}

// PopulateBuildDetails populates details about a build in a WriteNodes request.
//
// Most build stage attempt should have the details when they are advanced to
// SCHEDULED. But to ensure the builds have the details at all cases (e.g. the
// stage attempt could be advanced to RUNNING directly if that WriteNodes reaches
// TurboCI before the SCHEDULED one), Buildbucket adds the same details in the
// WriteNodes to advance the attempt to RUNNING (or to INCOMPLETE if it fails to
// start the build).
func PopulateBuildDetails(bld *pb.Build) []proto.Message {
	bldDetails := &pb.BuildStageDetails{
		Result: &pb.BuildStageDetails_Id{
			Id: bld.Id,
		},
	}
	commonDetails := stagepb.CommonStageAttemptDetails_builder{
		ViewUrls: map[string]*stagepb.CommonStageAttemptDetails_UrlDetails{
			"Buildbucket": stagepb.CommonStageAttemptDetails_UrlDetails_builder{
				Url: proto.String(fmt.Sprintf("https://%s/build/%d", bld.GetInfra().GetBuildbucket().GetHostname(), bld.Id)),
			}.Build(),
		},
	}.Build()
	return []proto.Message{bldDetails, commonDetails}
}
