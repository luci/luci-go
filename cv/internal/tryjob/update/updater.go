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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
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
	env     *common.Env
	mutator *tryjob.Mutator

	rwmutex  sync.RWMutex // guards `backends`
	backends map[string]updaterBackend
}

// NewUpdater creates a new Updater.
//
// Starts without backends, but they should be added via RegisterBackend().
func NewUpdater(env *common.Env, tn *tryjob.Notifier, rm rmNotifier) *Updater {
	u := &Updater{
		env:      env,
		mutator:  tryjob.NewMutator(rm),
		backends: make(map[string]updaterBackend, 1),
	}
	tn.Bindings.Update.AttachHandler(func(ctx context.Context, payload proto.Message) error {
		err := u.handleTask(ctx, payload.(*tryjob.UpdateTryjobTask))
		return common.TQifyError(ctx, err)
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
		return errors.Fmt("unknown Tryjob with ExternalID %s", eid)
	}
	backend, err := u.backendFor(tj)
	if err != nil {
		return errors.Fmt("resolving backend for %v: %w", tj, err)
	}

	switch status, result, err := backend.Parse(ctx, data); {
	case err != nil:
		return errors.Fmt("failed to parse status and result for tryjob %s", eid)
	default:
		return u.conditionallyUpdate(ctx, tj.ID, status, result)
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
				return errors.New("the given internal and external IDs for the Tryjob do not match")
			}
		case datastore.ErrNoSuchEntity:
			return errors.Fmt("unknown Tryjob with ID %d: %w", task.Id, err)
		default:
			return transient.Tag.Apply(errors.Fmt("loading Tryjob with ID %d: %w", task.Id, err))
		}
	case task.GetExternalId() != "":
		var err error
		switch tj, err = tryjob.ExternalID(task.ExternalId).Load(ctx); {
		case err != nil:
			return transient.Tag.Apply(errors.Fmt("loading Tryjob with ExternalID %s: %w", task.ExternalId, err))
		case tj == nil:
			return errors.Fmt("unknown Tryjob with ExternalID %s", task.ExternalId)
		}
	default:
		return errors.Fmt("expected at least one of {Id, ExternalId} in %+v", task)
	}

	backend, err := u.backendFor(tj)
	if err != nil {
		return errors.Fmt("resolving backend for %v: %w", tj, err)
	}

	switch status, result, err := backend.Fetch(ctx, tj.LUCIProject(), tj.ExternalID); {
	case err != nil:
		return errors.Fmt("failed to read status and result from %q", tj.ExternalID)
	default:
		return u.conditionallyUpdate(ctx, tj.ID, status, result)
	}
}

// conditionallyUpdate conditionally updates the Tryjob entity with given
// status and result.
//
// Following conditions should be met before updating the Tryjob:
//   - the status and result are different from the stored status and result
//   - the updateTime in the stored result must not be later than the
//     updateTime in the result. This indicates the Tryjob stored in CV is more
//     fresh.
//
// Best effort reports TryjobEnded metric if the Tryjob has transitioned to the
// Status_ENDED.
func (u *Updater) conditionallyUpdate(ctx context.Context, id common.TryjobID, status tryjob.Status, result *tryjob.Result) error {
	var priorStatus tryjob.Status
	tj, err := u.mutator.Update(ctx, id, func(tj *tryjob.Tryjob) error {
		switch {
		case status == tj.Status && isSemanticallyEqual(tj.Result, result):
			return tryjob.ErrStopMutation
		case tj.Result.GetUpdateTime() != nil && result.GetUpdateTime() != nil && tj.Result.GetUpdateTime().AsTime().After(result.GetUpdateTime().AsTime()):
			// the stored Tryjob is newer, skip update
			return tryjob.ErrStopMutation
		}
		priorStatus = tj.Status
		tj.Status = status
		tj.Result = result
		return nil
	})
	if err != nil {
		return err
	}

	if tj.LaunchedBy != "" && status == tryjob.Status_ENDED && priorStatus != status {
		// Tryjob has transitioned to end status
		if err := u.reportTryjobEndedStatus(ctx, tj); err != nil {
			logging.Warningf(ctx, "failed to report tryjob_ended metric: %s", err)
		}
	}
	return nil
}

// isSemanticallyEqual returns whether two tryjob Results are semantically equal.
//
// Two results are semantically equal if all their attributes are equal except
// for the UpdateTime
func isSemanticallyEqual(left, right *tryjob.Result) bool {
	// Clearing the UpdateTime for both left and right for comparison but cloning
	// the results instead of modifying the inputs.
	left, right = proto.Clone(left).(*tryjob.Result), proto.Clone(right).(*tryjob.Result)
	if left != nil {
		left.UpdateTime = nil
	}
	if right != nil {
		right.UpdateTime = nil
	}
	return proto.Equal(left, right)
}

func (u *Updater) reportTryjobEndedStatus(ctx context.Context, tj *tryjob.Tryjob) error {
	r := &run.Run{ID: tj.LaunchedBy}
	if err := datastore.Get(ctx, r); err != nil {
		return transient.Tag.Apply(errors.Fmt("failed to load Run %s: %w", r.ID, err))
	}
	project, configGroup := r.ID.LUCIProject(), r.ConfigGroupID.Name()
	isRetry := true
	for _, exec := range r.Tryjobs.GetState().GetExecutions() {
		if len(exec.GetAttempts()) > 0 && exec.GetAttempts()[0].TryjobId == int64(tj.ID) {
			isRetry = false
			break
		}
	}
	tryjob.RunWithBuilderMetricsTarget(ctx, u.env, tj.Definition, func(ctx context.Context) {
		metrics.Public.TryjobEnded.Add(ctx, 1,
			project,
			configGroup,
			tj.Definition.GetCritical(),
			isRetry,
			versioning.TryjobStatusV0(tj.Status, tj.Result.GetStatus()).String(),
		)
	})
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
	return nil, errors.Fmt("%q backend is not supported", kind)
}
