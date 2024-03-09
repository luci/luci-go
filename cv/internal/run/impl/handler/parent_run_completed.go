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

package handler

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/impl/state"
)

// OnParentRunCompleted implements Handler interface.
func (impl *Impl) OnParentRunCompleted(ctx context.Context, rs *state.RunState) (*Result, error) {
	if run.IsEnded(rs.Status) {
		// Run already failed or was already cancelled.
		return &Result{State: rs}, nil
	}

	runs, errs := run.LoadRunsFromIDs(rs.DepRuns...).Do(ctx)
	for i, r := range runs {
		switch err := errs[i]; {
		case err == datastore.ErrNoSuchEntity:
			panic(err)
		case err != nil:
			return nil, errors.Annotate(err, "failed to load run %s", r.ID).Tag(transient.Tag).Err()
		default:
			switch r.Status {
			case run.Status_STATUS_UNSPECIFIED:
				err := errors.Reason("CRITICAL: can't start a Run %q, parent Run %s has unspecified status", rs.ID, r.ID).Err()
				common.LogError(ctx, err)
				panic(err)
			case run.Status_FAILED, run.Status_CANCELLED:
				return impl.onParentRunUnsuccessful(ctx, rs, r)
			case run.Status_SUCCEEDED:
				// If all rs.RunDeps were successful, we will submit this run.
			default:
				if !run.IsEnded(r.Status) {
					// If not all rs.RunDeps have ended there's nothing to do.
					return &Result{State: rs}, nil
				}
				panic(fmt.Sprintf("Unknown status %q for a RunDep %s", r.Status, r.ID))
			}
		}
	}

	// If we get this far, it means all parent runs have succeeded.
	// We can submit this run.
	if rs.Status == run.Status_WAITING_FOR_SUBMISSION {
		return &Result{
			State: rs,
			SideEffectFn: func(ctx context.Context) error {
				if err := impl.RM.NotifyReadyForSubmission(ctx, rs.ID, time.Time{}); err != nil {
					return err
				}
				return nil
			},
		}, nil
	}
	// All parents have succeeded but rs is not ready to be submitted yet.
	return &Result{State: rs}, nil
}

func (impl *Impl) onParentRunUnsuccessful(ctx context.Context, rs *state.RunState, pr *run.Run) (*Result, error) {
	if rs.Status == run.Status_SUBMITTING {
		panic(fmt.Errorf(
			"onParentRunUnsuccessful: run status is SUBMITTING, but at least one parent run %s has status %v", pr.ID, pr.Status))
	}

	rs = rs.ShallowCopy()
	rims := make(map[common.CLID]reviewInputMeta, len(rs.CLs))
	whoms := rs.Mode.GerritNotifyTargets()
	meta := reviewInputMeta{
		notify: whoms,
		// Add the same set of group/people to the attention set.
		addToAttention: whoms,
		reason:         fmt.Sprintf("Cancelled this run because parent run %v", pr.Status),
		message:        "Cancelled because an upstream CL run failed or was cancelled.",
	}
	for _, id := range rs.CLs {
		if !rs.HasRootCL() || rs.RootCL == id {
			rims[id] = meta
		}
	}
	scheduleTriggersReset(ctx, rs, rims, run.Status_CANCELLED)
	return &Result{
		State: rs,
	}, nil
}
