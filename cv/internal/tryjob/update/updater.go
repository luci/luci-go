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

// Package tjupdate contains Updater, which handles an UpdateTryjobTask.
//
// This involves updating a Tryjob by querying the backend system, and updating
// Tryjob entities in Datastore.
package tjupdate

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/metrics"
	"go.chromium.org/luci/cv/internal/rpc/versioning"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/tryjob"
)

type updaterBackend interface {
	// Kind identifies the backend.
	//
	// It's also the first part of the Tryjob's ExternalID, e.g. "buildbucket".
	// Must NOT contain a slash.
	Kind() string
	// Fetch fetches the Tryjobs status and result from the Tryjob backend.
	Fetch(ctx context.Context, luciProject string, eid tryjob.ExternalID) (tryjob.Status, *tryjob.Result, error)
	// Parse parses the Tryjobs status and result from the data provided by
	// the Tryjob backend.
	Parse(ctx context.Context, data any) (tryjob.Status, *tryjob.Result, error)
}

// rmNotifier abstracts out the Run Manager's Notifier.
type rmNotifier interface {
	NotifyTryjobsUpdated(context.Context, common.RunID, *tryjob.TryjobUpdatedEvents) error
}

// Updater knows how to update Tryjobs, notifying other CV parts as needed.
type Updater struct {
	env        *common.Env
	rmNotifier rmNotifier

	rwmutex  sync.RWMutex // guards `backends`
	backends map[string]updaterBackend
}

// NewUpdater creates a new Updater.
//
// Starts without backends, but they should be added via RegisterBackend().
func NewUpdater(env *common.Env, tn *tryjob.Notifier, rm rmNotifier) *Updater {
	u := &Updater{
		env:        env,
		rmNotifier: rm,
		backends:   make(map[string]updaterBackend, 1),
	}
	tn.Bindings.Update.AttachHandler(func(ctx context.Context, payload proto.Message) error {
		err := u.handleTask(ctx, payload.(*tryjob.UpdateTryjobTask))
		return common.TQIfy{
			KnownRetry: []error{errTryjobEntityHasChanged},
		}.Error(ctx, err)
	})
	return u
}

// RegisterBackend registers a backend.
//
// Panics if backend for the same kind is already registered.
func (u *Updater) RegisterBackend(b updaterBackend) {
	kind := b.Kind()
	if strings.ContainsRune(kind, '/') {
		panic(fmt.Errorf("backend %T of kind %q must not contain '/'", b, kind))
	}
	u.rwmutex.Lock()
	defer u.rwmutex.Unlock()
	if _, exists := u.backends[kind]; exists {
		panic(fmt.Errorf("backend %q is already registered", kind))
	}
	u.backends[kind] = b
}

// Update updates the Tryjob entity associated with the given `eid`.
//
// `data` should contain the latest information of the Tryjob from the
// Tryjob backend system (e.g. Build proto from Buildbucket pubsub).
//
// No-op if the Tryjob data stored in CV appears to be newer than the provided
// data (e.g. has newer Tryjob.Result.UpdateTime)
func (u *Updater) Update(ctx context.Context, eid tryjob.ExternalID, data any) error {
	tj, err := eid.Load(ctx)
	switch {
	case err != nil:
		return err
	case tj == nil:
		return errors.Reason("unknown Tryjob with ExternalID %s", eid).Err()
	}
	backend, err := u.backendFor(tj)
	if err != nil {
		return errors.Annotate(err, "resolving backend for %v", tj).Err()
	}

	switch status, result, err := backend.Parse(ctx, data); {
	case err != nil:
		return errors.Reason("failed to parse status and result for tryjob %s", eid).Err()
	case tj.Result.GetUpdateTime() != nil && result.GetUpdateTime() != nil && tj.Result.GetUpdateTime().AsTime().After(result.GetUpdateTime().AsTime()):
		// Tryjob data in CV is newer
		return nil
	case status == tj.Status && proto.Equal(tj.Result, result):
		return nil
	default:
		return u.conditionallyUpdate(ctx, tj.ID, status, result, tj.EVersion)
	}
}

