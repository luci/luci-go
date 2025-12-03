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

package engine

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/appengine/tq"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/realms"

	"go.chromium.org/luci/scheduler/appengine/internal"
	"go.chromium.org/luci/scheduler/appengine/task"
)

// errUpdateConflict means Invocation is being modified by two TaskController's
// concurrently. It should not be happening often. The most likely situation
// is when the same invocation is poked via a timer and via an external PubSub
// event at the exact same time.
//
// This error is marked as transient to indicate to the scheduler guts that this
// is a transient condition (there's a bunch of transient.Tag.In checks which
// are sensitive to this). At the same time we don't want to keep retrying
// the second part of a two-part transaction: it's useless, the result will be
// the same conflict. So the error is tagged with abortTransaction to exit the
// inner retry loop in runTxn.
//
// Finally, we do want to retry the entire two-part transaction. To indicate
// this, the error is tagged with tq.Retry. It signals to the task queue layer
// (and to the pubsub) that the task should be redelivered later.
var errUpdateConflict = errors.New(
	"concurrent modifications of an Invocation",
	transient.Tag,
	abortTransaction,
	tq.Retry,
)

// taskController manages execution of single invocation.
//
// It is short-lived object spawned to handle some single event in the lifecycle
// of the invocation.
type taskController struct {
	ctx context.Context // for DebugLog logging only
	eng *engineImpl     // for prepareTopic and invChanging

	task    proto.Message // extracted from saved.Task blob
	manager task.Manager
	request task.Request

	saved    Invocation        // what have been given initially or saved in Save()
	state    task.State        // state mutated by TaskManager
	debugLog string            // mutated by DebugLog
	timers   []*internal.Timer // new timers, mutated by AddTimer

	triggers     []*internal.Trigger // new outgoing triggers, mutated by EmitTrigger
	triggerIndex int64               // incremented with each new emitted trigger

	consumedTimers stringset.Set // timers popped in consumeTimer
}

// controllerForInvocation returns new instance of taskController configured
// to work with given invocation.
//
// If task definition can't be deserialized, returns both controller and error.
func controllerForInvocation(c context.Context, e *engineImpl, inv *Invocation) (*taskController, error) {
	ctl := &taskController{
		ctx:   c,
		eng:   e,
		saved: *inv,
	}
	var err error
	if ctl.task, err = e.cfg.Catalog.UnmarshalTask(c, inv.Task, ctl.RealmID()); err != nil {
		return ctl, fmt.Errorf("failed to unmarshal the task - %s", err)
	}
	if ctl.manager = e.cfg.Catalog.GetTaskManager(ctl.task); ctl.manager == nil {
		return ctl, fmt.Errorf("TaskManager is unexpectedly missing")
	}
	if err = ctl.populateState(); err != nil {
		return ctl, fmt.Errorf("failed to construct task.State - %s", err)
	}
	if ctl.request, err = getRequestFromInv(&ctl.saved); err != nil {
		return ctl, fmt.Errorf("failed to construct task.Request - %s", err)
	}
	return ctl, nil
}

// populateState populates 'state' using data in 'saved'.
func (ctl *taskController) populateState() error {
	ctl.state = task.State{
		Status:   ctl.saved.Status,
		ViewURL:  ctl.saved.ViewURL,
		TaskData: append([]byte(nil), ctl.saved.TaskData...), // copy
	}
	return nil
}

// consumeTimer removes the given timer from the invocation, if it is there.
//
// Returns true if it was there, false if it wasn't, or an error on
// deserialization errors.
//
// We can't modify 'saved' in place, since it is supposed to always match
// what is (or has been) in the datastore. So delay the entity modification
// until Save() call.
func (ctl *taskController) consumeTimer(timerID string) (bool, error) {
	if ctl.consumedTimers != nil && ctl.consumedTimers.Has(timerID) {
		return false, nil // already consumed
	}
	timers, err := unmarshalTimersList(ctl.saved.PendingTimersRaw)
	if err != nil {
		return false, err
	}
	for _, t := range timers {
		if t.Id == timerID {
			if ctl.consumedTimers == nil {
				ctl.consumedTimers = stringset.New(1)
			}
			ctl.consumedTimers.Add(timerID)
			return true, nil
		}
	}
	return false, nil
}

