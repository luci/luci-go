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

// Package poller implements stateful Gerrit polling.
package poller

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/gerrit"
	"go.chromium.org/luci/cv/internal/gerrit/gobmap"
)

const taskClassID = "poll-gerrit"

// pmNotifier encapsulates interaction with Project Manager by the Poller.
//
// In production, implemented by prjmanager.Notifier.
type pmNotifier interface {
	NotifyCLsUpdated(ctx context.Context, luciProject string, cls *changelist.CLUpdatedEvents) error
}

// CLUpdater encapsulates interaction with Gerrit CL Updater by the Poller.
type CLUpdater interface {
	Schedule(context.Context, *changelist.UpdateCLTask) error
	ScheduleDelayed(context.Context, *changelist.UpdateCLTask, time.Duration) error
}

// Poller polls Gerrit to discover new CLs and modifications of the existing
// ones.
type Poller struct {
	tqd       *tq.Dispatcher
	gFactory  gerrit.Factory
	clUpdater CLUpdater
	pm        pmNotifier
}

// New creates a new Poller, registering it in the given TQ dispatcher.
func New(tqd *tq.Dispatcher, g gerrit.Factory, clUpdater CLUpdater, pm pmNotifier) *Poller {
	p := &Poller{tqd, g, clUpdater, pm}
	tqd.RegisterTaskClass(tq.TaskClass{
		ID:           taskClassID,
		Prototype:    &PollGerritTask{},
		Queue:        "poll-gerrit",
		Quiet:        true,
		QuietOnError: true,
		Kind:         tq.NonTransactional,
		Handler: func(ctx context.Context, payload proto.Message) error {
			task := payload.(*PollGerritTask)
			ctx = logging.SetField(ctx, "project", task.GetLuciProject())
			err := p.poll(ctx, task.GetLuciProject(), task.GetEta().AsTime())
			return common.TQIfy{
				KnownRetry: []error{errConcurrentStateUpdate},
			}.Error(ctx, err)
		},
	})
	return p
}

// Poke schedules the next poll via task queue.
//
// Under perfect operation, this is redundant, but not harmful.
// Given bugs or imperfect operation, this ensures poller continues operating.
//
// Must not be called inside a datastore transaction.
func (p *Poller) Poke(ctx context.Context, luciProject string) error {
	if datastore.CurrentTransaction(ctx) != nil {
		return errors.New("must be called outside of transaction context")
	}
	return p.schedule(ctx, luciProject, time.Time{})
}

var errConcurrentStateUpdate = transient.Tag.Apply(errors.New("concurrent change to poller state"))

// poll executes the next poll with the latest known to poller config.
//
// For each discovered CL, enqueues a task for CL updater to refresh CL state.
// Automatically enqueues a new task to perform next poll.
func (p *Poller) poll(ctx context.Context, luciProject string, eta time.Time) error {
	if delay := clock.Now(ctx).Sub(eta); delay > maxAcceptableDelay {
		logging.Warningf(ctx, "poll %s arrived %s late; scheduling next poll instead", eta, delay)
		return p.schedule(ctx, luciProject, time.Time{})
	}
	// TODO(tandrii): avoid concurrent polling of the same project via cheap
	// best-effort locking in Redis.
	meta, err := prjcfg.GetLatestMeta(ctx, luciProject)
	switch {
	case err != nil:
	case (meta.Status == prjcfg.StatusDisabled || meta.Status == prjcfg.StatusNotExists):
		if err := gobmap.Update(ctx, &meta, nil); err != nil {
			return err
		}
		if err = datastore.Delete(ctx, &State{LuciProject: luciProject}); err != nil {
			return errors.Fmt("failed to disable poller for %q: %w", luciProject, err)
		}
		return nil
	case meta.Status == prjcfg.StatusEnabled:
		err = p.pollWithConfig(ctx, luciProject, meta)
	default:
		return fmt.Errorf("unknown project config status: %d", meta.Status)
	}

	switch {
	case err == nil:
		return p.schedule(ctx, luciProject, eta)
	case clock.Now(ctx).After(eta.Add(pollInterval - time.Second)):
		// Time to finish this task despite error, and trigger a new one.
		err = errors.Fmt("failed to do poll %s for %q: %w", eta, luciProject, err)
		common.LogError(ctx, err, errConcurrentStateUpdate)
		return p.schedule(ctx, luciProject, eta)
	default:
		return err
	}
}

