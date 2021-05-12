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
	"sort"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/gerrit"
	"go.chromium.org/luci/cv/internal/gerrit/cancel"
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
		// transaction to release if this Run currently occupies the submit queue.
		if err := releaseSubmitQueueIfTaken(ctx, rs.Run.ID, impl.RM); err != nil {
			return nil, err
		}
		return &Result{State: rs}, nil
	case status == run.Status_SUBMITTING:
		return continueSubmissionIfPossible(ctx, rs, impl.RM)
	case status == run.Status_RUNNING || status == run.Status_WAITING_FOR_SUBMISSION:
		// TODO(yiwzhang): fail if partially submitted.
		rs, err := markSubmitting(ctx, rs)
		if err != nil {
			return nil, err
		}
		s, err := newSubmitter(ctx, rs, impl.RM)
		if err != nil {
			return nil, err
		}
		return &Result{
			State:         rs,
			PostProcessFn: s.submit,
		}, nil
	default:
		panic(fmt.Errorf("impossible status %s", status))
	}
}

// OnCLSubmitted implements Handler interface.
func (*Impl) OnCLSubmitted(ctx context.Context, rs *state.RunState, clids common.CLIDs) (*Result, error) {
	rs = rs.ShallowCopy()
	sub := rs.Run.Submission
	submitted := clids.Set()
	for _, clid := range sub.GetSubmittedCls() {
		submitted[common.CLID(clid)] = struct{}{}
	}
	if sub.GetSubmittedCls() != nil {
		sub.SubmittedCls = sub.SubmittedCls[:0]
	}
	for _, cl := range sub.GetCls() {
		clid := common.CLID(cl)
		if _, ok := submitted[clid]; ok {
			sub.SubmittedCls = append(sub.SubmittedCls, cl)
			delete(submitted, clid)
		}
	}
	if len(submitted) > 0 {
		unexpected := make(sort.IntSlice, 0, len(submitted))
		for clid := range submitted {
			unexpected = append(unexpected, int(clid))
		}
		unexpected.Sort()
		return nil, errors.Reason("received CLSubmitted event for cls not belonging to this Run: %v", unexpected).Err()
	}
	return &Result{State: rs}, nil
}

// OnSubmissionCompleted implements Handler interface.
func (*Impl) OnSubmissionCompleted(ctx context.Context, rs *state.RunState, sc *eventpb.SubmissionCompleted) (*Result, error) {
	panic("implement")
}

func acquireSubmitQueue(ctx context.Context, rs *state.RunState, rm RM) (waitlisted bool, err error) {
	cg, err := rs.LoadConfigGroup(ctx)
	if err != nil {
		return false, err
	}
	now := clock.Now(ctx).UTC()
	rid := rs.Run.ID
	var innerErr error
	err = datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		waitlisted, innerErr = submit.TryAcquire(ctx, rm.NotifyReadyForSubmission, rid, cg.SubmitOptions)
		switch {
		case innerErr != nil:
			return innerErr
		case !waitlisted:
			// If not waitlisted, RM will proceed as if ReadyForSubmission event is
			// received. Sends a ReadyForSubmission event 10 seconds later in case
			// the event processing has failed in the middle.
			return rm.NotifyReadyForSubmission(ctx, rid, now.Add(10*time.Second))
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

func releaseSubmitQueueIfTaken(ctx context.Context, runID common.RunID, rm RM) error {
	switch current, err := submit.CurrentRun(ctx, runID.LUCIProject()); {
	case err != nil:
		return err
	case current == runID:
		var innerErr error
		err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
			innerErr = submit.Release(ctx, rm.NotifyReadyForSubmission, runID)
			return innerErr
		}, nil)
		switch {
		case innerErr != nil:
			return innerErr
		case err != nil:
			return errors.Annotate(err, "failed to release submit queue").Tag(transient.Tag).Err()
		}
	}
	return nil
}