// JobID is part of task.Controller interface.
func (ctl *taskController) JobID() string {
	return ctl.saved.JobID
}

// InvocationID is part of task.Controller interface.
func (ctl *taskController) InvocationID() int64 {
	return ctl.saved.ID
}

// RealmID is part of task.Controller interface.
func (ctl *taskController) RealmID() string {
	if ctl.saved.RealmID != "" {
		return ctl.saved.RealmID
	}
	proj := strings.Split(ctl.saved.JobID, "/")[0]
	return realms.Join(proj, realms.LegacyRealm)
}

// Task is part of task.Controller interface.
func (ctl *taskController) Task() proto.Message {
	return ctl.task
}

// Request is part of task.Controller interface.
func (ctl *taskController) Request() task.Request {
	return ctl.request
}

// State is part of task.Controller interface.
func (ctl *taskController) State() *task.State {
	return &ctl.state
}

// DebugLog is part of task.Controller interface.
func (ctl *taskController) DebugLog(format string, args ...any) {
	logging.Infof(ctl.ctx, format, args...)
	debugLog(ctl.ctx, &ctl.debugLog, format, args...)
}

// AddTimer is part of Controller interface.
func (ctl *taskController) AddTimer(ctx context.Context, delay time.Duration, title string, payload []byte) {
	// ID for the new timer. It is guaranteed to be unique when we land the
	// transaction because all ctl.saved modifications are serialized in time (see
	// MutationsCount checks in Save), and we include serial number of such
	// modification into the timer id (and suffix it with the index of the timer
	// emitted by this particular modification). If two modification happen
	// concurrently, they may temporary get same timer ID, but only one will
	// actually land.
	timerID := fmt.Sprintf("%s:%d:%d:%d",
		ctl.JobID(), ctl.InvocationID(),
		ctl.saved.MutationsCount, len(ctl.timers))

	ctl.DebugLog("Scheduling timer %q (%s) after %s", title, timerID, delay)

	now := clock.Now(ctx)
	ctl.timers = append(ctl.timers, &internal.Timer{
		Id:      timerID,
		Created: timestamppb.New(now),
		Eta:     timestamppb.New(now.Add(delay)),
		Title:   title,
		Payload: payload,
	})
}

// PrepareTopic is part of task.Controller interface.
func (ctl *taskController) PrepareTopic(ctx context.Context, publisher string) (topic string, token string, err error) {
	return ctl.eng.prepareTopic(ctx, &topicParams{
		jobID:     ctl.JobID(),
		invID:     ctl.InvocationID(),
		manager:   ctl.manager,
		publisher: publisher,
	})
}

// GetClient is part of task.Controller interface
func (ctl *taskController) GetClient(ctx context.Context, opts ...auth.RPCOption) (*http.Client, error) {
	opts = append(opts, auth.WithProject(ctl.saved.GetProjectID()))
	t, err := auth.GetRPCTransport(ctx, auth.AsProject, opts...)
	if err != nil {
		return nil, err
	}
	return &http.Client{Transport: t}, nil
}

// EmitTrigger is part of task.Controller interface.
func (ctl *taskController) EmitTrigger(ctx context.Context, trigger *internal.Trigger) {
	ctl.DebugLog("Emitting a trigger %s", trigger.Id)
	trigger.JobId = ctl.JobID()
	trigger.InvocationId = ctl.InvocationID()

	// See docs for internal.Trigger proto. Tuple (created, order_in_batch) used
	// for casual ordering of triggers emitted by an invocation. Callers of
	// EmitTrigger are free to override this by supplying their own timestamp. In
	// such case they also should provide order_in_batch. Otherwise we build the
	// tuple for them based on the order of EmitTrigger calls.
	if trigger.Created == nil {
		trigger.Created = timestamppb.New(clock.Now(ctx))
		trigger.OrderInBatch = ctl.triggerIndex
	}

	ctl.triggers = append(ctl.triggers, trigger)
	ctl.triggerIndex++ // note: this is NOT reset in Save, unlike ctl.triggers.
}

