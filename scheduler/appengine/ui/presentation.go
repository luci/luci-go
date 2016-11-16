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

	"github.com/luci/luci-go/scheduler/appengine/catalog"
	"github.com/luci/luci-go/scheduler/appengine/engine"
	"github.com/luci/luci-go/scheduler/appengine/messages"
	"github.com/luci/luci-go/scheduler/appengine/schedule"
	"github.com/luci/luci-go/scheduler/appengine/task"
)

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

	sortKey []string
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

func makeJob(j *engine.Job, now time.Time) *schedulerJob {
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
		// Retries happen when invocation fails to launch (move from STARTING to
		// RUNNING state). Such invocation is retried (as new invocation). When
		// a retry is enqueued, we display the job state as "RETRYING" (even though
		// technically it is still "QUEUED").
		state = "RETRYING"
		labelClass = "label-danger"
	case j.State.State == engine.JobStateQueued:
		// An invocation has been added to the task queue, but didn't move to
		// RUNNING state yet.
		state = "STARTING"
		labelClass = "label-default"
	case j.State.State == engine.JobStateSlowQueue:
		// Job invocation is still in the task queue, but new invocation should be
		// starting now (so the queue is lagging for some reason).
		state = "STARTING"
		labelClass = "label-warning"
	case j.Paused && j.State.State == engine.JobStateSuspended:
		// Paused jobs don't have a schedule, so they are always in SUSPENDED state.
		// Make it clearer that they are just paused. This applies to both triggered
		// and periodic jobs.
		state = "PAUSED"
		labelClass = "label-default"
	case j.State.State == engine.JobStateSuspended && j.Flavor == catalog.JobFlavorTriggered:
		// Triggered jobs don't run on a schedule. They are in SUSPENDED state
		// between triggering events, rename it to WAITING for clarity.
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

		sortKey: []string{chunks[0], sortGroup, chunks[1]},
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
	if len(s[i].sortKey) < len(s[j].sortKey) {
		return true
	}
	if len(s[i].sortKey) > len(s[j].sortKey) {
		return false
	}
	for idx := range s[i].sortKey {
		if s[i].sortKey[idx] < s[j].sortKey[idx] {
			return true
		}
	}
	return false
}

func convertToSortedJobs(jobs []*engine.Job, now time.Time) sortedJobs {
	out := make(sortedJobs, len(jobs))
	for i, job := range jobs {
		out[i] = makeJob(job, now)
	}
	sort.Sort(out)
	return out
}

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

func makeInvocation(projecID, jobID string, i *engine.Invocation, now time.Time) *invocation {
	triggeredBy := "-"
	if i.TriggeredBy != "" {
		triggeredBy = string(i.TriggeredBy)
		if i.TriggeredBy.Email() != "" {
			triggeredBy = i.TriggeredBy.Email() // triggered by a user (not a service)
		}
	}
	finished := i.Finished
	if finished.IsZero() {
		finished = now
	}
	duration := humanize.RelTime(i.Started, finished, "", "")
	if duration == "now" {
		duration = "1 second" // "now" looks weird for durations
	}
	return &invocation{
		ProjectID:   projecID,
		JobID:       jobID,
		InvID:       i.ID,
		Attempt:     i.RetryCount + 1,
		Revision:    i.Revision,
		RevisionURL: i.RevisionURL,
		Definition:  taskToText(i.Task),
		TriggeredBy: triggeredBy,
		Started:     humanize.RelTime(i.Started, now, "ago", "from now"),
		Duration:    duration,
		Status:      string(i.Status),
		DebugLog:    i.DebugLog,
		RowClass:    statusToRowClass[i.Status],
		LabelClass:  statusToLabelClass[i.Status],
		ViewURL:     i.ViewURL,
	}
}

func convertToInvocations(projectID, jobID string, invs []*engine.Invocation, now time.Time) []*invocation {
	out := make([]*invocation, len(invs))
	for i, inv := range invs {
		out[i] = makeInvocation(projectID, jobID, inv, now)
	}
	return out
}