const submissionDuration = 20 * time.Minute

func markSubmitting(ctx context.Context, rs *state.RunState) (*state.RunState, error) {
	ret := rs.ShallowCopy()
	ret.Run.Status = run.Status_SUBMITTING
	if ret.Run.Submission == nil {
		ret.Run.Submission = &run.Submission{}
		var err error
		if ret.Run.Submission.Cls, err = orderCLIDsInSubmissionOrder(ctx, ret.Run.CLs, ret.Run.ID, ret.Run.Submission); err != nil {
			return nil, err
		}
	}
	ret.Run.Submission.Deadline = timestamppb.New(clock.Now(ctx).UTC().Add(submissionDuration))
	ret.Run.Submission.TaskId = mustTaskIDFromContext(ctx)
	return ret, nil
}

func continueSubmissionIfPossible(ctx context.Context, rs *state.RunState, rm RM) (*Result, error) {
	deadline := rs.Run.Submission.GetDeadline()
	taskID := rs.Run.Submission.GetTaskId()
	switch {
	case deadline == nil:
		panic(fmt.Errorf("impossible: run %q is submitting but Run.Submission.Deadline is not set", rs.Run.ID))
	case taskID == "":
		panic(fmt.Errorf("impossible: run %q is submitting but Run.Submission.TaskId is not set", rs.Run.ID))
	}

	switch expired := clock.Now(ctx).After(deadline.AsTime()); {
	case expired:
		switch submittedCnt := len(rs.Run.Submission.GetSubmittedCls()); {
		case submittedCnt > 0 && submittedCnt == len(rs.Run.Submission.GetCls()):
			// fully submitted
			rs.Run.Status = run.Status_SUCCEEDED
		default: // None submitted or partially submitted
			rs.Run.Status = run.Status_FAILED
			// synthesize submission completed event for timeout.
			sc := &eventpb.SubmissionCompleted{
				Result: eventpb.SubmissionResult_FAILED_PERMANENT,
				FailureReason: &eventpb.SubmissionCompleted_Timeout{
					Timeout: true,
				},
			}
			if err := cancelNotSubmittedCLTriggers(ctx, rs.Run.ID, rs.Run.Submission, sc); err != nil {
				return nil, err
			}
		}
		if err := releaseSubmitQueueIfTaken(ctx, rs.Run.ID, rm); err != nil {
			return nil, err
		}
		return &Result{State: rs}, nil
	case taskID == mustTaskIDFromContext(ctx):
		// Matching taskID indicates current task is the retry of a previous
		// submitting task that has failed transiently. Continue the submission.
		s, err := newSubmitter(ctx, rs, rm)
		if err != nil {
			return nil, err
		}
		return &Result{
			State:         rs,
			PostProcessFn: s.submit,
		}, nil
	default:
		// Presumably another task is working on the submission at this time. So
		// poke as soon as the deadline expires.
		if err := rm.PokeAt(ctx, rs.Run.ID, deadline.AsTime()); err != nil {
			return nil, err
		}
		return &Result{State: rs}, nil
	}
}

