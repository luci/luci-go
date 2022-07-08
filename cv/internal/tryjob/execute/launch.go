// Copyright 2022 The LUCI Authors.
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

package execute

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/tryjob"
)

const launchRetryClockTag = "retry-launch"

// launchTryjobs launches the provided tryjobs against the backend system.
//
// All provided tryjobs MUST be in PENDING status and SHOULD NOT have
// external ID populated. If fails to launch any tryjob, the tryjob will be
// returned in UNTRIGGERED status and the failure reason will be written to
// `UntriggeredReason` field.
//
// It's possible that the returned tryjob has a different ID than the input
// Tryjob, that is typically due to CV discovers that the external id of the
// launched tryjob already maps to an existing record. The original Tryjob
// in the input will be set to untriggered.
func (w *worker) launchTryjobs(ctx context.Context, tryjobs []*tryjob.Tryjob) ([]*tryjob.Tryjob, error) {
	toBeLaunched := make([]*tryjob.Tryjob, len(tryjobs))
	for i, tj := range tryjobs {
		switch {
		case tj.Status != tryjob.Status_PENDING:
			panic(fmt.Errorf("expected PENDING status for tryjob %d; got %s", tj.ID, tj.Status))
		case tj.ExternalID != "":
			panic(fmt.Errorf("expected empty external ID for tryjob %d; got %s", tj.ID, tj.ExternalID))
		default:
			toBeLaunched[i] = tj
		}
	}

	launchFailures := make(map[*tryjob.Tryjob]error)

	_ = retry.Retry(clock.Tag(ctx, launchRetryClockTag), retryFactory, func() error {
		var hasFatal bool
		launchFailures, hasFatal = w.tryLaunchTryjobsOnce(ctx, toBeLaunched)
		switch {
		case len(launchFailures) == 0:
			return nil
		case hasFatal: // stop the retry
			return nil
		default: // all failures can be retried
			toBeLaunched = toBeLaunched[:0] // reuse existing slice
			for tj := range launchFailures {
				toBeLaunched = append(toBeLaunched, tj)
			}
			return errors.New("please retry") // returns an arbitrary error to retry
		}
	}, func(error, time.Duration) {
		var sb strings.Builder
		sb.WriteString("retrying following tryjobs:")
		for tj, err := range launchFailures {
			sb.WriteString("\n  * ")
			sb.WriteString(strconv.FormatInt(int64(tj.ID), 10))
			sb.WriteString(": ")
			sb.WriteString(err.Error())
		}
		logging.Warningf(ctx, sb.String())
	})

	for _, tj := range tryjobs {
		if err, ok := launchFailures[tj]; ok {
			tj.Status = tryjob.Status_UNTRIGGERED
			switch grpcStatus, ok := status.FromError(errors.Unwrap(err)); {
			case !ok:
				// log the error detail but don't leak the internal error.
				logging.Errorf(ctx, "unexpected internal error when launching tryjob: %s", err)
				tj.UntriggeredReason = "unexpected internal error"
			default:
				tj.UntriggeredReason = fmt.Sprintf("received %s from %s", grpcStatus.Code(), w.backend.Kind())
				if msg := grpcStatus.Message(); msg != "" {
					tj.UntriggeredReason += ". message: " + msg
				}
			}
		}
		tj.EVersion += 1
		tj.EntityUpdateTime = datastore.RoundTime(clock.Now(ctx).UTC())
	}
	return w.saveLaunchedTryjobs(ctx, tryjobs)
}

func (w *worker) tryLaunchTryjobsOnce(ctx context.Context, tryjobs []*tryjob.Tryjob) (failures map[*tryjob.Tryjob]error, hasFatal bool) {
	err := w.backend.Launch(ctx, tryjobs, w.run, w.cls)
	if err == nil {
		return nil, false
	}
	for i, err := range err.(errors.MultiError) {
		if err != nil {
			if failures == nil {
				failures = make(map[*tryjob.Tryjob]error, 1)
			}
			failures[tryjobs[i]] = err
			if !canRetryBackendError(err) {
				hasFatal = true
			}
		}
	}
	return failures, hasFatal
}

func canRetryBackendError(err error) bool {
	grpcStatus, ok := status.FromError(errors.Unwrap(err))
	return ok && retriableBackendErrorCodes[grpcStatus.Code()]
}

