// Copyright 2017 The LUCI Authors.
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

// Package demo shows how cron.Machines can be hosted with Datastore and TQ.
package demo

import (
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"golang.org/x/net/context"

	"github.com/golang/protobuf/proto"
	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/gae/service/info"
	"go.chromium.org/gae/service/memcache"

	"go.chromium.org/luci/appengine/memlock"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/appengine/gaemiddleware/standard"
	"go.chromium.org/luci/appengine/tq"
	"go.chromium.org/luci/scheduler/appengine/engine/cron"
	"go.chromium.org/luci/scheduler/appengine/engine/dsset"
	"go.chromium.org/luci/scheduler/appengine/engine/internal"
	"go.chromium.org/luci/scheduler/appengine/schedule"
)

var dispatcher = tq.Dispatcher{}

// CronState represents the state of a cron task.
type CronState struct {
	_extra datastore.PropertyMap `gae:"-,extra"`

	ID    string     `gae:"$id"`
	State cron.State `gae:",noindex"`

	NextInvID  int64   `gae:",noindex"`
	RunningSet []int64 `gae:",noindex"`
}

func (s *CronState) schedule() *schedule.Schedule {
	parsed, err := schedule.Parse(s.ID, 0)
	if err != nil {
		panic(err)
	}
	return parsed
}

func pendingTriggersSet(c context.Context, jobID string) *dsset.Set {
	return &dsset.Set{
		ID:              "triggers:" + jobID,
		ShardCount:      8,
		TombstonesRoot:  datastore.KeyForObj(c, &CronState{ID: jobID}),
		TombstonesDelay: 15 * time.Minute,
	}
}

func recentlyFinishedSet(c context.Context, jobID string) *dsset.Set {
	return &dsset.Set{
		ID:              "finished:" + jobID,
		ShardCount:      8,
		TombstonesRoot:  datastore.KeyForObj(c, &CronState{ID: jobID}),
		TombstonesDelay: 15 * time.Minute,
	}
}

func pokeMachine(c context.Context, entity *CronState, cb func(context.Context, *cron.Machine) error) error {
	machine := &cron.Machine{
		Now:      clock.Now(c),
		Schedule: entity.schedule(),
		Nonce:    func() int64 { return mathrand.Get(c).Int63() + 1 },
		State:    entity.State,
	}

	if err := cb(c, machine); err != nil {
		return err
	}

	tasks := []*tq.Task{}
	for _, action := range machine.Actions {
		switch a := action.(type) {
		case cron.TickLaterAction:
			logging.Infof(c, "Scheduling tick %d after %s", a.TickNonce, a.When.Sub(time.Now()))
			tasks = append(tasks, &tq.Task{
				Payload: &internal.TickLaterTask{JobId: entity.ID, TickNonce: a.TickNonce},
				ETA:     a.When,
			})
		case cron.StartInvocationAction:
			tasks = append(tasks, &tq.Task{
				Payload: &internal.TriggerInvocationTask{JobId: entity.ID, TriggerId: mathrand.Get(c).Int63()},
			})
		default:
			panic("unknown action type")
		}
	}
	if err := dispatcher.AddTask(c, tasks...); err != nil {
		return err
	}

	entity.State = machine.State
	return nil
}

func evolve(c context.Context, id string, cb func(context.Context, *cron.Machine) error) error {
	err := datastore.RunInTransaction(c, func(c context.Context) error {
		entity := &CronState{ID: id}
		if err := datastore.Get(c, entity); err != nil && err != datastore.ErrNoSuchEntity {
			return err
		}
		if err := pokeMachine(c, entity, cb); err != nil {
			return err
		}
		return datastore.Put(c, entity)
	}, nil)
	return transient.Tag.Apply(err)
}

func startJob(c context.Context, id string) error {
	return evolve(c, id, func(c context.Context, m *cron.Machine) error {
		// Forcefully restart the chain of tasks.
		m.Disable()
		m.Enable()
		return nil
	})
}

func addTrigger(c context.Context, jobID, triggerID string) error {
	logging.Infof(c, "Triggering %q - %q", jobID, triggerID)

	// Add the trigger request to the pending set.
	if err := pendingTriggersSet(c, jobID).Add(c, []dsset.Item{{ID: triggerID}}); err != nil {
		return err
	}

	// Run a task that examines the pending set and makes decisions.
	return kickTriageTask(c, jobID)
}