// pollInterval is an approximate and merely best-effort average interval
// between polls of a single project.
//
// TODO(tandrii): revisit interval and error handling in pollWithConfig once CV
// subscribes to Gerrit PubSub.
const pollInterval = 10 * time.Second

// maxAcceptableDelay prevents polls which arrive too late from doing actual
// polling.
//
// maxAcceptableDelay / pollInterval effectively limits # concurrent polls of
// the same project that may happen due to task retries, delays, and queue
// throttling.
//
// Do not set too low, as this may prevent actual polling from happening at all
// if the poll TQ is overloaded.
const maxAcceptableDelay = 6 * pollInterval

// schedule schedules the future poll.
//
// Optional `after` can be set to the current task's ETA to ensure that next
// poll's task isn't de-duplicated with the current task.
func (p *Poller) schedule(ctx context.Context, luciProject string, after time.Time) error {
	// Desired properties:
	//   * for a single LUCI project, minimize p99 of actually observed poll
	//     intervals.
	//   * keep polling load on Gerrit at `1/pollInterval` per LUCI project;
	//   * avoid bursts of polls on Gerrit, i.e. distribute polls of diff projects
	//     throughout `pollInterval`.
	//
	// So,
	//   * de-duplicate poll tasks to 1 task per LUCI project per pollInterval;
	//   * vary epoch time, from which increments of pollInterval are done, by
	//     LUCI project. See projectOffset().
	if now := clock.Now(ctx); after.IsZero() || now.After(after) {
		after = now
	}
	offset := common.DistributeOffset(pollInterval, "gerrit-poller", luciProject)
	offset = offset.Truncate(time.Millisecond) // more readable logs
	eta := after.UTC().Truncate(pollInterval).Add(offset)
	for !eta.After(after) {
		eta = eta.Add(pollInterval)
	}
	task := &tq.Task{
		Title: luciProject,
		Payload: &PollGerritTask{
			LuciProject: luciProject,
			Eta:         timestamppb.New(eta),
		},
		ETA:              eta,
		DeduplicationKey: fmt.Sprintf("%s:%d", luciProject, eta.UnixNano()),
	}
	if err := p.tqd.AddTask(ctx, task); err != nil {
		return err
	}
	return nil
}

// State persists poller's State in datastore.
//
// State is exported for exposure via Admin API for debugging/observation.
// It must not be used elsewhere.
type State struct {
	_kind string `gae:"$kind,GerritPoller"`

	// Project is the name of the LUCI Project for which poller is working.
	LuciProject string `gae:"$id"`
	// UpdateTime is the timestamp when this state was last updated.
	UpdateTime time.Time `gae:",noindex"`
	// EVersion is the latest version number of the state.
	//
	// It increments by 1 every time state is updated either due to new project config
	// being updated OR after each successful poll.
	EVersion int64 `gae:",noindex"`
	// ConfigHash defines which Config version was last worked on.
	ConfigHash string `gae:",noindex"`
	// QueryStates tracks states of individual queries.
	//
	// Most LUCI projects will run just 1 query per Gerrit host.
	// But, if a LUCI project is watching many Gerrit projects (a.k.a. Git repos),
	// then the Gerrit projects may be split between several queries.
	//
	// TODO(tandrii): rename the datastore property name.
	QueryStates *QueryStates `gae:"SubPollers"`
}

