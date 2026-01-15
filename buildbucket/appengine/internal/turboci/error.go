// Copyright 2026 The LUCI Authors.
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

package turboci

import (
	"context"
	"fmt"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/turboci/rpc"
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"

	"go.chromium.org/luci/buildbucket/appengine/model"
	"go.chromium.org/luci/buildbucket/appengine/tasks"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

// HandleStageAttemptStatusConflict handles the *orchestratorpb.StageAttemptCurrentState
// from err's details.
func HandleStageAttemptStatusConflict(ctx context.Context, b *pb.Build, err error) error {
	sacs, sErr := rpc.StageAttemptCurrentState(err)
	if sErr != nil {
		logging.Errorf(ctx, "Failed to extract StageAttemptCurrentState from error: %s, original error: %s", sErr, err)
		return err
	}

	if sacs == nil {
		return err
	}

	switch curState := sacs.GetState(); curState {
	// RUNNING and TEARING_DOWN can only be set by the executor (Buildbucket),
	// getting these states from the error meaning another Buildbucket process
	// also touched the attempt and that is fine.
	case orchestratorpb.StageAttemptState_STAGE_ATTEMPT_STATE_RUNNING,
		orchestratorpb.StageAttemptState_STAGE_ATTEMPT_STATE_TEARING_DOWN:
		return nil
	case orchestratorpb.StageAttemptState_STAGE_ATTEMPT_STATE_CANCELLING:
		return handleStageAttemptCancelling(ctx, b.Id)
	case orchestratorpb.StageAttemptState_STAGE_ATTEMPT_STATE_INCOMPLETE,
		orchestratorpb.StageAttemptState_STAGE_ATTEMPT_STATE_COMPLETE:
		return handleStageAttemptEnded(ctx, b.Id, curState)
	default:
		// We don't expect PENDING, SCHEDULED states from the error.
		return appstatus.Errorf(codes.InvalidArgument, "status %s is not supported", curState)
	}
}

func handleStageAttemptCancelling(ctx context.Context, bID int64) error {
	needCancel := false

	err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		bld := &model.Build{ID: bID}
		switch err := datastore.Get(ctx, bld); {
		case err == datastore.ErrNoSuchEntity:
			return appstatus.Error(codes.NotFound, "build not found")
		case err != nil:
			return err
		}

		switch {
		case bld.Status == pb.Status_CANCELED:
			// Cancellation is done.
			return nil
		case bld.Status == pb.Status_STARTED && bld.Proto.CancelTime != nil:
			// Cancellation is on going.
			return nil
		case protoutil.IsEnded(bld.Status):
			// The build has ended but it was not cancelled.
			// Log the mismatch. But No need to update TurboCI - the status updator
			// that ended the build should have done it.
			logging.Debugf(ctx, "bld %d ends with status %s, while TurboCI is cancelling it.", bID, bld.Status)
			return nil
		default:
			needCancel = true
			return nil
		}
	}, nil)
	if err != nil {
		return errors.Fmt("failed to check build for handling TurboCI stage attempt state mismatch: %d: %w", bID, err)
	}

	if !needCancel {
		return nil
	}

	_, err = tasks.StartCancel(ctx, bID, "Cancelled by TurboCI")
	return err
}

func handleStageAttemptEnded(ctx context.Context, bID int64, curAttemptState orchestratorpb.StageAttemptState) error {
	endStatusMap := map[pb.Status]orchestratorpb.StageAttemptState{
		pb.Status_CANCELED:      orchestratorpb.StageAttemptState_STAGE_ATTEMPT_STATE_INCOMPLETE,
		pb.Status_INFRA_FAILURE: orchestratorpb.StageAttemptState_STAGE_ATTEMPT_STATE_INCOMPLETE,
		pb.Status_FAILURE:       orchestratorpb.StageAttemptState_STAGE_ATTEMPT_STATE_COMPLETE,
		pb.Status_SUCCESS:       orchestratorpb.StageAttemptState_STAGE_ATTEMPT_STATE_COMPLETE,
	}

	needCancel := false

	err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		bld := &model.Build{ID: bID}
		switch err := datastore.Get(ctx, bld); {
		case err == datastore.ErrNoSuchEntity:
			return appstatus.Error(codes.NotFound, "build not found")
		case err != nil:
			return err
		}

		if protoutil.IsEnded(bld.Status) {
			expectedAttemptState := endStatusMap[bld.Status]
			if curAttemptState != expectedAttemptState {
				logging.Debugf(ctx, "build %d ends with status %s, while the stageattempt ends with state %s.", bID, bld.Status, curAttemptState)
			}
			return nil
		}

		needCancel = true
		return nil
	}, nil)
	if err != nil {
		return errors.Fmt("failed to check build for handling TurboCI stage attempt state mismatch: %d: %w", bID, err)
	}

	if !needCancel {
		return nil
	}

	_, err = tasks.StartCancel(ctx, bID, fmt.Sprintf("Cancelled because TurboCI stage attempt ends with state %s", curAttemptState))
	return err
}
