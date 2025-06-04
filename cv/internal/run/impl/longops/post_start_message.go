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

package longops

import (
	"context"
	"fmt"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/common/lease"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/gerrit"
	"go.chromium.org/luci/cv/internal/gerrit/botdata"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/eventpb"
	"go.chromium.org/luci/cv/internal/run/impl/util"
	"go.chromium.org/luci/cv/internal/usertext"
)

// notPosted means start message wasn't posted yet.
//
// exists for readability only.
var notPosted time.Time

// PostStartMessageOp posts a start message on each of the Run CLs.
//
// PostStartMessageOp is a single-use object.
type PostStartMessageOp struct {
	// All public fields must be set.

	*Base
	GFactory gerrit.Factory
	Env      *common.Env

	// These private fields are set internally as implementation details.

	rcls []*run.RunCL
	cfg  *prjcfg.ConfigGroup

	lock           sync.Mutex
	latestPostedAt time.Time
}

// Do actually posts the start messages.
func (op *PostStartMessageOp) Do(ctx context.Context) (*eventpb.LongOpCompleted, error) {
	op.assertCalledOnce()

	if op.IsCancelRequested() {
		return &eventpb.LongOpCompleted{Status: eventpb.LongOpCompleted_CANCELLED}, nil
	}
	if err := op.prepare(ctx); err != nil {
		return nil, err
	}
	if op.Op.GetDeadline() == nil {
		panic(errors.New("PostStartMessageOp: missing deadline"))
	}

	errs := make(errors.MultiError, len(op.rcls))
	poolError := parallel.WorkPool(min(len(op.rcls), 8), func(work chan<- func() error) {
		for i, rcl := range op.rcls {
			if op.Run.HasRootCL() && rcl.ID != op.Run.RootCL {
				continue
			}
			work <- func() error {
				switch posted, err := op.doCL(ctx, rcl); {
				case err != nil:
					// all errors will be aggregated across all CLs below.
					errs[i] = err
				default:
					op.lock.Lock()
					if op.latestPostedAt.IsZero() || op.latestPostedAt.Before(posted) {
						op.latestPostedAt = posted
					}
					op.lock.Unlock()
				}
				return nil
			}
		}
	})
	if poolError != nil {
		panic(fmt.Errorf("unexpected WorkPool error %s", poolError))
	}

	var hasCancelled, hasFailed bool
	for i, err := range errs {
		switch {
		case err == nil:
		case errors.Unwrap(err) == errCancelHonored:
			hasCancelled = true
		default:
			hasFailed = true
			logging.Warningf(ctx, "failed to post start message on CL %d %q: %s",
				op.rcls[i].ID, op.rcls[i].ExternalID, err)
		}
	}
	result := &eventpb.LongOpCompleted{
		Result: &eventpb.LongOpCompleted_PostStartMessage_{
			PostStartMessage: &eventpb.LongOpCompleted_PostStartMessage{},
		},
	}
	switch {
	case hasFailed:
		result.Status = eventpb.LongOpCompleted_FAILED
	case hasCancelled:
		result.Status = eventpb.LongOpCompleted_CANCELLED
	default:
		result.Status = eventpb.LongOpCompleted_SUCCEEDED
		result.GetPostStartMessage().Time = timestamppb.New(op.latestPostedAt)
	}
	// doCL() retries on transient failures until 30 secs before the op
	// deadline. If any failure cases, this returns nil in error to prevent
	// the TQ task from being retried.
	return result, nil
}

func (op *PostStartMessageOp) prepare(ctx context.Context) error {
	eg, eCtx := errgroup.WithContext(ctx)
	eg.Go(func() (err error) {
		op.rcls, err = run.LoadRunCLs(eCtx, op.Run.ID, op.Run.CLs)
		return
	})
	eg.Go(func() (err error) {
		op.cfg, err = prjcfg.GetConfigGroup(eCtx, op.Run.ID.LUCIProject(), op.Run.ConfigGroupID)
		return
	})
	if err := eg.Wait(); err != nil {
		return err
	}

	return nil
}

