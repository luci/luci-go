// Copyright 2015 The LUCI Authors.
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

// Package engine implements the core logic of the scheduler service.
package engine

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/api/pubsub/v1"

	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/gae/service/info"
	"go.chromium.org/gae/service/taskqueue"

	"go.chromium.org/luci/appengine/tq"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/identity"
	"go.chromium.org/luci/server/auth/signing"
	"go.chromium.org/luci/server/tokens"

	"go.chromium.org/luci/scheduler/appengine/catalog"
	"go.chromium.org/luci/scheduler/appengine/task"
)

var (
	// ErrNoOwnerPermission indicates the caller is not a job owner.
	ErrNoOwnerPermission = errors.New("no OWNER permission on a job")
	// ErrNoSuchJob indicates the job doesn't exist or not visible.
	ErrNoSuchJob = errors.New("no such job")
	// ErrNoSuchInvocation indicates the invocation doesn't exist or not visible.
	ErrNoSuchInvocation = errors.New("the invocation doesn't exist")
)

// Engine manages all scheduler jobs: keeps track of their state, runs state
// machine transactions, starts new invocations, etc.
//
// A method returns errors.Transient if the error is non-fatal and the call
// should be retried later. Any other error means that retry won't help.
//
// ACLs are enforced with the following implications:
//  * if caller lacks READER access to Jobs, methods behave as if Jobs do not
//    exist.
//  * if caller lacks OWNER access, calling mutating methods will result in
//    ErrNoOwnerPermission (assuming caller has READER access, else see above).
//
// Use EngineInternal if you need to skip ACL checks.
type Engine interface {
	// GetVisibleJobs returns all enabled visible jobs.
	//
	// Returns them in no particular order.
	GetVisibleJobs(c context.Context) ([]*Job, error)

	// GetVisibleProjectJobs returns enabled visible jobs belonging to a project.
	//
	// Returns them in no particular order.
	GetVisibleProjectJobs(c context.Context, projectID string) ([]*Job, error)

	// GetVisibleJob returns a single visible job given its full ID.
	//
	// ErrNoSuchJob error is returned if either:
	//   * job doesn't exist,
	//   * job is disabled (i.e. was removed from its project config),
	//   * job isn't visible due to lack of READER access.
	GetVisibleJob(c context.Context, jobID string) (*Job, error)

	// ListVisibleInvocations returns invocations of a visible job, most recent
	// first.
	//
	// Returns invocations and a cursor string if there's more.
	// Returns ErrNoSuchJob if job doesn't exist or isn't visible.
	//
	// For v2 jobs, the listing includes only finished invocations and it is
	// eventually consistent (i.e. very recently finished invocations may no be
	// listed there yet).
	//
	// TODO(vadimsh): Expose an endpoint for fetching currently running
	// invocations in a perfectly consistent way.
	ListVisibleInvocations(c context.Context, jobID string, pageSize int, cursor string) ([]*Invocation, string, error)

	// GetVisibleInvocation returns single invocation of some job given its ID.
	//
	// ErrNoSuchInvocation is returned if either job or invocation doesn't exist
	// or job and hence invocation isn't visible.
	GetVisibleInvocation(c context.Context, jobID string, invID int64) (*Invocation, error)

	// PauseJob prevents new automatic invocations of a job.
	//
	// It replaces job's schedule with "triggered", effectively preventing it from
	// running automatically (until it is resumed).
	//
	// Manual invocations (via ForceInvocation) are still allowed. Does nothing if
	// the job is already paused. Any pending or running invocations are still
	// executed.
	PauseJob(c context.Context, jobID string) error

	// ResumeJob resumes paused job. Does nothing if the job is not paused.
	ResumeJob(c context.Context, jobID string) error

	// AbortJob resets the job to scheduled state, aborting all currently pending
	// or running invocations (if any).
	AbortJob(c context.Context, jobID string) error

	// AbortInvocation forcefully moves the invocation to failed state.
	//
	// It opportunistically tries to send "abort" signal to a job runner if it
	// supports cancellation, but it doesn't wait for reply. It proceeds to
	// modifying local state in the scheduler service datastore immediately.
	//
	// AbortInvocation can be used to manually "unstuck" jobs that got stuck due
	// to missing PubSub notifications or other kinds of unexpected conditions.
	//
	// Does nothing if invocation is already in some final state.
	AbortInvocation(c context.Context, jobID string, invID int64) error

	// ForceInvocation launches job invocation right now if job isn't running now.
	//
	// Used by "Run now" UI button.
	//
	// Returns an object that can be waited on to grab a corresponding Invocation
	// when it appears (if ever).
	ForceInvocation(c context.Context, jobID string) (FutureInvocation, error)
}

// EngineInternal is a variant of engine API that skips ACL checks.
//
// Used by the scheduler service guts that executed outside of a context of some
// end user.
type EngineInternal interface {
	// PublicAPI returns ACL-enforcing API.
	PublicAPI() Engine

	// GetAllProjects returns projects that have at least one enabled job.
	GetAllProjects(c context.Context) ([]string, error)

	// UpdateProjectJobs adds new, removes old and updates existing jobs.
	UpdateProjectJobs(c context.Context, projectID string, defs []catalog.Definition) error

	// ResetAllJobsOnDevServer forcefully resets state of all enabled jobs.
	//
	// Supposed to be used only on devserver, where task queue stub state is not
	// preserved between appserver restarts and it messes everything.
	ResetAllJobsOnDevServer(c context.Context) error

	// ExecuteSerializedAction is called via a task queue to execute an action
	// produced by job state machine transition.
	//
	// These actions are POSTed to TimersQueue and InvocationsQueue defined in
	// Config by Engine.
	//
	// 'retryCount' is 0 on first attempt, 1 if task queue service retries
	// request once, 2 - if twice, and so on.
	//
	// Returning transient errors here causes the task queue to retry the task.
	ExecuteSerializedAction(c context.Context, body []byte, retryCount int) error

	// ProcessPubSubPush is called whenever incoming PubSub message is received.
	ProcessPubSubPush(c context.Context, body []byte) error

	// PullPubSubOnDevServer is called on dev server to pull messages from PubSub
	// subscription associated with given publisher.
	//
	// It is needed to be able to manually tests PubSub related workflows on dev
	// server, since dev server can't accept PubSub push messages.
	PullPubSubOnDevServer(c context.Context, taskManagerName, publisher string) error
}

// FutureInvocation is returned by ForceInvocation.
//
// It can be used to wait for a triggered invocation to appear.
type FutureInvocation interface {
	// InvocationID returns an ID of the invocation or 0 if not started yet.
	//
	// Returns only transient errors.
	InvocationID(context.Context) (int64, error)
}

// Config contains parameters for the engine.
type Config struct {
	Catalog              catalog.Catalog // provides task.Manager's to run tasks
	Dispatcher           *tq.Dispatcher  // dispatcher for task queue tasks
	TimersQueuePath      string          // URL of a task queue handler for timer ticks
	TimersQueueName      string          // queue name for timer ticks
	InvocationsQueuePath string          // URL of a task queue handler that starts jobs
	InvocationsQueueName string          // queue name for job starts
	PubSubPushPath       string          // URL to use in PubSub push config
}

