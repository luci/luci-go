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
	"strings"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/buildbucket/appengine/internal/turboci"
	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

// writeStageAttemptOnBuildCompletion updates a stage attempt to TurboCI when
// its underlying build completes.
//
// Returns TQ-annotated errors, i.e. any error **not** tagged as tq.Fatal will
// result in an retry.
func writeStageAttemptOnBuildCompletion(ctx context.Context, bID int64) error {
	// Get the build and its infra, if it is already available.
	bld := &model.Build{ID: bID}
	infra := &model.BuildInfra{Build: datastore.KeyForObj(ctx, bld)}
	if err := datastore.Get(ctx, bld, infra); err != nil {
		merr, ok := err.(errors.MultiError)
		if !ok || len(merr) != 2 {
			return transient.Tag.Apply(errors.Fmt("error fetching build %d: %w", bID, err))
		}
		for _, err := range merr {
			if err != nil && err != datastore.ErrNoSuchEntity {
				return transient.Tag.Apply(errors.Fmt("error fetching build %d: %w", bID, err))
			}
		}
		if merr[0] == datastore.ErrNoSuchEntity {
			return tq.Fatal.Apply(errors.Fmt("build %d not found", bID))
		}
	}
	bld.Proto.Infra = infra.Proto

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
		return transient.Tag.Apply(err)
	}
	cl := &turboci.Client{
		Creds: creds,
		Token: bld.StageAttemptToken,
		Build: bld.Proto,
	}

	switch bld.Proto.Status {
	case pb.Status_SUCCESS, pb.Status_FAILURE:
		return cl.CompleteAttempt(ctx)
	case pb.Status_CANCELED:
		// See CancelStage where "turboci" is set as the canceler.
		var msg string
		if bld.Proto.CanceledBy != "turboci" {
			msg = "Cancelled via Buildbucket API"
		}
		return cl.AbandonAttempt(ctx, msg)
	case pb.Status_INFRA_FAILURE:
		return cl.FailAttempt(ctx, attemptFailure(bld.Proto))
	default:
		return tq.Fatal.Apply(errors.Fmt("invalid status %s", bld.Proto.Status))
	}
}

// attemptFailure converts a failed Build to AttemptFailure for Turbo CI.
func attemptFailure(bp *pb.Build) *turboci.AttemptFailure {
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

	return &turboci.AttemptFailure{
		Err:     errors.New(errMsg),
		Details: bp.GetStatusDetails(),
	}
}
