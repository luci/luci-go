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

// Package tjcancel contains code in charge of cancelling stale tryjobs.
//
// Cancellator responds to tasks scheduled when a new patch is uploaded,
// looking for and cancelling stale tryjobs.
package tjcancel

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/tryjob"
)

// Cancellator is patterned after Updater to support multiple tryjob backends.
type Cancellator struct {
	tn *tryjob.Notifier

	// guards backends map.
	rwmutex  sync.RWMutex
	backends map[string]cancellatorBackend
}

func NewCancellator(tn *tryjob.Notifier) *Cancellator {
	c := &Cancellator{
		tn:       tn,
		backends: make(map[string]cancellatorBackend),
	}
	tn.Bindings.CancelStale.AttachHandler(func(ctx context.Context, payload proto.Message) error {
		task := payload.(*tryjob.CancelStaleTryjobsTask)
		ctx = logging.SetField(ctx, "CLID", task.GetClid())
		return common.TQifyError(ctx, c.handleTask(ctx, task))
	})
	return c
}

// RegisterBackend registers a backend.
//
// Panics if backend for the same kind is already registered.
func (c *Cancellator) RegisterBackend(b cancellatorBackend) {
	kind := b.Kind()
	if strings.ContainsRune(kind, '/') {
		panic(fmt.Errorf("backend %T of kind %q must not contain '/'", b, kind))
	}
	c.rwmutex.Lock()
	defer c.rwmutex.Unlock()
	if _, exists := c.backends[kind]; exists {
		panic(fmt.Errorf("backend %q is already registered", kind))
	}
	c.backends[kind] = b
}

func (c *Cancellator) handleTask(ctx context.Context, task *tryjob.CancelStaleTryjobsTask) error {
	if task.PreviousMinEquivPatchset >= task.CurrentMinEquivPatchset {
		panic(fmt.Errorf("patchset numbers expected to increase monotonically"))
	}
	cl := &changelist.CL{ID: common.CLID(task.GetClid())}
	if err := datastore.Get(ctx, cl); err != nil {
		return transient.Tag.Apply(errors.Fmt("failed to load CL %d: %w", cl.ID, err))
	}
	preserveTryjob := false
	for _, metadata := range cl.Snapshot.GetMetadata() {
		if metadata.Key == common.FooterCQDoNotCancelTryjobs && strings.ToLower(strings.TrimSpace(metadata.Value)) == "true" {
			preserveTryjob = true
		}
	}
	if preserveTryjob {
		logging.Infof(ctx, "skipping cancelling Tryjob as the latest CL has specified %s footer", common.FooterCQDoNotCancelTryjobs)
		return nil
	}

	candidates, err := c.fetchCandidates(ctx, cl.ID, task.GetPreviousMinEquivPatchset(), task.GetCurrentMinEquivPatchset())
	switch {
	case err != nil:
		return err
	case len(candidates) == 0:
		logging.Infof(ctx, "no stale Tryjobs to cancel")
		return nil
	default:
		tryjobIDs := make([]string, len(candidates))
		for i, tj := range candidates {
			tryjobIDs[i] = strconv.Itoa(int(tj.ID))
		}
		logging.Infof(ctx, "found stale Tryjobs to cancel: [%s]", strings.Join(tryjobIDs, ", "))
		return c.cancelTryjobs(ctx, candidates)
	}
}

const cancelLaterDuration = 10 * time.Second

func (c *Cancellator) fetchCandidates(ctx context.Context, clid common.CLID, prevMinEquiPS, curMinEquiPS int32) ([]*tryjob.Tryjob, error) {
	q := datastore.NewQuery(tryjob.TryjobKind).
		Gte("CLPatchsets", tryjob.MakeCLPatchset(clid, prevMinEquiPS)).
		Lt("CLPatchsets", tryjob.MakeCLPatchset(clid, curMinEquiPS))
	var candidates []*tryjob.Tryjob
	err := datastore.Run(ctx, q, func(tj *tryjob.Tryjob) error {
		switch {
		case tj.ExternalID == "":
			// Most likely Tryjob hasn't been triggered in the backend yet.
		case tj.IsEnded():
		case tj.LaunchedBy == "":
			// Not launched by LUCI CV, may be through `git cl try` command line.
		case tj.Definition.GetSkipStaleCheck():
		default:
			candidates = append(candidates, tj)
		}
		return nil
	})
	switch {
	case err != nil:
		return nil, transient.Tag.Apply(errors.Fmt("failed to run the query to fetch candidate tryjobs for cancellation: %w", err))
	case len(candidates) == 0:
		return nil, nil
	}

	hasAllWatchingRunsEndedFn, err := makeHasAllWatchingRunEndedFn(ctx, candidates)
	if err != nil {
		return nil, err
	}
	var ret = candidates[:0] // reuse the same slice
	var cancelLaterScheduled bool
	for _, candidate := range candidates {
		switch nonEndedRuns := hasAllWatchingRunsEndedFn(candidate); {
		case len(nonEndedRuns) > 0 && !cancelLaterScheduled:
			eta := clock.Now(ctx).UTC().Add(cancelLaterDuration)
			if err := c.tn.ScheduleCancelStale(ctx, clid, prevMinEquiPS, curMinEquiPS, eta); err != nil {
				return nil, err
			}
			cancelLaterScheduled = true
			fallthrough
		case len(nonEndedRuns) > 0:
			logging.Warningf(ctx, "tryjob %d is still watched by non ended runs %s. This is likely a race condition and those runs will end soon. Will retry cancellation after %s.", candidate.ID, nonEndedRuns, cancelLaterDuration)
		default:
			ret = append(ret, candidate)
		}
	}
	return ret, nil

}

