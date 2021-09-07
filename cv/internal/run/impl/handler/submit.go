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

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	commonpb "go.chromium.org/luci/cv/api/common/v1"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/gerrit"
	"go.chromium.org/luci/cv/internal/run"
	submitpb "go.chromium.org/luci/cv/internal/run/commonpb"
	"go.chromium.org/luci/cv/internal/run/impl/state"
	"go.chromium.org/luci/cv/internal/run/impl/submit"
)

// OnReadyForSubmission implements Handler interface.
func (impl *Impl) OnReadyForSubmission(ctx context.Context, rs *state.RunState) (*Result, error) {
	switch status := rs.Run.Status; {
	case run.IsEnded(status):
		// It is safe to discard this event because this event either:
		//  * arrives after Run gets cancelled while waiting for submission, or
		//  * is sent by OnCQDVerificationCompleted handler as a fail-safe and
		//    Run submission has already completed.
		logging.Debugf(ctx, "received ReadyForSubmission event when Run is %s", status)
		rs = rs.ShallowCopy()
		// Under certain race conditions, this Run may still occupy the submit
		// queue. So, check first without a transaction and then initiate a
		// transaction to release if this Run currently occupies the submit queue.
		if err := releaseSubmitQueueIfTaken(ctx, rs, impl.RM); err != nil {
			return nil, err
		}
		return &Result{State: rs}, nil
	case status == commonpb.Run_SUBMITTING:
		// Discard this event if this Run is currently submitting. If submission
		// is stopped and should be resumed (e.g. transient failure, app crashing),
		// it should be handled in `OnSubmissionCompleted` or `TryResumeSubmission`.
		logging.Debugf(ctx, "received ReadyForSubmission event when Run is submitting")
		return &Result{State: rs}, nil
	case status == commonpb.Run_RUNNING:
		// This may happen when this Run transitioned from RUNNING status to
		// WAITING_FOR_SUBMISSION, prepared for submission but failed to
		// save the state transition. This Run is receiving this event because
		// of the fail-safe task sent while acquiring the Submit Queue. CV should
		// treat this Run as WAITING_FOR_SUBMISSION status.
		rs = rs.ShallowCopy()
		rs.Run.Status = commonpb.Run_WAITING_FOR_SUBMISSION
		fallthrough
	case status == commonpb.Run_WAITING_FOR_SUBMISSION:
		if len(rs.Run.Submission.GetSubmittedCls()) > 0 {
			panic(fmt.Errorf("impossible; Run %q is in Status_WAITING_FOR_SUBMISSION status but has submitted CLs ", rs.Run.ID))
		}
		switch waitlisted, err := acquireSubmitQueue(ctx, rs, impl.RM); {
		case err != nil:
			return nil, err
		case waitlisted:
			// This Run will be notified by Submit Queue once its turn comes.
			return &Result{State: rs}, nil
		}

		rs = rs.ShallowCopy()
		if rs.Run.Submission == nil {
			rs.Run.Submission = &run.Submission{}
		}

		switch treeOpen, err := rs.CheckTree(ctx, impl.TreeClient); {
		case err != nil:
			return nil, err
		case !treeOpen:
			err := parallel.WorkPool(2, func(work chan<- func() error) {
				work <- func() error {
					// Tree is closed, revisit after 1 minute.
					return impl.RM.PokeAfter(ctx, rs.Run.ID, 1*time.Minute)
				}
				work <- func() error {
					// Give up the Submit Queue while waiting for tree to open.
					return releaseSubmitQueue(ctx, rs, impl.RM)
				}
			})
			if err != nil {
				return nil, common.MostSevereError(err)
			}
			return &Result{State: rs}, nil
		default:
			if err := markSubmitting(ctx, rs); err != nil {
				return nil, err
			}
			s := newSubmitter(ctx, rs.Run.ID, rs.Run.Submission, impl.RM, impl.GFactory)
			rs.SubmissionScheduled = true
			return &Result{
				State:         rs,
				PostProcessFn: s.submit,
			}, nil
		}
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
	rs.LogEntries = append(rs.LogEntries, &run.LogEntry{
		Time: timestamppb.New(clock.Now(ctx)),
		Kind: &run.LogEntry_ClSubmitted{
			ClSubmitted: &run.LogEntry_CLSubmitted{
				NewlySubmittedCls: common.CLIDsAsInt64s(clids),
				TotalSubmitted:    int64(len(sub.SubmittedCls)),
			},
		},
	})
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
func (impl *Impl) OnSubmissionCompleted(ctx context.Context, rs *state.RunState, sc *submitpb.SubmissionCompleted) (*Result, error) {
	switch status := rs.Run.Status; {
	case run.IsEnded(status):
		logging.Warningf(ctx, "received SubmissionCompleted event when Run is %s", status)
		rs = rs.ShallowCopy()
		if err := releaseSubmitQueueIfTaken(ctx, rs, impl.RM); err != nil {
			return nil, err
		}
		return &Result{State: rs}, nil
	case status != commonpb.Run_SUBMITTING:
		return nil, errors.Reason("expected SUBMITTING status; got %s", status).Err()
	case sc.GetResult() == submitpb.SubmissionResult_SUCCEEDED:
		rs = rs.ShallowCopy()
		se := impl.endRun(ctx, rs, commonpb.Run_SUCCEEDED)
		return &Result{
			State:        rs,
			SideEffectFn: se,
		}, nil
	case sc.GetResult() == submitpb.SubmissionResult_FAILED_TRANSIENT:
		rs = rs.ShallowCopy()
		rs.LogEntries = append(rs.LogEntries, &run.LogEntry{
			Time: timestamppb.New(clock.Now(ctx)),
			Kind: &run.LogEntry_SubmissionFailure_{
				SubmissionFailure: &run.LogEntry_SubmissionFailure{
					Event: sc,
				},
			},
		})
		return impl.tryResumeSubmission(ctx, rs, sc)
	case sc.GetResult() == submitpb.SubmissionResult_FAILED_PERMANENT:
		rs = rs.ShallowCopy()
		if clFailures := sc.GetClFailures(); clFailures != nil {
			failedCLs := make([]int64, len(clFailures.GetFailures()))
			for i, f := range clFailures.GetFailures() {
				failedCLs[i] = f.GetClid()
			}
			rs.Run.Submission.FailedCls = failedCLs
			rs.LogEntries = append(rs.LogEntries, &run.LogEntry{
				Time: timestamppb.New(clock.Now(ctx)),
				Kind: &run.LogEntry_SubmissionFailure_{
					SubmissionFailure: &run.LogEntry_SubmissionFailure{
						Event: sc,
					},
				},
			})
		}
		cg, err := rs.LoadConfigGroup(ctx)
		if err != nil {
			return nil, err
		}
		if err := impl.cancelNotSubmittedCLTriggers(ctx, rs.Run.ID, rs.Run.Submission, sc, cg); err != nil {
			return nil, err
		}
		se := impl.endRun(ctx, rs, commonpb.Run_FAILED)
		return &Result{
			State:        rs,
			SideEffectFn: se,
		}, nil
	default:
		panic(fmt.Errorf("impossible submission result %s", sc.GetResult()))
	}
}

// TryResumeSubmission implements Handler interface.
func (impl *Impl) TryResumeSubmission(ctx context.Context, rs *state.RunState) (*Result, error) {
	return impl.tryResumeSubmission(ctx, rs, nil)
}

func (impl *Impl) tryResumeSubmission(ctx context.Context, rs *state.RunState, sc *submitpb.SubmissionCompleted) (*Result, error) {
	switch {
	case rs.Run.Status != commonpb.Run_SUBMITTING || rs.SubmissionScheduled:
		return &Result{State: rs}, nil
	case sc != nil && sc.Result != submitpb.SubmissionResult_FAILED_TRANSIENT:
		panic(fmt.Errorf("submission can only be resumed on nil submission completed event or event reporting transient failure; got %s", sc))
	}

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
		rs = rs.ShallowCopy()
		var status commonpb.Run_Status
		switch submittedCnt := len(rs.Run.Submission.GetSubmittedCls()); {
		case submittedCnt > 0 && submittedCnt == len(rs.Run.Submission.GetCls()):
			// fully submitted
			status = commonpb.Run_SUCCEEDED
		default: // None submitted or partially submitted
			status = commonpb.Run_FAILED
			// synthesize submission completed event with permanent failure.
			if clFailures := sc.GetClFailures(); clFailures != nil {
				rs.Run.Submission.FailedCls = make([]int64, len(clFailures.GetFailures()))
				sc = &submitpb.SubmissionCompleted{
					Result: submitpb.SubmissionResult_FAILED_PERMANENT,
					FailureReason: &submitpb.SubmissionCompleted_ClFailures{
						ClFailures: &submitpb.SubmissionCompleted_CLSubmissionFailures{
							Failures: make([]*submitpb.SubmissionCompleted_CLSubmissionFailure, len(clFailures.GetFailures())),
						},
					},
				}
				for i, f := range clFailures.GetFailures() {
					rs.Run.Submission.FailedCls[i] = f.GetClid()
					sc.GetClFailures().Failures[i] = &submitpb.SubmissionCompleted_CLSubmissionFailure{
						Clid:    f.GetClid(),
						Message: fmt.Sprintf("CL failed to submit because of transient failure: %s. However, CV is running out of time to retry.", f.GetMessage()),
					}
				}
			} else {
				sc = &submitpb.SubmissionCompleted{
					Result: submitpb.SubmissionResult_FAILED_PERMANENT,
					FailureReason: &submitpb.SubmissionCompleted_Timeout{
						Timeout: true,
					},
				}
			}
			cg, err := rs.LoadConfigGroup(ctx)
			if err != nil {
				return nil, err
			}
			if err := impl.cancelNotSubmittedCLTriggers(ctx, rs.Run.ID, rs.Run.Submission, sc, cg); err != nil {
				return nil, err
			}
		}
		if err := releaseSubmitQueueIfTaken(ctx, rs, impl.RM); err != nil {
			return nil, err
		}
		se := impl.endRun(ctx, rs, status)
		return &Result{
			State:        rs,
			SideEffectFn: se,
		}, nil
	case taskID == mustTaskIDFromContext(ctx):
		// Matching taskID indicates current task is the retry of a previous
		// submitting task that has failed transiently. Continue the submission.
		rs = rs.ShallowCopy()
		s := newSubmitter(ctx, rs.Run.ID, rs.Run.Submission, impl.RM, impl.GFactory)
		rs.SubmissionScheduled = true
		return &Result{
			State:         rs,
			PostProcessFn: s.submit,
		}, nil
	default:
		// Presumably another task is working on the submission at this time.
		// So wake up RM as soon as the submission expires. Meanwhile, don't
		// consume the event as the retries of submission task will process
		// this event. It's probably a race condition that this task sees this
		// event first.
		if err := impl.RM.Invoke(ctx, rs.Run.ID, deadline.AsTime()); err != nil {
			return nil, err
		}
		return &Result{
			State:          rs,
			PreserveEvents: true,
		}, nil
	}
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
			// It is possible that RM fails before successfully completing the state
			// transition. In that case, this Run will block Submit Queue infinitely.
			// Sending a ReadyForSubmission event after 10s as a fail-safe to ensure
			// Run keeps making progress.
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
	case waitlisted:
		rs.LogEntries = append(rs.LogEntries, &run.LogEntry{
			Time: timestamppb.New(clock.Now(ctx)),
			Kind: &run.LogEntry_Waitlisted_{
				Waitlisted: &run.LogEntry_Waitlisted{},
			},
		})
		logging.Debugf(ctx, "Waitlisted in Submit Queue")
		return true, nil
	default:
		rs.LogEntries = append(rs.LogEntries, &run.LogEntry{
			Time: timestamppb.New(clock.Now(ctx)),
			Kind: &run.LogEntry_AcquiredSubmitQueue_{
				AcquiredSubmitQueue: &run.LogEntry_AcquiredSubmitQueue{},
			},
		})
		logging.Debugf(ctx, "Acquired Submit Queue")
		return false, nil
	}
}

