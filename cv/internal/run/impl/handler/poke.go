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

	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/impl/state"
	"go.chromium.org/luci/cv/internal/tryjob"
)

const (
	treeCheckInterval          = time.Minute
	clRefreshInterval          = 10 * time.Minute
	tryjobRefreshInterval      = 150 * time.Second
	treeStatusFailureTimeLimit = 10 * time.Minute
)

// Poke implements Handler interface.
func (impl *Impl) Poke(ctx context.Context, rs *state.RunState) (*Result, error) {
	if !run.IsEnded(rs.Status) && clock.Since(ctx, rs.CreateTime) > common.MaxRunTotalDuration {
		return impl.Cancel(ctx, rs, []string{
			fmt.Sprintf("max run duration of %s has reached", common.MaxRunTotalDuration),
		})
	}

	rs = rs.ShallowCopy()
	if shouldCheckTree(ctx, rs.Status, rs.Submission) {
		rs.CloneSubmission()
		switch open, err := rs.CheckTree(ctx, impl.TreeClient); {
		case err != nil && clock.Since(ctx, rs.Submission.TreeErrorSince.AsTime()) > treeStatusFailureTimeLimit:
			// The tree has been erroring for too long. Reset the triggers and
			// fail the run.
			cg, err := prjcfg.GetConfigGroup(ctx, rs.ID.LUCIProject(), rs.ConfigGroupID)
			if err != nil {
				return nil, err
			}
			rims := make(map[common.CLID]reviewInputMeta, len(rs.CLs))
			whoms := rs.Mode.GerritNotifyTargets()
			for _, cid := range rs.CLs {
				rims[common.CLID(cid)] = reviewInputMeta{
					notify: whoms,
					// Add the same set of group/people to the attention set.
					addToAttention: whoms,
					reason:         submissionFailureAttentionReason,
					message:        fmt.Sprintf(persistentTreeStatusAppFailureTemplate, cg.Content.GetVerifiers().GetTreeStatus().GetUrl()),
				}
			}
			scheduleTriggersReset(ctx, rs, rims, run.Status_FAILED)
			return &Result{
				State: rs,
			}, nil
		case err != nil:
			logging.Warningf(ctx, "tree status check failed with error: %s", err)
			fallthrough
		case !open:
			if err := impl.RM.PokeAfter(ctx, rs.ID, treeCheckInterval); err != nil {
				return nil, err
			}
		default:
			return impl.OnReadyForSubmission(ctx, rs)
		}
	}

	// If it's scheduled to be cancelled, skip the refresh.
	// The long op might have been expired, but it should be removed at the end
	// of this call first, and then the next Poke() will run this check again.
	if !isCurrentlyResettingTriggers(rs) && shouldRefreshCLs(ctx, rs) {
		cg, runCLs, cls, err := loadCLsAndConfig(ctx, rs, rs.CLs)
		if err != nil {
			return nil, err
		}
		switch ok, err := checkRunCreate(ctx, rs, cg, runCLs, cls); {
		case err != nil:
			return nil, err
		case ok:
			if err := impl.CLUpdater.ScheduleBatch(
				ctx, rs.ID.LUCIProject(), cls,
				changelist.UpdateCLTask_RUN_POKE); err != nil {
				return nil, err
			}
			rs.LatestCLsRefresh = datastore.RoundTime(clock.Now(ctx).UTC())
		}
	}

	if shouldRefreshTryjobs(ctx, rs) {
		executions := rs.Tryjobs.GetState().GetExecutions()
		errs := errors.NewLazyMultiError(len(executions))
		poolErr := parallel.WorkPool(min(8, len(executions)), func(workCh chan<- func() error) {
			for i, execution := range executions {
				// Only care about the latest attempt with the assumption that all
				// earlier attempt should have been ended already.
				switch latestAttempt := tryjob.LatestAttempt(execution); {
				case latestAttempt == nil:
				case latestAttempt.GetExternalId() == "":
				case latestAttempt.GetStatus() == tryjob.Status_TRIGGERED:
					// Only update Tryjob if it has been triggered at the Tryjob backend.
					i := i
					workCh <- func() error {
						errs.Assign(i, impl.TN.ScheduleUpdate(ctx,
							common.TryjobID(latestAttempt.GetTryjobId()),
							tryjob.ExternalID(latestAttempt.GetExternalId())))
						return nil
					}
				}

			}
		})
		switch {
		case poolErr != nil:
			panic(poolErr)
		case errs.Get() != nil:
			return nil, common.MostSevereError(errs.Get())
		default:
			rs.LatestTryjobsRefresh = datastore.RoundTime(clock.Now(ctx).UTC())
		}
	}

	return impl.processExpiredLongOps(ctx, rs)
}

func shouldCheckTree(ctx context.Context, st run.Status, sub *run.Submission) bool {
	switch {
	case st != run.Status_WAITING_FOR_SUBMISSION:
	case sub.GetLastTreeCheckTime() == nil:
		return true
	case !sub.GetTreeOpen():
		return clock.Now(ctx).Sub(sub.GetLastTreeCheckTime().AsTime()) >= treeCheckInterval
	}
	return false
}

func shouldRefreshCLs(ctx context.Context, rs *state.RunState) bool {
	return shouldRefresh(ctx, rs, rs.LatestCLsRefresh, clRefreshInterval)
}

func shouldRefreshTryjobs(ctx context.Context, rs *state.RunState) bool {
	return shouldRefresh(ctx, rs, rs.LatestTryjobsRefresh, tryjobRefreshInterval)
}

func shouldRefresh(ctx context.Context, rs *state.RunState, last time.Time, interval time.Duration) bool {
	switch {
	case run.IsEnded(rs.Status):
		return false
	case last.IsZero():
		last = rs.CreateTime
		fallthrough
	default:
		return clock.Since(ctx, last) > interval
	}
}