// pollWithConfig performs the poll and if necessary updates to newest project config.
func (p *Poller) pollWithConfig(ctx context.Context, luciProject string, meta prjcfg.Meta) error {
	stateBefore := State{LuciProject: luciProject}
	switch err := datastore.Get(ctx, &stateBefore); {
	case err != nil && err != datastore.ErrNoSuchEntity:
		return transient.Tag.Apply(errors.Fmt("failed to get poller state for %q: %w", luciProject, err))
	case err == datastore.ErrNoSuchEntity || stateBefore.ConfigHash != meta.Hash():
		if err = p.updateConfig(ctx, &stateBefore, meta); err != nil {
			return err
		}
	}

	// Use WorkPool to limit concurrency, but keep track of errors per query
	// ourselves because WorkPool doesn't guarantee specific errors order.
	errs := make(errors.MultiError, len(stateBefore.QueryStates.GetStates()))
	err := parallel.WorkPool(10, func(work chan<- func() error) {
		for i, qs := range stateBefore.QueryStates.GetStates() {
			work <- func() error {
				ctx := logging.SetField(ctx, "gHost", qs.GetHost())
				err := p.doOneQuery(ctx, luciProject, qs)
				errs[i] = errors.WrapIf(err, "query %s", qs)
				return nil
			}
		}
	})
	if err != nil {
		return err
	}
	// Save state regardless of failure of individual queries.
	if saveErr := save(ctx, &stateBefore); saveErr != nil {
		// saving error supersedes per-query errors.
		return saveErr
	}
	err = common.MostSevereError(errs)
	switch n, first := errs.Summary(); {
	case n == len(errs):
		return errors.WrapIf(first, "no progress on any poller, first error")
	case err != nil:
		// Some progress. We'll retry during next poll.
		// TODO(tandrii): revisit this logic once CV subscribes to PubSub and makes
		// polling much less frequent.
		err = errors.Fmt("failed %d/%d queries for %q. The most severe error:: %w", n, len(errs), luciProject, err)
		common.LogError(ctx, err)
	}
	return nil
}

// updateConfig fetches latest config, updates gobmap and poller's own state.
func (p *Poller) updateConfig(ctx context.Context, s *State, meta prjcfg.Meta) error {
	s.ConfigHash = meta.Hash()
	cgs, err := meta.GetConfigGroups(ctx)
	if err != nil {
		return err
	}
	if err := gobmap.Update(ctx, &meta, cgs); err != nil {
		return err
	}
	proposed := partitionConfig(cgs)
	toUse, discarded := reuseIfPossible(s.QueryStates.GetStates(), proposed)
	for _, d := range discarded {
		if err := p.notifyOnUnmatchedCLs(
			ctx, s.LuciProject, d.GetHost(), d.Changes,
			changelist.UpdateCLTask_UPDATE_CONFIG); err != nil {
			return err
		}
	}
	s.QueryStates = &QueryStates{States: toUse}
	return nil
}

// save saves the state of poller after the poll.
func save(ctx context.Context, s *State) error {
	var innerErr error
	var copied State
	err := datastore.RunInTransaction(ctx, func(ctx context.Context) (err error) {
		defer func() { innerErr = err }()
		latest := State{LuciProject: s.LuciProject}
		switch err = datastore.Get(ctx, &latest); {
		case err == datastore.ErrNoSuchEntity:
			if s.EVersion > 0 {
				// At the beginning of the poll, we read an existing state.
				// So, there was a concurrent deletion.
				return errors.New("poller state was unexpectedly missing")
			}
			// Then, we'll create it.
		case err != nil:
			return transient.Tag.Apply(errors.Fmt("failed to get poller state: %w", err))
		case latest.EVersion != s.EVersion:
			return errConcurrentStateUpdate
		}
		copied = *s
		copied.EVersion++
		copied.UpdateTime = clock.Now(ctx).UTC()
		if err = datastore.Put(ctx, &copied); err != nil {
			return transient.Tag.Apply(errors.Fmt("failed to save poller state: %w", err))
		}
		return nil
	}, nil)

	switch {
	case innerErr != nil:
		return innerErr
	case err != nil:
		return transient.Tag.Apply(errors.Fmt("failed to save poller state: %w", err))
	default:
		*s = copied
		return nil
	}
}
