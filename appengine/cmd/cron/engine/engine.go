// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package engine implements the core logic of the cron service.
package engine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gorhill/cronexpr"
	"github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/info"
	"github.com/luci/gae/service/taskqueue"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/mathrand"
	"github.com/luci/luci-go/common/stringset"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/appengine/cmd/cron/catalog"
)

// Engine manages all cron jobs: keeps track of their state, runs state machine
// transactions, starts new invocations, etc. A method returns errors.Transient
// if the error is non-fatal and the call should be retried later. Any other
// error means that retry won't help.
type Engine interface {
	// GetAllProjects returns a list of all projects that have at least one
	// enabled cron job.
	GetAllProjects(c context.Context) ([]string, error)

	// UpdateProjectJobs adds new, removes old and updates existing jobs.
	UpdateProjectJobs(c context.Context, projectID string, defs []catalog.Definition) error

	// ResetAllJobsOnDevServer forcefully resets state of all enabled jobs.
	// Supposed to be used only on devserver, where task queue stub state is not
	// preserved between appserver restarts and it messes everything.
	ResetAllJobsOnDevServer(c context.Context) error

	// ExecuteSerializedAction is called via a task queue to execute an action
	// produced by job state machine transition. These actions are POSTed
	// to TimersQueue and InvocationsQueue defined in Config by Engine.
	// 'retryCount' is 0 on first attempt, 1 if task queue service retries
	// request once, 2 - if twice, and so on.
	ExecuteSerializedAction(c context.Context, body []byte, retryCount int) error
}

// Config contains parameters for the engine.
type Config struct {
	Catalog              catalog.Catalog // provides task.Manager's to run tasks
	TimersQueuePath      string          // URL of a task queue handler for timer ticks
	TimersQueueName      string          // queue name for timer ticks
	InvocationsQueuePath string          // URL of a task queue handler that starts jobs
	InvocationsQueueName string          // queue name for job starts
}

// NewEngine returns default implementation of Engine.
func NewEngine(conf Config) Engine {
	return &engineImpl{conf}
}

//// Implementation.

// actionTaskPayload is payload for task queue jobs emitted by the engine.
// Serialized as JSON, produced by enqueueActions, used as inputs in
// ExecuteSerializedAction. Union of all possible payloads for simplicity.
type actionTaskPayload struct {
	JobID           string // ID of relevant jobEntity
	Kind            string // defines what fields below to examine
	TickNonce       int64  // valid for "TickLaterAction" kind
	InvocationNonce int64  // valid for "StartInvocationAction" kind
}

// jobEntity stores the last known definition of a cron job, as well as its
// current state. Root entity, its kind is "CronJob".
type jobEntity struct {
	_kind string `gae:"$kind,CronJob"`

	// JobID is '<ProjectID>/<JobName>' string. JobName is unique with a project,
	// but not globally. JobID is unique globally.
	JobID string `gae:"$id"`

	// ProjectID exists for indexing. It matches <projectID> portion of JobID.
	ProjectID string

	// Enabled is false if cron job was disabled or removed from config.
	Enabled bool

	// Revision is last seen job definition revision, to skip useless updates.
	Revision string `gae:",noindex"`

	// Schedule is cron job schedule in regular cron expression format.
	Schedule string `gae:",noindex"`

	// Task is cron job payload in serialized form. Opaque from the point of view
	// of the engine. See Catalog.UnmarshalTask().
	Task []byte `gae:",noindex"`

	// State is cron job state machine state, see StateMachine.
	State JobState
}

// isEqual returns true iff 'e' is equal to 'other'.
func (e *jobEntity) isEqual(other *jobEntity) bool {
	return e == other || (e.JobID == other.JobID &&
		e.ProjectID == other.ProjectID &&
		e.Revision == other.Revision &&
		e.Enabled == other.Enabled &&
		e.Schedule == other.Schedule &&
		bytes.Equal(e.Task, other.Task) &&
		e.State == other.State)
}

// matches returns true if job definition in the entity matches the one
// specified by catalog.Definition struct.
func (e *jobEntity) matches(def catalog.Definition) bool {
	return e.JobID == def.JobID && e.Schedule == def.Schedule && bytes.Equal(e.Task, def.Task)
}

////

type engineImpl struct {
	Config
}

func (e *engineImpl) GetAllProjects(c context.Context) ([]string, error) {
	ds := datastore.Get(c)
	q := datastore.NewQuery("CronJob").
		Eq("Enabled", true).
		Project("ProjectID").
		Distinct(true)
	entities := []jobEntity{}
	if err := ds.GetAll(q, &entities); err != nil {
		return nil, errors.WrapTransient(err)
	}
	// Filter out duplicates, sort.
	projects := stringset.New(len(entities))
	for _, ent := range entities {
		projects.Add(ent.ProjectID)
	}
	out := projects.ToSlice()
	sort.Strings(out)
	return out, nil
}