// handleTask handles an UpdateTryjobTask.
//
// This task involves checking the status of a Tryjob and updating its
// Datastore entity, and notifying Runs which care about this Tryjob.
func (u *Updater) handleTask(ctx context.Context, task *tryjob.UpdateTryjobTask) error {
	tj := &tryjob.Tryjob{ID: common.TryjobID(task.Id)}
	switch {
	case task.GetId() != 0:
		switch err := datastore.Get(ctx, tj); err {
		case nil:
			if task.GetExternalId() != "" && task.GetExternalId() != string(tj.ExternalID) {
				return errors.Reason("the given internal and external IDs for the Tryjob do not match").Err()
			}
		case datastore.ErrNoSuchEntity:
			return errors.Annotate(err, "unknown Tryjob with ID %d", task.Id).Err()
		default:
			return errors.Annotate(err, "loading Tryjob with ID %d", task.Id).Tag(transient.Tag).Err()
		}
	case task.GetExternalId() != "":
		var err error
		switch tj, err = tryjob.ExternalID(task.ExternalId).Load(ctx); {
		case err != nil:
			return errors.Annotate(err, "loading Tryjob with ExternalID %s", task.ExternalId).Tag(transient.Tag).Err()
		case tj == nil:
			return errors.Reason("unknown Tryjob with ExternalID %s", task.ExternalId).Err()
		}
	default:
		return errors.Reason("expected at least one of {Id, ExternalId} in %+v", task).Err()
	}

	backend, err := u.backendFor(tj)
	if err != nil {
		return errors.Annotate(err, "resolving backend for %v", tj).Err()
	}

	status, result, err := backend.Fetch(ctx, tj.LUCIProject(), tj.ExternalID)
	switch {
	case err != nil:
		return errors.Annotate(err, "reading status and result from %q", tj.ExternalID).Err()
	case status == tj.Status && proto.Equal(tj.Result, result):
		return nil
	}
	return u.conditionallyUpdate(ctx, tj.ID, status, result, tj.EVersion)
}

// errTryjobEntityHasChanged is returned if there is a race updating Tryjob
// entity.
var errTryjobEntityHasChanged = errors.New("Tryjob entity has changed", transient.Tag)

// conditionallyUpdate updates the Tryjob entity if the EVersion of the current
// Tryjob entity in the datastore matches the `expectedEVersion`.
//
// Returns errTryjobEntityHasChanged if the Tryjob entity in the datastore has
// been modified already (i.e. EVersion no longer matches `expectedEVersion`).
func (u *Updater) conditionallyUpdate(ctx context.Context, id common.TryjobID, status tryjob.Status, result *tryjob.Result, expectedEVersion int64) error {
	// Capture the error that may cause the transaction to commit, and any
	// relevant tags.
	var innerErr error
	err := datastore.RunInTransaction(ctx, func(ctx context.Context) (err error) {
		defer func() {
			innerErr = err
		}()
		tj := &tryjob.Tryjob{ID: common.TryjobID(id)}
		if err := datastore.Get(ctx, tj); err != nil {
			return errors.Annotate(err, "failed to load Tryjob %d", tj.ID).Tag(transient.Tag).Err()
		}
		if expectedEVersion != tj.EVersion {
			return errTryjobEntityHasChanged
		}

		if tj.LaunchedBy != "" && status == tryjob.Status_ENDED && tj.Status != status {
			// Tryjob launched by CV has transitioned to end status
			r := &run.Run{ID: tj.LaunchedBy}
			if err := datastore.Get(ctx, r); err != nil {
				return errors.Annotate(err, "failed to load Run %s", r.ID).Tag(transient.Tag).Err()
			}
			project, configGroup := r.ID.LUCIProject(), r.ConfigGroupID.Name()
			isRetry := true
			for _, exec := range r.Tryjobs.GetState().GetExecutions() {
				if len(exec.GetAttempts()) > 0 && exec.GetAttempts()[0].TryjobId == int64(tj.ID) {
					isRetry = false
					break
				}
			}
			txndefer.Defer(ctx, func(ctx context.Context) {
				tryjob.RunWithBuilderMetricsTarget(ctx, u.env, tj.Definition, func(ctx context.Context) {
					metrics.Public.TryjobEnded.Add(ctx, 1,
						project,
						configGroup,
						tj.Definition.GetCritical(),
						isRetry,
						versioning.TryjobStatusV0(tj.Status, tj.Result.GetStatus()).String(),
					)
				})
			})
		}
		tj.EntityUpdateTime = datastore.RoundTime(clock.Now(ctx).UTC())
		tj.EVersion++
		tj.Status = status
		tj.Result = result
		if err := datastore.Put(ctx, tj); err != nil {
			return errors.Annotate(err, "failed to save Tryjob %d", tj.ID).Tag(transient.Tag).Err()
		}
		for _, run := range tj.AllWatchingRuns() {
			err := u.rmNotifier.NotifyTryjobsUpdated(
				ctx, run, &tryjob.TryjobUpdatedEvents{
					Events: []*tryjob.TryjobUpdatedEvent{
						{TryjobId: int64(tj.ID)},
					},
				},
			)
			if err != nil {
				return err
			}
		}
		return nil
	}, nil)
	switch {
	case innerErr != nil:
		return innerErr
	case err != nil:
		return errors.Annotate(err, "failed to commit transaction updating tryjob %d", id).Tag(transient.Tag).Err()
	}
	return nil
}

func (u *Updater) backendFor(t *tryjob.Tryjob) (updaterBackend, error) {
	kind, err := t.ExternalID.Kind()
	if err != nil {
		return nil, err
	}
	u.rwmutex.RLock()
	defer u.rwmutex.RUnlock()
	if b, exists := u.backends[kind]; exists {
		return b, nil
	}
	return nil, errors.Reason("%q backend is not supported", kind).Err()
}
