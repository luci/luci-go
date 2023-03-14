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

package postaction

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/luci/common/errors"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/common/lease"
	"go.chromium.org/luci/cv/internal/gerrit"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/impl/util"
)

// Executor executes a PostAction for a Run termination event.
type Executor struct {
	GFactory          gerrit.Factory
	Run               *run.Run
	Payload           *run.OngoingLongOps_Op_ExecutePostActionPayload
	IsCancelRequested func() bool

	actName string // for logging
	// TODO(ddoman): report run log

	// test function for unit test.
	//
	// Called before CL mutation.
	testBeforeCLMutation func(ctx context.Context, rcl *run.RunCL, req *gerritpb.SetReviewRequest)
}

// Do executes the payload.
func (exe *Executor) Do(ctx context.Context) error {
	act := exe.Payload.GetAction()
	exe.actName = fmt.Sprintf("post-action-%s", act.GetName())

	// determine the action handler.
	switch w := act.GetAction().(type) {
	case *cfgpb.ConfigGroup_PostAction_VoteGerritLabels_:
		return exe.voteGerritLabels(ctx, w.VoteGerritLabels.GetVotes())
	case nil:
		panic(errors.New("action == nil"))
	default:
		panic(errors.Reason("unknown action type %q", w).Err())
	}
}

func (exe *Executor) makeVoteLabelRetryFactory() retry.Factory {
	return lease.RetryIfLeased(transient.Only(func() retry.Iterator {
		return &retry.ExponentialBackoff{
			Limited: retry.Limited{
				Delay:   100 * time.Millisecond,
				Retries: -1, // unlimited
			},
			Multiplier: 2,
			MaxDelay:   1 * time.Minute,
		}
	}))
}

func (exe *Executor) voteGerritLabels(ctx context.Context, votes []*cfgpb.ConfigGroup_PostAction_VoteGerritLabels_Vote) error {
	if exe.IsCancelRequested() {
		return errors.Reason("CancelRequested for Run %q", exe.Run.ID).Err()
	}
	if len(votes) == 0 {
		// This should never happen, as the validation requires the config to
		// have at least one vote in the message.
		return errors.Reason("no votes in the config").Err()
	}
	labelsToSet := make(map[string]int32, len(votes))
	for _, vote := range votes {
		labelsToSet[vote.GetName()] = vote.GetValue()
	}

	// TODO(ddoman): skip the votes, if any of the snapshot(s) changed.
	rcls, err := run.LoadRunCLs(ctx, exe.Run.ID, exe.Run.CLs)
	if err != nil {
		return err
	}

	// This retries the Gerrit APIs by itself until the context is cancelled, or
	// all the executions are permanent failed/succeeded.
	return parallel.WorkPool(min(len(rcls), 8), func(work chan<- func() error) {
		for _, rcl := range rcls {
			rcl := rcl
			work <- func() error {
				req := &gerritpb.SetReviewRequest{
					Project:    rcl.Detail.GetGerrit().GetInfo().GetProject(),
					Number:     rcl.Detail.GetGerrit().GetInfo().GetNumber(),
					RevisionId: rcl.Detail.GetGerrit().GetInfo().GetCurrentRevision(),
					Notify:     gerritpb.Notify_NOTIFY_NONE,
					Labels:     labelsToSet,
				}
				return retry.Retry(ctx, exe.makeVoteLabelRetryFactory(), func() error {
					if exe.testBeforeCLMutation != nil {
						exe.testBeforeCLMutation(ctx, rcl, req)
					}
					if exe.IsCancelRequested() {
						return errors.Reason("CL %d: CancelRequested for Run %q", rcl.ID, exe.Run.ID).Err()
					}
					return util.MutateGerritCL(ctx, exe.GFactory, rcl, req, 2*time.Minute, exe.actName)
				}, nil)
			}
		}
	})
}

func min(i, j int) int {
	if i < j {
		return i
	}
	return j
}