// NewEngine returns default implementation of EngineInternal.
func NewEngine(cfg Config) EngineInternal {
	return &engineImpl{cfg: cfg}
}

type engineImpl struct {
	cfg      Config
	opsCache opsCache

	// configureTopic is used by prepareTopic, mocked in tests.
	configureTopic func(c context.Context, topic, sub, pushURL, publisher string) error
}

// jobController is a part of engine that directly deals with state transitions
// of a single job and its invocations.
//
// It is short-lived. It is instantiated, used and discarded. Never retained.
//
// All onJob* methods are called from within a Job transaction.
// All onInv* methods are called from within an Invocation transaction.
//
// There are two implementations of the controller (jobControllerV1 and
// jobControllerV2).
type jobController interface {
	onJobScheduleChange(c context.Context, job *Job) error
	onJobEnabled(c context.Context, job *Job) error
	onJobDisabled(c context.Context, job *Job) error
	onJobAbort(c context.Context, job *Job) (invs []int64, err error)
	onJobForceInvocation(c context.Context, job *Job) (FutureInvocation, error)

	onInvUpdating(c context.Context, old, fresh *Invocation, timers []invocationTimer, triggers []task.Trigger) error
}

////////////////////////////////////////////////////////////////////////////////
// Engine interface implementation.

// GetVisibleJobs returns all enabled visible jobs.
//
// Part of the public interface, checks ACLs.
func (e *engineImpl) GetVisibleJobs(c context.Context) ([]*Job, error) {
	q := ds.NewQuery("Job").Eq("Enabled", true)
	return e.queryEnabledVisibleJobs(c, q)
}

// GetVisibleProjectJobs enabled visible jobs belonging to a project.
//
// Part of the public interface, checks ACLs.
func (e *engineImpl) GetVisibleProjectJobs(c context.Context, projectID string) ([]*Job, error) {
	q := ds.NewQuery("Job").Eq("Enabled", true).Eq("ProjectID", projectID)
	return e.queryEnabledVisibleJobs(c, q)
}

// GetVisibleJob returns a single visible job given its full ID.
//
// Part of the public interface, checks ACLs.
func (e *engineImpl) GetVisibleJob(c context.Context, jobID string) (*Job, error) {
	job, err := e.getJob(c, jobID)
	if err != nil {
		return nil, err
	} else if job == nil || !job.Enabled {
		return nil, ErrNoSuchJob
	}
	if ok, err := job.IsVisible(c); err != nil {
		return nil, err
	} else if !ok {
		return nil, ErrNoSuchJob
	}
	return job, nil
}

// ListVisibleInvocations returns invocations of a visible job, most recent
// first.
//
// Part of the public interface, checks ACLs.
//
// Supports both v1 and v2 invocations.
func (e *engineImpl) ListVisibleInvocations(c context.Context, jobID string, pageSize int, cursor string) ([]*Invocation, string, error) {
	if _, err := e.GetVisibleJob(c, jobID); err != nil {
		return nil, "", err
	}

	if pageSize == 0 || pageSize > 500 {
		pageSize = 500
	}

	// Deserialize the cursor.
	var cursorObj ds.Cursor
	if cursor != "" {
		var err error
		cursorObj, err = ds.DecodeCursor(c, cursor)
		if err != nil {
			return nil, "", err
		}
	}

	// Prepare the query. Fetch 'pageSize' worth of entities as a single batch.
	q := ds.NewQuery("Invocation")
	if e.isV2Job(jobID) {
		q = q.Eq("IndexedJobID", jobID)
	} else {
		q = q.Ancestor(ds.NewKey(c, "Job", jobID, 0, nil))
	}
	q = q.Order("__key__").Limit(int32(pageSize))
	if cursorObj != nil {
		q = q.Start(cursorObj)
	}

	// Fetch pageSize worth of invocations, then grab the cursor.
	out := make([]*Invocation, 0, pageSize)
	var newCursor string
	err := ds.Run(c, q, func(obj *Invocation, getCursor ds.CursorCB) error {
		out = append(out, obj)
		if len(out) < pageSize {
			return nil
		}
		cur, err := getCursor()
		if err != nil {
			return err
		}
		newCursor = cur.String()
		return ds.Stop
	})
	if err != nil {
		return nil, "", transient.Tag.Apply(err)
	}
	return out, newCursor, nil
}

// GetVisibleInvocation returns single invocation of some job given its ID.
//
// Part of the public interface, checks ACLs.
//
// Supports both v1 and v2 invocations.
func (e *engineImpl) GetVisibleInvocation(c context.Context, jobID string, invID int64) (*Invocation, error) {
	switch _, err := e.GetVisibleJob(c, jobID); {
	case err == ErrNoSuchJob:
		return nil, ErrNoSuchInvocation
	case err != nil:
		return nil, err
	default:
		return e.getInvocation(c, jobID, invID)
	}
}

// PauseJob prevents new automatic invocations of a job.
//
// Part of the public interface, checks ACLs.
func (e *engineImpl) PauseJob(c context.Context, jobID string) error {
	return e.setJobPausedFlag(c, jobID, true, auth.CurrentIdentity(c))
}

// ResumeJob resumes paused job. Does nothing if the job is not paused.
//
// Part of the public interface, checks ACLs.
func (e *engineImpl) ResumeJob(c context.Context, jobID string) error {
	return e.setJobPausedFlag(c, jobID, false, auth.CurrentIdentity(c))
}

// AbortJob resets the job to scheduled state, aborting all currently pending
// or running invocations (if any).
//
// Part of the public interface, checks ACLs.
func (e *engineImpl) AbortJob(c context.Context, jobID string) error {
	// First, we check ACLs.
	if _, err := e.getOwnedJob(c, jobID); err != nil {
		return err
	}

	// Second, we switch the job to the default state and disassociate the running
	// invocations (if any) from the job entity.
	var invs []int64
	err := e.jobTxn(c, jobID, func(c context.Context, job *Job, isNew bool) (err error) {
		if isNew {
			return errSkipPut // the job was removed, nothing to abort
		}
		invs, err = e.jobController(jobID).onJobAbort(c, job)
		return err
	})
	if err != nil {
		return err
	}

	// Now we kill the invocations. We do it separately because it may involve
	// an RPC to remote service (e.g. to cancel a task) that can't be done from
	// the transaction.
	wg := sync.WaitGroup{}
	wg.Add(len(invs))
	errs := errors.NewLazyMultiError(len(invs))
	for i, invID := range invs {
		go func(i int, invID int64) {
			defer wg.Done()
			errs.Assign(i, e.abortInvocation(c, jobID, invID))
		}(i, invID)
	}
	wg.Wait()
	if err := errs.Get(); err != nil {
		return transient.Tag.Apply(err)
	}
	return nil
}

// AbortInvocation forcefully moves the invocation to failed state.
//
// Part of the public interface, checks ACLs.
func (e *engineImpl) AbortInvocation(c context.Context, jobID string, invID int64) error {
	if _, err := e.getOwnedJob(c, jobID); err != nil {
		return err
	}
	return e.abortInvocation(c, jobID, invID)
}

