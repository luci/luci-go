// Copyright 2021 The LUCI Authors.
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

package handler

import (
	"context"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"

	migrationpb "go.chromium.org/luci/cv/api/migration"
	"go.chromium.org/luci/cv/internal/migration"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/impl/state"
	"go.chromium.org/luci/cv/internal/tree"
)

// OnCQDVerificationCompleted implements Handler interface.
func (impl *Impl) OnCQDVerificationCompleted(ctx context.Context, rs *state.RunState) (*Result, error) {
	switch status := rs.Run.Status; {
	case run.IsEnded(status):
		logging.Debugf(ctx, "Received CQDVerificationCompleted event after Run is ended")
		return &Result{State: rs}, nil
	case status != run.Status_RUNNING:
		return nil, errors.Reason("expected RUNNING status, got %s", status).Err()
	}

	rid := rs.Run.ID
	vr := migration.VerifiedCQDRun{ID: rid}
	switch err := datastore.Get(ctx, &vr); {
	case err == datastore.ErrNoSuchEntity:
		return nil, errors.New("received CQDVerificationCompleted event but VerifiedRun entity doesn't exist")
	case err != nil:
		return nil, errors.Annotate(err, "failed to load VerifiedRun").Tag(transient.Tag).Err()
	}

	rs = rs.ShallowCopy()
	switch vr.Payload.Action {
	case migrationpb.ReportVerifiedRunRequest_ACTION_SUBMIT:
		rs.Run.Status = run.Status_WAITING_FOR_SUBMISSION
		rs.Run.Submission = &run.Submission{}
		var err error
		if rs.Run.Submission.Cls, err = orderCLIDsInSubmissionOrder(ctx, rs.Run.CLs, rs.Run.ID, rs.Run.Submission); err != nil {
			return nil, err
		}
		switch treeOpen, err := checkTreeOpen(ctx, rs); {
		case err != nil:
			return nil, err
		case !treeOpen:
			if err := run.PokeAfter(ctx, rid, 1*time.Minute); err != nil {
				// Tree is closed, revisit after 1 minute.
				return nil, err
			}
			return &Result{State: rs}, nil
		}

		switch waitlisted, err := acquireSubmitQueue(ctx, rs); {
		case err != nil:
			return nil, err
		case !waitlisted:
			return impl.OnReadyForSubmission(ctx, rs)
		default:
			// This Run expects to be notified with a ReadyForSubmission
			// event sent from submit queue once its turn is coming.
			return &Result{State: rs}, nil
		}
	case migrationpb.ReportVerifiedRunRequest_ACTION_DRY_RUN_OK:
		return nil, errors.New("not implemented")
	case migrationpb.ReportVerifiedRunRequest_ACTION_FAIL:
		return nil, errors.New("not implemented")
	default:
		return nil, errors.Reason("unknown action %s", vr.Payload.Action).Err()
	}
}

func checkTreeOpen(ctx context.Context, rs *state.RunState) (bool, error) {
	cg, err := rs.LoadConfigGroup(ctx)
	if err != nil {
		return false, err
	}
	treeOpen := true
	if treeURL := cg.Content.GetVerifiers().GetTreeStatus().GetUrl(); treeURL != "" {
		status, err := tree.FetchLatest(ctx, treeURL)
		if err != nil {
			return false, err
		}
		treeOpen = status.State == tree.Open || status.State == tree.Throttled
	}
	rs.Run.Submission.TreeOpen = treeOpen
	rs.Run.Submission.LastTreeCheckTime = timestamppb.New(clock.Now(ctx).UTC())
	return treeOpen, nil
}