func (e *engineImpl) UpdateProjectJobs(c context.Context, projectID string, defs []catalog.Definition) error {
	// JobID -> *jobEntity map.
	existing, err := e.getProjectJobs(c, projectID)
	if err != nil {
		return err
	}
	// JobID -> new definition revision map.
	updated := make(map[string]string, len(defs))
	for _, def := range defs {
		updated[def.JobID] = def.Revision
	}
	// List of job ids to disable.
	toDisable := []string{}
	for id := range existing {
		if updated[id] == "" {
			toDisable = append(toDisable, id)
		}
	}

	wg := sync.WaitGroup{}

	// Add new jobs, update existing ones.
	updateErrs := errors.NewLazyMultiError(len(defs))
	for i, def := range defs {
		if ent := existing[def.JobID]; ent != nil {
			if ent.Enabled && ent.matches(def) {
				continue
			}
		}
		wg.Add(1)
		go func(i int, def catalog.Definition) {
			updateErrs.Assign(i, e.updateJob(c, def))
			wg.Done()
		}(i, def)
	}

	// Disable old jobs.
	disableErrs := errors.NewLazyMultiError(len(toDisable))
	for i, jobID := range toDisable {
		wg.Add(1)
		go func(i int, jobID string) {
			disableErrs.Assign(i, e.disableJob(c, jobID))
			wg.Done()
		}(i, jobID)
	}

	wg.Wait()
	if updateErrs.Get() == nil && disableErrs.Get() == nil {
		return nil
	}
	return errors.WrapTransient(errors.NewMultiError(updateErrs.Get(), disableErrs.Get()))
}

func (e *engineImpl) ResetAllJobsOnDevServer(c context.Context) error {
	if !info.Get(c).IsDevAppServer() {
		return errors.New("ResetAllJobsOnDevServer must not be used in production")
	}
	ds := datastore.Get(c)
	q := datastore.NewQuery("CronJob").Eq("Enabled", true)
	keys := []*datastore.Key{}
	if err := ds.GetAll(q, &keys); err != nil {
		return errors.WrapTransient(err)
	}
	wg := sync.WaitGroup{}
	errs := errors.NewLazyMultiError(len(keys))
	for i, key := range keys {
		wg.Add(1)
		go func(i int, key *datastore.Key) {
			errs.Assign(i, e.resetJob(c, key.StringID()))
			wg.Done()
		}(i, key)
	}
	wg.Wait()
	return errors.WrapTransient(errs.Get())
}

// getProjectJobs fetches from datastore all enabled jobs belonging to a given
// project.
func (e *engineImpl) getProjectJobs(c context.Context, projectID string) (map[string]*jobEntity, error) {
	ds := datastore.Get(c)
	q := datastore.NewQuery("CronJob").
		Eq("Enabled", true).
		Eq("ProjectID", projectID)
	entities := []*jobEntity{}
	if err := ds.GetAll(q, &entities); err != nil {
		return nil, errors.WrapTransient(err)
	}
	out := make(map[string]*jobEntity, len(entities))
	for _, job := range entities {
		if job.Enabled && job.ProjectID == projectID {
			out[job.JobID] = job
		}
	}
	return out, nil
}

// txnCallback is passed to 'txn' and it modifies 'job' in place. 'txn' then
// puts it into datastore. The callback may return errSkipPut to instruct 'txn'
// not to call datastore 'Put'. The callback may do other transactional things
// using the context.
type txnCallback func(c context.Context, job *jobEntity, isNew bool) error

// errSkipPut can be returned by txnCallback to cancel ds.Put call.
var errSkipPut = errors.New("errSkipPut")

// defaultTransactionOptions is used for all transactions. Cron service has
// no user facing API, all activity is in background task queues. So tune it
// to do more retries.
var defaultTransactionOptions = datastore.TransactionOptions{
	Attempts: 10,
}

