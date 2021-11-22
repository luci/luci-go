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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/cv/internal/common"
)

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
	NotifyTryjobUpdated(context.Context, common.RunID, common.TryjobID, int64)
}

// Updater knows how to update Tryjobs, notifying other CV parts as needed.
type Updater struct {
	tqd        *tq.Dispatcher
	rmNotifier rmNotifier

	mutex    sync.Mutex // guards `backends`
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
	// TODO(crbug.com/1227363): implement.
	// u.tqd.RegisterTaskClass(..., Handler: u.handleTask, ...)
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
	u.mutex.Lock()
	defer u.mutex.Unlock()
	if _, exists := u.backends[kind]; exists {
		panic(fmt.Errorf("backend %q is already registered", kind))
	}
	u.backends[kind] = b
}

// Schedule dispatches a TQ task to update the given tryjob.
//
// At least one ID must be given.
func (u *Updater) Schedule(ctx context.Context, id common.TryjobID, eid ExternalID) error {
	// TODO(crbug.com/1227363): implement.
	return nil
}

// implementation details.

func (u *Updater) handleTask(ctx context.Context, task *UpdateTryjobTask) error {
	// TODO(crbug.com/1227363): implement.
	// 1. Load Tryjob depending supporting both task.GetId or task.GetExternalID()
	// being set.
	// 2. Dispatch to backend.
	// 3. Update && notify Runs, if necessary.
	return nil
}

func (u *Updater) backendFor(t *Tryjob) (updaterBackend, error) {
	kind, err := t.ExternalID.kind()
	if err != nil {
		return nil, err
	}
	u.mutex.Lock()
	defer u.mutex.Unlock()
	if b, exists := u.backends[kind]; exists {
		return b, nil
	}
	return nil, errors.Reason("%q backend is not supported", kind).Err()
}
