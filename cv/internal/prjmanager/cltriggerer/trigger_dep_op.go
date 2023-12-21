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

package cltriggerer

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/grpc/grpcutil"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/common/lease"
	"go.chromium.org/luci/cv/internal/gerrit"
	"go.chromium.org/luci/cv/internal/gerrit/trigger"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
	"go.chromium.org/luci/cv/internal/run"
)

type triggerDepResult struct {
	voteDone bool
	lastErr  error
}

type triggerDepOp struct {
	depCLID     int64
	originCLURL string
	trigger     *run.Trigger

	result     triggerDepResult
	isCanceled *atomic.Bool
}

func makeTriggerDepOps(originCLURL string, payload *prjpb.TriggeringCLDeps, isCanceled *atomic.Bool) []*triggerDepOp {
	deps := payload.GetDepClids()
	if len(deps) == 0 {
		return nil
	}
	ret := make([]*triggerDepOp, len(deps))
	for i, dep := range deps {
		ret[i] = &triggerDepOp{
			depCLID:     dep,
			originCLURL: originCLURL,
			trigger:     payload.GetTrigger(),
			isCanceled:  isCanceled,
		}
	}
	return ret
}

func (op *triggerDepOp) isSucceeded() bool {
	return op != nil && op.result.voteDone
}

func (op *triggerDepOp) isPermanentlyFailed() bool {
	if op.isSucceeded() {
		return false
	}
	switch err := errors.Unwrap(op.result.lastErr); err {
	case nil, context.Canceled, context.DeadlineExceeded:
		return false
	default:
		return !transient.Tag.In(err)
	}
}

// getCLError() returns changelist.CLError_TriggerDeps for the permanent error.
//
// Panic if the lastErr is not a permanent error.
func (op *triggerDepOp) getCLError() *changelist.CLError_TriggerDeps {
	if !op.isPermanentlyFailed() {
		panic(fmt.Errorf("FIXME: triggerDepOp.getCLError() called for non-permanent error"))
	}

	failure := &changelist.CLError_TriggerDeps{}
	switch grpcutil.Code(op.result.lastErr) {
	case codes.OK:
		panic(fmt.Errorf("FIXME: triggerDepOp.result.lastErr with codes.OK"))
	case codes.PermissionDenied:
		failure.PermissionDenied = append(failure.PermissionDenied,
			&changelist.CLError_TriggerDeps_PermissionDenied{
				Clid:  op.depCLID,
				Email: op.trigger.GetEmail(),
			},
		)
	case codes.NotFound:
		failure.NotFound = append(failure.NotFound, op.depCLID)
	default:
		failure.InternalGerritError = append(failure.InternalGerritError, op.depCLID)
	}
	return failure
}

func isAlreadyVoted(ctx context.Context, depCL *changelist.CL) bool {
	// Skip voting if the CL already have a CQ vote.
	switch mode := findCQTriggerMode(depCL); mode {
	case "":
	case string(run.FullRun):
		logging.Infof(ctx, "the CL is voted already; skip triggering")
		return true
	default:
		logging.Infof(ctx, "the CL is voted for %q; overriding", mode)
	}
	return false
}

func (op *triggerDepOp) execute(ctx context.Context, gFactory gerrit.Factory, luciPrj string, clm *changelist.Mutator, clu clUpdater) error {
	if op.isCanceled.Load() {
		return nil
	}
	// Lease the CL to prevent other unexpected CL updates, during the vote
	// process.
	ctx, leaseClose, lErr := lease.ApplyOnCL(ctx, common.CLID(op.depCLID), 2*time.Minute, "triggerDepOp")
	if lErr != nil {
		return lErr
	}
	defer leaseClose()

	depCL := &changelist.CL{ID: common.CLID(op.depCLID)}
	if err := changelist.LoadCLs(ctx, []*changelist.CL{depCL}); err != nil {
		return err
	}
	// if the snapshot has the vote and fresh already, skip other operations.
	if isAlreadyVoted(ctx, depCL) && depCL.Snapshot.GetOutdated() == nil {
		op.result.voteDone = true
		return nil
	}
	if err := op.vote(ctx, gFactory, luciPrj, depCL); err != nil {
		return errors.Annotate(err, "op.vote").Err()
	}
	return errors.Annotate(op.markOutdated(ctx, luciPrj, clm, clu, depCL), "triggerDepOp.markOutdated").Err()
}

