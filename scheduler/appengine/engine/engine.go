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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	"google.golang.org/api/pubsub/v1"

	"go.chromium.org/luci/appengine/tq"
	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"
	ds "go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/gae/service/info"
	"go.chromium.org/luci/gae/service/memcache"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/auth/signing"
	"go.chromium.org/luci/server/tokens"

	"go.chromium.org/luci/scheduler/appengine/catalog"
	"go.chromium.org/luci/scheduler/appengine/engine/cron"
	"go.chromium.org/luci/scheduler/appengine/engine/policy"
	"go.chromium.org/luci/scheduler/appengine/internal"
	"go.chromium.org/luci/scheduler/appengine/task"
)

// TODO(vadimsh): Use annotated errors instead of constants, so they can have
// more information.

var (
	// ErrNoPermission indicates the caller doesn't not have permission to perform
	// desired action.
	ErrNoPermission = errors.New("insufficient rights on a job")
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
// The general pattern for doing something to a job is to get a reference to
// it via GetVisibleJob() (this call checks "scheduler.jobs.get" permission),
// and then pass *Job to desired methods (which may additionally check for more
// permissions).
//
// ACLs are enforced with the following implication:
//   - if a caller lacks "scheduler.jobs.get" permission for a job, methods
//     behave as if the job doesn't exist.
//   - if a caller has "scheduler.jobs.get" permission but lacks some other
//     permission required to execute an action (e.g. "scheduler.jobs.abort"),
//     ErrNoPermission will be returned.
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
	//   * job isn't visible due to lack of "scheduler.jobs.get" permission.
	GetVisibleJob(c context.Context, jobID string) (*Job, error)

	// GetVisibleJobBatch is like GetVisibleJob, except it operates on a batch of
	// jobs at once.
	//
	// Returns a mapping (jobID => *Job) with only visible jobs. If the check
	// fails returns a transient error.
	GetVisibleJobBatch(c context.Context, jobIDs []string) (map[string]*Job, error)

	// ListInvocations returns invocations of a given job, sorted by their
	// creation time (most recent first).
	//
	// Can optionally return only active invocations (i.e. ones that are pending,
	// starting or running) or only finished ones. See ListInvocationsOpts.
	//
	// Returns invocations and a cursor string if there's more. Returns only
	// transient errors.
	ListInvocations(c context.Context, job *Job, opts ListInvocationsOpts) ([]*Invocation, string, error)

	// GetInvocation returns an invocation of a given job.
	//
	// ErrNoSuchInvocation is returned if the invocation doesn't exist.
	GetInvocation(c context.Context, job *Job, invID int64) (*Invocation, error)

	// PauseJob prevents new automatic invocations of a job.
	//
	// It clears the pending triggers queue, and makes the job ignore all incoming
	// triggers until it is resumed.
	//
	// For cron jobs it also replaces job's schedule with "triggered", effectively
	// preventing them from running automatically (until unpaused).
	//
	// Does nothing if the job is already paused. Any pending or running
	// invocations are still executed.
	PauseJob(c context.Context, job *Job, reason string) error

	// ResumeJob resumes paused job. Does nothing if the job is not paused.
	ResumeJob(c context.Context, job *Job, reason string) error

	// AbortJob aborts all currently pending or running invocations (if any).
	AbortJob(c context.Context, job *Job) error

	// AbortInvocation forcefully moves the invocation to a failed state.
	//
	// It opportunistically tries to send "abort" signal to a job runner if it
	// supports cancellation, but it doesn't wait for reply (proceeds to
	// modifying the local state in the scheduler service datastore immediately).
	//
	// AbortInvocation can be used to manually "unstuck" jobs that got stuck due
	// to missing PubSub notifications or other kinds of unexpected conditions.
	//
	// Does nothing if the invocation is already in some final state.
	AbortInvocation(c context.Context, job *Job, invID int64) error

	// EmitTriggers puts one or more triggers into pending trigger queues of the
	// specified jobs.
	//
	// If the caller has no permission to trigger at least one job, the entire
	// call is aborted. Otherwise, the call is NOT transactional, but can be
	// safely retried (triggers are deduplicated based on their IDs).
	EmitTriggers(c context.Context, perJob map[*Job][]*internal.Trigger) error

	// ListTriggers returns list of job's pending triggers sorted by time, most
	// recent last.
	ListTriggers(c context.Context, job *Job) ([]*internal.Trigger, error)

	// GetJobTriageLog returns a log from the latest job triage procedure.
	//
	// Returns nil if it is not available (for example, the job was just created).
	GetJobTriageLog(c context.Context, job *Job) (*JobTriageLog, error)
}

// EngineInternal is a variant of engine API that skips ACL checks.
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

	// ProcessPubSubPush is called whenever incoming PubSub message is received.
	//
	// May return an error tagged with tq.Retry or transient.Tag. They indicate
	// the message should be redelivered later.
	ProcessPubSubPush(c context.Context, body []byte, urlValues url.Values) error

	// PullPubSubOnDevServer is called on dev server to pull messages from PubSub
	// subscription associated with given publisher.
	//
	// It is needed to be able to manually tests PubSub related workflows on dev
	// server, since dev server can't accept PubSub push messages.
	PullPubSubOnDevServer(c context.Context, taskManagerName, publisher string) error

	// GetDebugJobState is used by Admin RPC interface for debugging jobs.
	//
	// It fetches Job entity, pending triggers and pending completion
	// notifications.
	GetDebugJobState(c context.Context, jobID string) (*DebugJobState, error)
}

// DebugJobState contains detailed information about a job.
//
// The state is not a transactional snapshot. Shouldn't be used for anything
// other than just displaying it to humans.
type DebugJobState struct {
	Job                 *Job
	FinishedInvocations []*internal.FinishedInvocation // unmarshalled Job.FinishedInvocationsRaw
	RecentlyFinishedSet []int64                        // in-flight notifications from recentlyFinishedSet()
	PendingTriggersSet  []*internal.Trigger            // triggers from pendingTriggersSet()
	ManagerState        *internal.DebugManagerState    // whatever task.Manager wants to report
}

// ListInvocationsOpts are passed to ListInvocations method.
type ListInvocationsOpts struct {
	PageSize     int
	Cursor       string
	FinishedOnly bool
	ActiveOnly   bool
}

// Config contains parameters for the engine.
type Config struct {
	Catalog        catalog.Catalog // provides task.Manager's to run tasks
	Dispatcher     *tq.Dispatcher  // dispatcher for task queue tasks
	PubSubPushPath string          // URL to use in PubSub push config
}

// NewEngine returns default implementation of EngineInternal.
func NewEngine(cfg Config) EngineInternal {
	eng := &engineImpl{cfg: cfg}
	eng.init()
	return eng
}

type engineImpl struct {
	cfg      Config
	opsCache opsCache

	// configureTopic is used by prepareTopic, mocked in tests.
	configureTopic func(c context.Context, topic, sub, pushURL, publisher string) error
}

