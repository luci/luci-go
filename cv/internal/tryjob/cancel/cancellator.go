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

// Package cancel contains code in charge of cancelling stale tryjobs.
//
// Cancellator responds to tasks scheduled when a new patch is uploaded,
// looking for and cancelling stale tryjobs.
package cancel

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/tryjob"
)

const CancelStaleTaskClass = "cancel-stale-tryjobs"

// Cancellator is patterned after Updater to support multiple tryjob backends.
type Cancellator struct {
	tqd *tq.Dispatcher

	// guards backends map.
	rwmutex  sync.RWMutex
	backends map[string]cancellatorBackend
}

func NewCancellator(tqd *tq.Dispatcher) *Cancellator {
	c := &Cancellator{
		tqd:      tqd,
		backends: make(map[string]cancellatorBackend),
	}
	c.tqd.RegisterTaskClass(tq.TaskClass{
		ID:        CancelStaleTaskClass,
		Prototype: &tryjob.CancelStaleTryjobsTask{},
		Queue:     "cancel-stale-tryjobs",
		Kind:      tq.Transactional,
		Handler: func(ctx context.Context, payload proto.Message) error {
			return common.TQifyError(ctx, c.handleTask(ctx, payload.(*tryjob.CancelStaleTryjobsTask)))
		},
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
	// TODO(crbug/1301244): Implement.
	// Should get the tryjobs to cancel, and if unwatched cancel them using the
	// appropriate backend, also save the new status to datastore.
	return nil
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
