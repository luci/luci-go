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
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/gerrit"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/eventpb"
	"go.chromium.org/luci/cv/internal/run/impl/state"
	"go.chromium.org/luci/cv/internal/run/impl/submit"
	"go.chromium.org/luci/cv/internal/tree"
)

// OnReadyForSubmission implements Handler interface.
func (impl *Impl) OnReadyForSubmission(ctx context.Context, rs *state.RunState) (*Result, error) {
	switch status := rs.Run.Status; {
	case run.IsEnded(status):
		// It is safe to discard this event because this event either
		//  * arrives after Run gets cancelled while waiting for submission.
		//  * is sent by OnCQDVerificationCompleted handler as a fail-safe and Run
		//    submission has already completed.
		logging.Debugf(ctx, "received ReadyForSubmission event when Run is %s", status)
		// Under certain race condition, this Run may still occupy the submit
		// queue. So, check first without a transaction and then initiate a
		// transaction to release if this Run is current.
		switch current, err := submit.CurrentRun(ctx, rs.Run.ID.LUCIProject()); {
		case err != nil:
			return nil, err
		case current == rs.Run.ID:
			var innerErr error
			err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
				innerErr = submit.Release(ctx, rs.Run.ID)
				return innerErr
			}, nil)
			switch {
			case innerErr != nil:
				return nil, innerErr
			case err != nil:
				return nil, errors.Annotate(err, "failed to release submit queue").Tag(transient.Tag).Err()
			}
		}
		return &Result{State: rs}, nil
	case status == run.Status_SUBMITTING:
		if rs.Run.Submission.GetDeadline() == nil {
			panic(fmt.Errorf("impossible; run %q is submitting but Run.Submission.Deadline is not set", rs.Run.ID))
		}
		if deadline := rs.Run.Submission.Deadline.AsTime(); deadline.After(clock.Now(ctx)) {
			// Deadline hasn't expired yet. Presumably another task is still working
			// on the submission. So poke as soon as the deadline expires.
			if err := run.PokeAt(ctx, rs.Run.ID, deadline); err != nil {
				return nil, err
			}
			return &Result{State: rs}, nil
		}
		// Deadline has already expired. Try to acquire submit queue again
		// and attempt another submission if not waitlisted. Otherwise,
		// falls back to WAITING_FOR_SUBMISSION status.
		switch waitlist, err := acquireSubmitQueue(ctx, rs); {
		case err != nil:
			return nil, err
		case waitlist:
			rs = rs.ShallowCopy()
			rs.Run.Status = run.Status_WAITING_FOR_SUBMISSION
			rs.Run.Submission.Deadline = nil
			return &Result{State: rs}, nil
		}
		fallthrough // re-acquired submit queue successfully, try submitting again.
	case status == run.Status_RUNNING || status == run.Status_WAITING_FOR_SUBMISSION:
		rs = rs.ShallowCopy()
		markSubmitting(ctx, rs)
		submission := rs.Run.Submission
		s := submitter{
			runID:    rs.Run.ID,
			deadline: submission.GetDeadline().AsTime(),
			attempt:  submission.AttemptCount,
			clids:    make(common.CLIDs, 0, len(submission.GetCls())-len(submission.GetSubmittedCls())),
		}
		cg, err := rs.LoadConfigGroup(ctx)
		if err != nil {
			return nil, err
		}
		s.treeURL = cg.Content.GetVerifiers().GetTreeStatus().GetUrl()
		submittedCLs := make(map[int64]struct{}, len(submission.GetSubmittedCls()))
		for _, clid := range submission.GetSubmittedCls() {
			submittedCLs[clid] = struct{}{}
		}
		for _, clid := range rs.Run.Submission.GetCls() {
			if _, ok := submittedCLs[clid]; !ok {
				s.clids = append(s.clids, common.CLID(clid))
			}
		}
		return &Result{
			State:         rs,
			PostProcessFn: s.submit,
		}, nil
	default:
		panic(fmt.Errorf("impossible status %s", status))
	}
}

const defaultSubmissionDuration = 30 * time.Second