func (op *PostStartMessageOp) doCL(ctx context.Context, rcl *run.RunCL) (time.Time, error) {
	ctx = logging.SetField(ctx, "cl", rcl.ID)
	if rcl.Detail.GetGerrit() == nil {
		panic(fmt.Errorf("CL %d is not a Gerrit CL", rcl.ID))
	}
	req, err := op.makeSetReviewReq(rcl)
	if err != nil {
		return notPosted, err
	}
	var lastNonDeadlineErr error
	var postedAt time.Time
	queryOpts := []gerritpb.QueryOption{gerritpb.QueryOption_MESSAGES}
	err = retry.Retry(clock.Tag(ctx, common.LaunchRetryClockTag), op.makeRetryFactory(), func() error {
		if op.IsCancelRequested() {
			return errCancelHonored
		}

		var err error
		switch postedAt, err = util.IsActionTakenOnGerritCL(ctx, op.GFactory, rcl, queryOpts, op.hasStartMessagePosted); {
		case postedAt != notPosted:
			logging.Debugf(ctx, "PostStartMessageOp: the CL already has the starting message at %s", postedAt)
			return nil
		case err == nil:
		case errors.Unwrap(err) != context.DeadlineExceeded:
			lastNonDeadlineErr = err
			fallthrough
		default:
			return errors.Fmt("failed to check if message was already posted: %w", err)
		}
		switch err = util.MutateGerritCL(ctx, op.GFactory, rcl, req, 2*time.Minute, "post-start-message"); {
		case err == nil:
			// NOTE: to avoid another round-trip to Gerrit, use the CV time here even
			// though it isn't the same as what Gerrit recorded.
			postedAt = clock.Now(ctx).Truncate(time.Second)
		case errors.Unwrap(err) != context.DeadlineExceeded:
			lastNonDeadlineErr = err
			fallthrough
		default:
			logging.Debugf(ctx, "PostStartMessageOp: failed to mutate Gerrit CL: %s", err)
		}
		return err
	}, nil)

	switch {
	case err == nil:
		return postedAt, nil
	case errors.Unwrap(err) == context.DeadlineExceeded && lastNonDeadlineErr != nil:
		// if the deadline error occurred after retries, then returns the last
		// error before the deadline error. It should be more informative than
		// `context deadline exceeded`.
		return notPosted, lastNonDeadlineErr
	default:
		return notPosted, err
	}
}

// hasStartMessagePosted returns when the start message was posted on a CL or
// zero time.
func (op *PostStartMessageOp) hasStartMessagePosted(rcl *run.RunCL, ci *gerritpb.ChangeInfo) time.Time {
	// Look at latest messages first for efficiency,
	// and skip all messages which are too old.
	clTriggeredAt := rcl.Trigger.Time.AsTime()
	for i := len(ci.GetMessages()) - 1; i >= 0; i-- {
		m := ci.GetMessages()[i]
		t := m.GetDate().AsTime()
		if t.Before(clTriggeredAt) {
			// i-th message is too old, no need to check even older ones.
			return notPosted
		}
		switch data, ok := botdata.Parse(m); {
		case !ok:
		case data.Action != botdata.Start:
		case data.TriggeredAt != clTriggeredAt:
		case data.Revision != rcl.Detail.GetGerrit().GetInfo().GetCurrentRevision():
		default:
			return t
		}
	}
	return notPosted
}

func (op *PostStartMessageOp) makeSetReviewReq(rcl *run.RunCL) (*gerritpb.SetReviewRequest, error) {
	// TODO(yiwzhang): for root CL, include link to all RunCLs in the start
	// message so that user would have a full view understanding what's included
	// in the run and what's not.
	humanMsg := usertext.OnRunStartedGerritMessage(op.Run, op.cfg, op.Env)
	bd := botdata.BotData{
		Action:      botdata.Start,
		Revision:    rcl.Detail.GetGerrit().GetInfo().GetCurrentRevision(),
		TriggeredAt: rcl.Trigger.Time.AsTime(),
	}
	msg, err := botdata.Append(humanMsg, bd)
	if err != nil {
		return nil, errors.Fmt("failed to generate the starting message: %w", err)
	}
	return &gerritpb.SetReviewRequest{
		Project:                          rcl.Detail.GetGerrit().GetInfo().GetProject(),
		Number:                           rcl.Detail.GetGerrit().GetInfo().GetNumber(),
		RevisionId:                       rcl.Detail.GetGerrit().GetInfo().GetCurrentRevision(),
		Tag:                              op.Run.Mode.GerritMessageTag(),
		Notify:                           gerritpb.Notify_NOTIFY_NONE,
		Message:                          msg,
		IgnoreAutomaticAttentionSetRules: true,
	}, nil
}

func (op *PostStartMessageOp) makeRetryFactory() retry.Factory {
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