func cancelNotSubmittedCLTriggers(ctx context.Context, runID common.RunID, submission *run.Submission, sc *eventpb.SubmissionCompleted) error {
	allCLIDs := common.MakeCLIDs(submission.GetCls()...)
	allRunCLs, err := run.LoadRunCLs(ctx, runID, allCLIDs)
	if err != nil {
		return err
	}
	runCLExternalIDs := make([]changelist.ExternalID, len(allRunCLs))
	for i, runCL := range allRunCLs {
		runCLExternalIDs[i] = runCL.ExternalID
	}

	if len(allRunCLs) == 1 { // single CL Run
		var msg string
		switch {
		case sc.GetClFailure() != nil:
			msg = sc.GetClFailure().GetMessage()
		case sc.GetTimeout():
			msg = timeoutMsg
		default:
			msg = defaultMsg
		}
		return cancelCLTriggers(ctx, runID, allRunCLs, runCLExternalIDs, msg)
	}

	// Multi CLs Run
	submitted, pending, failed := splitRunCLs(allRunCLs, submission, sc)
	msgSuffix := makeSubmissionMsgSuffix(submitted, failed, pending)
	switch {
	case sc.GetClFailure() != nil:
		var wg sync.WaitGroup
		wg.Add(2)
		errs := make(errors.MultiError, 2)
		go func() {
			defer wg.Done()
			msg := fmt.Sprintf("%s\n\n%s", sc.GetClFailure().GetMessage(), msgSuffix)
			errs[0] = cancelCLTriggers(ctx, runID, []*run.RunCL{failed}, runCLExternalIDs, msg)
		}()
		go func() {
			defer wg.Done()
			if len(pending) > 0 {
				notAttemptedMsg := fmt.Sprintf("CV didn't attempt to submit this CL because CV failed to submit its dependent CL(s): %s\n%s", failed.ExternalID.MustURL(), msgSuffix)
				errs[1] = cancelCLTriggers(ctx, runID, pending, runCLExternalIDs, notAttemptedMsg)
			}
		}()
		wg.Wait()
		return common.MostSevereError(errs)
	case sc.GetTimeout():
		msg := fmt.Sprintf("%s\n\n%s", timeoutMsg, msgSuffix)
		return cancelCLTriggers(ctx, runID, pending, runCLExternalIDs, msg)
	default:
		msg := fmt.Sprintf("%s\n\n%s", defaultMsg, msgSuffix)
		return cancelCLTriggers(ctx, runID, pending, runCLExternalIDs, msg)
	}
}

func cancelCLTriggers(ctx context.Context, runID common.RunID, toCancel []*run.RunCL, runCLExternalIDs []changelist.ExternalID, message string) error {
	clids := make(common.CLIDs, len(toCancel))
	for i, runCL := range toCancel {
		clids[i] = runCL.ID
	}
	cls, err := changelist.LoadCLs(ctx, clids)
	if err != nil {
		return err
	}

	luciProject := runID.LUCIProject()
	err = parallel.WorkPool(10, func(work chan<- func() error) {
		for i := range toCancel {
			i := i
			work <- func() error {
				err := cancel.Cancel(ctx, cancel.Input{
					CL:               cls[i],
					Trigger:          toCancel[i].Trigger,
					LUCIProject:      luciProject,
					Message:          message,
					Requester:        "Run Manager",
					Notify:           cancel.OWNER | cancel.VOTERS,
					LeaseDuration:    time.Minute,
					RunCLExternalIDs: runCLExternalIDs,
				})
				return errors.Annotate(err, "failed to cancel triggers for cl %d", cls[i].ID).Err()
			}
		}
	})
	if err != nil {
		switch merr, ok := err.(errors.MultiError); {
		case !ok:
			return err
		default:
			return common.MostSevereError(merr)
		}
	}
	return nil
}

func makeSubmissionMsgSuffix(submitted []*run.RunCL, failed *run.RunCL, pending []*run.RunCL) string {
	submittedURLs := make([]string, len(submitted))
	for i, cl := range submitted {
		submittedURLs[i] = cl.ExternalID.MustURL()
	}
	notSubmittedURLs := make([]string, 0, len(pending)+1)
	if failed != nil {
		notSubmittedURLs = append(notSubmittedURLs, failed.ExternalID.MustURL())
	}
	for _, cl := range pending {
		notSubmittedURLs = append(notSubmittedURLs, cl.ExternalID.MustURL())
	}
	if len(submittedURLs) > 0 { // partial submission
		return fmt.Sprintf(partiallySubmittedMsgSuffixFmt,
			strings.Join(notSubmittedURLs, ", "),
			strings.Join(submittedURLs, ", "),
		)
	}
	return fmt.Sprintf(noneSubmittedMsgSuffixFmt, strings.Join(notSubmittedURLs, ", "))
}

