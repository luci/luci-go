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

	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/impl/state"
)

// Poke implements Handler interface.
func (impl *Impl) Poke(ctx context.Context, rs *state.RunState) (*Result, error) {
	if shouldCheckTree(ctx, rs.Run.Status, rs.Run.Submission) {
		rs = rs.ShallowCopy()
		switch open, err := rs.CheckTree(ctx, impl.TreeClient); {
		case err != nil:
			return nil, err
		case !open:
			// check again after 1 minute.
			if err := impl.RM.PokeAfter(ctx, rs.Run.ID, 1*time.Minute); err != nil {
				return nil, err
			}
			return &Result{State: rs}, nil
		default:
			return impl.OnReadyForSubmission(ctx, rs)
		}
	}
	return &Result{State: rs}, nil
}

func shouldCheckTree(ctx context.Context, st run.Status, sub *run.Submission) bool {
	return st == run.Status_WAITING_FOR_SUBMISSION &&
		// Tree was closed in last tree check.
		(sub.GetLastTreeCheckTime() != nil && !sub.GetTreeOpen()) &&
		// Last Tree check was more than 1 minute ago so that CV doesn't call
		// Tree App too often.
		clock.Now(ctx).Sub(sub.GetLastTreeCheckTime().AsTime()) >= 1*time.Minute
}