// ForceInvocation launches job invocation right now if job isn't running now.
//
// Part of the public interface, checks ACLs.
func (e *engineImpl) ForceInvocation(c context.Context, jobID string) (FutureInvocation, error) {
	if _, err := e.getOwnedJob(c, jobID); err != nil {
		return nil, err
	}

	var noSuchJob bool
	var future FutureInvocation
	err := e.jobTxn(c, jobID, func(c context.Context, job *Job, isNew bool) (err error) {
		if isNew || !job.Enabled {
			noSuchJob = true
			return errSkipPut
		}
		future, err = e.jobController(jobID).onJobForceInvocation(c, job)
		return err
	})

	switch {
	case noSuchJob:
		return nil, ErrNoSuchJob
	case err != nil:
		return nil, err
	}

	return future, nil
}

////////////////////////////////////////////////////////////////////////////////
// EngineInternal interface implementation.

// PublicAPI returns ACL-enforcing API.
func (e *engineImpl) PublicAPI() Engine {
	return e
}

// GetAllProjects returns projects that have at least one enabled job.
//
// Part of the internal interface, doesn't check ACLs.
func (e *engineImpl) GetAllProjects(c context.Context) ([]string, error) {
	q := ds.NewQuery("Job").
		Eq("Enabled", true).
		Project("ProjectID").
		Distinct(true)
	entities := []Job{}
	if err := ds.GetAll(c, q, &entities); err != nil {
		return nil, transient.Tag.Apply(err)
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

// UpdateProjectJobs adds new, removes old and updates existing jobs.
//
// Part of the internal interface, doesn't check ACLs.
func (e *engineImpl) UpdateProjectJobs(c context.Context, projectID string, defs []catalog.Definition) error {
	// JobID -> *Job map.
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
			if ent.Enabled && ent.MatchesDefinition(def) {
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
	return transient.Tag.Apply(errors.NewMultiError(updateErrs.Get(), disableErrs.Get()))
}

// ResetAllJobsOnDevServer forcefully resets state of all enabled jobs.
//
// Part of the internal interface, doesn't check ACLs.
func (e *engineImpl) ResetAllJobsOnDevServer(c context.Context) error {
	if !info.IsDevAppServer(c) {
		return errors.New("ResetAllJobsOnDevServer must not be used in production")
	}
	q := ds.NewQuery("Job").Eq("Enabled", true)
	keys := []*ds.Key{}
	if err := ds.GetAll(c, q, &keys); err != nil {
		return transient.Tag.Apply(err)
	}
	wg := sync.WaitGroup{}
	errs := errors.NewLazyMultiError(len(keys))
	for i, key := range keys {
		wg.Add(1)
		go func(i int, key *ds.Key) {
			errs.Assign(i, e.resetJobOnDevServer(c, key.StringID()))
			wg.Done()
		}(i, key)
	}
	wg.Wait()
	return transient.Tag.Apply(errs.Get())
}

// ExecuteSerializedAction is called via a task queue to execute an action
// produced by job state machine transition.
//
// Part of the internal interface, doesn't check ACLs.
func (e *engineImpl) ExecuteSerializedAction(c context.Context, action []byte, retryCount int) error {
	payload := actionTaskPayload{}
	if err := json.Unmarshal(action, &payload); err != nil {
		return err
	}
	if payload.InvID == 0 {
		return e.executeJobAction(c, &payload, retryCount)
	}
	return e.executeInvAction(c, &payload, retryCount)
}

// ProcessPubSubPush is called whenever incoming PubSub message is received.
//
// Part of the internal interface, doesn't check ACLs.
func (e *engineImpl) ProcessPubSubPush(c context.Context, body []byte) error {
	var pushBody struct {
		Message pubsub.PubsubMessage `json:"message"`
	}
	if err := json.Unmarshal(body, &pushBody); err != nil {
		return err
	}
	return e.handlePubSubMessage(c, &pushBody.Message)
}

// PullPubSubOnDevServer is called on dev server to pull messages from PubSub
// subscription associated with given publisher.
//
// Part of the internal interface, doesn't check ACLs.
func (e *engineImpl) PullPubSubOnDevServer(c context.Context, taskManagerName, publisher string) error {
	_, sub := e.genTopicAndSubNames(c, taskManagerName, publisher)
	msg, ack, err := pullSubcription(c, sub, "")
	if err != nil {
		return err
	}
	if msg == nil {
		logging.Infof(c, "No new PubSub messages")
		return nil
	}
	err = e.handlePubSubMessage(c, msg)
	if err == nil || !transient.Tag.In(err) {
		ack() // ack only on success of fatal errors (to stop redelivery)
	}
	return err
}

////////////////////////////////////////////////////////////////////////////////
// Job related methods.

// txnCallback is passed to 'txn' and it modifies 'job' in place. 'txn' then
// puts it into datastore. The callback may return errSkipPut to instruct 'txn'
// not to call datastore 'Put'. The callback may do other transactional things
// using the context.
type txnCallback func(c context.Context, job *Job, isNew bool) error

// errSkipPut can be returned by txnCallback to cancel ds.Put call.
var errSkipPut = errors.New("errSkipPut")

// jobTxn reads Job entity, calls the callback, then dumps the modified entity
// back into datastore (unless the callback returns errSkipPut).
func (e *engineImpl) jobTxn(c context.Context, jobID string, callback txnCallback) error {
	c = logging.SetField(c, "JobID", jobID)
	return runTxn(c, func(c context.Context) error {
		stored := Job{JobID: jobID}
		err := ds.Get(c, &stored)
		if err != nil && err != ds.ErrNoSuchEntity {
			return transient.Tag.Apply(err)
		}
		modified := stored // make a copy of Job struct
		switch err = callback(c, &modified, err == ds.ErrNoSuchEntity); {
		case err == errSkipPut:
			return nil // asked to skip the update
		case err != nil:
			return err // a real error (transient or fatal)
		case !modified.IsEqual(&stored):
			return transient.Tag.Apply(ds.Put(c, &modified))
		}
		return nil
	})
}

// rollSM is called under transaction to perform a single state machine step.
//
// It sets up StateMachine instance, calls the callback, mutates job.State in
// place (with a new state) and enqueues all emitted actions to task queues.
func (e *engineImpl) rollSM(c context.Context, job *Job, cb func(*StateMachine) error) error {
	assertInTransaction(c)
	sched, err := job.ParseSchedule()
	if err != nil {
		return fmt.Errorf("bad schedule %q - %s", job.EffectiveSchedule(), err)
	}
	now := clock.Now(c).UTC()
	rnd := mathrand.Get(c)
	sm := StateMachine{
		State:    job.State,
		Now:      now,
		Schedule: sched,
		Nonce:    func() int64 { return rnd.Int63() + 1 },
		Context:  c,
	}
	// All errors returned by state machine transition changes are transient.
	// Fatal errors (when we have them) should be reflected as a state changing
	// into "BROKEN" state.
	if err := cb(&sm); err != nil {
		return transient.Tag.Apply(err)
	}
	if len(sm.Actions) != 0 {
		if err := e.enqueueJobActions(c, job.JobID, sm.Actions); err != nil {
			return err
		}
	}
	if sm.State.State != job.State.State {
		logging.Infof(c, "%s -> %s", job.State.State, sm.State.State)
	}
	job.State = sm.State
	return nil
}

// isV2Job returns true if the given job is using v2 scheduler engine.
func (e *engineImpl) isV2Job(jobID string) bool {
	return false
}

// jobController returns an appropriate implementation of the jobController
// depending of a version of the engine the job is using (v1 or v2).
func (e *engineImpl) jobController(jobID string) jobController {
	if e.isV2Job(jobID) {
		return &jobControllerV2{eng: e}
	}
	return &jobControllerV1{eng: e}
}

// getJob returns a job if it exists or nil if not.
//
// Doesn't check ACLs.
func (e *engineImpl) getJob(c context.Context, jobID string) (*Job, error) {
	job := &Job{JobID: jobID}
	switch err := ds.Get(c, job); {
	case err == nil:
		return job, nil
	case err == ds.ErrNoSuchEntity:
		return nil, nil
	default:
		return nil, transient.Tag.Apply(err)
	}
}

// getOwnedJob returns a job if the current caller owns it.
//
// Returns ErrNoOwnerPermission or ErrNoSuchJob otherwise (based on ACLs).
func (e *engineImpl) getOwnedJob(c context.Context, jobID string) (*Job, error) {
	job, err := e.getJob(c, jobID)
	if err != nil {
		return nil, err
	} else if job == nil {
		return nil, ErrNoSuchJob
	}

	switch owner, err := job.IsOwned(c); {
	case err != nil:
		return nil, err
	case owner:
		return job, nil
	}

	// Not owner, but maybe reader? Give nicer error in such case.
	switch reader, err := job.IsVisible(c); {
	case err != nil:
		return nil, err
	case reader:
		return nil, ErrNoOwnerPermission
	default:
		return nil, ErrNoSuchJob
	}
}

// getProjectJobs fetches from ds all enabled jobs belonging to a given
// project.
func (e *engineImpl) getProjectJobs(c context.Context, projectID string) (map[string]*Job, error) {
	q := ds.NewQuery("Job").
		Eq("Enabled", true).
		Eq("ProjectID", projectID)
	entities := []*Job{}
	if err := ds.GetAll(c, q, &entities); err != nil {
		return nil, transient.Tag.Apply(err)
	}
	out := make(map[string]*Job, len(entities))
	for _, job := range entities {
		if job.Enabled && job.ProjectID == projectID {
			out[job.JobID] = job
		}
	}
	return out, nil
}

// queryEnabledVisibleJobs fetches all jobs from the query and keeps only ones
// that are enabled and visible by the current caller.
func (e *engineImpl) queryEnabledVisibleJobs(c context.Context, q *ds.Query) ([]*Job, error) {
	entities := []*Job{}
	if err := ds.GetAll(c, q, &entities); err != nil {
		return nil, transient.Tag.Apply(err)
	}
	// Non-ancestor query used, need to recheck filters.
	filtered := make([]*Job, 0, len(entities))
	for _, job := range entities {
		if !job.Enabled {
			continue
		}
		// TODO(tandrii): improve batch ACLs check here to take advantage of likely
		// shared ACLs between most jobs of the same project.
		if ok, err := job.IsVisible(c); err != nil {
			return nil, err
		} else if ok {
			filtered = append(filtered, job)
		}
	}
	return filtered, nil
}

// setJobPausedFlag is implementation of PauseJob/ResumeJob.
func (e *engineImpl) setJobPausedFlag(c context.Context, jobID string, paused bool, who identity.Identity) error {
	// First, we check ACLs outside of transaction. Yes, this allows for races
	// between (un)pausing and ACLs changes but these races have no impact.
	if _, err := e.getOwnedJob(c, jobID); err != nil {
		return err
	}
	return e.jobTxn(c, jobID, func(c context.Context, job *Job, isNew bool) error {
		switch {
		case isNew || !job.Enabled:
			return ErrNoSuchJob
		case job.Paused == paused:
			return errSkipPut
		}
		if paused {
			logging.Warningf(c, "Job is paused by %s", who)
		} else {
			logging.Warningf(c, "Job is resumed by %s", who)
		}
		job.Paused = paused
		return e.jobController(jobID).onJobScheduleChange(c, job)
	})
}

// updateJob updates an existing job if its definition has changed, adds
// a completely new job or enables a previously disabled job.
func (e *engineImpl) updateJob(c context.Context, def catalog.Definition) error {
	return e.jobTxn(c, def.JobID, func(c context.Context, job *Job, isNew bool) error {
		if !isNew && job.Enabled && job.MatchesDefinition(def) {
			return errSkipPut
		}
		if isNew {
			// JobID is <projectID>/<name>, it's ensured by Catalog.
			chunks := strings.Split(def.JobID, "/")
			if len(chunks) != 2 {
				return fmt.Errorf("unexpected jobID format: %s", def.JobID)
			}
			*job = Job{
				JobID:           def.JobID,
				ProjectID:       chunks[0],
				Flavor:          def.Flavor,
				Enabled:         false, // to trigger 'if !oldEnabled' below
				Schedule:        def.Schedule,
				Task:            def.Task,
				State:           JobState{State: JobStateDisabled},
				TriggeredJobIDs: def.TriggeredJobIDs,
			}
		}
		oldEnabled := job.Enabled
		oldEffectiveSchedule := job.EffectiveSchedule()

		// Update the job in full before running any state changes.
		job.Flavor = def.Flavor
		job.Revision = def.Revision
		job.RevisionURL = def.RevisionURL
		job.Acls = def.Acls
		job.Enabled = true
		job.Schedule = def.Schedule
		job.Task = def.Task
		job.TriggeredJobIDs = def.TriggeredJobIDs

		// Kick off task queue tasks.
		ctl := e.jobController(job.JobID)
		if !oldEnabled {
			if err := ctl.onJobEnabled(c, job); err != nil {
				return err
			}
		}
		if job.EffectiveSchedule() != oldEffectiveSchedule {
			logging.Infof(c, "Job's schedule changed: %q -> %q",
				job.EffectiveSchedule(), oldEffectiveSchedule)
			if err := ctl.onJobScheduleChange(c, job); err != nil {
				return err
			}
		}
		return nil
	})
}

// disableJob moves a job to disabled state.
func (e *engineImpl) disableJob(c context.Context, jobID string) error {
	return e.jobTxn(c, jobID, func(c context.Context, job *Job, isNew bool) error {
		if isNew || !job.Enabled {
			return errSkipPut
		}
		job.Enabled = false
		return e.jobController(jobID).onJobDisabled(c, job)
	})
}

// resetJobOnDevServer sends "off" signal followed by "on" signal.
//
// It effectively cancels any pending actions and schedules new ones. Used only
// on dev server.
func (e *engineImpl) resetJobOnDevServer(c context.Context, jobID string) error {
	return e.jobTxn(c, jobID, func(c context.Context, job *Job, isNew bool) error {
		if isNew || !job.Enabled {
			return errSkipPut
		}
		logging.Infof(c, "Resetting job")
		ctl := e.jobController(jobID)
		if err := ctl.onJobDisabled(c, job); err != nil {
			return err
		}
		if err := ctl.onJobEnabled(c, job); err != nil {
			return err
		}
		return nil
	})
}

////////////////////////////////////////////////////////////////////////////////
// Invocations related methods.

// getInvocation returns an existing invocation or nil.
//
// Doesn't check ACLs.
//
// Supports both v1 and v2 invocations.
func (e *engineImpl) getInvocation(c context.Context, jobID string, invID int64) (*Invocation, error) {
	isV2 := e.isV2Job(jobID)
	inv := &Invocation{ID: invID}
	if !isV2 {
		inv.JobKey = ds.NewKey(c, "Job", jobID, 0, nil)
	}
	switch err := ds.Get(c, inv); {
	case err == nil:
		if isV2 && inv.JobID != jobID {
			logging.Errorf(c,
				"Invocation %d is associated with job %q, not %q. Treating it as missing.",
				invID, inv.JobID, jobID)
			return nil, nil
		}
		return inv, nil
	case err == ds.ErrNoSuchEntity:
		return nil, nil
	default:
		return nil, transient.Tag.Apply(err)
	}
}

// newInvocation allocates invocation ID and populates related fields of the
// Invocation struct: ID, JobKey, JobID. It doesn't store the invocation in
// the datastore.
//
// On success returns exact same 'inv' for convenience.
//
// Must be called within a transaction, since it verifies an allocated ID is
// not used yet.
//
// Supports both v1 and v2 invocations.
func (e *engineImpl) newInvocation(c context.Context, jobID string, inv *Invocation) (*Invocation, error) {
	assertInTransaction(c)
	isV2 := e.isV2Job(jobID)
	var jobKey *ds.Key
	if !isV2 {
		jobKey = ds.NewKey(c, "Job", jobID, 0, nil)
	}
	invID, err := generateInvocationID(c, jobKey)
	if err != nil {
		return nil, err
	}
	inv.ID = invID
	if isV2 {
		inv.JobID = jobID
	} else {
		inv.JobKey = jobKey
	}
	return inv, nil
}

// abortInvocation marks some invocation as aborted.
//
// Supports both v1 and v2 invocations.
func (e *engineImpl) abortInvocation(c context.Context, jobID string, invID int64) error {
	return e.withController(c, jobID, invID, "manual abort", func(c context.Context, ctl *taskController) error {
		ctl.DebugLog("Invocation is manually aborted by %q", auth.CurrentUser(c))
		if err := ctl.manager.AbortTask(c, ctl); err != nil {
			logging.WithError(err).Errorf(c, "Failed to abort the task")
			return err
		}
		ctl.State().Status = task.StatusAborted
		return nil
	})
}

////////////////////////////////////////////////////////////////////////////////
// Job related task queue messages and routing.

// actionTaskPayload is payload for task queue jobs emitted by the engine.
//
// Serialized as JSON, produced by enqueueJobActions, enqueueInvTimers, and
// enqueueTriggers, used as inputs in ExecuteSerializedAction.
//
// Union of all possible payloads for simplicity.
type actionTaskPayload struct {
	JobID string `json:",omitempty"` // ID of relevant Job
	InvID int64  `json:",omitempty"` // ID of relevant Invocation

	// For Job actions and timers (InvID == 0).
	Kind                string         `json:",omitempty"` // defines what fields below to examine
	TickNonce           int64          `json:",omitempty"` // valid for "TickLaterAction" kind
	InvocationNonce     int64          `json:",omitempty"` // valid for "StartInvocationAction" kind
	TriggeredBy         string         `json:",omitempty"` // valid for "StartInvocationAction" kind
	Triggers            []task.Trigger `json:",omitempty"` // valid for "StartInvocationAction" and "EnqueueTriggersAction" kind
	Overruns            int            `json:",omitempty"` // valid for "RecordOverrunAction" kind
	RunningInvocationID int64          `json:",omitempty"` // valid for "RecordOverrunAction" kind

	// For Invocation actions and timers (InvID != 0).
	InvTimer *invocationTimer `json:",omitempty"` // used for AddTimer calls
}

// invocationTimer is carried as part of task queue task payload for tasks
// created by AddTimer calls.
//
// It will be serialized to JSON, so all fields are public.
type invocationTimer struct {
	Delay   time.Duration
	Name    string
	Payload []byte
}

// enqueueJobActions submits all actions emitted by a job state transition by
// adding corresponding tasks to task queues.
//
// See executeJobAction for a place where these actions are interpreted.
func (e *engineImpl) enqueueJobActions(c context.Context, jobID string, actions []Action) error {
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
			qs[e.cfg.TimersQueueName] = append(qs[e.cfg.TimersQueueName], &taskqueue.Task{
				Path:    e.cfg.TimersQueuePath,
				ETA:     a.When,
				Payload: payload,
			})
		case StartInvocationAction:
			payload, err := json.Marshal(actionTaskPayload{
				JobID:           jobID,
				Kind:            "StartInvocationAction",
				InvocationNonce: a.InvocationNonce,
				TriggeredBy:     string(a.TriggeredBy),
				Triggers:        a.Triggers,
			})
			if err != nil {
				return err
			}
			qs[e.cfg.InvocationsQueueName] = append(qs[e.cfg.InvocationsQueueName], &taskqueue.Task{
				Path:    e.cfg.InvocationsQueuePath,
				Delay:   time.Second, // give the transaction time to land
				Payload: payload,
				RetryOptions: &taskqueue.RetryOptions{
					// Give 5 attempts to mark the job as failed. See 'startInvocation'.
					RetryLimit: invocationRetryLimit + 5,
					MinBackoff: time.Second,
					MaxBackoff: maxInvocationRetryBackoff,
					AgeLimit:   time.Duration(invocationRetryLimit+5) * maxInvocationRetryBackoff,
				},
			})
		case EnqueueTriggersAction:
			payload, err := json.Marshal(actionTaskPayload{
				JobID:    jobID,
				Kind:     "EnqueueTriggersAction",
				Triggers: a.Triggers,
			})
			if err != nil {
				return err
			}
			qs[e.cfg.InvocationsQueueName] = append(qs[e.cfg.InvocationsQueueName], &taskqueue.Task{
				Path:    e.cfg.InvocationsQueuePath,
				Payload: payload,
			})
		case RecordOverrunAction:
			payload, err := json.Marshal(actionTaskPayload{
				JobID:               jobID,
				Kind:                "RecordOverrunAction",
				Overruns:            a.Overruns,
				RunningInvocationID: a.RunningInvocationID,
			})
			if err != nil {
				return err
			}
			qs[e.cfg.InvocationsQueueName] = append(qs[e.cfg.InvocationsQueueName], &taskqueue.Task{
				Path:    e.cfg.InvocationsQueuePath,
				Delay:   time.Second, // give the transaction time to land
				Payload: payload,
			})
		default:
			logging.Errorf(c, "Unexpected action type %T, skipping", a)
		}
	}
	wg := sync.WaitGroup{}
	errs := errors.NewLazyMultiError(len(qs))
	i := 0
	for queueName, tasks := range qs {
		wg.Add(1)
		go func(i int, queueName string, tasks []*taskqueue.Task) {
			errs.Assign(i, taskqueue.Add(c, queueName, tasks...))
			wg.Done()
		}(i, queueName, tasks)
		i++
	}
	wg.Wait()
	return transient.Tag.Apply(errs.Get())
}

// executeJobAction routes an action that targets a job.
func (e *engineImpl) executeJobAction(c context.Context, payload *actionTaskPayload, retryCount int) error {
	switch payload.Kind {
	case "TickLaterAction":
		return e.jobTimerTick(c, payload.JobID, payload.TickNonce)
	case "StartInvocationAction":
		return e.startInvocation(
			c, payload.JobID, payload.InvocationNonce,
			identity.Identity(payload.TriggeredBy), payload.Triggers, retryCount)
	case "RecordOverrunAction":
		return e.recordOverrun(c, payload.JobID, payload.Overruns, payload.RunningInvocationID)
	case "EnqueueTriggersAction":
		return e.newTriggers(c, payload.JobID, payload.Triggers)
	default:
		return fmt.Errorf("unexpected job action kind %q", payload.Kind)
	}
}

// jobTimerTick is invoked via task queue in a task with some ETA. It what makes
// cron tick.
//
// Used by v1 engine only!
func (e *engineImpl) jobTimerTick(c context.Context, jobID string, tickNonce int64) error {
	return e.jobTxn(c, jobID, func(c context.Context, job *Job, isNew bool) error {
		if isNew {
			logging.Errorf(c, "Scheduled job is unexpectedly gone")
			return errSkipPut
		}
		logging.Infof(c, "Tick %d has arrived", tickNonce)
		return e.rollSM(c, job, func(sm *StateMachine) error { return sm.OnTimerTick(tickNonce) })
	})
}

// recordOverrun is invoked via task queue when a job should have been started,
// but previous invocation was still running.
//
// It creates new phony Invocation entity (in 'FAILED' state) in the datastore,
// to keep record of all overruns. Doesn't modify Job entity.
//
// Supports both v1 and v2 invocations.
func (e *engineImpl) recordOverrun(c context.Context, jobID string, overruns int, runningInvID int64) error {
	return runTxn(c, func(c context.Context) error {
		now := clock.Now(c).UTC()
		inv, err := e.newInvocation(c, jobID, &Invocation{
			Started:  now,
			Finished: now,
			Status:   task.StatusOverrun,
		})
		if err != nil {
			return err
		}
		if runningInvID == 0 {
			inv.debugLog(c, "New invocation should be starting now, but previous one is still starting")
		} else {
			inv.debugLog(c, "New invocation should be starting now, but previous one is still running: %d", runningInvID)
		}
		inv.debugLog(c, "Total overruns thus far: %d", overruns)
		return transient.Tag.Apply(ds.Put(c, &inv))
	})
}

// newTriggers is invoked via task queue when a job receives new Triggers.
//
// It adds these triggers to job's state. If job isn't yet running, this may
// result in StartInvocationAction being emitted.
//
// Used by v1 engine only!
func (e *engineImpl) newTriggers(c context.Context, jobID string, triggers []task.Trigger) error {
	return e.jobTxn(c, jobID, func(c context.Context, job *Job, isNew bool) error {
		if isNew {
			logging.Errorf(c, "Triggered job is unexpectedly gone")
			return errSkipPut
		}
		logging.Infof(c, "Triggered %d times", len(triggers))
		return e.rollSM(c, job, func(sm *StateMachine) error {
			sm.OnNewTriggers(triggers)
			return nil
		})
	})
}

////////////////////////////////////////////////////////////////////////////////
// Invocation related task queue routing.

// executeInvAction routes an action that targets an invocation.
func (e *engineImpl) executeInvAction(c context.Context, payload *actionTaskPayload, retryCount int) error {
	switch {
	case payload.InvTimer != nil:
		return e.invocationTimerTick(c, payload.JobID, payload.InvID, payload.InvTimer)
	default:
		return fmt.Errorf("unexpected invocation action kind %q", payload)
	}
}

// invocationTimerTick is called via Task Queue to handle AddTimer callbacks.
//
// See also handlePubSubMessage, it is quite similar.
func (e *engineImpl) invocationTimerTick(c context.Context, jobID string, invID int64, timer *invocationTimer) error {
	action := fmt.Sprintf("timer %q tick", timer.Name)
	return e.withController(c, jobID, invID, action, func(c context.Context, ctl *taskController) error {
		err := ctl.manager.HandleTimer(c, ctl, timer.Name, timer.Payload)
		switch {
		case err == nil:
			return nil // success! save the invocation
		case transient.Tag.In(err):
			return err // ask for redelivery on transient errors, don't touch the invocation
		}
		// On fatal errors, move the invocation to failed state (if not already).
		if ctl.State().Status != task.StatusFailed {
			ctl.DebugLog("Fatal error when handling timer, aborting invocation - %s", err)
			ctl.State().Status = task.StatusFailed
		}
		return nil // need to save the invocation, even on fatal errors
	})
}

////////////////////////////////////////////////////////////////////////////////
// Task controller and invocation launch.

const (
	// invocationRetryLimit is how many times to retry an invocation before giving
	// up and resuming the job's schedule.
	invocationRetryLimit = 5

	// maxInvocationRetryBackoff is how long to wait before retrying a failed
	// invocation.
	maxInvocationRetryBackoff = 10 * time.Second
)

// withController fetches the invocation, instantiates the task controller,
// calls the callback, and saves back the modified invocation state, initiating
// all necessary engine transitions along the way.
//
// Does nothing and returns nil if the invocation is already in a final state.
// The callback is not called in this case at all.
//
// Skips saving the invocation if the callback returns non-nil.
//
// 'action' is used exclusively for logging. It's a human readable cause of why
// the controller is instantiated.
func (e *engineImpl) withController(c context.Context, jobID string, invID int64, action string, cb func(context.Context, *taskController) error) error {
	c = logging.SetField(c, "JobID", jobID)
	c = logging.SetField(c, "InvID", invID)

	logging.Infof(c, "Handling %s", action)

	inv, err := e.getInvocation(c, jobID, invID)
	switch {
	case err != nil:
		logging.WithError(err).Errorf(c, "Failed to fetch the invocation")
		return err
	case inv == nil:
		logging.WithError(ErrNoSuchInvocation).Errorf(c, "No such invocation")
		return ErrNoSuchInvocation
	case inv.Status.Final():
		logging.Infof(c, "Skipping %s, the invocation is in final state %q", action, inv.Status)
		return nil
	}

	ctl, err := controllerForInvocation(c, e, inv)
	if err != nil {
		logging.WithError(err).Errorf(c, "Cannot get the controller")
		return err
	}

	if err := cb(c, ctl); err != nil {
		logging.WithError(err).Errorf(c, "Failed to perform %s, skipping saving the invocation", action)
		return err
	}

	if err := ctl.Save(c); err != nil {
		logging.WithError(err).Errorf(c, "Error when saving the invocation")
		return err
	}

	return nil
}

// startInvocation is called via task queue to start running a job.
//
// This call may be retried by task queue service.
//
// Used by v1 engine only!
func (e *engineImpl) startInvocation(c context.Context, jobID string, invocationNonce int64,
	triggeredBy identity.Identity, triggers []task.Trigger, retryCount int) error {

	c = logging.SetField(c, "JobID", jobID)
	c = logging.SetField(c, "InvNonce", invocationNonce)
	c = logging.SetField(c, "Attempt", retryCount)

	// TODO(vadimsh): This works only for v1 jobs currently.
	if e.isV2Job(jobID) {
		panic("must be v1 job")
	}

	// Create new Invocation entity in StatusStarting state and associated it with
	// Job entity.
	//
	// Task queue guarantees not to execute same task concurrently (i.e. retry
	// happens only if previous attempt finished already).
	// There are 4 possibilities here:
	// 1) It is a first attempt. In that case we generate new Invocation in
	//    state STARTING and update Job with a reference to it.
	// 2) It is a retry and previous attempt is still starting (indicated by
	//    IsExpectingInvocation returning true). Assume it failed to start
	//    and launch a new one. Mark old one as obsolete.
	// 3) It is a retry and previous attempt has already started (in this case
	//    the job is in RUNNING state and IsExpectingInvocation returns
	//    false). Assume this retry was unnecessary and skip it.
	// 4) It is a retry and we have retried too many times. Mark the invocation
	//    as failed and resume the job's schedule. This branch executes only if
	//    retryCount check at the end of this function failed to run (e.g. request
	//    handler crashed).
	inv := Invocation{}
	skipRunning := false
	err := e.jobTxn(c, jobID, func(c context.Context, job *Job, isNew bool) error {
		if isNew {
			logging.Errorf(c, "Queued job is unexpectedly gone")
			skipRunning = true
			return errSkipPut
		}
		if !job.State.IsExpectingInvocation(invocationNonce) {
			logging.Errorf(c, "No longer need to start invocation with nonce %d", invocationNonce)
			skipRunning = true
			return errSkipPut
		}
		inv = Invocation{
			Started:          clock.Now(c).UTC(),
			InvocationNonce:  invocationNonce,
			TriggeredBy:      triggeredBy,
			IncomingTriggers: triggers,
			Revision:         job.Revision,
			RevisionURL:      job.RevisionURL,
			Task:             job.Task,
			TriggeredJobIDs:  job.TriggeredJobIDs,
			RetryCount:       int64(retryCount),
			Status:           task.StatusStarting,
		}
		if _, err := e.newInvocation(c, job.JobID, &inv); err != nil {
			return err
		}
		inv.debugLog(c, "Invocation initiated (attempt %d)", retryCount+1)
		if triggeredBy != "" {
			inv.debugLog(c, "Manually triggered by %s", triggeredBy)
		}
		if retryCount >= invocationRetryLimit {
			logging.Errorf(c, "Too many attempts, giving up")
			inv.debugLog(c, "Too many attempts, giving up")
			inv.Status = task.StatusFailed
			inv.Finished = clock.Now(c).UTC()
			skipRunning = true
		}
		if err := ds.Put(c, &inv); err != nil {
			return transient.Tag.Apply(err)
		}
		// Move previous invocation (if any) to failed state. It has failed to
		// start.
		if job.State.InvocationID != 0 {
			prev := Invocation{
				ID:     job.State.InvocationID,
				JobKey: inv.JobKey, // works only for v1!
			}
			err := ds.Get(c, &prev)
			if err != nil && err != ds.ErrNoSuchEntity {
				return transient.Tag.Apply(err)
			}
			if err == nil && !prev.Status.Final() {
				prev.debugLog(c, "New invocation is starting (%d), marking this one as failed.", inv.ID)
				prev.Status = task.StatusFailed
				prev.Finished = clock.Now(c).UTC()
				prev.MutationsCount++
				prev.trimDebugLog()
				if err := ds.Put(c, &prev); err != nil {
					return transient.Tag.Apply(err)
				}
			}
		}
		// Store the reference to the new invocation ID. Unblock the job if we are
		// giving up on retrying.
		return e.rollSM(c, job, func(sm *StateMachine) error {
			sm.OnInvocationStarting(invocationNonce, inv.ID, retryCount)
			if inv.Status == task.StatusFailed {
				sm.OnInvocationStarted(inv.ID)
				sm.OnInvocationDone(inv.ID)
			}
			return nil
		})
	})

	if err != nil {
		logging.WithError(err).Errorf(c, "Failed to update job state")
		return err
	}

	if skipRunning {
		logging.Warningf(c, "No need to start the invocation anymore")
		return nil
	}

	c = logging.SetField(c, "InvID", inv.ID)
	return e.launchTask(c, &inv)
}

// launchTask instantiates an invocation controller and calls its LaunchTask
// method, saving the invocation state when its done.
//
// It returns a transient error if the launch attempt should be retried.
//
// Supports both v1 and v2 invocations.
func (e *engineImpl) launchTask(c context.Context, inv *Invocation) error {
	// Now we have a new Invocation entity in the datastore in StatusStarting
	// state. Grab corresponding TaskManager and launch task through it, keeping
	// track of the progress in created Invocation entity.
	ctl, err := controllerForInvocation(c, e, inv)
	if err != nil {
		// Note: controllerForInvocation returns both ctl and err on errors, with
		// ctl not fully initialized (but good enough for what's done below).
		ctl.DebugLog("Failed to initialize task controller - %s", err)
		ctl.State().Status = task.StatusFailed
		return ctl.Save(c)
	}

	// Ask the manager to start the task. If it returns no errors, it should also
	// move the invocation out of StatusStarting state (a failure to do so is a
	// fatal error). If it returns an error, the invocation is forcefully moved to
	// StatusRetrying or StatusFailed state (depending on whether the error is
	// transient or not and how many retries are left). In either case, invocation
	// never ends up in StatusStarting state.
	err = ctl.manager.LaunchTask(c, ctl, inv.IncomingTriggers)
	if ctl.State().Status == task.StatusStarting && err == nil {
		err = fmt.Errorf("LaunchTask didn't move invocation out of StatusStarting")
	}
	if transient.Tag.In(err) && inv.RetryCount+1 >= invocationRetryLimit {
		err = fmt.Errorf("Too many retries, giving up (original error - %s)", err)
	}

	// The task must always end up in a non-starting state. Do it on behalf of the
	// controller if necessary.
	if ctl.State().Status == task.StatusStarting {
		if transient.Tag.In(err) {
			// Note: in v1 version of the engine this status is changed into
			// StatusFailed when new invocation attempt starts (see the transaction
			// above, in particular "New invocation is starting" part). In v2 this
			// will be handled differently (v2 will reuse same Invocation object for
			// retries).
			ctl.State().Status = task.StatusRetrying
		} else {
			ctl.State().Status = task.StatusFailed
		}
	}

	// We MUST commit the state of the invocation. A failure to save the state
	// may cause the job state machine to get stuck. If we can't save it, we need
	// to retry the whole launch attempt from scratch (redoing all the work,
	// a properly implemented LaunchTask should be idempotent).
	if err := ctl.Save(c); err != nil {
		logging.WithError(err).Errorf(c, "Failed to save invocation state")
		return err
	}

	// Task retries happen via the task queue, need to explicitly trigger a retry
	// by returning a transient error.
	if ctl.State().Status == task.StatusRetrying {
		return errors.New("task failed to start, retrying", transient.Tag)
	}

	return nil
}

// enqueueInvTimers submits all timers emitted by an invocation manager by
// adding corresponding tasks to the task queue.
//
// Called from a transaction around corresponding Invocation entity.
//
// See executeInvAction for a place where these actions are interpreted.
//
// Used by v1 engine only!
func (e *engineImpl) enqueueInvTimers(c context.Context, jobID string, invID int64, timers []invocationTimer) error {
	assertInTransaction(c)
	tasks := make([]*taskqueue.Task, len(timers))
	for i, timer := range timers {
		payload, err := json.Marshal(actionTaskPayload{
			JobID:    jobID,
			InvID:    invID,
			InvTimer: &timer,
		})
		if err != nil {
			return err
		}
		tasks[i] = &taskqueue.Task{
			Path:    e.cfg.TimersQueuePath,
			ETA:     clock.Now(c).Add(timer.Delay),
			Payload: payload,
		}
	}
	return transient.Tag.Apply(taskqueue.Add(c, e.cfg.TimersQueueName, tasks...))
}

// enqueueTriggers submits all triggers emitted by an invocation manager by
// adding corresponding tasks to the task queue.
//
// Called from a transaction around corresponding Invocation entity.
//
// See executeJobAction for a place where these actions are interpreted.
//
// Used by v1 engine only!
func (e *engineImpl) enqueueTriggers(c context.Context, triggeredJobIDs []string, triggers []task.Trigger) error {
	// TODO(tandrii): batch all enqueing into 1 TQ task because AE allows up to 5
	// tasks to be inserted transactionally. The batched TQ task will then
	// non-transactionally fan out into more TQ tasks.
	assertInTransaction(c)
	wg := sync.WaitGroup{}
	errs := errors.NewLazyMultiError(len(triggeredJobIDs))
	i := 0
	for _, jobID := range triggeredJobIDs {
		i++
		wg.Add(1)
		go func(i int, jobID string) {
			defer wg.Done()
			errs.Assign(i, e.enqueueJobActions(c, jobID, []Action{EnqueueTriggersAction{Triggers: triggers}}))
		}(i, jobID)
	}
	wg.Wait()
	return transient.Tag.Apply(errs.Get())
}

////////////////////////////////////////////////////////////////////////////////
// PubSub related methods.

// topicParams is passed to prepareTopic by task.Controller.
type topicParams struct {
	jobID     string       // the job invocation belongs to
	invID     int64        // ID of the invocation itself
	manager   task.Manager // task manager for the invocation
	publisher string       // name of publisher to add to PubSub topic.
}

// pubsubAuthToken describes how to generate HMAC protected tokens used to
// authenticate PubSub messages.
var pubsubAuthToken = tokens.TokenKind{
	Algo:       tokens.TokenAlgoHmacSHA256,
	Expiration: 48 * time.Hour,
	SecretKey:  "pubsub_auth_token",
	Version:    1,
}

// handlePubSubMessage routes the pubsub message to the invocation.
//
// See also invocationTimerTick, it is quite similar.
func (e *engineImpl) handlePubSubMessage(c context.Context, msg *pubsub.PubsubMessage) error {
	logging.Infof(c, "Received PubSub message %q", msg.MessageId)

	// Extract Job and Invocation ID from validated auth_token.
	var jobID string
	var invID int64
	data, err := pubsubAuthToken.Validate(c, msg.Attributes["auth_token"], nil)
	if err != nil {
		logging.WithError(err).Errorf(c, "Bad auth_token attribute")
		return err
	}
	jobID = data["job"]
	if invID, err = strconv.ParseInt(data["inv"], 10, 64); err != nil {
		logging.WithError(err).Errorf(c, "Could not parse 'inv' %q", data["inv"])
		return err
	}

	// Hand the message to the controller.
	action := fmt.Sprintf("pubsub message %q", msg.MessageId)
	return e.withController(c, jobID, invID, action, func(c context.Context, ctl *taskController) error {
		err := ctl.manager.HandleNotification(c, ctl, msg)
		switch {
		case err == nil:
			return nil // success! save the invocation
		case transient.Tag.In(err):
			return err // ask for redelivery on transient errors, don't touch the invocation
		}
		// On fatal errors, move the invocation to failed state (if not already).
		if ctl.State().Status != task.StatusFailed {
			ctl.DebugLog("Fatal error when handling PubSub notification, aborting invocation - %s", err)
			ctl.State().Status = task.StatusFailed
		}
		return nil // need to save the invocation, even on fatal errors
	})
}

// genTopicAndSubNames derives PubSub topic and subscription names to use for
// notifications from given publisher.
func (e *engineImpl) genTopicAndSubNames(c context.Context, manager, publisher string) (topic string, sub string) {
	// Avoid accidental override of the topic when running on dev server.
	prefix := "scheduler"
	if info.IsDevAppServer(c) {
		prefix = "dev-scheduler"
	}

	// Each publisher gets its own topic (and subscription), so it's clearer from
	// logs and PubSub console who's calling what. PubSub topics can't have "@" in
	// them, so replace "@" with "~". URL encoding could have been used too, but
	// Cloud Console confuses %40 with its own URL encoding and doesn't display
	// all pages correctly.
	id := fmt.Sprintf("%s+%s+%s",
		prefix,
		manager,
		strings.Replace(publisher, "@", "~", -1))

	appID := info.AppID(c)
	topic = fmt.Sprintf("projects/%s/topics/%s", appID, id)
	sub = fmt.Sprintf("projects/%s/subscriptions/%s", appID, id)
	return
}

// prepareTopic creates a pubsub topic that can be used to pass task related
// messages back to the task.Manager that handles the task.
//
// It returns full topic name, as well as a token that securely identifies the
// task. It should be put into 'auth_token' attribute of PubSub messages by
// whoever publishes them.
func (e *engineImpl) prepareTopic(c context.Context, params *topicParams) (topic string, tok string, err error) {
	// If given URL, ask the service for name of its default service account.
	// FetchServiceInfo implements efficient cache internally, so it's fine to
	// call it often.
	if strings.HasPrefix(params.publisher, "https://") {
		logging.Infof(c, "Fetching info about %q", params.publisher)
		serviceInfo, err := signing.FetchServiceInfoFromLUCIService(c, params.publisher)
		if err != nil {
			logging.Errorf(c, "Failed to fetch info about %q - %s", params.publisher, err)
			return "", "", err
		}
		logging.Infof(c, "%q is using %q", params.publisher, serviceInfo.ServiceAccountName)
		params.publisher = serviceInfo.ServiceAccountName
	}

	topic, sub := e.genTopicAndSubNames(c, params.manager.Name(), params.publisher)

	// Put same parameters in push URL to make them visible in logs. On dev server
	// use pull based subscription, since localhost push URL is not valid.
	pushURL := ""
	if !info.IsDevAppServer(c) {
		urlParams := url.Values{}
		urlParams.Add("kind", params.manager.Name())
		urlParams.Add("publisher", params.publisher)
		pushURL = fmt.Sprintf(
			"https://%s%s?%s", info.DefaultVersionHostname(c), e.cfg.PubSubPushPath, urlParams.Encode())
	}

	// Create and configure the topic. Do it only once.
	err = e.opsCache.Do(c, fmt.Sprintf("prepareTopic:v1:%s", topic), func() error {
		if e.configureTopic != nil {
			return e.configureTopic(c, topic, sub, pushURL, params.publisher)
		}
		return configureTopic(c, topic, sub, pushURL, params.publisher, "")
	})
	if err != nil {
		return "", "", err
	}

	// Encode full invocation identifier (job key + invocation ID) into HMAC
	// protected token.
	tok, err = pubsubAuthToken.Generate(c, nil, map[string]string{
		"job": params.jobID,
		"inv": fmt.Sprintf("%d", params.invID),
	}, 0)
	if err != nil {
		return "", "", err
	}

	return topic, tok, nil
}