func markSubmitting(ctx context.Context, rs *state.RunState) error {
	rs.Run.Status = run.Status_SUBMITTING
	if rs.Run.Submission == nil {
		rs.Run.Submission = &run.Submission{}
		var err error
		if rs.Run.Submission.Cls, err = orderCLIDsInSubmissionOrder(ctx, rs.Run.CLs, rs.Run.ID, rs.Run.Submission); err != nil {
			return err
		}
	}
	deadline, ok := ctx.Deadline()
	if ok {
		rs.Run.Submission.Deadline = timestamppb.New(deadline.UTC())
	} else {
		rs.Run.Submission.Deadline = timestamppb.New(clock.Now(ctx).UTC().Add(defaultSubmissionDuration))
	}
	rs.Run.Submission.AttemptCount += 1
	return nil
}

// OnCLSubmitted implements Handler interface.
func (*Impl) OnCLSubmitted(ctx context.Context, rs *state.RunState, clids common.CLIDs) (*Result, error) {
	panic("implement")
}

// OnSubmissionCompleted implements Handler interface.
func (*Impl) OnSubmissionCompleted(ctx context.Context, rs *state.RunState, sr eventpb.SubmissionResult, attempt int32) (*Result, error) {
	panic("implement")
}

func acquireSubmitQueue(ctx context.Context, rs *state.RunState) (waitlisted bool, err error) {
	cg, err := rs.LoadConfigGroup(ctx)
	if err != nil {
		return false, err
	}
	now := clock.Now(ctx).UTC()
	rid := rs.Run.ID
	var innerErr error
	err = datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		waitlisted, innerErr = submit.TryAcquire(ctx, rid, cg.SubmitOptions)
		switch {
		case innerErr != nil:
			return innerErr
		case !waitlisted:
			// If not waitlisted, RM will proceed as if ReadyForSubmission event is
			// received. Sends a ReadyForSubmission event 10 seconds later in case
			// the event processing has failed in the middle.
			return run.NotifyReadyForSubmission(ctx, rid, now.Add(10*time.Second))
		default:
			return nil
		}
	}, nil)
	switch {
	case innerErr != nil:
		return false, innerErr
	case err != nil:
		return false, errors.Annotate(err, "failed to run the transaction to acquire submit queue").Tag(transient.Tag).Err()
	default:
		return waitlisted, nil
	}
}

func orderCLIDsInSubmissionOrder(ctx context.Context, clids common.CLIDs, runID common.RunID, sub *run.Submission) ([]int64, error) {
	cls, err := loadRunCLs(ctx, clids, runID)
	if err != nil {
		return nil, err
	}
	cls, err = submit.ComputeOrder(cls)
	if err != nil {
		return nil, err
	}
	ret := make([]int64, len(cls))
	for i, cl := range cls {
		ret[i] = int64(cl.ID)
	}
	return ret, nil
}

func loadRunCLs(ctx context.Context, clids common.CLIDs, runID common.RunID) ([]*run.RunCL, error) {
	cls := make([]*run.RunCL, len(clids))
	runKey := datastore.MakeKey(ctx, run.RunKind, string(runID))
	for i, clID := range clids {
		cls[i] = &run.RunCL{
			ID:  clID,
			Run: runKey,
		}
	}
	err := datastore.Get(ctx, cls)
	switch merr, ok := err.(errors.MultiError); {
	case ok:
		for i, err := range merr {
			if err == datastore.ErrNoSuchEntity {
				return nil, errors.Reason("RunCL %d not found in Datastore", cls[i].ID).Err()
			}
		}
		count, err := merr.Summary()
		return nil, errors.Annotate(err, "failed to load %d out of %d RunCLs", count, len(cls)).Tag(transient.Tag).Err()
	case err != nil:
		return nil, errors.Annotate(err, "failed to load %d RunCLs", len(cls)).Tag(transient.Tag).Err()
	}
	return cls, nil
}

