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

// Package implements stateful Gerrit polling.
package poller

import (
	"fmt"
	"hash/fnv"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/protobuf/proto"
	timestamppb "google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/cv/internal"
	"go.chromium.org/luci/cv/internal/config"
	"go.chromium.org/luci/cv/internal/gerrit/poller/task"
)

// Poke schedules the next poll via task queue.
//
// Under perfect operation, this is redundant, but not harmful.
// Given bugs or imperfect operation, this ensures poller continues operating.
//
// Must not be called inside a datastore transaction.
func Poke(ctx context.Context, luciProject string) error {
	if datastore.CurrentTransaction(ctx) != nil {
		panic("must be called outside of transaction context")
	}
	return schedule(ctx, luciProject, time.Time{})
}

// poll executes the next poll with the latest known to poller config.
//
// For each discovered CL, enqueues a task for CL updater to refresh CL state.
// Automatically enqueues a new task to perform next poll.
func poll(ctx context.Context, luciProject string, eta time.Time) error {
	if now := clock.Now(ctx); now.Add(10 * time.Millisecond).Before(eta) {
		// NOTE: if this happens in production, it probably means that we have
		// concurrent polling of gerrit than intended.
		// TODO(tandrii): avoid concurent polling of the same project via cheap
		// best-effort locking in Redis.
		logging.Warningf(ctx, "TQ triggered this task before eta %s (now: %s)", eta, now)
	}
	meta, err := config.GetLatestMeta(ctx, luciProject)
	switch {
	case err != nil:

	case (meta.Status == config.StatusDisabled ||
		meta.Status == config.StatusNotExists):
		if err = datastore.Delete(ctx, &state{LuciProject: luciProject}); err != nil {
			return errors.Annotate(err, "failed to disable poller for %q", luciProject).Err()
		}
		return nil
	case meta.Status == config.StatusEnabled:
		err = pollWithConfig(ctx, luciProject, meta)
	default:
		panic(fmt.Errorf("unknown project config status: %d", meta.Status))
	}

	switch {
	case err == nil:
		return schedule(ctx, luciProject, eta)
	case clock.Now(ctx).After(eta.Add(pollInterval - time.Second)):
		// Time to finish this task despite error, and trigger a new one.
		errors.Log(ctx, errors.Annotate(err, "failed to do poll %s", eta).Err())
		return schedule(ctx, luciProject, eta)
	default:
		return err
	}
}

func init() {
	tq.RegisterTaskClass(tq.TaskClass{
		ID:        task.ClassID,
		Prototype: &task.PollGerritTask{},
		Queue:     "poll-gerrit",
		Handler: func(ctx context.Context, payload proto.Message) error {
			task := payload.(*task.PollGerritTask)
			if err := poll(ctx, task.GetLuciProject(), task.GetEta().AsTime()); err != nil {
				errors.Log(ctx, err)
				if !transient.Tag.In(err) {
					err = tq.Fatal.Apply(err)
				}
				return err
			}
			return nil
		},
	})
}

// pollInterval is approximate and merely best effort average interval between
// polls of a single project.
//
// TODO(tandrii): revisit interval and error handling in pollWithConfig once CV
// subscribes to Gerrit PubSub.
const pollInterval = 10 * time.Second

// schedule schedules the future poll.
//
// Optional `after` can be set to the current task's ETA to ensure that next
// poll's task isn't de-duplicated with the current task.
func schedule(ctx context.Context, luciProject string, after time.Time) error {
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
	offset := projectOffset(luciProject, pollInterval)
	offset = offset.Truncate(time.Millisecond) // more readable logs
	eta := after.UTC().Truncate(pollInterval).Add(offset)
	for !eta.After(after) {
		eta = eta.Add(pollInterval)
	}
	task := &tq.Task{
		Payload: &task.PollGerritTask{
			LuciProject: luciProject,
			Eta:         timestamppb.New(eta),
		},
		ETA:              eta,
		DeduplicationKey: fmt.Sprintf("%s:%d", luciProject, eta.UnixNano()),
	}
	if err := tq.AddTask(ctx, task); err != nil {
		return err
	}
	logging.Debugf(ctx, "scheduled next poll for %q at %s", luciProject, eta)
	return nil
}

