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

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/eventpb"
	"go.chromium.org/luci/cv/internal/run/impl/state"
	"go.chromium.org/luci/cv/internal/run/impl/submit"
)

// OnReadyForSubmission implements Handler interface.
func (*Impl) OnReadyForSubmission(ctx context.Context, rs *state.RunState) (*Result, error) {
	// TODO(yiwzhang): implement
	return &Result{State: rs}, nil
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