////////////////////////////////////////////////////////////////////////////////
// Helper methods

var fakeTaskIDKey = "used in handler tests only for setting the mock taskID"

func mustTaskIDFromContext(ctx context.Context) string {
	if taskID, ok := ctx.Value(&fakeTaskIDKey).(string); ok {
		return taskID
	}
	switch executionInfo := tq.TaskExecutionInfo(ctx); {
	case executionInfo == nil:
		panic("must be called within a task handler")
	case executionInfo.TaskID == "":
		panic("taskID in task executionInfo is empty")
	default:
		return executionInfo.TaskID
	}
}

func orderCLIDsInSubmissionOrder(ctx context.Context, clids common.CLIDs, runID common.RunID, sub *run.Submission) ([]int64, error) {
	cls, err := run.LoadRunCLs(ctx, runID, clids)
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

func splitRunCLs(cls []*run.RunCL, submission *run.Submission, sc *eventpb.SubmissionCompleted) (submitted, pending []*run.RunCL, failed *run.RunCL) {
	submittedSet := common.MakeCLIDs(submission.GetSubmittedCls()...).Set()
	failedCLID := common.CLID(sc.GetClFailure().GetClid())
	if _, ok := submittedSet[failedCLID]; ok {
		panic(fmt.Errorf("impossible; cl %d is marked both submitted and failed", failedCLID))
	}
	submitted = make([]*run.RunCL, 0, len(submittedSet))
	pending = make([]*run.RunCL, 0, len(cls)-len(submittedSet))
	for _, cl := range cls {
		switch _, ok := submittedSet[cl.ID]; {
		case ok:
			submitted = append(submitted, cl)
		case cl.ID == failedCLID:
			failed = cl
		default:
			pending = append(pending, cl)
		}
	}
	return submitted, pending, failed
}

////////////////////////////////////////////////////////////////////////////////
// Submitter Implementation

type submitter struct {
	// All fields are immutable.

	// runID is the ID of the Run to be submitted.
	runID common.RunID
	// deadline is when this submission should be stopped.
	deadline time.Time
	// treeURL is used to check if tree is closed at the beginning
	// of submission.
	treeURL string
	// clids contains ids of cls to be submitted in submission order.
	clids common.CLIDs
	// rm is used to interact with Run Manager.
	rm RM
}

func newSubmitter(ctx context.Context, rs *state.RunState, rm RM) (*submitter, error) {
	cg, err := rs.LoadConfigGroup(ctx)
	if err != nil {
		return nil, err
	}
	submission := rs.Run.Submission
	notSubmittedCLs := make(common.CLIDs, 0, len(submission.GetCls())-len(submission.GetSubmittedCls()))
	submitted := common.MakeCLIDs(submission.GetSubmittedCls()...).Set()
	for _, cl := range submission.GetCls() {
		clid := common.CLID(cl)
		if _, ok := submitted[clid]; !ok {
			notSubmittedCLs = append(notSubmittedCLs, clid)
		}
	}
	return &submitter{
		runID:    rs.Run.ID,
		deadline: submission.GetDeadline().AsTime(),
		treeURL:  cg.Content.GetVerifiers().GetTreeStatus().GetUrl(),
		clids:    notSubmittedCLs,
		rm:       rm,
	}, nil
}

// ErrTransientSubmissionFailure indicates that the submission has failed
// transiently and the same task should be retried.
var ErrTransientSubmissionFailure = errors.New("submission failed transiently", transient.Tag)

func (s submitter) submit(ctx context.Context) error {
	var sc *eventpb.SubmissionCompleted
	switch passed, err := s.checkPrecondition(ctx); {
	case err != nil:
		sc = classifyErr(ctx, err)
	case !passed:
		sc = &eventpb.SubmissionCompleted{
			Result: eventpb.SubmissionResult_FAILED_PRECONDITION,
		}
	default: // precondition check passed
		if cls, err := run.LoadRunCLs(ctx, s.runID, s.clids); err != nil {
			sc = classifyErr(ctx, err)
		} else {
			dctx, cancel := clock.WithDeadline(ctx, s.deadline)
			defer cancel()
			sc = s.submitCLs(dctx, cls)
		}
	}

	if sc.GetResult() == eventpb.SubmissionResult_FAILED_TRANSIENT {
		// Do not release queue for transient failure.
		if err := s.rm.NotifySubmissionCompleted(ctx, s.runID, sc, true); err != nil {
			return err
		}
		return ErrTransientSubmissionFailure
	}
	var innerErr error
	err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		if innerErr = submit.Release(ctx, s.rm.NotifyReadyForSubmission, s.runID); innerErr != nil {
			return innerErr
		}
		if innerErr = s.rm.NotifySubmissionCompleted(ctx, s.runID, sc, false); innerErr != nil {
			return innerErr
		}
		return nil
	}, nil)
	switch {
	case innerErr != nil:
		return innerErr
	case err != nil:
		return errors.Annotate(err, "failed to release submit queue and notify RM").Tag(transient.Tag).Err()
	}
	// TODO(yiwzhang): optimization for happy path: for successful submission,
	// invoke the RM within the same task to reduce latency.
	return s.rm.Invoke(ctx, s.runID, time.Time{})
}

