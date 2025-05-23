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

package longops

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

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
	"go.chromium.org/luci/cv/internal/gerrit"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/eventpb"
	"go.chromium.org/luci/cv/internal/run/impl/util"
)

// PostGerritMessageOp posts the given message to gerrit.
//
// PostGerritMessageOp is a single-use object.
type PostGerritMessageOp struct {
	// All public fields must be set.
	*Base
	GFactory gerrit.Factory
	Env      *common.Env

	// These private fields are set internally as implementation details.
	lock           sync.Mutex
	latestPostedAt time.Time
}

// Do actually posts the message.
func (op *PostGerritMessageOp) Do(ctx context.Context) (*eventpb.LongOpCompleted, error) {
	op.assertCalledOnce()

	if op.IsCancelRequested() {
		return &eventpb.LongOpCompleted{Status: eventpb.LongOpCompleted_CANCELLED}, nil
	}

	rcls, err := run.LoadRunCLs(ctx, op.Run.ID, op.Run.CLs)
	if err != nil {
		return nil, err
	}

	if op.Op.GetDeadline() == nil {
		panic(errors.New("PostGerritMessageOp: missing deadline"))
	}

	errs := make(errors.MultiError, len(rcls))
	poolError := parallel.WorkPool(min(len(rcls), 8), func(work chan<- func() error) {
		for i, rcl := range rcls {
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
		panic(fmt.Errorf("unexpected WorkPool error %w", poolError))
	}

	var hasCancelled, hasFailed bool
	for i, err := range errs {
		switch {
		case err == nil:
		case errors.Unwrap(err) == errCancelHonored:
			hasCancelled = true
		default:
			hasFailed = true
			logging.Warningf(ctx, "failed to post gerrit message on CL %d %q: %s",
				rcls[i].ID, rcls[i].ExternalID, err)
		}
	}
	result := &eventpb.LongOpCompleted{
		Result: &eventpb.LongOpCompleted_PostGerritMessage_{
			PostGerritMessage: &eventpb.LongOpCompleted_PostGerritMessage{},
		},
	}
	switch {
	case hasFailed:
		result.Status = eventpb.LongOpCompleted_FAILED
	case hasCancelled:
		result.Status = eventpb.LongOpCompleted_CANCELLED
	default:
		result.Status = eventpb.LongOpCompleted_SUCCEEDED
		result.GetPostGerritMessage().Time = timestamppb.New(op.latestPostedAt)
	}
	// doCL() retries on transient failures until 30 secs before the op
	// deadline. If any failure cases, this returns nil in error to prevent
	// the TQ task from being retried.
	return result, nil
}

func (op *PostGerritMessageOp) doCL(ctx context.Context, rcl *run.RunCL) (time.Time, error) {
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
		switch postedAt, err = util.IsActionTakenOnGerritCL(ctx, op.GFactory, rcl, queryOpts, op.hasGerritMessagePosted); {
		case postedAt != notPosted:
			logging.Debugf(ctx, "PostGerritMessageOp: the CL already has this message at %s", postedAt)
			return nil
		case err == nil:
		case errors.Unwrap(err) != context.DeadlineExceeded:
			lastNonDeadlineErr = err
			fallthrough
		default:
			return errors.Annotate(err, "failed to check if message was already posted").Err()
		}
		switch err = util.MutateGerritCL(ctx, op.GFactory, rcl, req, 2*time.Minute, "post-gerrit-message"); {
		case err == nil:
			// NOTE: to avoid another round-trip to Gerrit, use the CV time here even
			// though it isn't the same as what Gerrit recorded.
			postedAt = clock.Now(ctx).Truncate(time.Second)
		case errors.Unwrap(err) != context.DeadlineExceeded:
			lastNonDeadlineErr = err
			fallthrough
		default:
			logging.Debugf(ctx, "PostGerritMessageOp: failed to mutate Gerrit CL: %s", err)
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

// hasGerritMessagePosted returns when the gerrit message was posted on a CL or
// zero time.
func (op *PostGerritMessageOp) hasGerritMessagePosted(rcl *run.RunCL, ci *gerritpb.ChangeInfo) time.Time {
	// Look at latest messages first for efficiency,
	// and skip all messages which are too old.
	clTriggeredAt := rcl.Trigger.Time.AsTime()
	msg := strings.TrimSpace(op.Op.GetPostGerritMessage().GetMessage())
	for i := len(ci.GetMessages()) - 1; i >= 0; i-- {
		switch m := ci.GetMessages()[i]; {
		case m.GetDate().AsTime().Before(clTriggeredAt):
			return notPosted
		// Gerrit might prepend some metadata around the posted msg such as the
		// patchset this is posted on to the msg. We use contains check instead
		// of equality to work around this.
		case strings.Contains(m.Message, msg):
			// This msg has been already posted to gerrit.
			return m.GetDate().AsTime()
		}
	}

	return notPosted
}

func (op *PostGerritMessageOp) makeSetReviewReq(rcl *run.RunCL) (*gerritpb.SetReviewRequest, error) {
	return &gerritpb.SetReviewRequest{
		Project:    rcl.Detail.GetGerrit().GetInfo().GetProject(),
		Number:     rcl.Detail.GetGerrit().GetInfo().GetNumber(),
		RevisionId: rcl.Detail.GetGerrit().GetInfo().GetCurrentRevision(),
		Tag:        op.Run.Mode.GerritMessageTag(),
		Notify:     gerritpb.Notify_NOTIFY_NONE,
		Message:    op.Op.GetPostGerritMessage().GetMessage(),
	}, nil
}

func (op *PostGerritMessageOp) makeRetryFactory() retry.Factory {
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