// init registers task queue handlers.
func (e *engineImpl) init() {
	// TODO(vadimsh): Figure out retry parameters for all tasks.
	e.cfg.Dispatcher.RegisterTask(&internal.LaunchInvocationsBatchTask{}, e.execLaunchInvocationsBatchTask, "batches", nil)
	e.cfg.Dispatcher.RegisterTask(&internal.LaunchInvocationTask{}, e.execLaunchInvocationTask, "launches", nil)
	e.cfg.Dispatcher.RegisterTask(&internal.TriageJobStateTask{}, e.execTriageJobStateTask, "triages", nil)
	e.cfg.Dispatcher.RegisterTask(&internal.KickTriageTask{}, e.execKickTriageTask, "triages", nil)
	e.cfg.Dispatcher.RegisterTask(&internal.InvocationFinishedTask{}, e.execInvocationFinishedTask, "completions", nil)
	e.cfg.Dispatcher.RegisterTask(&internal.FanOutTriggersTask{}, e.execFanOutTriggersTask, "triggers", nil)
	e.cfg.Dispatcher.RegisterTask(&internal.EnqueueTriggersTask{}, e.execEnqueueTriggersTask, "triggers", nil)
	e.cfg.Dispatcher.RegisterTask(&internal.ScheduleTimersTask{}, e.execScheduleTimersTask, "timers", nil)
	e.cfg.Dispatcher.RegisterTask(&internal.TimerTask{}, e.execTimerTask, "timers", nil)
	e.cfg.Dispatcher.RegisterTask(&internal.CronTickTask{}, e.execCronTickTask, "crons", nil)
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
	switch {
	case err != nil:
		return nil, err
	case job == nil || !job.Enabled:
		return nil, ErrNoSuchJob
	}
	if err := CheckPermission(c, job, PermJobsGet); err != nil {
		if err == ErrNoPermission {
			err = ErrNoSuchJob // pretend protected jobs don't exist
		}
		return nil, err
	}
	return job, nil
}

// GetVisibleJobBatch is like GetVisibleJob, except it operates on a batch of
// jobs at once.
//
// Part of the public interface.
//
// Part of the public interface, checks ACLs.
func (e *engineImpl) GetVisibleJobBatch(c context.Context, jobIDs []string) (map[string]*Job, error) {
	// TODO(vadimsh): This can be parallelized to be single GetMulti RPC to fetch
	// jobs and single filterByPerm to check ACLs. In practice O(len(jobIDs)) is
	// small, so there's no pressing need to do this.
	visible := make(map[string]*Job, len(jobIDs))
	for _, id := range jobIDs {
		switch job, err := e.GetVisibleJob(c, id); {
		case err == nil:
			visible[id] = job
		case err != ErrNoSuchJob:
			return nil, err
		}
	}
	return visible, nil
}

// ListInvocations returns invocations of a given job, most recent first.
//
// Part of the public interface.
func (e *engineImpl) ListInvocations(c context.Context, job *Job, opts ListInvocationsOpts) ([]*Invocation, string, error) {
	if opts.ActiveOnly && opts.FinishedOnly {
		return nil, "", fmt.Errorf("using both ActiveOnly and FinishedOnly is not allowed")
	}

	if opts.PageSize <= 0 || opts.PageSize > 500 {
		opts.PageSize = 500
	}

	var cursor internal.InvocationsCursor
	if err := decodeInvCursor(opts.Cursor, &cursor); err != nil {
		return nil, "", err
	}

	// We are going to merge results of multiple queries:
	//   1) Over historical finished invocations in the datastore.
	//   2) Over recently finished invocations, stored inline in the Job entity.
	//   3) Over active invocations, also stored inline in the Job entity.
	var qs []invQuery
	if !opts.ActiveOnly {
		// Most of the historical invocations came from the datastore query. But it
		// may not have recently finished invocations yet (due to Datastore eventual
		// consistently).
		q := finishedInvQuery(c, job, cursor.LastScanned)
		defer q.close()
		qs = append(qs, q)
		// Use recently finished invocations from the Job, since they may be more
		// up-to-date and do not depend on Datastore index consistency lag.
		qs = append(qs, recentInvQuery(c, job, cursor.LastScanned))
	}
	if !opts.FinishedOnly {
		qs = append(qs, activeInvQuery(c, job, cursor.LastScanned))
	}

	out := make([]*Invocation, 0, opts.PageSize)

	// Build the full page out of potentially incomplete (due to post-filtering)
	// smaller pages. Note that most of the time 'fetchInvsPage' will return the
	// full page right away.
	var page invsPage
	var err error
	for opts.PageSize > 0 {
		out, page, err = fetchInvsPage(c, qs, opts, out)
		switch {
		case err != nil:
			return nil, "", err
		case page.final:
			return out, "", nil // return empty cursor to indicate we are done
		}
		opts.PageSize -= page.count
	}

	// We end up here if the last fetched mini-page wasn't final, need new cursor.
	cursorStr, err := encodeInvCursor(&internal.InvocationsCursor{
		LastScanned: page.lastScanned,
	})
	if err != nil {
		return nil, "", errors.Annotate(err, "failed to serialize the cursor").Err()
	}
	return out, cursorStr, nil
}

// GetInvocation returns some invocation of a given job.
//
// Part of the public interface.
func (e *engineImpl) GetInvocation(c context.Context, job *Job, invID int64) (*Invocation, error) {
	// Note: we want public API users to go through GetVisibleJob to check ACLs,
	// thus usage of *Job, even though JobID string is sufficient in this case.
	return e.getInvocation(c, job.JobID, invID)
}

// PauseJob prevents new automatic invocations of a job.
//
// Part of the public interface, checks ACLs.
func (e *engineImpl) PauseJob(c context.Context, job *Job, reason string) error {
	if err := CheckPermission(c, job, PermJobsPause); err != nil {
		return err
	}
	return e.setJobPausedFlag(c, job, true, auth.CurrentIdentity(c), reason)
}

// ResumeJob resumes paused job. Does nothing if the job is not paused.
//
// Part of the public interface, checks ACLs.
func (e *engineImpl) ResumeJob(c context.Context, job *Job, reason string) error {
	if err := CheckPermission(c, job, PermJobsResume); err != nil {
		return err
	}
	return e.setJobPausedFlag(c, job, false, auth.CurrentIdentity(c), reason)
}