func processGerritErr(ctx context.Context, err error) error {
	switch grpcutil.Code(err) {
	case codes.OK:
		return nil
	case codes.PermissionDenied:
		return err
	case codes.NotFound:
		// This is known to happen on new CLs or on recently created
		// revisions.
		return grpcutil.NotFoundTag.Apply(gerrit.ErrStaleData)
	default:
		return gerrit.UnhandledError(ctx, err, "Gerrit.SetReview")
	}
}

func (op *triggerDepOp) makeSetReviewRequest(depCL *changelist.CL) *gerritpb.SetReviewRequest {
	mode := op.trigger.GetMode()
	return &gerritpb.SetReviewRequest{
		Project:    depCL.Snapshot.GetGerrit().GetInfo().GetProject(),
		Number:     depCL.Snapshot.GetGerrit().GetInfo().GetNumber(),
		RevisionId: "current",
		Labels: map[string]int32{
			trigger.CQLabelName: trigger.CQVoteByMode(run.Mode(op.trigger.GetMode())),
		},
		// The author will be notified by the run start message, anyways.
		Notify:     gerritpb.Notify_NOTIFY_NONE,
		OnBehalfOf: op.trigger.GetGerritAccountId(),
		Message: fmt.Sprintf(
			"Triggering %s, because %s is triggered on %s, which depends on this CL",
			mode, mode, op.originCLURL),
		Tag: gerrit.Tag("trigger-dep-cl", ""),
	}
}

func (op *triggerDepOp) vote(ctx context.Context, gFactory gerrit.Factory, luciPrj string, depCL *changelist.CL) error {
	if op.result.voteDone {
		return nil
	}
	gc, err := gFactory.MakeClient(ctx, depCL.Snapshot.GetGerrit().GetHost(), luciPrj)
	if err != nil {
		return errors.Annotate(err, "gFactory.MakeClient").Err()
	}
	voteReq := op.makeSetReviewRequest(depCL)
	vErr := gFactory.MakeMirrorIterator(ctx).RetryIfStale(func(opt grpc.CallOption) error {
		if op.isCanceled.Load() {
			return nil
		}
		_, err := gc.SetReview(ctx, voteReq, opt)
		op.result.lastErr = processGerritErr(ctx, err)
		if op.result.lastErr != nil {
			return op.result.lastErr
		}
		return nil
	})
	op.result.voteDone = vErr == nil
	return vErr
}

func (op *triggerDepOp) markOutdated(ctx context.Context, luciPrj string, clm *changelist.Mutator, clu clUpdater, depCL *changelist.CL) error {
	var isOutdated bool
	_, err := clm.Update(ctx, luciPrj, depCL.ID, func(cl *changelist.CL) error {
		if cl.Snapshot == nil || cl.Snapshot.GetOutdated() != nil {
			return changelist.ErrStopMutation // noop
		}
		isOutdated = true
		cl.Snapshot.Outdated = &changelist.Snapshot_Outdated{}
		return nil
	})
	switch {
	case err != nil:
		return errors.Annotate(err, "CLMutator.Update").Err()
	case isOutdated:
		// TODO(crbug.com/1284393): use Gerrit's consistency-on-demand.
		return clu.Schedule(ctx, &changelist.UpdateCLTask{
			LuciProject: luciPrj,
			ExternalId:  string(depCL.ExternalID),
			Id:          int64(depCL.ID),
			Requester:   changelist.UpdateCLTask_DEP_CL_TRIGGERER,
		})
	default:
		return nil
	}
}

func findCQTriggerMode(cl *changelist.CL) string {
	trs := trigger.Find(&trigger.FindInput{
		ChangeInfo: cl.Snapshot.GetGerrit().GetInfo(),
	})
	if trs == nil {
		return ""
	}
	return trs.CqVoteTrigger.GetMode()
}