func kickTriageTask(c context.Context, jobID string) error {
	// Throttle to once per 2 sec (and make sure it is always in the future).
	eta := clock.Now(c).Unix()
	eta = (eta/2 + 1) * 2
	dedupKey := fmt.Sprintf("triage:%s:%d", jobID, eta)

	// Use cheaper but crappier memcache as a first guard.
	itm := memcache.NewItem(c, dedupKey).SetExpiration(time.Minute)
	if memcache.Get(c, itm) == nil {
		logging.Infof(c, "The triage task has already been scheduled")
		return nil // already added!
	}

	err := dispatcher.AddTask(c, &tq.Task{
		DeduplicationKey: dedupKey,
		ETA:              time.Unix(eta, 0),
		Payload:          &internal.TriageTriggersTask{JobId: jobID},
	})
	if err != nil {
		return err
	}
	logging.Infof(c, "Scheduled the triage task")

	// Best effort in setting memcache flag. No big deal if it fails.
	memcache.Set(c, itm)
	return nil
}

func handleTick(c context.Context, task proto.Message) error {
	msg := task.(*internal.TickLaterTask)
	return evolve(c, msg.JobId, func(c context.Context, m *cron.Machine) error {
		return m.OnTimerTick(msg.TickNonce)
	})
}

func handleTrigger(c context.Context, task proto.Message) error {
	msg := task.(*internal.TriggerInvocationTask)
	return addTrigger(c, msg.JobId, fmt.Sprintf("cron:%d", msg.TriggerId))
}

func handleTriage(c context.Context, task proto.Message) error {
	msg := task.(*internal.TriageTriggersTask)
	logging.Infof(c, "Triaging requests for %q", msg.JobId)

	err := memlock.TryWithLock(c, "triageLock:"+msg.JobId, info.RequestID(c), func(context.Context) error {
		logging.Infof(c, "Got the lock!")
		return runTriage(c, msg.JobId)
	})
	return transient.Tag.Apply(err)
}

func runTriage(c context.Context, jobID string) error {
	wg := sync.WaitGroup{}
	wg.Add(2)

	var triggersList *dsset.Listing
	var triggersErr error

	var finishedList *dsset.Listing
	var finishedErr error

	// Grab all pending requests (and stuff to cleanup).
	triggersSet := pendingTriggersSet(c, jobID)
	go func() {
		defer wg.Done()
		triggersList, triggersErr = triggersSet.List(c)
		if triggersErr == nil {
			logging.Infof(c, "Triggers: %d items, %d tombs to cleanup",
				len(triggersList.Items), len(triggersList.Garbage))
		}
	}()

	// Same for recently finished invocations.
	finishedSet := recentlyFinishedSet(c, jobID)
	go func() {
		defer wg.Done()
		finishedList, finishedErr = finishedSet.List(c)
		if finishedErr == nil {
			logging.Infof(c, "Finished: %d items, %d tombs to cleanup",
				len(finishedList.Items), len(finishedList.Garbage))
		}
	}()

	wg.Wait()
	switch {
	case triggersErr != nil:
		return triggersErr
	case finishedErr != nil:
		return finishedErr
	}

	// Do cleanups first.
	if err := dsset.CleanupGarbage(c, triggersList.Garbage, finishedList.Garbage); err != nil {
		return err
	}

	var cleanup dsset.Garbage
	err := datastore.RunInTransaction(c, func(c context.Context) error {
		state := &CronState{ID: jobID}
		if err := datastore.Get(c, state); err != nil && err != datastore.ErrNoSuchEntity {
			return err
		}

		popOps := []*dsset.PopOp{}

		// Tidy RunningSet by removing all recently finished invocations.
		if len(finishedList.Items) != 0 || len(finishedList.Garbage) != 0 {
			op, err := finishedSet.BeginPop(c, finishedList)
			if err != nil {
				return err
			}
			popOps = append(popOps, op)

			reallyFinished := map[int64]struct{}{}
			for _, itm := range finishedList.Items {
				if op.Pop(itm.ID) {
					id, _ := strconv.ParseInt(itm.ID, 10, 64)
					reallyFinished[id] = struct{}{}
				}
			}

			filtered := []int64{}
			for _, id := range state.RunningSet {
				if _, yep := reallyFinished[id]; yep {
					logging.Infof(c, "Invocation finished-%d is acknowledged as finished", id)
				} else {
					filtered = append(filtered, id)
				}
			}
			state.RunningSet = filtered
		}

		// Launch new invocations for each pending trigger.
		if len(triggersList.Items) != 0 || len(triggersList.Garbage) != 0 {
			op, err := triggersSet.BeginPop(c, triggersList)
			if err != nil {
				return err
			}
			popOps = append(popOps, op)

			batch := internal.LaunchInvocationsBatchTask{JobId: state.ID}
			for _, trigger := range triggersList.Items {
				if op.Pop(trigger.ID) {
					logging.Infof(c, "Launching new launch-%d for trigger %s", state.NextInvID, trigger.ID)
					state.RunningSet = append(state.RunningSet, state.NextInvID)
					batch.InvId = append(batch.InvId, state.NextInvID)
					state.NextInvID++
				}
			}
			// Transactionally trigger a batch with new invocations.
			if len(batch.InvId) != 0 {
				if err := dispatcher.AddTask(c, &tq.Task{Payload: &batch}); err != nil {
					return err
				}
			}
		}

		// Submit set changes.
		var err error
		if cleanup, err = dsset.FinishPop(c, popOps...); err != nil {
			return err
		}

		logging.Infof(c, "Running invocations - %v", state.RunningSet)

		// If nothing is running, poke the cron machine. Maybe it wants to start
		// something.
		if len(state.RunningSet) == 0 {
			err = pokeMachine(c, state, func(c context.Context, m *cron.Machine) error {
				m.RewindIfNecessary()
				return nil
			})
			if err != nil {
				return err
			}
		}

		// Done!
		return datastore.Put(c, state)
	}, nil)

	if err == nil && len(cleanup) != 0 {
		// Best effort cleanup of storage of consumed items.
		logging.Infof(c, "Cleaning up storage of %d items", len(cleanup))
		if err := dsset.CleanupGarbage(c, cleanup); err != nil {
			logging.Warningf(c, "Best effort cleanup failed - %s", err)
		}
	}

	return transient.Tag.Apply(err)
}

