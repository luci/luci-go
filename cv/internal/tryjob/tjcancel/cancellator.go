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
	"strings"
	"sync"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/tryjob"
)

// Cancellator is patterned after Updater to support multiple tryjob backends.
type Cancellator struct {
	tqd *tq.Dispatcher

	// guards backends map.
	rwmutex  sync.RWMutex
	backends map[string]cancellatorBackend
}

func NewCancellator(tn *tryjob.Notifier) *Cancellator {
	c := &Cancellator{
		backends: make(map[string]cancellatorBackend),
	}
	tn.Bindings.CancelStale.AttachHandler(func(ctx context.Context, payload proto.Message) error {
		return common.TQifyError(ctx, c.handleTask(ctx, payload.(*tryjob.CancelStaleTryjobsTask)))
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
	atOrAfter := tryjob.MakeCLPatchset(common.CLID(task.GetClid()), task.GetPreviousMinEquivPatchset())
	before := tryjob.MakeCLPatchset(common.CLID(task.GetClid()), task.GetCurrentMinEquivPatchset())
	q := datastore.NewQuery(tryjob.TryjobKind).Gte("CLPatchsets", atOrAfter).Lt("CLPatchsets", before)

	toCancel := make([]*tryjob.Tryjob, 0)
	// The task should be safe to re-run even if it makes partial progress.
	// If the cancellation succeeded but the entity was not successfully
	// updated, on the next run the cancellation should be a no-op and the
	// entity update will be tried again.
	// Any tryjobs that are successfully cancelled and their entities updated
	// will not appear in subsequent runs of the query, ensuring progress.

	dsErr := datastore.Run(ctx, q, func(tj *tryjob.Tryjob) error {
		if tj.IsEnded() || tj.TriggeredBy == "" || tj.Definition.SkipStaleCheck {
			return nil
		}
		switch ended, err := c.allWatchingRunsEnded(ctx, tj); {
		case err != nil:
			return err
		// External ID is required to cancel tryjob.
		// It may be unset for a few reasons:
		//   - The task triggering the tryjob is stuck.
		//   - The transaction saving the external id got rolled back and is
		//     waiting to retry.
		//   - RM has a bug.
		case ended && tj.ExternalID != "":
			toCancel = append(toCancel, tj)
		}
		return nil
	})
	switch {
	case len(toCancel) == 0:
		return dsErr
	case dsErr != nil:
		logging.Warningf(ctx, "the query to fetch stale tryjobs returned an error along with partial results, will continue to cancel the returned tryjobs first")
		fallthrough
	default:
		if err := c.cancelAll(ctx, toCancel); err != nil {
			return err
		}
		return dsErr
	}
}

func (c *Cancellator) cancelAll(ctx context.Context, tjs []*tryjob.Tryjob) error {
	if len(tjs) == 0 {
		return nil
	}
	errs := parallel.WorkPool(min(8, len(tjs)), func(work chan<- func() error) {
		for _, tj := range tjs {
			tj := tj
			work <- func() error {
				be, err := c.backendFor(tj)
				if err != nil {
					return err
				}
				// TODO(crbug/1308930): use Buildbucket's batch API to reduce
				// number of RPCs.
				err = be.CancelTryjob(ctx, tj)
				if err != nil {
					return err
				}
				return datastore.RunInTransaction(ctx, func(ctx context.Context) error {
					if err := datastore.Get(ctx, tj); err != nil {
						return err
					}
					if tj.IsEnded() {
						return nil
					}
					tj.Status = tryjob.Status_CANCELLED
					tj.EVersion++
					tj.EntityUpdateTime = datastore.RoundTime(clock.Now(ctx).UTC())
					return datastore.Put(ctx, tj)
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
	CancelTryjob(ctx context.Context, tj *tryjob.Tryjob) error
}

// allWatchingRunsEnded checks if all of the runs that are watching the given
// tryjob have ended.
func (c *Cancellator) allWatchingRunsEnded(ctx context.Context, tryjob *tryjob.Tryjob) (bool, error) {
	runIds := tryjob.AllWatchingRuns()
	runs, errs := run.LoadRunsFromIDs(runIds...).Do(ctx)
	for i, r := range runs {
		switch {
		case errs[i] == datastore.ErrNoSuchEntity:
			return false, errors.Reason("Tryjob %s is associated with a non-existent Run %s", tryjob.ExternalID, runIds[i]).Err()
		case errs[i] != nil:
			return false, errors.Annotate(errs[i], "failed to load run %s", runIds[i]).Err()
		case !run.IsEnded(r.Status):
			return false, nil
		}
	}
	return true, nil
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
	return nil, errors.Reason("%q backend is not supported", kind).Err()
}

func min(x, y int) int {
	if x <= y {
		return x
	}
	return y
}
