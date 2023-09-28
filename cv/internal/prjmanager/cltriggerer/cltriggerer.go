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
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/gerrit"
	"go.chromium.org/luci/cv/internal/gerrit/trigger"
	"go.chromium.org/luci/cv/internal/prjmanager"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
	"go.chromium.org/luci/cv/internal/run"
)

// Triggerer triggers given CLs.
type Triggerer struct {
	pmNotifier *prjmanager.Notifier
	gFactory   gerrit.Factory
}

var errOriginVote = errors.New("The origin CL no longer has CQ+2")

// New creates a Triggerer.
func New(n *prjmanager.Notifier, gf gerrit.Factory) *Triggerer {
	v := &Triggerer{
		pmNotifier: n,
		gFactory:   gf,
	}
	n.TasksBinding.TriggerProjectCLs.AttachHandler(
		func(ctx context.Context, payload proto.Message) error {
			task := payload.(*prjpb.TriggeringCLsTask)
			ctx = logging.SetFields(ctx, logging.Fields{
				"project": task.GetLuciProject(),
			})
			return common.TQifyError(ctx,
				errors.Annotate(v.trigger(ctx, task), "triggerer.trigger").Err())
		},
	)
	return v
}

// Schedule schedules a task for CQVoteTask.
func (tr *Triggerer) Schedule(ctx context.Context, t *prjpb.TriggeringCLsTask) error {
	cls := t.GetTriggeringCls()
	if len(cls) == 0 {
		return nil
	}
	var title strings.Builder
	fmt.Fprintf(&title, "%s/%s/%d-%d", t.GetLuciProject(), cls[0].GetOperationId(), len(cls), cls[0].GetClid())
	for _, cl := range cls[1:] {
		fmt.Fprintf(&title, ".%d", cl.GetClid())
	}

	return tr.pmNotifier.TasksBinding.TQDispatcher.AddTask(ctx, &tq.Task{
		Payload: t,
		Title:   title.String(),
		// Not allowed in a transaction
		DeduplicationKey: "",
	})
}

// trigger triggers the CLs.
func (tr *Triggerer) trigger(ctx context.Context, task *prjpb.TriggeringCLsTask) error {
	luciPrj := task.GetLuciProject()
	tcls := task.GetTriggeringCls()
	var lastCLErr error
	var nSuccess int
	// This procesor retries on transient failures as many times as possible
	// until the task deadline gets exceeded.
	//
	// If any request exceeds the deadline before this loop votes all the CLs,
	// it's likely that the CL has too many dep CLs to vote all within
	// MaxTriggeringCLDuration. Or, there is an outage in Gerrit or CV.
	// In any cases, this returns nil to clear the TQ message, and notify PM
	// with a list of the succeede/failed/skipped votes.
	//
	// Then, PM will re-examine all the PCL statuses, and retriage the CLs
	// to either stop the vote process and schedule a new TQ task to continue
	// the vote process.
	for _, tcl := range tcls {
		// TODO: if tcl.GetOriginClid() is 0, the task will fail.
		// It's fine now because trigger task is always enqueued with
		// origin_clid. However, if this assumption changes, then this logic
		// should be updated.
		if lastCLErr = tr.triggerCL(ctx, luciPrj, tcl); lastCLErr != nil {
			switch errors.Unwrap(lastCLErr) {
			case errOriginVote, context.DeadlineExceeded:
				// Consider these as cancellation event, not failure event.
				lastCLErr = nil
			}
			break
		}
		nSuccess++
	}

	completed := &prjpb.TriggeringCLsCompleted{}
	if nSuccess > 0 {
		completed.Succeeded = make([]*prjpb.TriggeringCLsCompleted_OpResult, nSuccess)
		for i := 0; i < nSuccess; i++ {
			completed.Succeeded[i] = &prjpb.TriggeringCLsCompleted_OpResult{
				OperationId: tcls[i].GetOperationId(),
				OriginClid:  tcls[i].GetOriginClid(),
			}
		}
	}
	skippedAt := nSuccess
	if code := grpcutil.Code(lastCLErr); code != codes.OK {
		skippedAt++
		failedAt := nSuccess
		reason := &changelist.CLError_TriggerDeps{}
		clid := tcls[failedAt].GetClid()
		switch code {
		case codes.PermissionDenied:
			reason.PermissionDenied = append(reason.PermissionDenied,
				&changelist.CLError_TriggerDeps_PermissionDenied{
					Clid:  clid,
					Email: tcls[failedAt].GetTrigger().GetEmail(),
				},
			)
		case codes.NotFound:
			reason.NotFound = append(reason.NotFound, clid)
		default:
			reason.InternalGerritError = append(reason.InternalGerritError, clid)
		}
		completed.Failed = append(completed.Failed, &prjpb.TriggeringCLsCompleted_OpResult{
			OperationId: tcls[failedAt].GetOperationId(),
			OriginClid:  tcls[failedAt].GetOriginClid(),
			Reason:      reason,
		})
	}
	if skippedAt < len(tcls) {
		completed.Skipped = make([]*prjpb.TriggeringCLsCompleted_OpResult, 0, len(tcls)-skippedAt)
		for i := skippedAt; i < len(tcls); i++ {
			completed.Skipped = append(completed.Skipped, &prjpb.TriggeringCLsCompleted_OpResult{
				OperationId: tcls[i].GetOperationId(),
				OriginClid:  tcls[i].GetOriginClid(),
			})
		}
	}
	return tr.pmNotifier.NotifyTriggeringCLsCompleted(ctx, luciPrj, completed)
}