// txn reads jobEntity, calls callback, then dumps the modified entity
// back into datastore (unless callback returns errSkipPut).
func (e *engineImpl) txn(c context.Context, jobID string, txn txnCallback) error {
	c = logging.SetField(c, "JobID", jobID)
	fatal := false
	attempt := 0
	err := datastore.Get(c).RunInTransaction(func(c context.Context) error {
		attempt++
		if attempt != 1 {
			logging.Warningf(c, "Retrying transaction...")
		}
		ds := datastore.Get(c)
		stored := jobEntity{JobID: jobID}
		err := ds.Get(&stored)
		if err != nil && err != datastore.ErrNoSuchEntity {
			return err
		}
		modified := stored
		err = txn(c, &modified, err == datastore.ErrNoSuchEntity)
		if err != nil && err != errSkipPut {
			fatal = !errors.IsTransient(err)
			return err
		}
		if err != errSkipPut && !modified.isEqual(&stored) {
			return ds.Put(&modified)
		}
		return nil
	}, &defaultTransactionOptions)
	if err != nil {
		logging.Errorf(c, "Job transaction failed: %s", err)
		if fatal {
			return err
		}
		// By now err is already transient (since 'fatal' is false) or it is commit
		// error (i.e. produced by RunInTransaction itself, not by its callback).
		// Need to wrap commit errors too.
		return errors.WrapTransient(err)
	}
	if attempt > 1 {
		logging.Infof(c, "Committed on %d attempt", attempt)
	}
	return nil
}

// rollSM is called under transaction to perform a single cron job state machine
// transition. It sets up StateMachine instance, calls the callback, mutates
// job.State in place (with a new state) and enqueues all emitted actions to
// task queues.
func (e *engineImpl) rollSM(c context.Context, job *jobEntity, cb func(*StateMachine) error) error {
	expr, err := cronexpr.Parse(job.Schedule)
	if err != nil {
		return fmt.Errorf("bad schedule %q - %s", job.Schedule, err)
	}
	now := clock.Now(c).UTC()
	rnd := mathrand.Get(c)
	sm := StateMachine{
		InputState:         job.State,
		Now:                now,
		NextInvocationTime: expr.Next(now),
		Nonce:              func() int64 { return rnd.Int63() + 1 },
	}
	// All errors returned by state machine transition changes are transient.
	// Fatal errors (when we have them) should be reflected as a state changing
	// into "BROKEN" state.
	if err := cb(&sm); err != nil {
		return errors.WrapTransient(err)
	}
	if len(sm.Actions) != 0 {
		if err := e.enqueueActions(c, job.JobID, sm.Actions); err != nil {
			return err
		}
	}
	if sm.OutputState != nil {
		if sm.OutputState.State != job.State.State {
			logging.Infof(c, "%s -> %s", job.State.State, sm.OutputState.State)
		}
		job.State = *sm.OutputState
	}
	return nil
}

// enqueueActions commits all actions emitted by a state transition by adding
// corresponding tasks to task queues. See ExecuteSerializedAction for place
// where these actions are interpreted.
func (e *engineImpl) enqueueActions(c context.Context, jobID string, actions []Action) error {
	// AddMulti can't put tasks into multiple queues at once, split by queue name.
	qs := map[string][]*taskqueue.Task{}
	for _, a := range actions {
		switch a := a.(type) {
		case TickLaterAction:
			payload, err := json.Marshal(actionTaskPayload{
				JobID:     jobID,
				Kind:      "TickLaterAction",
				TickNonce: a.TickNonce,
			})
			if err != nil {
				return err
			}
			logging.Infof(c, "Scheduling tick %d after %.1f sec", a.TickNonce, a.When.Sub(time.Now()).Seconds())
			qs[e.TimersQueueName] = append(qs[e.TimersQueueName], &taskqueue.Task{
				Path:    e.TimersQueuePath,
				ETA:     a.When,
				Payload: payload,
			})
		case StartInvocationAction:
			payload, err := json.Marshal(actionTaskPayload{
				JobID:           jobID,
				Kind:            "StartInvocationAction",
				InvocationNonce: a.InvocationNonce,
			})
			if err != nil {
				return err
			}
			qs[e.InvocationsQueueName] = append(qs[e.InvocationsQueueName], &taskqueue.Task{
				Path:    e.InvocationsQueuePath,
				Delay:   time.Second, // give the transaction time to land
				Payload: payload,
			})
		default:
			logging.Errorf(c, "Unexpected action type %T, skipping", a)
		}
	}
	tq := taskqueue.Get(c)
	wg := sync.WaitGroup{}
	errs := errors.NewLazyMultiError(len(qs))
	i := 0
	for queueName, tasks := range qs {
		wg.Add(1)
		go func(i int, queueName string, tasks []*taskqueue.Task) {
			errs.Assign(i, tq.AddMulti(tasks, queueName))
			wg.Done()
		}(i, queueName, tasks)
		i++
	}
	wg.Wait()
	return errors.WrapTransient(errs.Get())
}

func (e *engineImpl) ExecuteSerializedAction(c context.Context, action []byte, retryCount int) error {
	payload := actionTaskPayload{}
	if err := json.Unmarshal(action, &payload); err != nil {
		return err
	}
	switch payload.Kind {
	case "TickLaterAction":
		return e.timerTick(c, payload.JobID, payload.TickNonce)
	case "StartInvocationAction":
		return e.startInvocation(c, payload.JobID, payload.InvocationNonce, retryCount)
	default:
		return fmt.Errorf("unexpected action kind %q", payload.Kind)
	}
}

