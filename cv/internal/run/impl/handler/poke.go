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
	"go.chromium.org/luci/gae/service/datastore"

	commonpb "go.chromium.org/luci/cv/api/common/v1"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/impl/state"
)

const (
	treeCheckInterval = time.Minute
	clRefreshInterval = 10 * time.Minute
)

// Poke implements Handler interface.
func (impl *Impl) Poke(ctx context.Context, rs *state.RunState) (*Result, error) {
	if shouldCheckTree(ctx, rs.Run.Status, rs.Run.Submission) {
		rs = rs.ShallowCopy()
		switch open, err := rs.CheckTree(ctx, impl.TreeClient); {
		case err != nil:
			return nil, err
		case !open:
			if err := impl.RM.PokeAfter(ctx, rs.Run.ID, treeCheckInterval); err != nil {
				return nil, err
			}
		default:
			return impl.OnReadyForSubmission(ctx, rs)
		}
	}

	if shouldRefreshCLs(ctx, rs) {
		switch cls, err := changelist.LoadCLs(ctx, rs.Run.CLs); {
		case err != nil:
			return nil, err
		default:
			if err := impl.CLUpdater.ScheduleBatch(ctx, rs.Run.ID.LUCIProject(), cls); err != nil {
				return nil, err
			}
			rs = rs.ShallowCopy()
			rs.Run.LatestCLsRefresh = datastore.RoundTime(clock.Now(ctx).UTC())
		}
	}
	return &Result{State: rs}, nil
}

func shouldCheckTree(ctx context.Context, st commonpb.Run_Status, sub *run.Submission) bool {
	return st == commonpb.Run_WAITING_FOR_SUBMISSION &&
		// Tree was closed during the last Tree check.
		(sub.GetLastTreeCheckTime() != nil && !sub.GetTreeOpen()) &&
		// Avoid too frequent refreshes of the Tree.
		clock.Now(ctx).Sub(sub.GetLastTreeCheckTime().AsTime()) >= treeCheckInterval
}

func shouldRefreshCLs(ctx context.Context, rs *state.RunState) bool {
	if run.IsEnded(rs.Run.Status) {
		return false
	}
	last := rs.Run.LatestCLsRefresh
	if last.IsZero() {
		last = rs.Run.CreateTime
	}
	return clock.Since(ctx, last) > clRefreshInterval
}
