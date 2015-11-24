// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/golang/protobuf/proto"
	"github.com/gorhill/cronexpr"

	"github.com/luci/luci-go/appengine/cmd/cron/engine"
	"github.com/luci/luci-go/appengine/cmd/cron/messages"
	"github.com/luci/luci-go/appengine/cmd/cron/task"
)

type cronJob struct {
	ProjectID  string
	JobID      string
	Schedule   string
	Definition string
	Revision   string
	State      string
	Overruns   int
	NextRun    string

	sortKey string
}

func makeCronJob(j *engine.CronJob, now time.Time) *cronJob {
	nextRun := ""
	expr, err := cronexpr.Parse(j.Schedule)
	if err == nil {
		nextRun = humanize.RelTime(expr.Next(now), now, "ago", "from now")
	} else {
		nextRun = fmt.Sprintf("bad schedule %q", j.Schedule)
	}

	// JobID has form <project>/<id>. Split it into components.
	chunks := strings.Split(j.JobID, "/")

	return &cronJob{
		ProjectID:  chunks[0],
		JobID:      chunks[1],
		Schedule:   j.Schedule,
		Definition: taskToText(j.Task),
		Revision:   j.Revision,
		State:      string(j.State.State),
		Overruns:   j.State.Overruns,
		NextRun:    nextRun,

		sortKey: j.JobID,
	}
}

func taskToText(task []byte) string {
	msg := messages.Task{}
	if err := proto.Unmarshal(task, &msg); err != nil {
		return fmt.Sprintf("Failed to unmarshal the task - %s", err)
	}
	buf := bytes.Buffer{}
	if err := proto.MarshalText(&buf, &msg); err != nil {
		return fmt.Sprintf("Failed to marshal the task - %s", err)
	}
	return buf.String()
}

type sortedCronJobs []*cronJob

func (s sortedCronJobs) Len() int           { return len(s) }
func (s sortedCronJobs) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s sortedCronJobs) Less(i, j int) bool { return s[i].sortKey < s[j].sortKey }

func convertToSortedCronJobs(jobs []*engine.CronJob, now time.Time) sortedCronJobs {
	out := make(sortedCronJobs, len(jobs))
	for i, job := range jobs {
		out[i] = makeCronJob(job, now)
	}
	sort.Sort(out)
	return out
}

type invocation struct {
	InvID      int64
	Attempt    int64
	Revision   string
	Definition string
	Started    string
	Duration   string
	Status     string
	DebugLog   string
	RowClass   string
}

var statusToRowClass = map[task.Status]string{
	task.StatusStarting:  "active",
	task.StatusRunning:   "warning",
	task.StatusSucceeded: "success",
	task.StatusFailed:    "danger",
}

func makeInvocation(i *engine.Invocation, now time.Time) *invocation {
	finished := i.Finished
	if finished.IsZero() {
		finished = now
	}
	duration := humanize.RelTime(i.Started, finished, "", "")
	if duration == "now" {
		duration = "1 second" // "now" looks weird for durations
	}
	return &invocation{
		InvID:      i.ID,
		Attempt:    i.RetryCount + 1,
		Revision:   i.Revision,
		Definition: taskToText(i.Task),
		Started:    humanize.RelTime(i.Started, now, "ago", "from now"),
		Duration:   duration,
		Status:     string(i.Status),
		DebugLog:   i.DebugLog,
		RowClass:   statusToRowClass[i.Status],
	}
}

func convertToInvocations(invs []*engine.Invocation, now time.Time) []*invocation {
	out := make([]*invocation, len(invs))
	for i, inv := range invs {
		out[i] = makeInvocation(inv, now)
	}
	return out
}