// releaseSubmitQueueIfTaken checks if submit queue is occupied by the given
// Run before trying to release.
func releaseSubmitQueueIfTaken(ctx context.Context, rs *state.RunState, rm RM) error {
	switch current, waitlist, err := submit.LoadCurrentAndWaitlist(ctx, rs.Run.ID); {
	case err != nil:
		return err
	case current == rs.Run.ID:
		return releaseSubmitQueue(ctx, rs, rm)
	default:
		for _, w := range waitlist {
			if w == rs.Run.ID {
				return releaseSubmitQueue(ctx, rs, rm)
			}
		}
	}
	return nil
}

func releaseSubmitQueue(ctx context.Context, rs *state.RunState, rm RM) error {
	var innerErr error
	err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		innerErr = submit.Release(ctx, rm.NotifyReadyForSubmission, rs.Run.ID)
		return innerErr
	}, nil)
	switch {
	case innerErr != nil:
		return innerErr
	case err != nil:
		return errors.Annotate(err, "failed to release submit queue").Tag(transient.Tag).Err()
	}
	rs.LogEntries = append(rs.LogEntries, &run.LogEntry{
		Time: timestamppb.New(clock.Now(ctx)),
		Kind: &run.LogEntry_ReleasedSubmitQueue_{
			ReleasedSubmitQueue: &run.LogEntry_ReleasedSubmitQueue{},
		},
	})
	logging.Debugf(ctx, "Released Submit Queue")
	return nil
}