func handleBatchLaunch(c context.Context, task proto.Message) error {
	msg := task.(*internal.LaunchInvocationsBatchTask)
	logging.Infof(c, "Batch launch for %q", msg.JobId)

	tasks := []*tq.Task{}
	for _, invId := range msg.InvId {
		logging.Infof(c, "Launching inv-%d", invId)
		tasks = append(tasks, &tq.Task{
			DeduplicationKey: fmt.Sprintf("inv:%s:%d", msg.JobId, invId),
			Payload: &internal.LaunchInvocationTask{
				JobId: msg.JobId,
				InvId: invId,
			},
		})
	}

	return dispatcher.AddTask(c, tasks...)
}

func handleLaunchTask(c context.Context, task proto.Message) error {
	msg := task.(*internal.LaunchInvocationTask)
	logging.Infof(c, "Executing invocation %q: exec-%d", msg.JobId, msg.InvId)

	// There can be more stuff here. But we just finish the invocation right away.

	finishedSet := recentlyFinishedSet(c, msg.JobId)
	err := finishedSet.Add(c, []dsset.Item{
		{ID: fmt.Sprintf("%d", msg.InvId)},
	})
	if err != nil {
		return err
	}

	// Kick the triage now that the set of running invocations has been modified.
	return kickTriageTask(c, msg.JobId)
}

func init() {
	r := router.New()
	standard.InstallHandlers(r)

	dispatcher.RegisterTask(&internal.TickLaterTask{}, handleTick, "timers", nil)
	dispatcher.RegisterTask(&internal.TriggerInvocationTask{}, handleTrigger, "triggers", nil)
	dispatcher.RegisterTask(&internal.TriageTriggersTask{}, handleTriage, "triages", nil)
	dispatcher.RegisterTask(&internal.LaunchInvocationsBatchTask{}, handleBatchLaunch, "launches", nil)
	dispatcher.RegisterTask(&internal.LaunchInvocationTask{}, handleLaunchTask, "invocations", nil)
	dispatcher.InstallRoutes(r, standard.Base())

	// Kick-start a bunch of jobs by visiting:
	//
	// http://localhost:8080/start/with 10s interval
	// http://localhost:8080/start/with 5s interval
	// http://localhost:8080/start/0 * * * * * * *
	//
	// And the look at the logs.

	r.GET("/start/:JobID", standard.Base(), func(c *router.Context) {
		jobID := c.Params.ByName("JobID")
		if err := startJob(c.Context, jobID); err != nil {
			panic(err)
		}
	})

	r.GET("/trigger/:JobID", standard.Base(), func(c *router.Context) {
		jobID := c.Params.ByName("JobID")
		triggerID := fmt.Sprintf("manual:%d", clock.Now(c.Context).UnixNano())
		if err := addTrigger(c.Context, jobID, triggerID); err != nil {
			panic(err)
		}
	})

	http.DefaultServeMux.Handle("/", r)
}
