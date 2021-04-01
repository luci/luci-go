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

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/eventpb"
	"go.chromium.org/luci/cv/internal/run/impl/state"
	"go.chromium.org/luci/cv/internal/run/impl/submit"
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
		s := submitter{}
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

type submitter struct {
}

func (s submitter) submit(ctx context.Context) error {
	return errors.New("not implemented")
}