type submitter struct {
	// runID is the ID of the Run to be submitted.
	runID common.RunID
	// deadline is when this submission should be stopped.
	deadline time.Time
	// attempt is the current submission attempt count.
	attempt int32
	// treeURL is used to check if tree is closed at the beginning
	// of submission.
	treeURL string
	// clids contains ids of cls to be submitted.
	//
	// SHOULD be in submission order.
	clids common.CLIDs

	// DO NOT SET. Internal fields used by receiver function.
	cachedGCByHost map[string]gerrit.Client
}

const defaultFatalMsg = "CV failed to submit your change because of " +
	"unexpected internal error. Please contact LUCI team: https://bit.ly/3sMReYs"

// reservedDuration is the time reserved to release queue and notify RM after
// finish an attempt to submit.
const reservedDuration = 500 * time.Millisecond

func (s submitter) submit(ctx context.Context) error {
	sc := &eventpb.SubmissionCompleted{
		Result:  eventpb.SubmissionResult_SUCCEEDED,
		Attempt: s.attempt,
	}
	switch passed, err := s.checkPrecondition(ctx); {
	case err != nil:
		errors.Log(ctx, err)
		if transient.Tag.In(err) {
			sc.Result = eventpb.SubmissionResult_FAILED_TRANSIENT
		} else {
			sc.Result = eventpb.SubmissionResult_FAILED_PERMANENT
			sc.FatalMessage = defaultFatalMsg
		}
	case !passed:
		sc.Result = eventpb.SubmissionResult_FAILED_PRECONDITION
	default: // precondition check passed
		dctx, cancel := clock.WithDeadline(ctx, s.deadline.Add(-reservedDuration))
		defer cancel()
		if fatalMsg, err := s.submitImpl(dctx); err != nil {
			errors.Log(ctx, err)
			switch {
			case transient.Tag.In(err):
				sc.Result = eventpb.SubmissionResult_FAILED_TRANSIENT
			case errors.Contains(err, context.DeadlineExceeded):
				// It is possible we get DeadlineExceeded error if we have
				// too many CL to submit in this submission and Gerrit is
				// slow. Explicitly mark it as transient so that the next
				// retry can pick up what's leftover.
				sc.Result = eventpb.SubmissionResult_FAILED_TRANSIENT
			default:
				sc.Result = eventpb.SubmissionResult_FAILED_PERMANENT
				sc.FatalMessage = fatalMsg
			}
		}
	}

	err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		return submit.Release(ctx, s.runID)
	}, nil)
	if err != nil {
		sc.SubmitQueueNotReleased = true
	}
	// TODO(yiwzhang): optimization for happy path: for successful submission,
	// invoke the RM within the same task to reduce latency.
	return notifySubmissionCompleted(ctx, s.runID, sc)
}

// submitImpl implements actual submission logic.
//
// If transient failure is returned, submission will be retried. Otherwise, RM
// will fail the submission and will post `fatalMsg` on all not-yet-submitted
// CLs and notify the users. Therefore, please be aware of what's included in
// the `fatalMsg` to avoid accidental leak of information.
func (s submitter) submitImpl(ctx context.Context) (fatalMsg string, err error) {
	cls, err := loadRunCLs(ctx, s.clids, s.runID)
	if err != nil {
		return defaultFatalMsg, err
	}
	for _, cl := range cls {
		if err := s.submitCL(ctx, cl); err != nil {
			if fatalMsg = triageGerritErr(err); fatalMsg != "" {
				// Ensure err is not tagged with transient.
				return fatalMsg, transient.Tag.Off().Apply(err)
			}
			return "", transient.Tag.Apply(err)
		}
		if err := notifyCLSubmitted(ctx, s.runID, cl.ID); err != nil {
			return defaultFatalMsg, err
		}
	}
	return "", nil
}

