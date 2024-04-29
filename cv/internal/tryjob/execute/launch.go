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
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/impl/submit"
	"go.chromium.org/luci/cv/internal/tryjob"
)

// launchTryjobs launches the provided Tryjobs using the backend system.
//
// All provided Tryjobs MUST be in PENDING status and SHOULD NOT have an
// external ID populated. If it fails to launch any Tryjob, the Tryjob will be
// returned in UNTRIGGERED status and the failure reason will be written to
// `UntriggeredReason` field.
//
// It's possible that the returned Tryjob has a different ID than the input
// Tryjob; that may typically happen when CV discovers that the external ID of
// the launched Tryjob already maps to an existing record. The original Tryjob
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
	clsInOrder, err := submit.ComputeOrder(w.cls)
	if err != nil {
		return nil, err
	}
	launchFailures := make(map[*tryjob.Tryjob]error)
	_ = retry.Retry(clock.Tag(ctx, common.LaunchRetryClockTag), retryFactory, func() error {
		var hasFatal bool
		launchFailures, hasFatal = w.tryLaunchTryjobsOnce(ctx, toBeLaunched, clsInOrder)
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

	launchFailureLogs := make([]*tryjob.ExecutionLogEntry_TryjobLaunchFailed, 0, len(launchFailures))
	for _, tj := range tryjobs {
		if err, ok := launchFailures[tj]; ok {
			tj.Status = tryjob.Status_UNTRIGGERED
			switch grpcStatus, ok := status.FromError(errors.Unwrap(err)); {
			case !ok:
				// Log the error detail but don't leak the internal error.
				logging.Errorf(ctx, "unexpected internal error when launching tryjob: %s", err)
				tj.UntriggeredReason = "unexpected internal error"
			default:
				tj.UntriggeredReason = fmt.Sprintf("received %s from %s", grpcStatus.Code(), w.backend.Kind())
				if msg := grpcStatus.Message(); msg != "" {
					tj.UntriggeredReason += ". message: " + msg
				}
			}
			launchFailureLogs = append(launchFailureLogs, &tryjob.ExecutionLogEntry_TryjobLaunchFailed{
				Definition: tj.Definition,
				Reason:     tj.UntriggeredReason,
			})
		}
	}
	if len(launchFailureLogs) > 0 {
		w.logEntries = append(w.logEntries, &tryjob.ExecutionLogEntry{
			Time: timestamppb.New(clock.Now(ctx).UTC()),
			Kind: &tryjob.ExecutionLogEntry_TryjobsLaunchFailed_{
				TryjobsLaunchFailed: &tryjob.ExecutionLogEntry_TryjobsLaunchFailed{
					Tryjobs: launchFailureLogs,
				},
			},
		})
	}
	return w.saveLaunchedTryjobs(ctx, tryjobs)
}

// tryLaunchTryjobsOnce attempts to launch Tryjobs once using the backend, and
// returns failures.
func (w *worker) tryLaunchTryjobsOnce(ctx context.Context, tryjobs []*tryjob.Tryjob, cls []*run.RunCL) (failures map[*tryjob.Tryjob]error, hasFatal bool) {
	launchLogs := make([]*tryjob.ExecutionLogEntry_TryjobSnapshot, 0, len(tryjobs))
	err := w.backend.Launch(ctx, tryjobs, w.run, cls)
	switch merrs, ok := err.(errors.MultiError); {
	case err == nil:
		for _, tj := range tryjobs {
			launchLogs = append(launchLogs, makeLogTryjobSnapshot(tj.Definition, tj, false))
		}
	case !ok:
		panic(fmt.Errorf("impossible; backend.Launch must return multi errors, got %s", err))
	default:
		for i, err := range merrs {
			if err != nil {
				if failures == nil {
					failures = make(map[*tryjob.Tryjob]error, 1)
				}
				failures[tryjobs[i]] = err
				hasFatal = hasFatal || !canRetryBackendError(err)
			} else {
				launchLogs = append(launchLogs, makeLogTryjobSnapshot(tryjobs[i].Definition, tryjobs[i], false))
			}
		}
	}

	if len(launchLogs) > 0 {
		w.logEntries = append(w.logEntries, &tryjob.ExecutionLogEntry{
			Time: timestamppb.New(clock.Now(ctx).UTC()),
			Kind: &tryjob.ExecutionLogEntry_TryjobsLaunched_{
				TryjobsLaunched: &tryjob.ExecutionLogEntry_TryjobsLaunched{
					Tryjobs: launchLogs,
				},
			},
		})
	}
	return failures, hasFatal
}

func canRetryBackendError(err error) bool {
	grpcStatus, ok := status.FromError(errors.Unwrap(err))
	return ok && retriableBackendErrorCodes[grpcStatus.Code()]
}

// A set of backend (e.g. Buildbucket) error codes that indicate that the
// launching can be retried, as the error may be transient.
var retriableBackendErrorCodes = map[codes.Code]bool{
	codes.Internal:    true,
	codes.Unknown:     true,
	codes.Unavailable: true,
	// TODO(crbug.com/1274781): as a temporary mitigation, retry NotFound error
	// to workaround the race condition for config ingestion in Buildbucket and
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

// saveLaunchedTryjobs saves launched Tryjobs to Datastore.
func (w *worker) saveLaunchedTryjobs(ctx context.Context, tryjobs []*tryjob.Tryjob) ([]*tryjob.Tryjob, error) {
	var innerErr error
	var reconciled []*tryjob.Tryjob
	err := datastore.RunInTransaction(ctx, func(ctx context.Context) (err error) {
		defer func() { innerErr = err }()
		// reload the tryjobs as the tryjobs could have been changed while LUCI CV
		// is launching the tryjobs against backend.
		currentTryjobs := make([]*tryjob.Tryjob, len(tryjobs))
		for i, tj := range tryjobs {
			currentTryjobs[i] = &tryjob.Tryjob{ID: tj.ID}
		}
		if err := datastore.Get(ctx, currentTryjobs); err != nil {
			return errors.Annotate(err, "failed to load tryjobs").Tag(transient.Tag).Err()
		}
		// update the tryjob with the eversion, update_time and all fields that
		// have been updated by backend.Launch(...)
		for i, tj := range tryjobs {
			currentTj := currentTryjobs[i]
			currentTj.ExternalID = tj.ExternalID
			currentTj.Status = tj.Status
			currentTj.Result = tj.Result
			currentTj.EVersion += 1
			currentTj.EntityUpdateTime = datastore.RoundTime(clock.Now(ctx).UTC())
		}
		// It's possible (though unlikely) that the external ID of the launched
		// Tryjob already has a record in CV. Reconcile to use the existing record
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

// reconcileWithExisting handles the edge cases where the external ID of the
// launched Tryjob already has a record in CV.
func reconcileWithExisting(ctx context.Context, tryjobs []*tryjob.Tryjob, rid common.RunID) (reconciled, dropped []*tryjob.Tryjob, err error) {
	eids := make([]tryjob.ExternalID, 0, len(tryjobs))
	for _, tj := range tryjobs {
		if eid := tj.ExternalID; eid != "" {
			eids = append(eids, eid)
		}
	}
	if len(eids) == 0 {
		// Nothing to reconcile.
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
		// Since eids are resolved following the order in tryjobs, the first entry
		// should always corresponds to this Tryjob.
		existing := existingTryjobs[0]
		existingTryjobs = existingTryjobs[1:]
		switch {
		case existing == nil:
			// The external ID has no existing associated Tryjob. This Tryjob should
			// be safe to create.
			reconciled[i] = tj
		case existing.ExternalID != tj.ExternalID:
			panic(fmt.Errorf("impossible; expect %s, got %s", tj.ExternalID, existing.ExternalID))
		default:
			// Update and use the existing Tryjob instead.
			existing.EVersion += 1
			existing.EntityUpdateTime = datastore.RoundTime(clock.Now(ctx).UTC())
			existing.Status = tj.Status
			existing.Result = tj.Result
			reconciled[i] = existing
			if tj.ID != existing.ID {
				switch {
				case existing.LaunchedBy == rid:
					// Expected.
				case existing.ReusedBy.Index(rid) < 0:
					existing.ReusedBy = append(existing.ReusedBy, rid)
					fallthrough
				default:
					logging.Warningf(ctx, "BUG: Tryjob %s was launched but has already "+
						"mapped to an existing Tryjob %d that are not launched by this "+
						"Run. This Tryjob should have been found reusable in an earlier stage.",
						eids[i], tj.ID)
				}
				tj.Status = tryjob.Status_UNTRIGGERED
				tj.UntriggeredReason = fmt.Sprintf("launched %s, but it already maps to Tryjob %d ", eids[i], tj.ID)
				tj.ExternalID = ""
				tj.Result = nil
				dropped = append(dropped, tj)
			}
		}
	}
	return reconciled, dropped, nil
}
