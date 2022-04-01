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

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/impl/state"
)

const (
	treeCheckInterval = time.Minute
	clRefreshInterval = 10 * time.Minute
)

// Poke implements Handler interface.
func (impl *Impl) Poke(ctx context.Context, rs *state.RunState) (*Result, error) {
	rs = rs.ShallowCopy()
	if shouldCheckTree(ctx, rs.Status, rs.Submission) {
		rs.Submission = proto.Clone(rs.Submission).(*run.Submission)
		switch open, err := rs.CheckTree(ctx, impl.TreeClient); {
		case err != nil:
			return nil, err
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
	if !isTriggersCancellationOngoing(rs) && shouldRefreshCLs(ctx, rs) {
		cg, runCLs, cls, err := loadCLsAndConfig(ctx, rs, rs.CLs)
		if err != nil {
			return nil, err
		}
		switch ok, err := checkRunCreate(ctx, rs, cg, runCLs, cls); {
		case err != nil:
			return nil, err
		case ok:
			if err := impl.CLUpdater.ScheduleBatch(ctx, rs.ID.LUCIProject(), cls); err != nil {
				return nil, err
			}
			rs.LatestCLsRefresh = datastore.RoundTime(clock.Now(ctx).UTC())
		}
	}
	return impl.processExpiredLongOps(ctx, rs)
}

func shouldCheckTree(ctx context.Context, st run.Status, sub *run.Submission) bool {
	return st == run.Status_WAITING_FOR_SUBMISSION &&
		// Tree was closed during the last Tree check.
		(sub.GetLastTreeCheckTime() != nil && !sub.GetTreeOpen()) &&
		// Avoid too frequent refreshes of the Tree.
		clock.Now(ctx).Sub(sub.GetLastTreeCheckTime().AsTime()) >= treeCheckInterval
}

func shouldRefreshCLs(ctx context.Context, rs *state.RunState) bool {
	if run.IsEnded(rs.Status) {
		return false
	}
	last := rs.LatestCLsRefresh
	if last.IsZero() {
		last = rs.CreateTime
	}
	return clock.Since(ctx, last) > clRefreshInterval
}
