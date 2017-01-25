// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package ui

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/logging"

	"github.com/luci/luci-go/common/data/sortby"
	"github.com/luci/luci-go/scheduler/appengine/catalog"
	"github.com/luci/luci-go/scheduler/appengine/engine"
	"github.com/luci/luci-go/scheduler/appengine/messages"
	"github.com/luci/luci-go/scheduler/appengine/schedule"
	"github.com/luci/luci-go/scheduler/appengine/task"
)

// schedulerJob is UI representation of engine.Job entity.
type schedulerJob struct {
	ProjectID      string
	JobID          string
	Schedule       string
	Definition     string
	Revision       string
	RevisionURL    string
	State          string
	Overruns       int
	NextRun        string
	Paused         bool
	LabelClass     string
	JobFlavorIcon  string
	JobFlavorTitle string

	sortGroup string      // used only for sorting, doesn't show up in UI
	now       time.Time   // as passed to makeJob
	traits    task.Traits // as extracted from corresponding task.Manager
}

var stateToLabelClass = map[engine.StateKind]string{
	engine.JobStateDisabled:  "label-default",
	engine.JobStateScheduled: "label-primary",
	engine.JobStateSuspended: "label-default",
	engine.JobStateRunning:   "label-info",
	engine.JobStateOverrun:   "label-warning",
}

var flavorToIconClass = []string{
	catalog.JobFlavorPeriodic:  "glyphicon-time",
	catalog.JobFlavorTriggered: "glyphicon-flash",
	catalog.JobFlavorTrigger:   "glyphicon-bell",
}

var flavorToTitle = []string{
	catalog.JobFlavorPeriodic:  "Periodic job",
	catalog.JobFlavorTriggered: "Triggered job",
	catalog.JobFlavorTrigger:   "Triggering job",
}

// makeJob builds UI presentation for engine.Job.
func makeJob(c context.Context, j *engine.Job) *schedulerJob {
	// Grab the task.Manager that's responsible for handling this job and
	// query if for the list of traits applying to this job.
	var manager task.Manager
	cat := config(c).Catalog
	taskDef, err := cat.UnmarshalTask(j.Task)
	if err != nil {
		logging.WithError(err).Warningf(c, "Failed to unmarshal task proto for %s", j.JobID)
	} else {
		manager = cat.GetTaskManager(taskDef)
	}
	traits := task.Traits{}
	if manager != nil {
		traits = manager.Traits()
	}

	now := clock.Now(c).UTC()
	nextRun := ""
	switch ts := j.State.TickTime; {
	case ts == schedule.DistantFuture:
		nextRun = "-"
	case !ts.IsZero():
		nextRun = humanize.RelTime(ts, now, "ago", "from now")
	default:
		nextRun = "not scheduled yet"
	}

	// Internal state names aren't very user friendly. Introduce some aliases.
	state := ""
	labelClass := ""
	switch {
	case j.State.IsRetrying():
		// Retries happen when invocation fails to launch (move from "STARTING" to
		// "RUNNING" state). Such invocation is retried (as new invocation). When
		// a retry is enqueued, we display the job state as "RETRYING" (even though
		// technically it is still "QUEUED").
		state = "RETRYING"
		labelClass = "label-danger"
	case !traits.Multistage && j.State.InvocationID != 0:
		// The job has an active invocation and the engine has called LaunchTask for
		// it already. Jobs with Multistage == false trait do all their work in
		// LaunchTask, so we display them as "RUNNING" (instead of "STARTING").
		state = "RUNNING"
		labelClass = "label-info"
	case j.State.State == engine.JobStateQueued:
		// An invocation has been added to the task queue, and the engine hasn't
		// attempted to launch it yet.
		state = "STARTING"
		labelClass = "label-default"
	case j.State.State == engine.JobStateSlowQueue:
		// Job invocation is still in the task queue, but new invocation should be
		// starting now (so the queue is lagging for some reason).
		state = "STARTING"
		labelClass = "label-warning"
	case j.Paused && j.State.State == engine.JobStateSuspended:
		// Paused jobs don't have a schedule, so they are always in "SUSPENDED"
		// state. Make it clearer that they are just paused. This applies to both
		// triggered and periodic jobs.
		state = "PAUSED"
		labelClass = "label-default"
	case j.State.State == engine.JobStateSuspended && j.Flavor == catalog.JobFlavorTriggered:
		// Triggered jobs don't run on a schedule. They are in "SUSPENDED" state
		// between triggering events, rename it to "WAITING" for clarity.
		state = "WAITING"
		labelClass = "label-default"
	default:
		state = string(j.State.State)
		labelClass = stateToLabelClass[j.State.State]
	}

	// Put triggers after regular jobs.
	sortGroup := "A"
	if j.Flavor == catalog.JobFlavorTrigger {
		sortGroup = "B"
	}

	// JobID has form <project>/<id>. Split it into components.
	chunks := strings.Split(j.JobID, "/")

	return &schedulerJob{
		ProjectID:      chunks[0],
		JobID:          chunks[1],
		Schedule:       j.Schedule,
		Definition:     taskToText(j.Task),
		Revision:       j.Revision,
		RevisionURL:    j.RevisionURL,
		State:          state,
		Overruns:       j.State.Overruns,
		NextRun:        nextRun,
		Paused:         j.Paused,
		LabelClass:     labelClass,
		JobFlavorIcon:  flavorToIconClass[j.Flavor],
		JobFlavorTitle: flavorToTitle[j.Flavor],

		sortGroup: sortGroup,
		now:       now,
		traits:    traits,
	}
}

