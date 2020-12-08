// Copyright 2020 The LUCI Authors.
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

package impl

import (
	"context"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/cv/internal/config"
	"go.chromium.org/luci/cv/internal/prjmanager/internal"
	"go.chromium.org/luci/gae/service/datastore"
)

// stateMachine implements a state machine of a ProjectManager.
type stateMachine struct {
	// pe is a project entity loaded from Datastore.
	//
	// It must be saved only after verifying transactionally that Datastore
	// version hasn't chagned since.
	pe *project

	// dirty is true if commit() should be called to save state.
	dirty bool
	// expectedEVersion is used by commit() to verify project didn't change since
	// readState().
	expectedEVersion int
	// consumed stores events that must be deleted in commit(),
	consumed []*internal.Event
	// commitClbks stores callbacks that must be called in commit().
	commitClbks []func(context.Context) error
}

// readState initializes stateMachine from Datastore.
func readState(ctx context.Context, luciProject string) (*stateMachine, error) {
	pe := &project{ID: luciProject}
	switch err := datastore.Get(ctx, pe); {
	case err == datastore.ErrNoSuchEntity || err == nil:
		return &stateMachine{pe: pe, expectedEVersion: pe.EVersion}, nil
	default:
		return nil, errors.Annotate(err, "failed to get %q", luciProject).Tag(
			transient.Tag).Err()
	}
}

// mutate consumes incoming events and performs the necessary mutation.
//
// It's not atomic (it may have side effects even when error is returned),
// but it's idempotent, so calling it twice with the same events is OK.
func (sm *stateMachine) mutate(ctx context.Context, tr *triageResult) error {
	mctx := logging.SetField(ctx, "mutating", "tentative")

	logging.Debugf(mctx, "started")
	// Prioritize config updates above everything else.
	_, mutErr := sm.updateConfig(mctx, tr)
	// TODO(tandrii): perform other mutations.
	logging.Debugf(mctx, "ended with dirty=%t", sm.dirty)
	if !sm.dirty {
		return mutErr
	}

	transErr := datastore.RunInTransaction(ctx, sm.commit, nil)
	switch {
	case transErr != nil:
		return errors.Annotate(transErr, "failed to commit %q", sm.luciProject()).Tag(
			transient.Tag).Err()
	case mutErr != nil:
		logging.Warningf(ctx, "partial success committed, but 1 mutation failed for %q",
			sm.luciProject())
		return mutErr
	default:
		return nil
	}
}

// commit commits changes buffered so far to datastore.
//
// Called within a transaction, hence it can be called multiple times.
// This is why sm.expectedEVersion is stored separate from sm.pe, which is
// modified in this func before trying to store it to datastoer.
func (sm *stateMachine) commit(ctx context.Context) error {
	// Ensure project.EVersion is what we read before.
	latest := project{ID: sm.luciProject()}
	switch err := datastore.Get(ctx, &latest); {
	case err == datastore.ErrNoSuchEntity || err == nil:
		if latest.EVersion != sm.expectedEVersion {
			return errors.Reason("concurrent update of %q to %d, have %d",
				sm.luciProject(), latest.EVersion, sm.expectedEVersion).Err()
		}
	default:
		return errors.Annotate(err, "failed to get latest").Err()
	}

	// TODO(tandrii): optimize to run in parallel if necessary.
	for _, clbk := range sm.commitClbks {
		if err := clbk(ctx); err != nil {
			return err
		}
	}

	// TODO(tandrii): this can be optimised using approach similar to dsset.Set:
	// Add 1 tombstone entity which list IDs of all consumed events, but don't
	// touch events themselves in a transaction. On success, delete event entities
	// and then the tombstone as a best effort. If cleanup fails, it's fine since
	// proper cleanup can be guaranteed right before the next mutation as part of
	// event triage by discoving the tombstone.
	if err := internal.Delete(ctx, sm.consumed); err != nil {
		return err
	}

	// Finally, save project entity itself.
	sm.pe.UpdateTime = clock.Now(ctx).UTC()
	sm.pe.EVersion = sm.expectedEVersion + 1
	return errors.Annotate(datastore.Put(ctx, sm.pe), "failed to Put newest").Err()
}

///////////////////////////////////////////////////////////////////////////////
// implementation of mutations

type mutation func(ctx context.Context, tr *triageResult) (shouldStop bool, err error)

func (sm *stateMachine) updateConfig(ctx context.Context, tr *triageResult) (
	shouldStop bool, err error) {
	if len(tr.updateConfig) == 0 {
		// There is no reason to continue processing events if PM is stopped
		// until there is new updateConfig event.
		shouldStop = (sm.pe.State == PMState_PM_STATE_STOPPED)
		return
	}
	var meta config.Meta
	meta, err = config.GetLatestMeta(ctx, sm.luciProject())
	if err != nil {
		return
	}
	sm.dirty = true
	sm.consume(tr.updateConfig...)

	switch meta.Status {
	case config.StatusEnabled:
		if sm.pe.State == PMState_PM_STATE_STARTED && meta.Hash() == sm.pe.ConfigHash {
			return // already up-to-date.
		}
		sm.notifyGoBPoller()
		sm.notifyActiveRuns()
		sm.pe.ConfigHash = meta.Hash()
		// NOTE: we may be in STOPPING phase, and some Runs are now finalizing
		// themselves, while others haven't yet even noticed the stopping.
		// The former will eventually be removed from sm.pe.ActiveRuns,
		// while the latter will continue running.
		sm.pe.State = PMState_PM_STATE_STARTED
		sm.reevalPending()

	case config.StatusDisabled, config.StatusNotExists:
		logging.Debugf(ctx, "config.Status %s", meta.Status)
		// NOTE: we are intentionally not catching up with new ConfigHash (if any),
		// since it's not actionable.
		switch sm.pe.State {
		case PMState_PM_STATE_STOPPED:
			// There is no reason to continue processing events.
			shouldStop = true
		case PMState_PM_STATE_STARTED:
			sm.notifyGoBPoller()
			sm.notifyActiveRuns()
			sm.pe.State = PMState_PM_STATE_STOPPING
			fallthrough
		case PMState_PM_STATE_STOPPING:
			if len(sm.pe.ActiveRuns) == 0 {
				sm.pe.State = PMState_PM_STATE_STOPPED
			}
		}
	}
	return
}

///////////////////////////////////////////////////////////////////////////////
// helpers

func (sm *stateMachine) luciProject() string {
	return sm.pe.ID
}

func (sm *stateMachine) consume(events ...*internal.Event) {
	sm.consumed = append(sm.consumed, events...)
}

func (sm *stateMachine) notifyGoBPoller() {
	sm.commitClbks = append(sm.commitClbks, func(ctx context.Context) error {
		// TODO(tandrii): call poller.Poke once it accepts non-transactional
		// context.
		// return poller.Poke(ctx, sm.luciProject())
		return nil
	})
}

func (sm *stateMachine) notifyActiveRuns() {
	// TODO(tandrii): notify all sm.pe.ActiveRuns.
}

func (sm *stateMachine) reevalPending() {
	// TODO(tandrii): re-evaluate currently pending CLs given new config.
}
