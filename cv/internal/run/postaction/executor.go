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
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/quota/quotapb"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/common/lease"
	"go.chromium.org/luci/cv/internal/gerrit"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/impl/util"
)

var errOpCancel = errors.New("CL mutation aborted due to op cancellation")

// Executor executes a PostAction for a Run termination event.
type Executor struct {
	GFactory          gerrit.Factory
	Run               *run.Run
	Payload           *run.OngoingLongOps_Op_ExecutePostActionPayload
	RM                rm
	QM                qm
	IsCancelRequested func() bool

	// test function for unit test.
	//
	// Called before CL mutation.
	testBeforeCLMutation func(ctx context.Context, rcl *run.RunCL, req *gerritpb.SetReviewRequest)
}

// qm encapsulates interaction with Quota Manager by the post action executor.
type qm interface {
	RunQuotaAccountID(r *run.Run) *quotapb.AccountID
	CreditRunQuota(ctx context.Context, r *run.Run) (*quotapb.OpResult, *cfgpb.UserLimit, error)
}

// rm encapsulates interaction with Run Manager by the post action executor.
type rm interface {
	Start(ctx context.Context, runID common.RunID) error
}

// Do executes the payload.
//
// Returns a summary of the action execution, and the error.
// The summary is meant to be added to run.LogEntries.
func (exe *Executor) Do(ctx context.Context) (string, error) {
	if exe.IsCancelRequested() {
		return "cancellation has been requested before the post action starts",
			errors.Reason("CancelRequested for Run %q", exe.Run.ID).Err()
	}
	switch k := exe.Payload.Kind.(type) {
	case *run.OngoingLongOps_Op_ExecutePostActionPayload_ConfigAction:
		// determine the action handler.
		switch a := k.ConfigAction.GetAction().(type) {
		case *cfgpb.ConfigGroup_PostAction_VoteGerritLabels_:
			return exe.voteGerritLabels(ctx, a.VoteGerritLabels.GetVotes())
		case nil:
			panic(errors.New("action == nil"))
		default:
			panic(fmt.Errorf("unknown action type %T", a))
		}
	case *run.OngoingLongOps_Op_ExecutePostActionPayload_CreditRunQuota_:
		return exe.creditQuota(ctx)
	default:
		panic(fmt.Errorf("unknown post action payload kind %T", k))
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

func (exe *Executor) voteGerritLabels(ctx context.Context, votes []*cfgpb.ConfigGroup_PostAction_VoteGerritLabels_Vote) (string, error) {
	if len(votes) == 0 {
		// This should never happen, as the validation requires the config to
		// have at least one vote in the message.
		return "", errors.Reason("no votes in the config").Err()
	}
	rcls, err := run.LoadRunCLs(ctx, exe.Run.ID, exe.Run.CLs)
	if err != nil {
		return "", err
	}

	// This retries the Gerrit APIs by itself until the context is cancelled, or
	// all the executions are permanent failed/succeeded.
	errs := make(errors.MultiError, len(rcls))
	perr := parallel.WorkPool(min(len(rcls), 8), func(work chan<- func() error) {
		labelsToSet := make(map[string]int32, len(votes))
		for _, vote := range votes {
			labelsToSet[vote.GetName()] = vote.GetValue()
		}
		tag := gerrit.Tag("post-action:vote-gerrit-labels", string(exe.Run.ID))
		for i, rcl := range rcls {
			i, rcl := i, rcl
			work <- func() error {
				req := &gerritpb.SetReviewRequest{
					Project:    rcl.Detail.GetGerrit().GetInfo().GetProject(),
					Number:     rcl.Detail.GetGerrit().GetInfo().GetNumber(),
					RevisionId: rcl.Detail.GetGerrit().GetInfo().GetCurrentRevision(),
					Notify:     gerritpb.Notify_NOTIFY_NONE,
					Labels:     labelsToSet,
					Tag:        tag,
				}
				errs[i] = retry.Retry(ctx, exe.makeVoteLabelRetryFactory(), func() error {
					if exe.testBeforeCLMutation != nil {
						exe.testBeforeCLMutation(ctx, rcl, req)
					}
					if exe.IsCancelRequested() {
						return errOpCancel
					}
					err := util.MutateGerritCL(ctx, exe.GFactory, rcl, req, 2*time.Minute,
						fmt.Sprintf("post-action-%s", exe.Payload.GetName()))
					if status.Code(err) == codes.FailedPrecondition && hasCLAbandoned(ctx, rcl) {
						logging.Infof(ctx, "CL %d is abandoned; skip post action", rcl.ID)
						return nil
					}
					return err
				}, nil)
				return nil
			}
		}
	})
	if perr != nil {
		panic(errors.Reason("parallel.WorkPool returned error %q", perr).Err())
	}
	return exe.voteSummary(ctx, rcls, errs), errors.Flatten(errs)
}

func (exe *Executor) voteSummary(ctx context.Context, rcls []*run.RunCL, errs errors.MultiError) string {
	var failed, cancelled []changelist.ExternalID
	succeeded := make([]changelist.ExternalID, 0, len(rcls))
	for i, err := range errs {
		switch err {
		case nil:
			succeeded = append(succeeded, rcls[i].ExternalID)
		case errOpCancel:
			cancelled = append(cancelled, rcls[i].ExternalID)
		default:
			failed = append(failed, rcls[i].ExternalID)
			logging.Errorf(ctx, "failed to vote labels on CL %d: %s", rcls[i].ID, err)
		}
	}

	var s strings.Builder
	switch {
	case len(succeeded) == len(rcls):
		fmt.Fprint(&s, "all votes succeeded")
	case len(failed) == len(rcls):
		fmt.Fprint(&s, "all votes failed")
	case len(cancelled) == len(rcls):
		fmt.Fprint(&s, "all votes cancelled")
	default:
		fmt.Fprintf(&s, "Results for Gerrit label votes")
		if len(succeeded) > 0 {
			fmt.Fprintf(&s, "\n- succeeded: %s", changelist.JoinExternalURLs(succeeded, ", "))
		}
		if len(failed) > 0 {
			fmt.Fprintf(&s, "\n- failed: %s", changelist.JoinExternalURLs(failed, ", "))
		}
		if len(cancelled) > 0 {
			fmt.Fprintf(&s, "\n- cancelled: %s", changelist.JoinExternalURLs(cancelled, ", "))
		}
	}
	return s.String()
}

// hasCLAbandoned fetches the latest snapshot of the CL and returns whether
// the CL is abaondoned.
func hasCLAbandoned(ctx context.Context, rcl *run.RunCL) bool {
	latest := &changelist.CL{ID: rcl.ID}
	if err := datastore.Get(ctx, latest); err != nil {
		// return false so that the callsite can return the original error
		// from SetReview().
		return false
	}
	return latest.Snapshot.GetGerrit().GetInfo().GetStatus() == gerritpb.ChangeStatus_ABANDONED
}