const submissionDuration = 20 * time.Minute

func markSubmitting(ctx context.Context, rs *state.RunState) error {
	rs.Run.Status = commonpb.Run_SUBMITTING
	var err error
	if rs.Run.Submission.Cls, err = orderCLIDsInSubmissionOrder(ctx, rs.Run.CLs, rs.Run.ID, rs.Run.Submission); err != nil {
		return err
	}
	rs.Run.Submission.Deadline = timestamppb.New(clock.Now(ctx).UTC().Add(submissionDuration))
	rs.Run.Submission.TaskId = mustTaskIDFromContext(ctx)
	return nil
}

func (impl *Impl) cancelNotSubmittedCLTriggers(ctx context.Context, runID common.RunID, submission *run.Submission, sc *submitpb.SubmissionCompleted, cg *prjcfg.ConfigGroup) error {
	allCLIDs := common.MakeCLIDs(submission.GetCls()...)
	allRunCLs, err := run.LoadRunCLs(ctx, runID, allCLIDs)
	if err != nil {
		return err
	}
	runCLExternalIDs := make([]changelist.ExternalID, len(allRunCLs))
	for i, runCL := range allRunCLs {
		runCLExternalIDs[i] = runCL.ExternalID
	}

	// Single-CL Run
	if len(allRunCLs) == 1 {
		var msg string
		switch {
		case sc.GetClFailures() != nil:
			failures := sc.GetClFailures().GetFailures()
			if len(failures) != 1 {
				panic(fmt.Errorf("expected exactly 1 failed CL, got %v", failures))
			}
			msg = failures[0].GetMessage()
		case sc.GetTimeout():
			msg = timeoutMsg
		default:
			msg = defaultMsg
		}
		return impl.cancelCLTriggers(ctx, runID, allRunCLs, runCLExternalIDs, msg, cg)
	}

	// Multi-CL Run
	submitted, failed, pending := splitRunCLs(allRunCLs, submission, sc)
	msgSuffix := makeSubmissionMsgSuffix(submitted, failed, pending)
	switch {
	case sc.GetClFailures() != nil:
		var wg sync.WaitGroup
		errs := make(errors.MultiError, len(failed)+len(pending))
		// cancel triggers of CLs that fail to submit.
		messages := make(map[common.CLID]string, len(sc.GetClFailures().GetFailures()))
		for _, f := range sc.GetClFailures().GetFailures() {
			messages[common.CLID(f.GetClid())] = f.GetMessage()
		}
		for i, failedCL := range failed {
			i, failedCL := i, failedCL
			wg.Add(1)
			go func() {
				defer wg.Done()
				msg := fmt.Sprintf("%s\n\n%s", messages[failedCL.ID], msgSuffix)
				errs[i] = impl.cancelCLTriggers(ctx, runID, []*run.RunCL{failedCL}, runCLExternalIDs, msg, cg)
			}()
		}
		// cancel triggers of CLs that CV won't try to submit.
		var sb strings.Builder
		fmt.Fprint(&sb, "CV didn't attempt to submit this CL because CV failed to submit its dependent CL(s):")
		// TODO(yiwzhang): Once CV learns how to submit multiple CLs in parallel,
		// this should be optimized to print out failed CLs that each pending CL
		// depends on instead of printing out all failed CLs.
		// Example: considering a CL group where CL B and CL C are submitted in
		// parallel and neither of them succeeds:
		//   A (submitted)
		//   |
		//   |--> B (failed) --> D (pending)
		//   |
		//   |--> C (failed) --> E (pending)
		// the message CV posts on CL D should only include the fact that CV fails
		// to submit CL B.
		for _, f := range failed {
			fmt.Fprintf(&sb, "\n  %s", f.ExternalID.MustURL())
		}
		fmt.Fprint(&sb, "\n\n")
		fmt.Fprint(&sb, msgSuffix)
		pendingMsg := sb.String()
		for i, pendingCL := range pending {
			i, pendingCL := i, pendingCL
			wg.Add(1)
			go func() {
				defer wg.Done()
				errs[len(failed)+i] = impl.cancelCLTriggers(ctx, runID, []*run.RunCL{pendingCL}, runCLExternalIDs, pendingMsg, cg)
			}()
		}
		wg.Wait()
		return common.MostSevereError(errs)
	case sc.GetTimeout():
		msg := fmt.Sprintf("%s\n\n%s", timeoutMsg, msgSuffix)
		return impl.cancelCLTriggers(ctx, runID, pending, runCLExternalIDs, msg, cg)
	default:
		msg := fmt.Sprintf("%s\n\n%s", defaultMsg, msgSuffix)
		return impl.cancelCLTriggers(ctx, runID, pending, runCLExternalIDs, msg, cg)
	}
}

