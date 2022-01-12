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

package tryjob

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/cv/internal/common"
)

const TaskClass = "update-tryjob"

type updaterBackend interface {
	// Kind identifies the backend.
	//
	// It's also the first part of the Tryjob's ExternalID, e.g. "buildbucket".
	// Must not contain a slash.
	Kind() string
	// Update should fetch the tryjob given the current entity in Datastore.
	//
	// MUST not modify the given Tryjob object.
	Update(ctx context.Context, saved *Tryjob) (Status, *Result, error)
}

// rmNotifier abstracts out Run Manager notifier.
type rmNotifier interface {
	NotifyTryjobsUpdated(context.Context, common.RunID, *TryjobUpdatedEvents) error
}

// Updater knows how to update Tryjobs, notifying other CV parts as needed.
type Updater struct {
	tqd        *tq.Dispatcher
	rmNotifier rmNotifier

	rwmutex  sync.RWMutex // guards `backends`
	backends map[string]updaterBackend
}

// NewUpdater creates a new Updater.
//
// Starts without backends, but they ought to be added via RegisterBackend().
func NewUpdater(tqd *tq.Dispatcher, rm rmNotifier) *Updater {
	u := &Updater{
		tqd:        tqd,
		rmNotifier: rm,
		backends:   make(map[string]updaterBackend, 1),
	}
	u.tqd.RegisterTaskClass(tq.TaskClass{
		ID:        TaskClass,
		Prototype: &UpdateTryjobTask{},
		Queue:     "update-tryjob",
		Kind:      tq.FollowsContext,
		Handler: func(ctx context.Context, payload proto.Message) error {
			return common.TQifyError(ctx, u.handleTask(ctx, payload.(*UpdateTryjobTask)))
		},
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

// Schedule dispatches a TQ task to update the given tryjob.
//
// At least one ID must be given.
func (u *Updater) Schedule(ctx context.Context, id common.TryjobID, eid ExternalID) error {
	if id == 0 && eid == "" {
		return errors.New("At least one of the tryjob's IDs must be given.")
	}
	// id will be set, but eid may not be. In such case, it's up to the task to
	// resolve it.
	return u.tqd.AddTask(ctx, &tq.Task{
		Title:   fmt.Sprintf("%d/%s", id, eid),
		Payload: &UpdateTryjobTask{ExternalId: string(eid), Id: int64(id)},
	})
}

func (u *Updater) handleTask(ctx context.Context, task *UpdateTryjobTask) error {
	tj := &Tryjob{ID: common.TryjobID(task.Id)}
	switch {
	case task.GetId() != 0:
		switch err := datastore.Get(ctx, tj); {
		case err == nil:
			if task.GetExternalId() != "" && task.GetExternalId() != string(tj.ExternalID) {
				return errors.Reason("the given internal and external ids for the tryjob do not match").Err()
			}
		case err == datastore.ErrNoSuchEntity:
			return errors.Annotate(err, "unknown Tryjob with id %d", task.Id).Err()
		default:
			return errors.Annotate(err, "loading Tryjob with id %d", task.Id).Tag(transient.Tag).Err()
		}
	case task.GetExternalId() != "":
		var err error
		switch tj, err = ExternalID(task.ExternalId).Load(ctx); {
		case err != nil:
			return errors.Annotate(err, "loading Tryjob with ExternalID %s", task.ExternalId).Tag(transient.Tag).Err()
		case tj == nil:
			return errors.Reason("unknown Tryjob with ExternalID %s", task.ExternalId).Err()
		}
	default:
		return errors.Reason("expected at least one of {Id, ExternalId} in %+v", task).Err()
	}
	loadedEVer := tj.EVersion

	backend, err := u.backendFor(tj)
	if err != nil {
		return errors.Annotate(err, "resolving backend for %v", tj).Err()
	}

	status, result, err := backend.Update(ctx, tj)
	switch {
	case err != nil:
		return errors.Annotate(err, "reading status and result from %q", tj.ExternalID).Err()
	case status == tj.Status && proto.Equal(tj.Result, result):
		return nil
	}
	// Captures the error that may cause the transaction to commit, and any relevant tags.
	var innerErr error
	err = datastore.RunInTransaction(ctx, func(ctx context.Context) (err error) {
		defer func() {
			innerErr = err
		}()
		tj = &Tryjob{ID: common.TryjobID(tj.ID)}
		if err := datastore.Get(ctx, tj); err != nil {
			return errors.Annotate(err, "failed to load Tryjob %d", tj.ID).Tag(transient.Tag).Err()
		}
		if loadedEVer != tj.EVersion {
			// A parallel task must have already updated this tryjob, retry.
			return errors.Reason("the tryjob data has changed").Tag(transient.Tag).Err()
		}
		tj.EntityUpdateTime = datastore.RoundTime(clock.Now(ctx).UTC())
		tj.EVersion++
		tj.Status = status
		tj.Result = result
		if err := datastore.Put(ctx, tj); err != nil {
			return errors.Annotate(err, "failed to save Tryjob %d", tj.ID).Tag(transient.Tag).Err()
		}
		for _, run := range tj.AllWatchingRuns() {
			if err := u.rmNotifier.NotifyTryjobsUpdated(
				ctx, run, &TryjobUpdatedEvents{Events: []*TryjobUpdatedEvent{{TryjobId: int64(tj.ID)}}},
			); err != nil {
				return err
			}
		}

		return nil
	}, nil)
	switch {
	case innerErr != nil:
		return innerErr
	case err != nil:
		return errors.Annotate(err, "failed to commit transaction updating tryjob %v", tj.ExternalID).Tag(transient.Tag).Err()
	}
	return nil
}

func (u *Updater) backendFor(t *Tryjob) (updaterBackend, error) {
	kind, err := t.ExternalID.kind()
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
