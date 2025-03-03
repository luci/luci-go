// Copyright 2022 The LUCI Authors.
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

package submit

import (
	"context"
	"fmt"
	"strings"
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
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/gerrit"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/eventpb"
)

// RM encapsulates interaction with Run Manager by `RunCLsSubmitter`.
type RM interface {
	Invoke(ctx context.Context, runID common.RunID, eta time.Time) error
	NotifyReadyForSubmission(ctx context.Context, runID common.RunID, eta time.Time) error
	NotifyCLsSubmitted(ctx context.Context, runID common.RunID, clids common.CLIDs) error
	NotifySubmissionCompleted(ctx context.Context, runID common.RunID, sc *eventpb.SubmissionCompleted, invokeRM bool) error
}

// RunCLsSubmitter submits CLs for a Run.
type RunCLsSubmitter struct {
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

// NewSubmitter creates a new RunCLsSubmitter.
func NewSubmitter(ctx context.Context, runID common.RunID, submission *run.Submission, rm RM, g gerrit.Factory) *RunCLsSubmitter {
	notSubmittedCLs := make(common.CLIDs, 0, len(submission.GetCls())-len(submission.GetSubmittedCls()))
	submitted := common.MakeCLIDs(submission.GetSubmittedCls()...).Set()
	for _, cl := range submission.GetCls() {
		clid := common.CLID(cl)
		if _, ok := submitted[clid]; !ok {
			notSubmittedCLs = append(notSubmittedCLs, clid)
		}
	}
	return &RunCLsSubmitter{
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

// Submit tries to incrementally submits all CLs within deadline.
//
// Notifies RunManager whenever a CL is submitted successfully. Reports the
// final submission result to RunManager and releases the submit queue at the
// end.
//
// Submission is considered as failure if this Run no longer holds the submit
// queue or can't submit all CLs within deadline.
//
// Returns `ErrTransientSubmissionFailure` or error that happened when reporting
// final result to Run Manager, or releasing submit queue only. The rest of the
// errors that happened during the submission will be reported to Run Manager as
// part of the submission result.
func (s RunCLsSubmitter) Submit(ctx context.Context) error {
	switch cur, err := CurrentRun(ctx, s.runID.LUCIProject()); {
	case err != nil:
		return s.endSubmission(ctx, classifyErr(ctx, err))
	case cur != s.runID:
		logging.Errorf(ctx, "BUG: run no longer holds submit queue, currently held by %q", cur)
		return s.endSubmission(ctx, &eventpb.SubmissionCompleted{
			Result: eventpb.SubmissionResult_FAILED_PERMANENT,
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
func (s RunCLsSubmitter) endSubmission(ctx context.Context, sc *eventpb.SubmissionCompleted) error {
	if sc.GetResult() == eventpb.SubmissionResult_FAILED_TRANSIENT {
		// Do not release queue for transient failure.
		if err := s.rm.NotifySubmissionCompleted(ctx, s.runID, sc, true); err != nil {
			return err
		}
		return ErrTransientSubmissionFailure
	}
	var innerErr error
	now := clock.Now(ctx)
	err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		if sc.Result == eventpb.SubmissionResult_SUCCEEDED {
			if innerErr = ReleaseOnSuccess(ctx, s.rm.NotifyReadyForSubmission, s.runID, now); innerErr != nil {
				return innerErr
			}
		} else {
			if innerErr = Release(ctx, s.rm.NotifyReadyForSubmission, s.runID); innerErr != nil {
				return innerErr
			}
		}
		sc.QueueReleaseTimestamp = timestamppb.New(now)
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
func (s RunCLsSubmitter) submitCLs(ctx context.Context, cls []*run.RunCL) *eventpb.SubmissionCompleted {
	for _, cl := range cls {
		if clock.Now(ctx).After(s.deadline) {
			return &eventpb.SubmissionCompleted{
				Result: eventpb.SubmissionResult_FAILED_PERMANENT,
				FailureReason: &eventpb.SubmissionCompleted_Timeout{
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
					return transient.Tag.ApplyValue(err, false)
				}
			}
			return s.rm.NotifyCLsSubmitted(ctx, s.runID, common.CLIDs{cl.ID})
		}, retry.LogCallback(ctx, fmt.Sprintf("submit cl [id=%d, external_id=%q]", cl.ID, cl.ExternalID)))

		if err != nil {
			evt := classifyErr(ctx, err)
			if !submitted {
				evt.FailureReason = &eventpb.SubmissionCompleted_ClFailures{
					ClFailures: &eventpb.SubmissionCompleted_CLSubmissionFailures{
						Failures: []*eventpb.SubmissionCompleted_CLSubmissionFailure{
							{Clid: int64(cl.ID), Message: msg},
						},
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

// submitCL makes a request to Gerrit to submit (merge) the CL.
func (s RunCLsSubmitter) submitCL(ctx context.Context, cl *run.RunCL) error {
	gc, err := s.gFactory.MakeClient(ctx, cl.Detail.GetGerrit().GetHost(), s.runID.LUCIProject())
	if err != nil {
		return err
	}
	ci := cl.Detail.GetGerrit().GetInfo()
	submitErr := s.gFactory.MakeMirrorIterator(ctx).RetryIfStale(func(opt grpc.CallOption) error {
		_, err := gc.SubmitRevision(ctx, &gerritpb.SubmitRevisionRequest{
			Number:     ci.GetNumber(),
			RevisionId: ci.GetCurrentRevision(),
			Project:    ci.GetProject(),
		})
		grpcStatus, ok := status.FromError(errors.Unwrap(err))
		if !ok {
			return err
		}
		switch code, msg := grpcStatus.Code(), grpcStatus.Message(); {
		case code == codes.OK:
			return nil
		case code == codes.Internal && strings.Contains(msg, "LOCK_FAILURE"):
			// 503 Lock Failure is normally caused by stale replica serving the
			// request.
			// Reference: go/gob/users/user-faq#an-update-of-the-change-is-failing-with-the-503-lock-failure
			return gerrit.ErrStaleData
		default:
			return err
		}
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
		}, opt)
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

// Determines the type of submission completion result based on the given error.
func classifyErr(ctx context.Context, err error) *eventpb.SubmissionCompleted {
	switch {
	case err == nil:
		return &eventpb.SubmissionCompleted{
			Result: eventpb.SubmissionResult_SUCCEEDED,
		}
	case transient.Tag.In(err):
		logging.Warningf(ctx, "Submission ended with FAILED_TRANSIENT: %s", err)
		return &eventpb.SubmissionCompleted{
			Result: eventpb.SubmissionResult_FAILED_TRANSIENT,
		}
	default:
		logging.Warningf(ctx, "Submission ended with FAILED_PERMANENT: %s", err)
		return &eventpb.SubmissionCompleted{
			Result: eventpb.SubmissionResult_FAILED_PERMANENT,
		}
	}
}

const (
	failedPreconditionMsgFmt = "Gerrit rejected submission of this CL with " +
		"error: %s\nHint: rebasing CL in Gerrit UI and re-submitting usually " +
		"works"
	gerritTimeoutMsg = "Failed to submit this CL because Gerrit took too " +
		"long to respond. If this error persists after retries, rebasing and " +
		"then re-submitting your CL might help."
	permDeniedMsg = "Failed to submit this CL due to permission denied. " +
		"Please contact your project admin to grant the submit access to " +
		"your LUCI project scoped account in Gerrit config."
	resourceExhaustedMsg = "Failed to submit this CL because Gerrit " +
		"throttled the submission."
	unexpectedMsgFmt = "Failed to submit this CL because of unexpected error " +
		"from Gerrit: %s"
)

// classifyGerritErr returns the message to be posted on the CL for the given
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
		// Gerrit returns 409. Either because the change can't be merged, or
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