func makeSubmissionMsgSuffix(submitted, failed, pending []*run.RunCL) string {
	submittedURLs := make([]string, len(submitted))
	for i, cl := range submitted {
		submittedURLs[i] = cl.ExternalID.MustURL()
	}
	notSubmittedURLs := make([]string, len(failed)+len(pending))
	for i, cl := range failed {
		notSubmittedURLs[i] = cl.ExternalID.MustURL()
	}
	for i, cl := range pending {
		notSubmittedURLs[len(failed)+i] = cl.ExternalID.MustURL()
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

func splitRunCLs(cls []*run.RunCL, submission *run.Submission, sc *submitpb.SubmissionCompleted) (submitted, failed, pending []*run.RunCL) {
	submittedSet := common.MakeCLIDsSet(submission.GetSubmittedCls()...)
	failedSet := make(common.CLIDsSet, len(sc.GetClFailures().GetFailures()))
	for _, f := range sc.GetClFailures().GetFailures() {
		if submittedSet.HasI64(f.GetClid()) {
			panic(fmt.Errorf("impossible; cl %d is marked both submitted and failed", f.GetClid()))
		}
		failedSet.AddI64(f.GetClid())
	}

	submitted = make([]*run.RunCL, 0, len(submittedSet))
	failed = make([]*run.RunCL, 0, len(failedSet))
	pending = make([]*run.RunCL, 0, len(cls)-len(submittedSet)-len(failedSet))
	for _, cl := range cls {
		switch {
		case submittedSet.Has(cl.ID):
			submitted = append(submitted, cl)
		case failedSet.Has(cl.ID):
			failed = append(failed, cl)
		default:
			pending = append(pending, cl)
		}
	}
	return submitted, failed, pending
}

////////////////////////////////////////////////////////////////////////////////
// Submitter Implementation

type submitter struct {
	// All fields are immutable.

	// runID is the ID of the Run to be submitted.
	runID common.RunID
	// deadline is when this submission should be stopped.
	deadline time.Time
	// clids contains ids of cls to be submitted in submission order.
	clids common.CLIDs
	// rm is used to interact with Run Manager.
	rm RM
	// gFactory is used to interact with Gerrit.
	gFactory gerrit.Factory
}

func newSubmitter(ctx context.Context, runID common.RunID, submission *run.Submission, rm RM, g gerrit.Factory) *submitter {
	notSubmittedCLs := make(common.CLIDs, 0, len(submission.GetCls())-len(submission.GetSubmittedCls()))
	submitted := common.MakeCLIDs(submission.GetSubmittedCls()...).Set()
	for _, cl := range submission.GetCls() {
		clid := common.CLID(cl)
		if _, ok := submitted[clid]; !ok {
			notSubmittedCLs = append(notSubmittedCLs, clid)
		}
	}
	return &submitter{
		runID:    runID,
		deadline: submission.GetDeadline().AsTime(),
		clids:    notSubmittedCLs,
		rm:       rm,
		gFactory: g,
	}
}

// ErrTransientSubmissionFailure indicates that the submission has failed
// transiently and the same task should be retried.
var ErrTransientSubmissionFailure = errors.New("submission failed transiently", transient.Tag)

func (s submitter) submit(ctx context.Context) error {
	switch cur, err := submit.CurrentRun(ctx, s.runID.LUCIProject()); {
	case err != nil:
		return s.endSubmission(ctx, classifyErr(ctx, err))
	case cur != s.runID:
		logging.Errorf(ctx, "BUG: run no longer holds submit queue, currently held by %q", cur)
		return s.endSubmission(ctx, &submitpb.SubmissionCompleted{
			Result: submitpb.SubmissionResult_FAILED_PERMANENT,
		})
	}

	cls, err := run.LoadRunCLs(ctx, s.runID, s.clids)
	if err != nil {
		return s.endSubmission(ctx, classifyErr(ctx, err))
	}
	dctx, cancel := clock.WithDeadline(ctx, s.deadline)
	defer cancel()
	sc := s.submitCLs(dctx, cls)
	return s.endSubmission(ctx, sc)
}

// endSubmission notifies RM about submission result and release Submit Queue
// if necessary.
func (s submitter) endSubmission(ctx context.Context, sc *submitpb.SubmissionCompleted) error {
	if sc.GetResult() == submitpb.SubmissionResult_FAILED_TRANSIENT {
		// Do not release queue for transient failure.
		if err := s.rm.NotifySubmissionCompleted(ctx, s.runID, sc, true); err != nil {
			return err
		}
		return ErrTransientSubmissionFailure
	}
	var innerErr error
	now := clock.Now(ctx)
	err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		if sc.Result == submitpb.SubmissionResult_SUCCEEDED {
			if innerErr = submit.ReleaseOnSuccess(ctx, s.rm.NotifyReadyForSubmission, s.runID, now); innerErr != nil {
				return innerErr
			}
		} else {
			if innerErr = submit.Release(ctx, s.rm.NotifyReadyForSubmission, s.runID); innerErr != nil {
				return innerErr
			}
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
func (s submitter) submitCLs(ctx context.Context, cls []*run.RunCL) *submitpb.SubmissionCompleted {
	for _, cl := range cls {
		if clock.Now(ctx).After(s.deadline) {
			return &submitpb.SubmissionCompleted{
				Result: submitpb.SubmissionResult_FAILED_PERMANENT,
				FailureReason: &submitpb.SubmissionCompleted_Timeout{
					Timeout: true,
				},
			}
		}
		var submitted bool
		var msg string
		err := retry.Retry(ctx, perCLRetryFactory, func() error {
			if !submitted {
				switch err := s.submitCL(ctx, cl); {
				case err == nil:
					submitted = true
				default:
					var isTransient bool
					msg, isTransient = classifyGerritErr(ctx, err)
					if isTransient {
						return transient.Tag.Apply(err)
					}
					// Ensure err is not tagged with transient.
					return transient.Tag.Off().Apply(err)
				}
			}
			return s.rm.NotifyCLSubmitted(ctx, s.runID, cl.ID)
		}, retry.LogCallback(ctx, fmt.Sprintf("submit cl [id=%d, external_id=%q]", cl.ID, cl.ExternalID)))

		if err != nil {
			evt := classifyErr(ctx, err)
			if !submitted {
				evt.FailureReason = &submitpb.SubmissionCompleted_ClFailures{
					ClFailures: &submitpb.SubmissionCompleted_CLSubmissionFailures{
						Failures: []*submitpb.SubmissionCompleted_CLSubmissionFailure{
							{Clid: int64(cl.ID), Message: msg},
						},
					},
				}
			}
			return evt
		}
	}
	return &submitpb.SubmissionCompleted{
		Result: submitpb.SubmissionResult_SUCCEEDED,
	}
}

func (s submitter) submitCL(ctx context.Context, cl *run.RunCL) error {
	gc, err := s.gFactory.MakeClient(ctx, cl.Detail.GetGerrit().GetHost(), s.runID.LUCIProject())
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
	merged := false
	_ = s.gFactory.MakeMirrorIterator(ctx).RetryIfStale(func(opt grpc.CallOption) error {
		latest, err := gc.GetChange(ctx, &gerritpb.GetChangeRequest{
			Number:  ci.GetNumber(),
			Project: ci.GetProject(),
		})
		switch {
		case err != nil:
			return err
		case latest.GetStatus() == gerritpb.ChangeStatus_MERGED:
			// It is possible that somebody else submitted the change, but this is
			// unlikely enough that we presume CV did it. If necessary, it's possible
			// to examine Change messages to see who actually did it.
			merged = true
			return nil
		case latest.GetUpdated().AsTime().Before(ci.GetUpdated().AsTime()):
			return gerrit.ErrStaleData
		default:
			merged = false
			return nil
		}
	})
	if merged {
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
		"throttled by Gerrit."
	gerritTimeoutMsg = "CV failed to submit this CL because Gerrit took too " +
		"long to respond."
	timeoutMsg = "CV timed out while trying to submit this CL. " +
		// TODO(yiwzhang): Generally, time out means CV is doing something
		// wrong and looping over internally, However, timeout could also
		// happen when submitting large CL stack and Gerrit is slow. In that
		// case, CV can't do anything about it. After launching m1, gather data
		// to see under what circumstance it may happen and revise this message
		// so that CV doesn't get blamed for timeout it isn't responsible for.
		"Please contact LUCI team https://bit.ly/3sMReYs."
	unexpectedMsgFmt = "CV failed to submit your CL because of unexpected error from Gerrit: %s"
)

// classifyGerritErr returns message to be posted on the CL for the given
// submission error and whether the error is transient.
func classifyGerritErr(ctx context.Context, err error) (msg string, isTransient bool) {
	grpcStatus, ok := status.FromError(errors.Unwrap(err))
	if !ok {
		return fmt.Sprintf(unexpectedMsgFmt, err), transient.Tag.In(err)
	}
	switch code, msg := grpcStatus.Code(), grpcStatus.Message(); code {
	case codes.PermissionDenied:
		return permDeniedMsg, false
	case codes.FailedPrecondition:
		// Gerrit returns 409. Either because change can't be merged, or
		// this revision isn't the latest.
		return fmt.Sprintf(failedPreconditionMsgFmt, msg), false
	case codes.ResourceExhausted:
		return resourceExhaustedMsg, true
	case codes.Internal:
		return fmt.Sprintf(unexpectedMsgFmt, msg), true
	case codes.DeadlineExceeded:
		return gerritTimeoutMsg, true
	default:
		logging.Warningf(ctx, "unclassified grpc code [%s] received from Gerrit. Full error: %s", code, err)
		return fmt.Sprintf(unexpectedMsgFmt, msg), false
	}
}

func classifyErr(ctx context.Context, err error) *submitpb.SubmissionCompleted {
	switch {
	case err == nil:
		return &submitpb.SubmissionCompleted{
			Result: submitpb.SubmissionResult_SUCCEEDED,
		}
	case transient.Tag.In(err):
		logging.Warningf(ctx, "Submission ended with FAILED_TRANSIENT: %s", err)
		return &submitpb.SubmissionCompleted{
			Result: submitpb.SubmissionResult_FAILED_TRANSIENT,
		}
	default:
		logging.Warningf(ctx, "Submission ended with FAILED_PERMANENT: %s", err)
		return &submitpb.SubmissionCompleted{
			Result: submitpb.SubmissionResult_FAILED_PERMANENT,
		}
	}
}