func makeHasAllWatchingRunEndedFn(ctx context.Context, tryjobs []*tryjob.Tryjob) (func(*tryjob.Tryjob) (nonEnded common.RunIDs), error) {
	runIDSet := stringset.New(1) // typically only one run.
	for _, tj := range tryjobs {
		for _, rid := range tj.AllWatchingRuns() {
			runIDSet.Add(string(rid))
		}
	}
	runs, errs := run.LoadRunsFromIDs(common.MakeRunIDs(runIDSet.ToSlice()...)...).Do(ctx)
	endedRunIDs := make(map[common.RunID]struct{}, len(runs))
	for i, r := range runs {
		switch err := errs[i]; {
		case err == datastore.ErrNoSuchEntity:
			return nil, errors.Fmt("Tryjob is associated with a non-existent Run %s", r.ID)
		case err != nil:
			return nil, transient.Tag.Apply(errors.Fmt("failed to load run %s: %w", r.ID, err))
		case run.IsEnded(r.Status):
			endedRunIDs[r.ID] = struct{}{}
		}
	}
	return func(tj *tryjob.Tryjob) common.RunIDs {
		var nonEnded common.RunIDs
		for _, rid := range tj.AllWatchingRuns() {
			if _, ended := endedRunIDs[rid]; !ended {
				nonEnded = append(nonEnded, rid)
			}
		}
		return nonEnded
	}, nil
}

const reason = "LUCI CV no longer needs this Tryjob"

func (c *Cancellator) cancelTryjobs(ctx context.Context, tjs []*tryjob.Tryjob) error {
	if len(tjs) == 0 {
		return nil
	}
	errs := parallel.WorkPool(min(8, len(tjs)), func(work chan<- func() error) {
		for _, tj := range tjs {
			work <- func() error {
				be, err := c.backendFor(tj)
				if err != nil {
					return err
				}
				// TODO(crbug/1308930): use Buildbucket's batch API to reduce
				// number of RPCs.
				err = be.CancelTryjob(ctx, tj, reason)
				if err != nil {
					return errors.Fmt("failed to cancel Tryjob [id=%d, eid=%s]: %w", tj.ID, tj.ExternalID, err)
				}
				return datastore.RunInTransaction(ctx, func(ctx context.Context) error {
					if err := datastore.Get(ctx, tj); err != nil {
						return transient.Tag.Apply(errors.Fmt("failed to load Tryjob %d: %w", tj.ID, err))
					}
					if tj.IsEnded() {
						return nil
					}
					tj.Status = tryjob.Status_CANCELLED
					tj.EVersion++
					tj.EntityUpdateTime = datastore.RoundTime(clock.Now(ctx).UTC())
					if err := datastore.Put(ctx, tj); err != nil {
						return transient.Tag.Apply(errors.Fmt("failed to save Tryjob %d: %w", tj.ID, err))
					}
					return nil
				}, nil)
			}
		}
	})
	return common.MostSevereError(errs)
}

// cancellatorBackend is implemented by tryjobs backends, e.g. buildbucket.
type cancellatorBackend interface {
	// Kind identifies the backend
	//
	// It's also the first part of the Tryjob's ExternalID, e.g. "buildbucket".
	// Must not contain a slash.
	Kind() string
	// CancelTryjob should cancel the tryjob given.
	//
	// MUST not modify the given Tryjob object.
	// If the tryjob was already cancelled, it should not return an error.
	CancelTryjob(ctx context.Context, tj *tryjob.Tryjob, reason string) error
}

func (c *Cancellator) backendFor(t *tryjob.Tryjob) (cancellatorBackend, error) {
	kind, err := t.ExternalID.Kind()
	if err != nil {
		return nil, err
	}
	c.rwmutex.RLock()
	defer c.rwmutex.RUnlock()
	if b, exists := c.backends[kind]; exists {
		return b, nil
	}
	return nil, errors.Fmt("%q backend is not supported", kind)
}