var perCLRetryFactory retry.Factory = transient.Only(func() retry.Iterator {
	return &retry.ExponentialBackoff{
		Limited: retry.Limited{
			Delay:   200 * time.Millisecond,
			Retries: 10,
		},
		Multiplier: 2,
	}
})

// submitCLs sequentially submits the provided CLs.
//
// Retries on transient failure of submitting individual CL based on
// `perCLRetryFactory`.
func (s submitter) submitCLs(ctx context.Context, cls []*run.RunCL) *eventpb.SubmissionCompleted {
	for _, cl := range cls {
		var submitted bool
		var msg string
		err := retry.Retry(ctx, perCLRetryFactory, func() error {
			if !submitted {
				switch err := s.submitCL(ctx, cl); {
				case err == nil:
					submitted = true
				default:
					var isTransient bool
					msg, isTransient = classifyGerritErr(err)
					if isTransient {
						return transient.Tag.Apply(err)
					}
					// Ensure err is not tagged with transient.
					return transient.Tag.Off().Apply(err)
				}
			}
			return s.rm.NotifyCLSubmitted(ctx, s.runID, cl.ID)
		}, retry.LogCallback(ctx, fmt.Sprintf("submit cl [id=%d, external_id=%q]", cl.ID, cl.ExternalID)))

		switch {
		case err == nil:
		case clock.Now(ctx).After(s.deadline):
			return &eventpb.SubmissionCompleted{
				Result: eventpb.SubmissionResult_FAILED_PERMANENT,
				FailureReason: &eventpb.SubmissionCompleted_Timeout{
					Timeout: true,
				},
			}
		default:
			evt := classifyErr(ctx, err)
			if !submitted {
				evt.FailureReason = &eventpb.SubmissionCompleted_ClFailure{
					ClFailure: &eventpb.SubmissionCompleted_CLSubmissionFailure{
						Clid:    int64(cl.ID),
						Message: msg,
					},
				}
			}
			return evt
		}
	}
	return &eventpb.SubmissionCompleted{
		Result: eventpb.SubmissionResult_SUCCEEDED,
	}
}

