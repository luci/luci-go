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
	"slices"
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
	launchResultByTryjobID := make(map[common.TryjobID]*tryjob.LaunchResult, len(toBeLaunched))
	errRetry := errors.New("please retry")
	err = retry.Retry(clock.Tag(ctx, common.LaunchRetryClockTag), retryFactory,
		func() error {
			launchResults := w.backend.Launch(ctx, toBeLaunched, w.run, clsInOrder)
			for i, result := range launchResults {
				launchResultByTryjobID[toBeLaunched[i].ID] = result
			}
			toBeLaunched = w.findRetriableTryjobs(ctx, toBeLaunched, launchResults)
			if len(toBeLaunched) == 0 {
				return nil
			}
			return errRetry
		}, func(error, time.Duration) {
			var sb strings.Builder
			sb.WriteString("retrying following tryjobs:")
			for _, tj := range toBeLaunched {
				sb.WriteString("\n  * ")
				sb.WriteString(strconv.FormatInt(int64(tj.ID), 10))
				sb.WriteString(": ")
				sb.WriteString(launchResultByTryjobID[tj.ID].Err.Error())
			}
			logging.Warningf(ctx, sb.String())
		})
	if err != nil && !errors.Is(err, errRetry) {
		// Returns the error if it is not the error to indicate retry.
		return nil, err
	}
	tryjobIDs := make(common.TryjobIDs, len(tryjobs))
	for i, tj := range tryjobs {
		tryjobIDs[i] = tj.ID
	}

	if tryjobs, err = w.saveLaunchedTryjobs(ctx, tryjobIDs, launchResultByTryjobID); err != nil {
		return nil, err
	}
	w.logLaunchFailures(ctx, tryjobs)
	return tryjobs, nil
}

// also log successfully launched tryjobs
func (w *worker) findRetriableTryjobs(ctx context.Context, tryjobs []*tryjob.Tryjob, launchResults []*tryjob.LaunchResult) (toRetry []*tryjob.Tryjob) {
	launchLogs := make([]*tryjob.ExecutionLogEntry_TryjobSnapshot, 0, len(tryjobs))
	for i, result := range launchResults {
		switch tj := tryjobs[i]; {
		case result.Err == nil:
			launchLogs = append(launchLogs, &tryjob.ExecutionLogEntry_TryjobSnapshot{
				Definition: tj.Definition,
				Id:         int64(tj.ID),
				ExternalId: string(result.ExternalID),
				Status:     result.Status,
				Result:     result.Result,
			})
		case canRetryBackendError(result.Err):
			toRetry = append(toRetry, tj)
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
	return toRetry
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
//
// Handles the edge case where the external ID already maps to another Tryjob
// in LUCI CV. See `reconcileWithExisting`.
func (w *worker) saveLaunchedTryjobs(ctx context.Context, tryjobIDs common.TryjobIDs, launchResultByTryjobID map[common.TryjobID]*tryjob.LaunchResult) ([]*tryjob.Tryjob, error) {
	result := make([]*tryjob.Tryjob, len(tryjobIDs))
	var innerErr error
	err := datastore.RunInTransaction(ctx, func(ctx context.Context) (err error) {
		defer func() { innerErr = err }()
		muts, err := w.mutator.BeginBatch(ctx, tryjobIDs)
		if err != nil {
			return err
		}
		for i, mut := range muts {
			tj := mut.Tryjob
			launchResult := launchResultByTryjobID[tj.ID]
			if launchResult.Err != nil {
				tj.Status = tryjob.Status_UNTRIGGERED
				switch grpcStatus, ok := status.FromError(errors.Unwrap(launchResult.Err)); {
				case !ok:
					// Log the error detail but don't leak the internal error.
					logging.Errorf(ctx, "unexpected internal error when launching tryjob: %s", launchResult.Err)
					tj.UntriggeredReason = "unexpected internal error"
				default:
					tj.UntriggeredReason = fmt.Sprintf("received %s from %s", grpcStatus.Code(), w.backend.Kind())
					if msg := grpcStatus.Message(); msg != "" {
						tj.UntriggeredReason += ". message: " + msg
					}
				}
			} else {
				tj.ExternalID = launchResult.ExternalID
				tj.Status = launchResult.Status
				tj.Result = launchResult.Result
			}

			var cErr *tryjob.ConflictTryjobsError
			switch tj, err := mut.Finalize(ctx); {
			case err == nil:
				result[i] = tj
			case errors.As(err, &cErr):
				if result[i], err = w.reconcileWithExisting(ctx, mut, cErr.Existing, launchResult); err != nil {
					return err
				}
			default:
				return err
			}
		}
		return nil
	}, nil)
	switch {
	case innerErr != nil:
		return nil, innerErr
	case err != nil:
		return nil, transient.Tag.Apply(errors.Fmt("failed to commit transaction: %w", err))
	default:
		return result, nil
	}
}

// reconcileWithExisting handles the edge cases where the external ID of the
// launched Tryjob X already maps to another Tryjob Y in LUCI CV.
//
// Updates Tryjob X to UNTRIGGERED status. Updates Tryjob Y using the
// launchResult. Returns the updated Tryjob Y.
func (w *worker) reconcileWithExisting(ctx context.Context, mut *tryjob.TryjobMutation, existingTryjobID common.TryjobID, launchResult *tryjob.LaunchResult) (*tryjob.Tryjob, error) {
	// Updates the Tryjob X
	mut.Tryjob.Status = tryjob.Status_UNTRIGGERED
	mut.Tryjob.UntriggeredReason = fmt.Sprintf("launched %s, but it already maps to another Tryjob %d", launchResult.ExternalID, existingTryjobID)
	mut.Tryjob.ExternalID = ""
	mut.Tryjob.Result = nil
	if _, err := mut.Finalize(ctx); err != nil {
		return nil, err
	}

	// Updates the Tryjob Y
	mut, err := w.mutator.Begin(ctx, existingTryjobID)
	if err != nil {
		return nil, err
	}
	mut.Tryjob.Status = launchResult.Status
	mut.Tryjob.Result = launchResult.Result
	switch {
	case mut.Tryjob.LaunchedBy == w.run.ID: // expected
	case slices.Contains(mut.Tryjob.ReusedBy, w.run.ID):
		mut.Tryjob.ReusedBy = append(mut.Tryjob.ReusedBy, w.run.ID)
		fallthrough
	default:
		logging.Warningf(ctx, "BUG: Tryjob %s was launched but has already "+
			"mapped to an existing Tryjob %d that are not launched by this "+
			"Run. This Tryjob should have been found reusable in an earlier stage.",
			mut.Tryjob.ExternalID, mut.Tryjob.ID)
	}
	tj, err := mut.Finalize(ctx)
	if err != nil {
		return nil, err
	}
	return tj, nil
}

func (w *worker) logLaunchFailures(ctx context.Context, tryjobs []*tryjob.Tryjob) {
	var launchFailureLogs []*tryjob.ExecutionLogEntry_TryjobLaunchFailed
	for _, tj := range tryjobs {
		if tj.Status == tryjob.Status_UNTRIGGERED {
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
}