func (tr *Triggerer) triggerCL(ctx context.Context, luciPrj string, tcl *prjpb.TriggeringCL) error {
	ctx = logging.SetFields(ctx, logging.Fields{"cl": tcl.GetClid()})
	ctx, cancel := clock.WithDeadline(ctx, tcl.GetDeadline().AsTime())
	defer cancel()

	return retry.Retry(ctx, makeTriggerCLRetryFactory(), func() error {
		cl, origin, err := loadCLs(ctx, tcl)
		if err != nil {
			return err
		}
		// CL may have already have a CQ vote. For example, it could be voted
		// manually after this task was created.
		switch mode := findCQTriggerMode(cl); mode {
		case "":
		case string(run.FullRun):
			logging.Debugf(ctx, "the CL is voted already; skip triggering")
			return nil
		default:
			logging.Infof(ctx, "the CL is voted for %q; overriding", mode)
		}
		// Check the originating CL. Maybe, it's unvoted to stop the vote chain.
		switch mode := findCQTriggerMode(origin); mode {
		case string(run.FullRun):
			// Happy path
		case "":
			logging.Infof(ctx, "the origin CL %d no longer has a CQ vote; stop voting", origin.ID)
			return errOriginVote
		default:
			// The originating CL now has CQ+1? This can only happen in
			// the following scenario.
			// - At t1, the originating CL got CQ+2 and the deps triager created
			// the TriggeringCLs.
			// - At t2, the originating CL got CQ+1.
			//
			// This should be considered as cancelling the CQ vote chain
			// process. It's OK to skip all the vote ops for the dep CLs.
			// Then, PM will retriage the originating CL, as necessary.
			logging.Infof(ctx, "the origin CL %d now has a CQ vote for %q; stop voting", mode)
			return errOriginVote
		}
		gc, err := tr.gFactory.MakeClient(ctx, cl.Snapshot.GetGerrit().GetHost(), luciPrj)
		if err != nil {
			return errors.Annotate(err, "gFactory.MakeClient").Err()
		}
		req := makeSetReviewRequest(tcl, cl, origin)
		return tr.gFactory.MakeMirrorIterator(ctx).RetryIfStale(func(opt grpc.CallOption) error {
			_, err := gc.SetReview(ctx, req, opt)
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
		})
	}, nil)
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

func makeSetReviewRequest(tcl *prjpb.TriggeringCL, cl, origin *changelist.CL) *gerritpb.SetReviewRequest {
	var msg string
	if origin != nil {
		mode := tcl.GetTrigger().GetMode()
		msg = fmt.Sprintf("Triggering %s, because %s is triggered on %s, which depends on this CL",
			mode, mode, origin.ExternalID.MustURL())
	}
	// TODO(crbug.com/1470341) - ideally, this should send two requests.
	// - one for label vote on behalf of the original triggerer.
	// - one for posting a message to indicate the reason of the vote.
	return &gerritpb.SetReviewRequest{
		Project:    cl.Snapshot.GetGerrit().GetInfo().GetProject(),
		Number:     cl.Snapshot.GetGerrit().GetInfo().GetNumber(),
		OnBehalfOf: tcl.GetTrigger().GetGerritAccountId(),
		RevisionId: "current",
		// The author will be notified by the run start message, anyways.
		Notify: gerritpb.Notify_NOTIFY_NONE,
		Labels: map[string]int32{
			trigger.CQLabelName: trigger.CQVoteByMode(run.Mode(tcl.GetTrigger().GetMode())),
		},
		Message: msg,
	}
}
func makeTriggerCLRetryFactory() retry.Factory {
	return transient.Only(func() retry.Iterator {
		return &retry.ExponentialBackoff{
			Limited: retry.Limited{
				Delay:   100 * time.Millisecond,
				Retries: -1, // unlimited
			},
			Multiplier: 2,
			MaxDelay:   30 * time.Second,
		}
	})
}

func loadCLs(ctx context.Context, tcl *prjpb.TriggeringCL) (cl, origin *changelist.CL, err error) {
	cls := []*changelist.CL{
		{ID: common.CLID(tcl.GetClid())},
		{ID: common.CLID(tcl.GetOriginClid())},
	}
	if err := changelist.LoadCLs(ctx, cls); err != nil {
		return nil, nil, err
	}
	cl = cls[0]
	if len(cls) > 1 {
		origin = cls[1]
	}
	return
}