// AbortJob aborts all currently pending or running invocations (if any).
//
// Part of the public interface, checks ACLs.
func (e *engineImpl) AbortJob(c context.Context, job *Job) error {
	if err := CheckPermission(c, job, PermJobsAbort); err != nil {
		return err
	}
	jobID := job.JobID

	var invs []int64
	err := e.jobTxn(c, jobID, func(c context.Context, job *Job, isNew bool) (err error) {
		if isNew {
			return errSkipPut // the job was removed, nothing to abort
		}
		// We just abort all active invocations. This should cause them to
		// eventually move to Aborted state, which will kick them out of
		// ActiveInvocations list.
		invs = job.ActiveInvocations
		// AbortJob is sometimes used manually to "reset" the job state if it got
		// stuck for some reason. We initiate a triage for this purpose. This is not
		// strictly necessary (but doesn't hurt either).
		return e.kickTriageLater(c, job.JobID, 0)
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

// AbortInvocation forcefully moves the invocation to a failed state.
//
// Part of the public interface, checks ACLs.
func (e *engineImpl) AbortInvocation(c context.Context, job *Job, invID int64) error {
	if err := CheckPermission(c, job, PermJobsAbort); err != nil {
		return err
	}
	return e.abortInvocation(c, job.JobID, invID)
}

// EmitTriggers puts one or more triggers into pending trigger queues of the
// specified jobs.
func (e *engineImpl) EmitTriggers(c context.Context, perJob map[*Job][]*internal.Trigger) error {
	// Make sure the caller has permissions to add triggers to all jobs.
	jobs := make([]*Job, 0, len(perJob))
	for j := range perJob {
		jobs = append(jobs, j)
	}
	switch filtered, err := e.filterByPerm(c, jobs, PermJobsTrigger); {
	case err != nil:
		return errors.Annotate(err, "transient error when checking permissions").Err()
	case len(filtered) != len(jobs):
		return ErrNoPermission // some jobs are not triggerable
	}

	// Actually trigger.
	return parallel.FanOutIn(func(tasks chan<- func() error) {
		for job, triggers := range perJob {
			jobID := job.JobID
			tasks <- func() error {
				return e.execEnqueueTriggersTask(c, &internal.EnqueueTriggersTask{
					JobId:    jobID,
					Triggers: triggers,
				})
			}
		}
	})
}

// ListTriggers returns sorted list of job's pending triggers.
func (e *engineImpl) ListTriggers(c context.Context, job *Job) ([]*internal.Trigger, error) {
	_, triggers, err := pendingTriggersSet(c, job.JobID).Triggers(c)
	if err != nil {
		return nil, transient.Tag.Apply(err)
	}
	sortTriggers(triggers)
	return triggers, nil
}

// GetJobTriageLog returns a log from the latest job triage procedure.
func (e *engineImpl) GetJobTriageLog(c context.Context, job *Job) (*JobTriageLog, error) {
	log := JobTriageLog{JobID: job.JobID}
	switch err := ds.Get(c, &log); {
	case err == ds.ErrNoSuchEntity:
		return nil, nil
	case err != nil:
		return nil, transient.Tag.Apply(err)
	}

	// We assume the log is stale if the latest triage transaction has landed
	// sufficiently log time ago, but JobTriageLog is still old. 1 second here
	// really means "we assume the log is stored no slower than 1 sec after the
	// triage transaction lands". In practice the log is stored immediately after
	// the transaction, so most of the time there should be no false positives
	// (but they are still possible).
	if !job.LastTriage.IsZero() && clock.Since(c, job.LastTriage) > time.Second {
		log.stale = log.LastTriage.Before(job.LastTriage)
	}

	return &log, nil
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
	var toDisable []string
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

// ProcessPubSubPush is called whenever incoming PubSub message is received.
//
// Part of the internal interface, doesn't check ACLs.
func (e *engineImpl) ProcessPubSubPush(c context.Context, body []byte, urlValues url.Values) error {
	// Grab the task manager name that will receive this push. See prepareTopic
	// and pushSubscriptionURLValues for where `kind` is setup.
	manager := urlValues.Get("kind")

	// Deserialize the message as a Cloud PubSub JSON struct.
	var pushBody struct {
		Message pubsub.PubsubMessage `json:"message"`
	}
	if err := json.Unmarshal(body, &pushBody); err != nil {
		return err
	}

	// Retry once after a slight adhoc delay (to let datastore transactions land)
	// instead of returning an error to PubSub immediately. We don't want errors
	// tagged with tq.Retry to engage PubSub flow control, since they are
	// semi-expected and it is also expected that an immediate retry helps. This
	// is still best effort only and on another error we let the PubSub retry
	// mechanism to handle it.
	err := e.handlePubSubMessage(c, manager, &pushBody.Message)
	if err == nil || !tq.Retry.In(err) {
		return err
	}
	logging.Warningf(c, "Attempting a quick retry after 1s: %s", err)
	clock.Sleep(c, time.Second)
	return e.handlePubSubMessage(c, manager, &pushBody.Message)
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
	switch err = e.handlePubSubMessage(c, taskManagerName, msg); {
	case err == nil:
		ack() // success
	case transient.Tag.In(err) || tq.Retry.In(err):
		// don't ack, ask for redelivery
	default:
		ack() // fatal error
	}
	return err
}

// GetDebugJobState is used by Admin RPC interface for debugging jobs.
//
// Part of the internal interface, doesn't check ACLs.
func (e *engineImpl) GetDebugJobState(c context.Context, jobID string) (*DebugJobState, error) {
	job, err := e.getJob(c, jobID)
	switch {
	case err != nil:
		return nil, errors.Annotate(err, "failed to fetch Job entity").Err()
	case job == nil:
		return nil, ErrNoSuchJob
	}

	state := &DebugJobState{Job: job}

	// Fill in FinishedInvocations.
	state.FinishedInvocations, err = unmarshalFinishedInvs(job.FinishedInvocationsRaw)
	if err != nil {
		return nil, errors.Annotate(err, "failed to unmarshal FinishedInvocationsRaw").Err()
	}

	// Fill in RecentlyFinishedSet.
	finishedSet := recentlyFinishedSet(c, jobID)
	listing, err := finishedSet.List(c)
	if err != nil {
		return nil, errors.Annotate(err, "failed to fetch recentlyFinishedSet").Err()
	}
	state.RecentlyFinishedSet = make([]int64, len(listing.Items))
	for i, itm := range listing.Items {
		state.RecentlyFinishedSet[i] = finishedSet.ItemToInvID(&itm)
	}

	// Fill in PendingTriggersSet.
	triggersSet := pendingTriggersSet(c, jobID)
	_, state.PendingTriggersSet, err = triggersSet.Triggers(c)
	if err != nil {
		return nil, errors.Annotate(err, "failed to fetch pendingTriggersSet").Err()
	}

	// Ask the corresponding task.Manager to fill in DebugManagerState. Pass it
	// a phony controller configured just enough to be useful for fetching the
	// state, but not anything else.
	ctl, err := controllerForInvocation(c, e, &Invocation{
		ID:    -1,
		JobID: jobID,
		Task:  job.Task,
	})
	if err == nil {
		state.ManagerState, err = ctl.manager.GetDebugState(c, ctl)
	}
	if state.ManagerState == nil {
		state.ManagerState = &internal.DebugManagerState{}
	}
	if err != nil {
		state.ManagerState.Error = err.Error()
	}
	if ctl != nil {
		state.ManagerState.DebugLog = ctl.debugLog
	}

	return state, nil
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
	enabled := make([]*Job, 0, len(entities))
	for _, job := range entities {
		if job.Enabled {
			enabled = append(enabled, job)
		}
	}
	// Keep only ones visible to the caller.
	return e.filterByPerm(c, enabled, PermJobsGet)
}

// filterByPerm returns jobs for which caller has the given permission.
//
// May return transient errors.
func (e *engineImpl) filterByPerm(c context.Context, jobs []*Job, perm realms.Permission) ([]*Job, error) {
	// TODO(tandrii): improve batch ACLs check here to take advantage of likely
	// shared ACLs between most jobs of the same project.
	filtered := make([]*Job, 0, len(jobs))
	for _, job := range jobs {
		switch err := CheckPermission(c, job, perm); {
		case err == nil:
			filtered = append(filtered, job)
		case err != ErrNoPermission:
			return nil, err // a transient error when checking
		}
	}
	return filtered, nil
}

// setJobPausedFlag is implementation of PauseJob/ResumeJob.
//
// Doesn't check ACLs, assumes the check was done already.
func (e *engineImpl) setJobPausedFlag(c context.Context, job *Job, paused bool, who identity.Identity, reason string) error {
	return e.jobTxn(c, job.JobID, func(c context.Context, job *Job, isNew bool) error {
		switch {
		case isNew || !job.Enabled:
			return ErrNoSuchJob
		case job.Paused == paused:
			return errSkipPut
		}

		job.Paused = paused
		job.PausedOrResumedWhen = clock.Now(c).UTC()
		job.PausedOrResumedBy = who
		job.PausedOrResumedReason = reason

		if reason == "" {
			reason = "no reason given"
		}
		if paused {
			logging.Warningf(c, "Job is paused by %s - %s", who, reason)
		} else {
			logging.Warningf(c, "Job is resumed by %s - %s", who, reason)
		}

		// Reschedule the tick if necessary.
		err := pokeCron(c, job, e.cfg.Dispatcher, func(m *cron.Machine) error {
			m.OnScheduleChange()
			return nil
		})
		if err != nil {
			return err
		}

		// If paused, kick the triage to clear the pending triggers. We can pop them
		// only from within the triage procedure.
		if job.Paused {
			return e.kickTriageLater(c, job.JobID, 0)
		}
		return nil
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
				JobID:               def.JobID,
				ProjectID:           chunks[0],
				RealmID:             def.RealmID,
				Flavor:              def.Flavor,
				Enabled:             false, // to trigger 'if wasDisabled' below
				Schedule:            def.Schedule,
				Task:                def.Task,
				TriggeringPolicyRaw: def.TriggeringPolicy,
				TriggeredJobIDs:     def.TriggeredJobIDs,
			}
		}
		wasDisabled := !job.Enabled
		oldEffectiveSchedule := job.EffectiveSchedule()
		oldTriggeringPolicy := job.TriggeringPolicyRaw

		// Update the job in full before running any state changes.
		job.RealmID = def.RealmID
		job.Flavor = def.Flavor
		job.Revision = def.Revision
		job.RevisionURL = def.RevisionURL
		job.Enabled = true
		job.Schedule = def.Schedule
		job.Task = def.Task
		job.TriggeringPolicyRaw = def.TriggeringPolicy
		job.TriggeredJobIDs = def.TriggeredJobIDs

		// If job triggering policy has changed, schedule a triage to potentially
		// act based on the new policy.
		if !bytes.Equal(oldTriggeringPolicy, job.TriggeringPolicyRaw) {
			logging.Infof(c, "Job's triggering policy has changed, scheduling a triage")
			if err := e.kickTriageLater(c, job.JobID, 0); err != nil {
				return err
			}
		}

		// If the job was just enabled or its schedule changed, poke the cron
		// machine to potentially schedule a new tick.
		return pokeCron(c, job, e.cfg.Dispatcher, func(m *cron.Machine) error {
			if wasDisabled {
				m.Enable()
			}
			if job.EffectiveSchedule() != oldEffectiveSchedule {
				logging.Infof(c, "Job's schedule changed: %q -> %q", oldEffectiveSchedule, job.EffectiveSchedule())
				m.OnScheduleChange()
			}
			return nil
		})
	})
}

// disableJob moves a job to the disabled state.
func (e *engineImpl) disableJob(c context.Context, jobID string) error {
	return e.jobTxn(c, jobID, func(c context.Context, job *Job, isNew bool) error {
		if isNew || !job.Enabled {
			return errSkipPut
		}
		job.Enabled = false

		// Stop the cron machine ticks.
		err := pokeCron(c, job, e.cfg.Dispatcher, func(m *cron.Machine) error {
			m.Disable()
			return nil
		})
		if err != nil {
			return err
		}

		// Kick the triage to clear the pending triggers. We can pop them only
		// from within the triage procedure.
		return e.kickTriageLater(c, job.JobID, 0)
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
		err := pokeCron(c, job, e.cfg.Dispatcher, func(m *cron.Machine) error {
			m.Disable()
			m.Enable()
			return nil
		})
		if err != nil {
			return err
		}
		return e.kickTriageLater(c, job.JobID, 0)
	})
}

////////////////////////////////////////////////////////////////////////////////
// Invocations related methods.

// getInvocation returns an existing invocation or ErrNoSuchInvocation error.
//
// Double checks that the invocation belongs to the given job. Returns
// ErrNoSuchInvocation if not.
func (e *engineImpl) getInvocation(c context.Context, jobID string, invID int64) (*Invocation, error) {
	// ID 0 is special in datastore. There are no invocations with ID 0, but
	// making the ds.Get call below to check this would fail with "invalid key"
	// error.
	if invID == 0 {
		return nil, ErrNoSuchInvocation
	}
	inv := &Invocation{ID: invID}
	switch err := ds.Get(c, inv); {
	case err == nil:
		if inv.JobID != jobID {
			logging.Errorf(c,
				"Invocation %d is associated with job %q, not %q. Treating it as missing.",
				invID, inv.JobID, jobID)
			return nil, ErrNoSuchInvocation
		}
		return inv, nil
	case err == ds.ErrNoSuchEntity:
		return nil, ErrNoSuchInvocation
	default:
		return nil, transient.Tag.Apply(err)
	}
}

// enqueueInvocations allocated a bunch of Invocation entities, adds them to
// ActiveInvocations list of the job and enqueues a tq task that kicks off their
// execution.
//
// Must be called within a Job transaction, but creates Invocation entities
// outside the transaction (since they are in different entity groups). If the
// transaction fails, these entities may keep hanging unreferenced by anything
// as garbage. This is fine, since they are not discoverable by any queries.
func (e *engineImpl) enqueueInvocations(c context.Context, job *Job, req []task.Request) ([]*Invocation, error) {
	assertInTransaction(c)

	// Create N new Invocation entities in Starting state.
	invs, err := e.allocateInvocations(c, job, req)
	if err != nil {
		return nil, err
	}

	// Enqueue a task that eventually calls 'launchInvocationTask' for each new
	// invocation.
	invIDs := make([]int64, len(invs))
	for i, inv := range invs {
		invIDs[i] = inv.ID
	}
	if err := e.kickLaunchInvocationsBatchTask(c, job.JobID, invIDs); err != nil {
		cleanupUnreferencedInvocations(c, invs)
		return nil, err
	}

	// Make the job know that there are invocations pending. This will make them
	// show up in UI and API after the current transaction lands. If it doesn't
	// land, new invocations will remain hanging as garbage, not referenced by
	// anything.
	job.ActiveInvocations = append(job.ActiveInvocations, invIDs...)
	return invs, nil
}

// allocateInvocation creates new Invocation entity in a separate transaction.
func (e *engineImpl) allocateInvocation(c context.Context, job *Job, req task.Request) (*Invocation, error) {
	var inv *Invocation
	err := runIsolatedTxn(c, func(c context.Context) (err error) {
		inv, err = e.initInvocation(c, &Invocation{
			JobID:           job.JobID,
			RealmID:         job.RealmID,
			Started:         clock.Now(c).UTC(),
			Revision:        job.Revision,
			RevisionURL:     job.RevisionURL,
			Task:            job.Task,
			TriggeredJobIDs: job.TriggeredJobIDs,
			Status:          task.StatusStarting,
		}, &req)
		if err != nil {
			return
		}
		inv.debugLog(c, "New invocation is queued and will start shortly")
		if req.TriggeredBy != "" {
			inv.debugLog(c, "Triggered by %s", req.TriggeredBy)
		}
		return transient.Tag.Apply(ds.Put(c, inv))
	})
	if err != nil {
		return nil, err
	}
	return inv, nil
}

// allocateInvocations is a batch version of allocateInvocation.
//
// It launches N independent transactions in parallel to create N invocations.
func (e *engineImpl) allocateInvocations(c context.Context, job *Job, req []task.Request) ([]*Invocation, error) {
	wg := sync.WaitGroup{}
	wg.Add(len(req))

	invs := make([]*Invocation, len(req))
	merr := errors.NewLazyMultiError(len(req))
	for i := range req {
		go func(i int) {
			defer wg.Done()
			inv, err := e.allocateInvocation(c, job, req[i])
			invs[i] = inv
			merr.Assign(i, err)
			if err != nil {
				logging.WithError(err).Errorf(c, "Failed to create invocation with %d triggers", len(req[i].IncomingTriggers))
			}
		}(i)
	}

	wg.Wait()

	// Bail if any of them failed. Try best effort cleanup.
	if err := merr.Get(); err != nil {
		cleanupUnreferencedInvocations(c, invs)
		return nil, transient.Tag.Apply(err)
	}

	return invs, nil
}

// initInvocation populates fields of Invocation struct.
//
// It allocates invocation ID and populates inv.ID field. It also copies data
// from the given task.Request object into the corresponding fields of the
// invocation entity (so they can be indexed etc).
//
// On success returns exact same 'inv' for convenience. It doesn't Put it into
// the datastore.
//
// Must be called within a transaction, since it verifies an allocated ID is
// not used yet.
func (e *engineImpl) initInvocation(c context.Context, inv *Invocation, req *task.Request) (*Invocation, error) {
	assertInTransaction(c)

	var err error
	if inv.ID, err = generateInvocationID(c); err != nil {
		return nil, errors.Annotate(err, "failed to generate invocation ID").Err()
	}

	if req != nil {
		if err := putRequestIntoInv(inv, req); err != nil {
			return nil, errors.Annotate(err, "failed to serialize task request").Err()
		}
		if req.DebugLog != "" {
			inv.DebugLog += "Debug output from the triage procedure:\n"
			inv.DebugLog += "---------------------------------------\n"
			inv.DebugLog += req.DebugLog
			inv.DebugLog += "---------------------------------------\n\n"
			inv.trimDebugLog() // in case it is HUGE
			inv.DebugLog += "Debug output from the invocation itself:\n"
		}
	}
	return inv, nil
}

// abortInvocation marks some invocation as aborted.
func (e *engineImpl) abortInvocation(c context.Context, jobID string, invID int64) error {
	return e.withController(c, jobID, invID, "manual abort", func(c context.Context, ctl *taskController) error {
		ctl.DebugLog("Invocation is manually aborted by %s", auth.CurrentIdentity(c))

		switch err := ctl.manager.AbortTask(c, ctl); {
		case transient.Tag.In(err):
			return err // ask for retry on transient errors, don't touch Invocation
		case err != nil:
			ctl.DebugLog("Fatal error when aborting the invocation - %s", err)
		}

		// On success or on a fatal error mark the task as aborted (unless the
		// manager already switched the state). We can't do anything about the
		// failed abort attempt anyway.
		if !ctl.State().Status.Final() {
			ctl.State().Status = task.StatusAborted
		}
		return nil
	})
}

////////////////////////////////////////////////////////////////////////////////
// Task controller and invocation launch and finish.

const (
	// invocationRetryLimit is how many times to retry an invocation before giving
	// up and resuming the job's schedule.
	invocationRetryLimit = 50
)

var (
	// errRetryingLaunch is returned by launchTask if the task failed to start and
	// the launch attempt should be tried again.
	errRetryingLaunch = errors.New("task failed to start, retrying", transient.Tag)
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

// launchTask instantiates an invocation controller and calls its LaunchTask
// method, saving the invocation state when its done.
//
// It returns a transient error if the launch attempt should be retried.
func (e *engineImpl) launchTask(c context.Context, inv *Invocation) error {
	assertNotInTransaction(c)

	// Grab the corresponding TaskManager to launch the task through it.
	ctl, err := controllerForInvocation(c, e, inv)
	if err != nil {
		// Note: controllerForInvocation returns both ctl and err on errors, with
		// ctl not fully initialized (but good enough for what's done below).
		ctl.DebugLog("Failed to initialize task controller - %s", err)
		ctl.State().Status = task.StatusFailed
		return ctl.Save(c)
	}

	// Ask the manager to start the task. If it returns no errors, it should also
	// move the invocation out of an initial state (a failure to do so is a fatal
	// error). If it returns an error, the invocation is forcefully moved to
	// StatusRetrying or StatusFailed state (depending on whether the error is
	// transient or not and how many retries are left). In either case, invocation
	// never ends up in StatusStarting state.
	err = ctl.manager.LaunchTask(c, ctl)
	if err != nil {
		logging.WithError(err).Errorf(c, "Failed to LaunchTask")
	}
	if status := ctl.State().Status; status.Initial() && err == nil {
		err = fmt.Errorf("LaunchTask didn't move invocation out of initial %s state", status)
	}
	if transient.Tag.In(err) && inv.RetryCount+1 >= invocationRetryLimit {
		err = fmt.Errorf("Too many retries, giving up (original error - %s)", err)
	}

	// The task must always end up in a non-initial state. Do it on behalf of the
	// controller if necessary.
	if ctl.State().Status.Initial() {
		if transient.Tag.In(err) {
			// This invocation object will be reused for a retry later.
			ctl.State().Status = task.StatusRetrying
		} else {
			// The invocation has crashed with the fatal error.
			ctl.State().Status = task.StatusFailed
		}
	}

	// Add a notice into the invocation log that we'll attempt to retry.
	isRetrying := ctl.State().Status == task.StatusRetrying
	if isRetrying {
		ctl.DebugLog("The invocation will be retried")
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
	if isRetrying {
		return errRetryingLaunch
	}

	return nil
}

// invChanging is called within transactions that update Invocation entities (in
// particular taskController.Save()) right before they are committed.
//
// The engine examines changes to the invocation state (comparing 'old' and
// 'fresh'), looks at all emitted timers and triggers, and schedules a bunch
// of TQ tasks accordingly.
//
// It can mutate 'fresh', which should be later saved to the datastore by the
// caller.
func (e *engineImpl) invChanging(c context.Context, old, fresh *Invocation, timers []*internal.Timer, triggers []*internal.Trigger) error {
	assertInTransaction(c)

	if fresh.Status.Final() && len(timers) > 0 {
		panic("finished invocations must not emit timer, ensured by taskController")
	}

	// Register new timers in the Invocation entity. Used to reject duplicate
	// task queue calls: only tasks that reference a timer in the pending timers
	// set are accepted.
	if len(timers) != 0 {
		err := mutateTimersList(&fresh.PendingTimersRaw, func(out *[]*internal.Timer) {
			*out = append(*out, timers...)
		})
		if err != nil {
			return err
		}
	}

	// Register emitted triggers in the Invocation entity. Used mostly for UI.
	if len(triggers) != 0 {
		err := mutateTriggersList(&fresh.OutgoingTriggersRaw, func(out *[]*internal.Trigger) {
			*out = append(*out, triggers...)
		})
		if err != nil {
			return err
		}
	}

	// Prepare FanOutTriggersTask if we are emitting triggers for real. Skip this
	// if no job is going to get them.
	var fanOutTriggersTask *internal.FanOutTriggersTask
	if len(triggers) != 0 && len(fresh.TriggeredJobIDs) != 0 {
		fanOutTriggersTask = &internal.FanOutTriggersTask{
			JobIds:   fresh.TriggeredJobIDs,
			Triggers: triggers,
		}
	}

	var tasks []*tq.Task

	if !old.Status.Final() && fresh.Status.Final() {
		// When invocation finishes, make it appear in the list of finished
		// invocations (by setting the indexed field), and notify the parent job
		// about the completion, so it can kick off a new one or otherwise react.
		// Note that we can't open Job transaction here and have to use a task queue
		// task. Bundle fanOutTriggersTask with this task, since we can. No need to
		// create two separate tasks.
		fresh.IndexedJobID = fresh.JobID
		tasks = append(tasks, &tq.Task{
			Payload: &internal.InvocationFinishedTask{
				JobId:    fresh.JobID,
				InvId:    fresh.ID,
				Triggers: fanOutTriggersTask,
			},
		})
	} else if fanOutTriggersTask != nil {
		tasks = append(tasks, &tq.Task{Payload: fanOutTriggersTask})
	}

	// When emitting more than 1 timer (this is rare) use an intermediary task,
	// to avoid getting close to limit of number of tasks in a transaction. When
	// emitting 1 timer (most common case), don't bother, since we aren't winning
	// anything.
	switch {
	case len(timers) == 1:
		tasks = append(tasks, &tq.Task{
			ETA: timers[0].Eta.AsTime(),
			Payload: &internal.TimerTask{
				JobId: fresh.JobID,
				InvId: fresh.ID,
				Timer: timers[0],
			},
		})
	case len(timers) > 1:
		tasks = append(tasks, &tq.Task{
			Payload: &internal.ScheduleTimersTask{
				JobId:  fresh.JobID,
				InvId:  fresh.ID,
				Timers: timers,
			},
		})
	}

	return e.cfg.Dispatcher.AddTask(c, tasks...)
}

////////////////////////////////////////////////////////////////////////////////
// Task queue handlers.

// kickLaunchInvocationsBatchTask enqueues LaunchInvocationsBatchTask that
// eventually launches new invocations.
func (e *engineImpl) kickLaunchInvocationsBatchTask(c context.Context, jobID string, invIDs []int64) error {
	payload := &internal.LaunchInvocationsBatchTask{
		Tasks: make([]*internal.LaunchInvocationTask, 0, len(invIDs)),
	}
	for _, invID := range invIDs {
		payload.Tasks = append(payload.Tasks, &internal.LaunchInvocationTask{
			JobId: jobID,
			InvId: invID,
		})
	}
	return e.cfg.Dispatcher.AddTask(c, &tq.Task{
		Payload: payload,
		Delay:   time.Second, // give some time to land Invocation transactions
	})
}

// execLaunchInvocationsBatchTask handles LaunchInvocationsBatchTask by fanning
// out the tasks.
//
// It is the entry point into starting new invocations. Even if the batch
// contains only one task, it still MUST come through LaunchInvocationsBatchTask
// since this is where we "gate" all launches (for example, we can pause the
// corresponding GAE task queue to shutdown new launches during an emergency).
func (e *engineImpl) execLaunchInvocationsBatchTask(c context.Context, tqTask proto.Message) error {
	batch := tqTask.(*internal.LaunchInvocationsBatchTask)

	tasks := []*tq.Task{}
	for _, subtask := range batch.Tasks {
		tasks = append(tasks, &tq.Task{
			DeduplicationKey: fmt.Sprintf("inv:%s:%d", subtask.JobId, subtask.InvId),
			Payload:          subtask,
		})
	}

	return e.cfg.Dispatcher.AddTask(c, tasks...)
}

// execLaunchInvocationTask handles LaunchInvocationTask.
//
// It can be redelivered a bunch of times in case the invocation fails to start.
func (e *engineImpl) execLaunchInvocationTask(c context.Context, tqTask proto.Message) error {
	msg := tqTask.(*internal.LaunchInvocationTask)

	c = logging.SetField(c, "JobID", msg.JobId)
	c = logging.SetField(c, "InvID", msg.InvId)

	hdrs, err := tq.RequestHeaders(c)
	if err != nil {
		return err
	}
	retryCount := hdrs.TaskExecutionCount // 0 for the first attempt
	if retryCount != 0 {
		logging.Warningf(c, "This is a retry (attempt %d)!", retryCount+1)
	}

	// Fetch up-to-date state of the invocation, verify we still need to start it.
	// Log that we are about to do it. We MUST write something to the datastore
	// before attempting the launch to make sure that if the datastore is in read
	// only mode (that happens), we don't spam LaunchTask retries when failing to
	// Save() the state in the end (better to fail now, before LaunchTask call).
	var skipLaunch bool
	var lastInvState Invocation
	logging.Infof(c, "Opening the invocation transaction")
	err = runTxn(c, func(c context.Context) error {
		skipLaunch = false // reset in case the transaction is retried

		// Grab up-to-date invocation state.
		inv := Invocation{ID: msg.InvId}
		switch err := ds.Get(c, &inv); {
		case err == ds.ErrNoSuchEntity:
			// This generally should not happen.
			logging.Warningf(c, "The invocation is unexpectedly gone")
			skipLaunch = true
			return nil
		case err != nil:
			return transient.Tag.Apply(err)
		case !inv.Status.Initial():
			logging.Warningf(c, "The invocation is already running or finished: %s", inv.Status)
			skipLaunch = true
			return nil
		}

		// The invocation is still starting or being retried now. Update its state
		// to indicate we are about to work with it. 'lastInvState' is later passed
		// to the task controller.
		lastInvState = inv
		lastInvState.RetryCount = retryCount
		lastInvState.MutationsCount++
		if retryCount >= invocationRetryLimit {
			logging.Errorf(c, "Too many attempts, giving up")
			lastInvState.debugLog(c, "Too many attempts, giving up")
			lastInvState.Status = task.StatusFailed
			lastInvState.Finished = clock.Now(c).UTC()
			skipLaunch = true
		} else {
			lastInvState.debugLog(c, "Starting the invocation (attempt %d)", retryCount+1)
		}

		// Make sure to trigger all necessary side effects, particularly important
		// if the invocation was moved to Failed state above.
		if err := e.invChanging(c, &inv, &lastInvState, nil, nil); err != nil {
			return err
		}

		// Store the updated invocation.
		lastInvState.trimDebugLog()
		return transient.Tag.Apply(ds.Put(c, &lastInvState))
	})

	switch {
	case err != nil:
		logging.WithError(err).Errorf(c, "Failed to update the invocation")
		return err
	case skipLaunch:
		logging.Warningf(c, "No need to start the invocation anymore")
		return nil
	}

	logging.Infof(c, "Actually launching the task")
	return e.launchTask(c, &lastInvState)
}

// execInvocationFinishedTask handles invocation completion notification.
//
// It is emitted by invChanging() when an invocation switches into a final
// state.
//
// It adds the invocation ID to the set of recently finished invocations and
// kicks off a job triage task that eventually updates Job.ActiveInvocations set
// and moves the cron state machine.
//
// Note that we can't just open a Job transaction right here, since the rate
// of "invocation finished" events is not controllable and can easily be over
// 1 QPS datastore limit, overwhelming the Job entity group.
//
// If the invocation emitted some triggers when it was finishing, we route them
// here as well.
func (e *engineImpl) execInvocationFinishedTask(c context.Context, tqTask proto.Message) error {
	msg := tqTask.(*internal.InvocationFinishedTask)

	c = logging.SetField(c, "JobID", msg.JobId)
	c = logging.SetField(c, "InvID", msg.InvId)

	if err := recentlyFinishedSet(c, msg.JobId).Add(c, []int64{msg.InvId}); err != nil {
		logging.WithError(err).Errorf(c, "Failed to update recently finished invocations set")
		return err
	}

	// Kick the triage task and fan out the emitted triggers in parallel. Retry
	// the whole thing if any of these operations fail. Everything that happens in
	// this handler is idempotent (including recentlyFinishedSet modification
	// above).

	wg := sync.WaitGroup{}
	errs := errors.MultiError{nil, nil}

	wg.Add(1)
	go func() {
		defer wg.Done()
		if errs[0] = e.kickTriageNow(c, msg.JobId); errs[0] != nil {
			logging.WithError(errs[0]).Errorf(c, "Failed to kick job triage task")
		}
	}()

	if msg.Triggers != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if errs[1] = e.execFanOutTriggersTask(c, msg.Triggers); errs[1] != nil {
				logging.WithError(errs[1]).Errorf(c, "Failed to fan out triggers")
			}
		}()
	}

	wg.Wait()

	if errs.First() != nil {
		return transient.Tag.Apply(errs)
	}

	return nil
}

////////////////////////////////////////////////////////////////////////////////
// Triggers handling.

// execFanOutTriggersTask handles a batch enqueue of triggers.
//
// It is enqueued transactionally by the invocation, and results in a bunch of
// non-transactional EnqueueTriggersTask tasks.
func (e *engineImpl) execFanOutTriggersTask(c context.Context, tqTask proto.Message) error {
	msg := tqTask.(*internal.FanOutTriggersTask)

	tasks := make([]*tq.Task, len(msg.JobIds))
	for i, jobID := range msg.JobIds {
		tasks[i] = &tq.Task{
			Payload: &internal.EnqueueTriggersTask{
				JobId:    jobID,
				Triggers: msg.Triggers,
			},
		}
	}

	return e.cfg.Dispatcher.AddTask(c, tasks...)
}

// execEnqueueTriggersTask adds a bunch of triggers to job's pending triggers
// set and kicks the triage process to process them.
//
// Note: it is invoked through TQ, and also directly from EmitTriggers RPC
// handler.
func (e *engineImpl) execEnqueueTriggersTask(c context.Context, tqTask proto.Message) error {
	msg := tqTask.(*internal.EnqueueTriggersTask)

	c = logging.SetField(c, "JobID", msg.JobId)

	logTriggers := func() {
		for _, t := range msg.Triggers {
			logging.Infof(c, "  %s (emitted by %q, inv %d)", t.Id, t.JobId, t.InvocationId)
		}
	}

	// Don't even bother if the job is paused or disabled. Note that if the job
	// became inactive after this check, the triage will get rid of pending
	// triggers itself. Thus the check here is just an optimization.
	job, err := e.getJob(c, msg.JobId)
	if err != nil {
		logging.WithError(err).Errorf(c, "Failed to grab Job entity")
		return err // transient error getting the job
	}
	if job == nil || !job.Enabled || job.Paused {
		logging.Warningf(c, "Discarding the following triggers since the job is inactive")
		logTriggers()
		return nil
	}

	logging.Infof(c, "Adding the following triggers to the pending triggers set")
	logTriggers()
	if err := pendingTriggersSet(c, msg.JobId).Add(c, msg.Triggers); err != nil {
		logging.WithError(err).Errorf(c, "Failed to update pending triggers set")
		return err
	}

	return e.kickTriageNow(c, msg.JobId)
}

////////////////////////////////////////////////////////////////////////////////
// Timers handling.

// execScheduleTimersTask adds a bunch of TimerTask tasks.
//
// It is emitted by Invocation transaction when it wants to schedule multiple
// timers.
func (e *engineImpl) execScheduleTimersTask(c context.Context, tqTask proto.Message) error {
	msg := tqTask.(*internal.ScheduleTimersTask)

	tasks := make([]*tq.Task, len(msg.Timers))
	for i, timer := range msg.Timers {
		tasks[i] = &tq.Task{
			ETA: timer.Eta.AsTime(),
			Payload: &internal.TimerTask{
				JobId: msg.JobId,
				InvId: msg.InvId,
				Timer: timer,
			},
		}
	}

	return e.cfg.Dispatcher.AddTask(c, tasks...)
}

// execTimerTask corresponds to a tick of a timer added via AddTimer.
func (e *engineImpl) execTimerTask(c context.Context, tqTask proto.Message) error {
	msg := tqTask.(*internal.TimerTask)
	timer := msg.Timer
	action := fmt.Sprintf("timer %q (%s)", timer.Title, timer.Id)

	return e.withController(c, msg.JobId, msg.InvId, action, func(c context.Context, ctl *taskController) error {
		// Pop the timer from the pending set, if it is still there. Return a fatal
		// error if it isn't to stop this task from being redelivered.
		switch consumed, err := ctl.consumeTimer(timer.Id); {
		case err != nil:
			return err
		case !consumed:
			return fmt.Errorf("no such timer: %s", timer.Id)
		}

		// Let the task manager handle the timer. It may add new timers.
		ctl.DebugLog("Handling timer %q (%s)", timer.Title, timer.Id)
		err := ctl.manager.HandleTimer(c, ctl, timer.Title, timer.Payload)
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

		// Need to save the invocation, even on fatal errors (to indicate that the
		// timer has been consumed). So return nil.
		return nil
	})
}

////////////////////////////////////////////////////////////////////////////////
// Cron handling.

// execCronTickTask corresponds to a delayed tick emitted by a cron state
// machine.
func (e *engineImpl) execCronTickTask(c context.Context, tqTask proto.Message) error {
	msg := tqTask.(*internal.CronTickTask)
	return e.jobTxn(c, msg.JobId, func(c context.Context, job *Job, isNew bool) error {
		if isNew {
			logging.Errorf(c, "Scheduled job is unexpectedly gone")
			return errSkipPut
		}
		logging.Infof(c, "Tick %d has arrived", msg.TickNonce)
		return pokeCron(c, job, e.cfg.Dispatcher, func(m *cron.Machine) error {
			// OnTimerTick returns an error if the tick happened too soon. Mark this
			// error as transient to trigger task queue retry at a later time.
			return transient.Tag.Apply(m.OnTimerTick(msg.TickNonce))
		})
	})
}

////////////////////////////////////////////////////////////////////////////////
// Triage procedure.

// kickTriageNow enqueues a task to perform a triage for some job, if no such
// task was enqueued recently.
//
// Does it even if the job no longer exists or has been disabled. Such triage
// will just be skipped later.
//
// Uses named tasks and memcache internally, thus can't be part of a
// transaction. If you want to kick the triage transactionally, use
// kickTriageLater().
func (e *engineImpl) kickTriageNow(c context.Context, jobID string) error {
	assertNotInTransaction(c)

	c = logging.SetField(c, "JobID", jobID)

	// Throttle to once per 2 sec (and make sure it is always in the future).
	eta := clock.Now(c).Unix()
	eta = (eta/2 + 1) * 2
	dedupKey := fmt.Sprintf("triage:%s:%d", jobID, eta)

	// Use cheaper but crappier memcache as a first dedup check.
	itm := memcache.NewItem(c, dedupKey).SetExpiration(time.Minute)
	if memcache.Get(c, itm) == nil {
		logging.Infof(c, "The triage task has already been scheduled")
		return nil
	}

	// Enqueue the triage task, if not already there. This is rock solid, but slow
	// second dedup check.
	err := e.cfg.Dispatcher.AddTask(c, &tq.Task{
		DeduplicationKey: dedupKey,
		ETA:              time.Unix(eta, 0),
		Payload:          &internal.TriageJobStateTask{JobId: jobID},
	})
	if err != nil {
		return err
	}
	logging.Infof(c, "Scheduled the triage task")

	// Best effort in setting dedup memcache flag. No big deal if it fails.
	if err := memcache.Set(c, itm); err != nil {
		logging.WithError(err).Warningf(c, "Failed to set memcache triage flag")
	}

	return nil
}

// kickTriageLater schedules a triage to be kicked later.
//
// Unlike kickTriageNow, this just posts a single TQ task, and thus can be
// used inside transactions.
func (e *engineImpl) kickTriageLater(c context.Context, jobID string, delay time.Duration) error {
	c = logging.SetField(c, "JobID", jobID)
	return e.cfg.Dispatcher.AddTask(c, &tq.Task{
		Payload: &internal.KickTriageTask{JobId: jobID},
		Delay:   delay,
	})
}

// execKickTriageTask handles delayed KickTriageTask by scheduling a triage.
func (e *engineImpl) execKickTriageTask(c context.Context, tqTask proto.Message) error {
	return e.kickTriageNow(c, tqTask.(*internal.KickTriageTask).JobId)
}

// execTriageJobStateTask performs the triage of a job.
//
// It is throttled to run at most once per 2 seconds.
//
// It looks at pending triggers and recently finished invocations and launches
// new invocations (or schedules timers to do it later).
func (e *engineImpl) execTriageJobStateTask(c context.Context, tqTask proto.Message) (err error) {
	jobID := tqTask.(*internal.TriageJobStateTask).JobId

	c = logging.SetField(c, "JobID", jobID)

	op := triageOp{
		jobID:              jobID,
		dispatcher:         e.cfg.Dispatcher,
		policyFactory:      policy.New,
		maxAllowedTriggers: 1000, // experimentally derived number
		enqueueInvocations: func(c context.Context, job *Job, req []task.Request) error {
			_, err := e.enqueueInvocations(c, job, req)
			return err
		},
	}

	// Store the triage log no matter what (even if 'prepare' fails or the
	// transaction fails to land). We want to surface these conditions if they
	// happen consistently.
	defer func() { op.finalize(c, err == nil) }()

	if err = op.prepare(c); err != nil {
		return err
	}

	err = e.jobTxn(c, jobID, func(c context.Context, job *Job, isNew bool) error {
		if isNew {
			logging.Warningf(c, "The job is unexpectedly gone")
			return errSkipPut
		}
		return op.transaction(c, job)
	})
	if err != nil {
		op.debugErrLog(c, err, "The triage transaction FAILED")
	}
	return err
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
func (e *engineImpl) handlePubSubMessage(c context.Context, manager string, msg *pubsub.PubsubMessage) error {
	logging.Infof(c, "Received PubSub message %q for %q", msg.MessageId, manager)

	// Ask the manager to extract the auth token from the message for us. Its
	// location within the message is an implementation detail of the particular
	// task manager.
	mgr := e.cfg.Catalog.GetTaskManagerByName(manager)
	if mgr == nil {
		return errors.Reason("unknown task manager %q", manager).Err()
	}
	authToken := mgr.ExamineNotification(c, msg)
	if authToken == "" {
		return errors.Reason("failed to extract the auth token from the pubsub message").Err()
	}

	// Validate authToken and extract Job and Invocation IDs from it.
	var jobID string
	var invID int64
	data, err := pubsubAuthToken.Validate(c, authToken, nil)
	if err != nil {
		logging.WithError(err).Errorf(c, "Bad PubSub auth token")
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
		case transient.Tag.In(err) || tq.Retry.In(err):
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
	id := fmt.Sprintf("%s.%s.%s",
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
// task.
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
		pushURL = fmt.Sprintf(
			"https://%s%s?%s",
			info.DefaultVersionHostname(c),
			e.cfg.PubSubPushPath,
			e.pushSubscriptionURLValues(params.manager, params.publisher).Encode(),
		)
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

// pushSubscriptionURLValues returns "?=..." params for a PubSub push URL.
func (e *engineImpl) pushSubscriptionURLValues(mgr task.Manager, publisher string) url.Values {
	return url.Values{
		"kind":      []string{mgr.Name()},
		"publisher": []string{publisher},
	}
}