// updateJob updates an existing job if its definition has changed, adds
// a completely new job or enables a previously disabled job.
func (e *engineImpl) updateJob(c context.Context, def catalog.Definition) error {
	return e.txn(c, def.JobID, func(c context.Context, job *jobEntity, isNew bool) error {
		if !isNew && job.Enabled && job.matches(def) {
			return errSkipPut
		}
		if isNew {
			// JobID is <projectID>/<name>, it's ensure by Catalog.
			chunks := strings.Split(def.JobID, "/")
			if len(chunks) != 2 {
				return fmt.Errorf("unexpected jobID format: %s", def.JobID)
			}
			*job = jobEntity{
				JobID:     def.JobID,
				ProjectID: chunks[0],
				Enabled:   false, // to trigger 'if !oldEnabled' below
				Schedule:  def.Schedule,
				Task:      def.Task,
				State:     JobState{State: JobStateDisabled},
			}
		}
		oldEnabled := job.Enabled
		oldSchedule := job.Schedule

		// Update the job in full before running any state changes.
		job.Revision = def.Revision
		job.Enabled = true
		job.Schedule = def.Schedule
		job.Task = def.Task

		// Do state machine transitions.
		if !oldEnabled {
			err := e.rollSM(c, job, func(sm *StateMachine) error { return sm.OnJobEnabled() })
			if err != nil {
				return err
			}
		}
		if job.Schedule != oldSchedule {
			logging.Infof(c, "Job's schedule changed")
			err := e.rollSM(c, job, func(sm *StateMachine) error { return sm.OnScheduleChange() })
			if err != nil {
				return err
			}
		}
		return nil
	})
}

// disableJob moves a job to disabled state.
func (e *engineImpl) disableJob(c context.Context, jobID string) error {
	return e.txn(c, jobID, func(c context.Context, job *jobEntity, isNew bool) error {
		if isNew || !job.Enabled {
			return errSkipPut
		}
		job.Enabled = false
		return e.rollSM(c, job, func(sm *StateMachine) error { return sm.OnJobDisabled() })
	})
}

// resetJob sends "off" signal followed by "on" signal. It effectively cancels
// any pending actions and schedules new ones.
func (e *engineImpl) resetJob(c context.Context, jobID string) error {
	return e.txn(c, jobID, func(c context.Context, job *jobEntity, isNew bool) error {
		if isNew || !job.Enabled {
			return errSkipPut
		}
		logging.Infof(c, "Resetting job")
		err := e.rollSM(c, job, func(sm *StateMachine) error { return sm.OnJobDisabled() })
		if err != nil {
			return err
		}
		return e.rollSM(c, job, func(sm *StateMachine) error { return sm.OnJobEnabled() })
	})
}

// timerTick is invoked via task queue in a task with some ETA. It what makes
// cron tick.
func (e *engineImpl) timerTick(c context.Context, jobID string, tickNonce int64) error {
	return e.txn(c, jobID, func(c context.Context, job *jobEntity, isNew bool) error {
		if isNew {
			logging.Errorf(c, "Scheduled job is unexpectedly gone")
			return errSkipPut
		}
		logging.Infof(c, "Tick %d has arrived", tickNonce)
		return e.rollSM(c, job, func(sm *StateMachine) error { return sm.OnTimerTick(tickNonce) })
	})
}

// startInvocation is called via task queue to start running a job. This call
// may be retried by task queue service.
func (e *engineImpl) startInvocation(c context.Context, jobID string, invocationNonce int64, retryCount int) error {
	// Still need to do this?
	c = logging.SetField(c, "JobID", jobID)
	job := jobEntity{JobID: jobID}
	err := datastore.Get(c).Get(&job)
	if err == datastore.ErrNoSuchEntity {
		logging.Errorf(c, "Queued job is unexpectedly gone")
		return nil
	}
	if err != nil {
		return errors.WrapTransient(err)
	}
	if job.State.InvocationNonce != invocationNonce {
		logging.Errorf(c, "No longer need to start invocation %d", invocationNonce)
		return nil
	}

	// TODO(vadimsh): Implement via task.TaskManager. Mark as "Done" for now.

	return e.txn(c, jobID, func(c context.Context, job *jobEntity, isNew bool) error {
		if isNew {
			logging.Errorf(c, "Queued job is unexpectedly gone")
			return errSkipPut
		}
		err := e.rollSM(c, job, func(sm *StateMachine) error { return sm.OnInvocationStart(invocationNonce) })
		if err != nil {
			return err
		}
		return e.rollSM(c, job, func(sm *StateMachine) error { return sm.OnInvocationDone(invocationNonce) })
	})
}