// projectOffset chooses an offset per LUCI project in [0..pollInterval) range
// and aims for uniform distribution across LUCI projects.
func projectOffset(luciProject string, pollInterval time.Duration) time.Duration {
	// Basic idea: interval/N*random(0..N), but deterministic on luciProject.
	// Use fast hash function, as we don't need strong collision resistance.
	h := fnv.New32a()
	h.Write([]byte(luciProject))
	r := h.Sum32()

	i := int64(pollInterval)
	// Avoid losing precision for low pollInterval values by first shifting them
	// the more significant bits.
	shifted := 0
	for i < (int64(1) << 55) {
		i = i << 7
		shifted += 7
	}
	// i = i / N * r, where N = 2^32 since r is (0..2^32-1).
	i = (i >> 32) * int64(r)
	return time.Duration(i >> shifted)
}

// state persists poller's state in datastore.
type state struct {
	_kind string `gae:"$kind,GerritPoller"`

	// Project is the name of the LUCI Project for which poller is works.
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
	// SubPollers track individual states of sub pollers.
	//
	// Most LUCI projects will have just 1 per Gerrit host,
	// but CV may split the set of watched Gerrit projects (aka Git repos) on the
	// same Gerrit host among several SubPollers.
	SubPollers *SubPollers
}

// pollWithConfig performs the poll and if necessary updates to newest project config.
func pollWithConfig(ctx context.Context, luciProject string, meta config.Meta) error {
	stateBefore := state{LuciProject: luciProject}
	switch err := datastore.Get(ctx, &stateBefore); {
	case err != nil && err != datastore.ErrNoSuchEntity:
		return errors.Annotate(err, "failed to get poller state for %q", luciProject).Tag(
			transient.Tag).Err()
	case err == datastore.ErrNoSuchEntity || stateBefore.ConfigHash != meta.Hash():
		if err = updateConfig(ctx, &stateBefore, meta); err != nil {
			return err
		}
	}

	// Use WorkPool to limit concurrency, but keep track of errors per SubPollers
	// ourselves because WorkPool doesn't guarantee specific errors order.
	errs := make(errors.MultiError, len(stateBefore.SubPollers.GetSubPollers()))
	err := parallel.WorkPool(10, func(work chan<- func() error) {
		for i, sp := range stateBefore.SubPollers.GetSubPollers() {
			i, sp := i, sp
			work <- func() error {
				errs[i] = subpoll(ctx, luciProject, sp)
				return nil
			}
		}
	})
	if err != nil {
		panic(err)
	}
	// Save state regardless of failure of individual subpollers.
	if saveErr := save(ctx, &stateBefore); saveErr != nil {
		// saving error supersedes subpoller errors.
		return saveErr
	}
	err = internal.MostSevereError(errs)
	switch n, first := errs.Summary(); {
	case n == len(errs):
		return errors.Annotate(first, "no progress on any poller, first error").Err()
	case err != nil:
		// Some progress. We'll retry during next poll.
		// TODO(tandrii): revisit this logic once CV subscribes to PubSub and makes
		// polling much less frequent.
		logging.Errorf(ctx, "failed %d/%d pollers", n, len(errs))
		errors.Log(ctx, err)
	}
	return nil
}

// updateConfig fetches latest config and updates poller's state
// in RAM only.
func updateConfig(ctx context.Context, s *state, meta config.Meta) error {
	s.ConfigHash = meta.Hash()
	cgs, err := meta.GetConfigGroups(ctx)
	if err != nil {
		return err
	}
	s.SubPollers = &SubPollers{SubPollers: partitionConfig(cgs)}
	return nil
}

// save saves state of poller after the poll.
func save(ctx context.Context, modified *state) error {
	err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		latest := state{LuciProject: modified.LuciProject}
		switch err := datastore.Get(ctx, &latest); {
		case err == datastore.ErrNoSuchEntity:
			if modified.EVersion > 0 {
				// At the beginning of the poll, we read an existing state.
				// So, there was a concurrent deletion.
				return errors.Reason("poller state for %q was unexpectedly missing",
					modified.LuciProject).Err()
			}
			// Then, we'll create it.
		case err != nil:
			return errors.Annotate(err, "failed to get poller state for %q", modified.LuciProject).Err()
		case latest.EVersion != modified.EVersion:
			return errors.Reason("concurrent change to poller state %q", modified.LuciProject).Err()
		}
		modified.EVersion++
		modified.UpdateTime = clock.Now(ctx).UTC()
		err := datastore.Put(ctx, modified)
		if err != nil {
			return errors.Annotate(err, "failed to save poller state for %s", modified.LuciProject).Err()
		}
		return nil
	}, nil)
	return errors.Annotate(err, "failed to save poller state for %q", modified.LuciProject).Tag(
		transient.Tag).Err()
}