func (s submitter) checkPrecondition(ctx context.Context) (passed bool, err error) {
	switch cur, err := submit.CurrentRun(ctx, s.runID.LUCIProject()); {
	case err != nil:
		return false, err
	case cur != s.runID:
		logging.Warningf(ctx, "run is no longer current in submit queue, current run is %q", cur)
		return false, nil
	}

	if s.treeURL != "" {
		switch status, err := tree.FetchLatest(ctx, s.treeURL); {
		case err != nil:
			return false, err
		case status.State != tree.Open && status.State != tree.Throttled:
			logging.Warningf(ctx, "tree %q is closed when submission starts", s.treeURL)
			return false, nil
		}
	}

	switch remaining := s.deadline.Sub(clock.Now(ctx)); {
	case remaining < 0:
		logging.Warningf(ctx, "submit deadline has already expired at %s", s.deadline)
		return false, nil
	case remaining <= reservedDuration:
		logging.Warningf(ctx, "not enough time left for submission")
		return false, nil
	}
	return true, nil
}

func (s submitter) submitCL(ctx context.Context, cl *run.RunCL) error {
	host := cl.Detail.GetGerrit().GetHost()
	if _, ok := s.cachedGCByHost[host]; !ok {
		gc, err := gerrit.CurrentClient(ctx, host, s.runID.LUCIProject())
		if err != nil {
			return err
		}
		if s.cachedGCByHost == nil {
			s.cachedGCByHost = make(map[string]gerrit.Client, 1)
		}
		s.cachedGCByHost[host] = gc
	}
	gc := s.cachedGCByHost[host]
	ci := cl.Detail.GetGerrit().GetInfo()
	_, submitErr := gc.SubmitRevision(ctx, &gerritpb.SubmitRevisionRequest{
		Number:     ci.GetNumber(),
		RevisionId: ci.GetCurrentRevision(),
		Project:    ci.GetProject(),
	})
	if submitErr == nil {
		return nil
	}
	// Sometimes, Gerrit may return error but change is actually merged.
	// Load the change again to check whether it is actually merged.
	latest, getErr := gc.GetChange(ctx, &gerritpb.GetChangeRequest{
		Number:  ci.GetNumber(),
		Project: ci.GetProject(),
	})
	if getErr == nil && latest.Status == gerritpb.ChangeStatus_MERGED {
		return nil
	}
	return submitErr
}

const (
	permDeniedMsg = "CV couldn't submit your CL because CV is not " +
		"allowed to do so in your Gerrit project config. Contact your " +
		"project admin or Chrome Operations team https://goo.gl/f3mzjN"
	failedPreconditionMsgFmt = "Gerrit rejected submission with error: " +
		"%s\nHint: rebasing CL in Gerrit UI and re-submitting through CQ " +
		"usually works"
	unexpectedMsgFmt = "CV failed to submit your change because of unexpected error from Gerrit: %s"
)

// triageGerritErr returns non-empty message for fatal error.
func triageGerritErr(err error) string {
	switch grpcutil.Code(err) {
	case codes.PermissionDenied:
		return permDeniedMsg
	case codes.FailedPrecondition:
		// Gerrit returns 409. Either because change can't be merged, or
		// this revision isn't the latest.
		return fmt.Sprintf(failedPreconditionMsgFmt, err)
	case codes.ResourceExhausted, codes.Internal:
		return ""
	default:
		return fmt.Sprintf(unexpectedMsgFmt, err)
	}
}

// notifyCLSubmitted informs RunManager that the provided CL is submitted.
func notifyCLSubmitted(ctx context.Context, runID common.RunID, clid common.CLID) error {
	// Unlike other event-sending funcs, this function only delivers the event
	// to Run's eventbox, but does not dispatch the task. This is because it is
	// okay to process all events of this kind together to record the submission
	// result for each individual CLs after the attempt to submit completes.
	// Waking up RM unnecessarily may increase the contention of RM.
	return eventpb.SendWithoutDispatch(ctx, runID, &eventpb.Event{
		Event: &eventpb.Event_ClSubmitted{
			ClSubmitted: &eventpb.CLSubmitted{
				Clid: int64(clid),
			},
		},
	})
}

func notifySubmissionCompleted(ctx context.Context, runID common.RunID, sc *eventpb.SubmissionCompleted) error {
	return eventpb.SendNow(ctx, runID, &eventpb.Event{
		Event: &eventpb.Event_SubmissionCompleted{
			SubmissionCompleted: sc,
		},
	})
}
