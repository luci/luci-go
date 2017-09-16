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
	"fmt"
	"net/http"
	"time"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/scheduler/appengine/task"
)

// taskController manages execution of single invocation.
//
// It is short-lived object spawned to handle some single event in the lifecycle
// of the invocation.
//
// Support both v1 and v2 invocations.
type taskController struct {
	ctx context.Context // for DebugLog logging only
	eng *engineImpl     // for prepareTopic and jobController

	task    proto.Message // extracted from saved.Task blob
	manager task.Manager

	saved    Invocation        // what have been given initially or saved in Save()
	state    task.State        // state mutated by TaskManager
	debugLog string            // mutated by DebugLog
	timers   []invocationTimer // mutated by AddTimer
	triggers []task.Trigger    // mutated by EmitTrigger
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
	ctl.populateState()
	var err error
	ctl.task, err = e.cfg.Catalog.UnmarshalTask(inv.Task)
	if err != nil {
		return ctl, fmt.Errorf("failed to unmarshal the task - %s", err)
	}
	ctl.manager = e.cfg.Catalog.GetTaskManager(ctl.task)
	if ctl.manager == nil {
		return ctl, fmt.Errorf("TaskManager is unexpectedly missing")
	}
	return ctl, nil
}

// populateState populates 'state' using data in 'saved'.
func (ctl *taskController) populateState() {
	ctl.state = task.State{
		Status:   ctl.saved.Status,
		ViewURL:  ctl.saved.ViewURL,
		TaskData: append([]byte(nil), ctl.saved.TaskData...), // copy
	}
}

// JobID is part of task.Controller interface.
func (ctl *taskController) JobID() string {
	return ctl.saved.jobID()
}

// InvocationID is part of task.Controller interface.
func (ctl *taskController) InvocationID() int64 {
	return ctl.saved.ID
}

// InvocationNonce is part of task.Controller interface.
func (ctl *taskController) InvocationNonce() int64 {
	return ctl.saved.InvocationNonce
}

// Task is part of task.Controller interface.
func (ctl *taskController) Task() proto.Message {
	return ctl.task
}

// State is part of task.Controller interface.
func (ctl *taskController) State() *task.State {
	return &ctl.state
}

// DebugLog is part of task.Controller interface.
func (ctl *taskController) DebugLog(format string, args ...interface{}) {
	logging.Infof(ctl.ctx, format, args...)
	debugLog(ctl.ctx, &ctl.debugLog, format, args...)
}

// AddTimer is part of Controller interface.
func (ctl *taskController) AddTimer(ctx context.Context, delay time.Duration, name string, payload []byte) {
	ctl.DebugLog("Scheduling timer %q after %s", name, delay)
	ctl.timers = append(ctl.timers, invocationTimer{
		Delay:   delay,
		Name:    name,
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
func (ctl *taskController) GetClient(ctx context.Context, timeout time.Duration, opts ...auth.RPCOption) (*http.Client, error) {
	// TODO(vadimsh): Use per-project service accounts, not a global service
	// account.
	ctx, _ = clock.WithTimeout(ctx, timeout)
	t, err := auth.GetRPCTransport(ctx, auth.AsSelf, opts...)
	if err != nil {
		return nil, err
	}
	return &http.Client{Transport: t}, nil
}

// EmitTrigger is part of task.Controller interface.
func (ctl *taskController) EmitTrigger(ctx context.Context, trigger task.Trigger) {
	ctl.DebugLog("Emitting a trigger %s", trigger.ID)
	trigger.JobID = ctl.JobID()
	trigger.InvocationID = ctl.InvocationID()
	trigger.Created = clock.Now(ctx).UTC()
	ctl.triggers = append(ctl.triggers, trigger)
}

// errUpdateConflict means Invocation is being modified by two TaskController's
// concurrently. It should not be happening often. If it happens, task queue
// call is retried to rerun the two-part transaction from scratch.
//
// This error is marked as transient, since it should trigger a retry on the
// task queue level. At the same time we don't want the transaction itself to be
// retried (it's useless to retry the second part of a two-part transaction, the
// result will be the same), so we tag the error with abortTransaction. See
// runTxn for more info.
var errUpdateConflict = errors.New("concurrent modifications of single Invocation", transient.Tag, abortTransaction)

// Save uploads updated Invocation to the datastore, updating the state of the
// corresponding job, if necessary.
//
// May return transient errors. In particular returns errUpdateConflict if the
// invocation was modified by some other task controller concurrently.
func (ctl *taskController) Save(ctx context.Context) (err error) {
	// Mutate copy in case transaction below fails. Also unpacks ctl.state back
	// into the entity (reverse of 'populateState').
	saving := ctl.saved
	saving.Status = ctl.state.Status
	saving.TaskData = append([]byte(nil), ctl.state.TaskData...)
	saving.ViewURL = ctl.state.ViewURL
	saving.DebugLog += ctl.debugLog
	if saving.isEqual(&ctl.saved) && len(ctl.timers) == 0 && len(ctl.triggers) == 0 { // no changes at all?
		return nil
	}
	saving.MutationsCount++

	// Update local copy of Invocation with what's in the datastore on success.
	defer func() {
		if err == nil {
			ctl.saved = saving
			ctl.debugLog = ""  // debug log was successfully flushed
			ctl.timers = nil   // timers were successfully scheduled
			ctl.triggers = nil // triggers were successfully emitted
		}
	}()

	hasFinished := !ctl.saved.Status.Final() && saving.Status.Final()
	if hasFinished {
		saving.Finished = clock.Now(ctx).UTC()
		saving.debugLog(
			ctx, "Invocation finished in %s with status %s",
			saving.Finished.Sub(saving.Started), saving.Status)
		// Finished invocations can't schedule timers.
		for _, t := range ctl.timers {
			saving.debugLog(ctx, "Ignoring timer %s...", t.Name)
		}
		ctl.timers = nil
	}

	if saving.Status == task.StatusRetrying {
		saving.debugLog(ctx, "The invocation will be retried")
	}

	// Update the invocation entity, notifying the engine about all changes.
	return runTxn(ctx, func(c context.Context) error {
		// Grab what's currently in the store to compare MutationsCount to what we
		// expect it to be.
		mostRecent := Invocation{
			ID:     saving.ID,
			JobKey: saving.JobKey, // may be nil for v2 invocations, this is OK
		}
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

		// Notify the engine about the invocation state change and all timers and
		// triggers. The engine may decide to update the corresponding job.
		jobCtl := ctl.eng.jobController(ctl.JobID())
		if err := jobCtl.onInvUpdating(c, &ctl.saved, &saving, ctl.timers, ctl.triggers); err != nil {
			return err
		}

		// Persist all changes to the invocation.
		saving.trimDebugLog()
		return transient.Tag.Apply(datastore.Put(c, &saving))
	})
}