// Save uploads updated Invocation to the datastore, updating the state of the
// corresponding job, if necessary.
//
// Returns errUpdateConflict if the invocation was modified by some other task
// controller concurrently.
//
// May also return transient errors (on RPC timeouts, etc).
func (ctl *taskController) Save(ctx context.Context) (err error) {
	// Log the intent to trigger jobs, if any.
	if len(ctl.triggers) != 0 && len(ctl.saved.TriggeredJobIDs) != 0 {
		ctl.DebugLog("Dispatching triggers (%d of them) to the following jobs:", len(ctl.triggers))
		for _, jobID := range ctl.saved.TriggeredJobIDs {
			ctl.DebugLog("  %s", jobID)
		}
	}

	// Mutate copy in case transaction below fails. Also unpacks ctl.state back
	// into the entity (reverse of 'populateState').
	saving := ctl.saved
	saving.Status = ctl.state.Status
	saving.TaskData = append([]byte(nil), ctl.state.TaskData...)
	saving.ViewURL = ctl.state.ViewURL
	saving.DebugLog += ctl.debugLog

	// Cleanup all consumed timers.
	if ctl.consumedTimers != nil {
		err := mutateTimersList(&saving.PendingTimersRaw, func(out *[]*internal.Timer) {
			filtered := make([]*internal.Timer, 0, len(*out))
			for _, t := range *out {
				if !ctl.consumedTimers.Has(t.Id) {
					filtered = append(filtered, t)
				}
			}
			*out = filtered
		})
		if err != nil {
			return err
		}
	}

	// No changes at all? Skip transaction.
	if saving.isEqual(&ctl.saved) && len(ctl.timers) == 0 && len(ctl.triggers) == 0 {
		return nil
	}

	saving.MutationsCount++

	hasFinished := !ctl.saved.Status.Final() && saving.Status.Final()

	// Update local copy of Invocation with what's in the datastore on success.
	defer func() {
		if err != nil {
			return
		}
		ctl.saved = saving
		ctl.debugLog = ""        // debug log was successfully flushed
		ctl.timers = nil         // new timers were successfully scheduled
		ctl.triggers = nil       // new triggers were successfully emitted
		ctl.consumedTimers = nil // pending timers were consumed
	}()

	if hasFinished {
		saving.Finished = clock.Now(ctx).UTC()
		saving.debugLog(
			ctx, "Invocation finished in %s with status %s",
			saving.Finished.Sub(saving.Started), saving.Status)
		// Finished invocations can't schedule timers.
		for _, t := range ctl.timers {
			saving.debugLog(ctx, "Ignoring timer %q (%s)...", t.Title, t.Id)
		}
		ctl.timers = nil
	}

	// Update the invocation entity, notifying the engine about all changes.
	err = runTxn(ctx, func(c context.Context) error {
		// Grab what's currently in the store to compare MutationsCount to what we
		// expect it to be.
		mostRecent := Invocation{ID: saving.ID}
		switch err := datastore.Get(c, &mostRecent); {
		case err == datastore.ErrNoSuchEntity: // should not happen
			logging.Errorf(c, "Invocation is suddenly gone")
			return errors.New("invocation is suddenly gone")
		case err != nil:
			return transient.Tag.Apply(err)
		}

		// Make sure no one touched it while we were handling the invocation.
		if saving.MutationsCount != mostRecent.MutationsCount+1 {
			logging.Errorf(c, "Invocation was modified by someone else while we were handling it")
			return errUpdateConflict
		}

		// Make sure not to override AsyncState: the engine could have updated it
		// while we were working on the invocation. Changes to AsyncState do not
		// bump MutationsCount, which allows them to happen asynchronously with
		// task.Manager long-running calls.
		saving.AsyncState = mostRecent.AsyncState

		// Notify the engine about the invocation state change and all timers and
		// triggers. The engine may decide to update the corresponding job.
		if err := ctl.eng.invChanging(c, &ctl.saved, &saving, ctl.timers, ctl.triggers); err != nil {
			return err
		}

		// Persist all changes to the invocation.
		saving.trimDebugLog()
		return transient.Tag.Apply(datastore.Put(c, &saving))
	})
	switch {
	case err != nil:
		return err
	case hasFinished:
		saving.reportCompletionMetrics(ctx)
	}
	return nil
}