func taskToText(task []byte) string {
	if len(task) == 0 {
		return ""
	}
	msg := messages.TaskDefWrapper{}
	if err := proto.Unmarshal(task, &msg); err != nil {
		return fmt.Sprintf("Failed to unmarshal the task - %s", err)
	}
	buf := bytes.Buffer{}
	if err := proto.MarshalText(&buf, &msg); err != nil {
		return fmt.Sprintf("Failed to marshal the task - %s", err)
	}
	return buf.String()
}

type sortedJobs []*schedulerJob

func (s sortedJobs) Len() int      { return len(s) }
func (s sortedJobs) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s sortedJobs) Less(i, j int) bool {
	return sortby.Chain{
		func(i, j int) bool { return s[i].ProjectID < s[j].ProjectID },
		func(i, j int) bool { return s[i].sortGroup < s[j].sortGroup },
		func(i, j int) bool { return s[i].JobID < s[j].JobID },
	}.Use(i, j)
}

// sortJobs instantiate a bunch of schedulerJob objects and sorts them in
// display order.
func sortJobs(c context.Context, jobs []*engine.Job) sortedJobs {
	out := make(sortedJobs, len(jobs))
	for i, job := range jobs {
		out[i] = makeJob(c, job)
	}
	sort.Sort(out)
	return out
}

// invocation is UI representation of engine.Invocation entity.
type invocation struct {
	ProjectID   string
	JobID       string
	InvID       int64
	Attempt     int64
	Revision    string
	RevisionURL string
	Definition  string
	TriggeredBy string
	Started     string
	Duration    string
	Status      string
	DebugLog    string
	RowClass    string
	LabelClass  string
	ViewURL     string
}

var statusToRowClass = map[task.Status]string{
	task.StatusStarting:  "active",
	task.StatusRunning:   "info",
	task.StatusSucceeded: "success",
	task.StatusFailed:    "danger",
	task.StatusOverrun:   "warning",
	task.StatusAborted:   "danger",
}

var statusToLabelClass = map[task.Status]string{
	task.StatusStarting:  "label-default",
	task.StatusRunning:   "label-info",
	task.StatusSucceeded: "label-success",
	task.StatusFailed:    "label-danger",
	task.StatusOverrun:   "label-warning",
	task.StatusAborted:   "label-danger",
}

// makeInvocation builds UI presentation of some Invocation of a job.
func makeInvocation(j *schedulerJob, i *engine.Invocation) *invocation {
	// Invocations with Multistage == false trait are never in "RUNNING" state,
	// they perform all their work in 'LaunchTask' while technically being in
	// "STARTING" state. We display them as "RUNNING" instead. See comment for
	// task.Traits.Multistage for more info.
	status := i.Status
	if !j.traits.Multistage && status == task.StatusStarting {
		status = task.StatusRunning
	}

	triggeredBy := "-"
	if i.TriggeredBy != "" {
		triggeredBy = string(i.TriggeredBy)
		if i.TriggeredBy.Email() != "" {
			triggeredBy = i.TriggeredBy.Email() // triggered by a user (not a service)
		}
	}

	finished := i.Finished
	if finished.IsZero() {
		finished = j.now
	}
	duration := humanize.RelTime(i.Started, finished, "", "")
	if duration == "now" {
		duration = "1 second" // "now" looks weird for durations
	}

	return &invocation{
		ProjectID:   j.ProjectID,
		JobID:       j.JobID,
		InvID:       i.ID,
		Attempt:     i.RetryCount + 1,
		Revision:    i.Revision,
		RevisionURL: i.RevisionURL,
		Definition:  taskToText(i.Task),
		TriggeredBy: triggeredBy,
		Started:     humanize.RelTime(i.Started, j.now, "ago", "from now"),
		Duration:    duration,
		Status:      string(status),
		DebugLog:    i.DebugLog,
		RowClass:    statusToRowClass[status],
		LabelClass:  statusToLabelClass[status],
		ViewURL:     i.ViewURL,
	}
}