var retriableBackendErrorCodes = map[codes.Code]bool{
	codes.Internal:    true,
	codes.Unknown:     true,
	codes.Unavailable: true,
	// TODO(crbug.com/1274781): as a temporary mitigation, retry NotFound error
	// to workaround the race condition for config ingestion in buildbucket and
	// LUCI CV.
	codes.NotFound:          true,
	codes.ResourceExhausted: true,
	codes.DeadlineExceeded:  true,
}

func retryFactory() retry.Iterator {
	return &retry.ExponentialBackoff{
		Limited: retry.Limited{
			Delay:   100 * time.Millisecond,
			Retries: -1,
			// TODO(yiwzhang): Model this as triggering deadline instead.
			MaxTotal: 2 * time.Minute,
		},
		MaxDelay:   10 * time.Second,
		Multiplier: 2,
	}
}

func (w *worker) saveLaunchedTryjobs(ctx context.Context, tryjobs []*tryjob.Tryjob) ([]*tryjob.Tryjob, error) {
	var innerErr error
	var reconciled []*tryjob.Tryjob
	err := datastore.RunInTransaction(ctx, func(ctx context.Context) (err error) {
		defer func() { innerErr = err }()
		// It's possible (though unlikely) that the external ID of the launched
		// tryjob already has a record in CV. Reconcile to use the existing record
		// and drop the record in the input by resetting the data and set the
		// status to UNTRIGGERED.
		var dropped []*tryjob.Tryjob
		reconciled, dropped, err = reconcileWithExisting(ctx, tryjobs, w.run.ID)
		if err != nil {
			return err
		}
		if err := tryjob.SaveTryjobs(ctx, append(reconciled, dropped...), w.rm.NotifyTryjobsUpdated); err != nil {
			return err
		}
		return nil
	}, nil)
	switch {
	case innerErr != nil:
		return nil, innerErr
	case err != nil:
		return nil, errors.Annotate(err, "failed to commit transaction").Tag(transient.Tag).Err()
	default:
		return reconciled, nil
	}
}

func reconcileWithExisting(ctx context.Context, tryjobs []*tryjob.Tryjob, rid common.RunID) (reconciled, dropped []*tryjob.Tryjob, err error) {
	eids := make([]tryjob.ExternalID, 0, len(tryjobs))
	for _, tj := range tryjobs {
		if eid := tj.ExternalID; eid != "" {
			eids = append(eids, eid)
		}
	}
	if len(eids) == 0 {
		// nothing to reconcile.
		return tryjobs, nil, nil
	}
	existingTryjobs, err := tryjob.ResolveToTryjobs(ctx, eids...)
	if err != nil {
		return nil, nil, err
	}
	reconciled = make([]*tryjob.Tryjob, len(tryjobs))
	for i, tj := range tryjobs {
		if tj.ExternalID == "" {
			reconciled[i] = tj
			continue
		}
		// since eids are resolved following the order in tryjobs, the first entry
		// should always corresponds to this Tryjob.
		existing := existingTryjobs[0]
		existingTryjobs = existingTryjobs[1:]
		switch {
		case existing == nil:
			// the external id has no existing associated tryjob. This tryjob should
			// be safe to create.
			reconciled[i] = tj
		case existing.ExternalID != tj.ExternalID:
			panic(fmt.Errorf("impossible; expect %s, got %s", tj.ExternalID, existing.ExternalID))
		default:
			// update and use the existing tryjob instead.
			existing.EVersion += 1
			existing.EntityUpdateTime = datastore.RoundTime(clock.Now(ctx).UTC())
			existing.Status = tj.Status
			existing.Result = tj.Result
			reconciled[i] = existing
			if tj.ID != existing.ID {
				switch {
				case existing.TriggeredBy == rid:
					// expected
				case existing.ReusedBy.Index(rid) < 0:
					existing.ReusedBy = append(existing.ReusedBy, rid)
					fallthrough
				default:
					logging.Warningf(ctx, "BUG: Tryjob %s was launched but has already "+
						"mapped to an existing Tryjob %d that are not launched by this "+
						"Run. This Tryjob should be found reusable in an earlier stage.",
						eids[i], tj.ID)
				}
				tj.Status = tryjob.Status_UNTRIGGERED
				tj.UntriggeredReason = fmt.Sprintf("launched %s, but it already maps to tryjob %d ", eids[i], tj.ID)
				tj.ExternalID = ""
				tj.Result = nil
				dropped = append(dropped, tj)
			}
		}
	}
	return reconciled, dropped, nil
}