func (s submitter) checkPrecondition(ctx context.Context) (passed bool, err error) {
	switch cur, err := submit.CurrentRun(ctx, s.runID.LUCIProject()); {
	case err != nil:
		return false, err
	case cur != s.runID:
		logging.Warningf(ctx, "run no longer holds submit queue, currently held by %q", cur)
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
	return true, nil
}

func (s submitter) submitCL(ctx context.Context, cl *run.RunCL) error {
	gc, err := gerrit.CurrentClient(ctx, cl.Detail.GetGerrit().GetHost(), s.runID.LUCIProject())
	if err != nil {
		return err
	}
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
		// It is possible that somebody else submitted the change, but this is
		// unlikely enough that we presume CV did it. If necessary, it's possible
		// to examine Change messages to see who actually did it.
		return nil
	}
	return submitErr
}

// TODO(yiwzhang/tandrii): normalize message with the template function
// used in clpurger/user_text.go.
const (
	defaultMsg = "CV failed to submit this CL because of " +
		"unexpected internal error. Please contact LUCI team: " +
		"https://bit.ly/3sMReYs"
	failedPreconditionMsgFmt = "Gerrit rejected submission with error: " +
		"%s\nHint: rebasing CL in Gerrit UI and re-submitting through CV " +
		"usually works"
	noneSubmittedMsgSuffixFmt = "None of the CLs in the Run were submitted " +
		"by CV.\nCLs: [%s]\n"
	partiallySubmittedMsgSuffixFmt = "CV partially submitted the CLs " +
		"in the Run.\nNot submitted: [%s]\nSubmitted: [%s]\n" +
		"Please, use your judgement to determine if already submitted CLs have " +
		"to be reverted, or if the remaining CLs could be manually submitted. " +
		"If you think the partially submitted CLs may have broken the " +
		"tip-of-tree of your project, consider notifying your infrastructure " +
		"team/gardeners/sheriffs."
	permDeniedMsg = "CV couldn't submit your CL because CV is not " +
		"allowed to do so in your Gerrit project config. Contact your " +
		"project admin or Chrome Operations team https://goo.gl/f3mzjN"
	resourceExhaustedMsg = "CV failed to submit this CL because it is " +
		"throttled by Gerrit"
	timeoutMsg = "CV timed out while trying to submit this CL. " +
		// TODO(yiwzhang): Generally, time out means CV is doing something
		// wrong and looping over internally, However, timeout could also
		// happen when submitting large CL stack and Gerrit is slow. In that
		// case, CV can do nothing about it. After launching m1, gather data
		// to see under what circumstance it may happen and revise this message
		// so that CV doesn't get blamed for timeout it isn't responsible for.
		"Please contact LUCI team https://bit.ly/3sMReYs."
	unexpectedMsgFmt = "CV failed to submit your CL because of unexpected error from Gerrit: %s"
)

// classifyGerritErr returns message to be posted on the CL for the given
// submission error and whether the error is transient.
func classifyGerritErr(err error) (msg string, isTransient bool) {
	switch grpcutil.Code(err) {
	case codes.PermissionDenied:
		return permDeniedMsg, false
	case codes.FailedPrecondition:
		// Gerrit returns 409. Either because change can't be merged, or
		// this revision isn't the latest.
		return fmt.Sprintf(failedPreconditionMsgFmt, err), false
	case codes.ResourceExhausted:
		return resourceExhaustedMsg, true
	case codes.Internal:
		return fmt.Sprintf(unexpectedMsgFmt, err), true
	default:
		return fmt.Sprintf(unexpectedMsgFmt, err), false
	}
}

func classifyErr(ctx context.Context, err error) *eventpb.SubmissionCompleted {
	switch {
	case err == nil:
		return &eventpb.SubmissionCompleted{
			Result: eventpb.SubmissionResult_SUCCEEDED,
		}
	case transient.Tag.In(err):
		errors.Log(ctx, err)
		return &eventpb.SubmissionCompleted{
			Result: eventpb.SubmissionResult_FAILED_TRANSIENT,
		}
	default:
		errors.Log(ctx, err)
		return &eventpb.SubmissionCompleted{
			Result: eventpb.SubmissionResult_FAILED_PERMANENT,
		}
	}
}
