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

package ui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	structpb "github.com/golang/protobuf/ptypes/struct"
	"golang.org/x/net/context"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/sortby"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/google"

	"go.chromium.org/luci/scheduler/appengine/catalog"
	"go.chromium.org/luci/scheduler/appengine/engine"
	"go.chromium.org/luci/scheduler/appengine/internal"
	"go.chromium.org/luci/scheduler/appengine/messages"
	"go.chromium.org/luci/scheduler/appengine/presentation"
	"go.chromium.org/luci/scheduler/appengine/schedule"
	"go.chromium.org/luci/scheduler/appengine/task"
)

// schedulerJob is UI representation of engine.Job entity.
type schedulerJob struct {
	ProjectID      string
	JobName        string
	IsV2           bool
	Schedule       string
	Definition     string
	Revision       string
	RevisionURL    string
	State          string
	NextRun        string
	Paused         bool
	LabelClass     string
	JobFlavorIcon  string
	JobFlavorTitle string

	sortGroup string      // used only for sorting, doesn't show up in UI
	now       time.Time   // as passed to makeJob
	traits    task.Traits // as extracted from corresponding task.Manager
}

var stateToLabelClass = map[presentation.PublicStateKind]string{
	presentation.PublicStateRetrying:  "label-danger",
	presentation.PublicStatePaused:    "label-default",
	presentation.PublicStateScheduled: "label-primary",
	presentation.PublicStateSuspended: "label-default",
	presentation.PublicStateRunning:   "label-info",
	presentation.PublicStateOverrun:   "label-warning",
	presentation.PublicStateWaiting:   "label-warning",
	presentation.PublicStateStarting:  "label-default",
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
	traits, err := presentation.GetJobTraits(c, config(c).Catalog, j)
	if err != nil {
		logging.WithError(err).Warningf(c, "Failed to get task traits for %s", j.JobID)
	}

	now := clock.Now(c).UTC()
	nextRun := ""
	switch ts := j.CronTickTime(); {
	case ts == schedule.DistantFuture:
		nextRun = "-"
	case !ts.IsZero():
		nextRun = humanize.RelTime(ts, now, "ago", "from now")
	default:
		nextRun = "not scheduled yet"
	}

	// Internal state names aren't very user friendly. Introduce some aliases.
	state := presentation.GetPublicStateKind(j, traits)
	labelClass := stateToLabelClass[state]

	// Put triggers after regular jobs.
	sortGroup := "A"
	if j.Flavor == catalog.JobFlavorTrigger {
		sortGroup = "B"
	}

	return &schedulerJob{
		ProjectID:      j.ProjectID,
		JobName:        j.JobName(),
		IsV2:           j.IsV2(),
		Schedule:       j.Schedule,
		Definition:     taskToText(j.Task),
		Revision:       j.Revision,
		RevisionURL:    j.RevisionURL,
		State:          string(state),
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
		func(i, j int) bool { return s[i].JobName < s[j].JobName },
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
	ProjectID        string
	JobName          string
	InvID            int64
	Attempt          int64
	Revision         string
	RevisionURL      string
	Definition       string
	TriggeredBy      string
	Properties       string
	Tags             []string
	IncomingTriggers []trigger
	OutgoingTriggers []trigger
	Started          string
	Duration         string
	Status           string
	DebugLog         string
	RowClass         string
	LabelClass       string
	ViewURL          string
}

var statusToRowClass = map[task.Status]string{
	task.StatusStarting:  "active",
	task.StatusRetrying:  "warning",
	task.StatusRunning:   "info",
	task.StatusSucceeded: "success",
	task.StatusFailed:    "danger",
	task.StatusOverrun:   "warning",
	task.StatusAborted:   "danger",
}

var statusToLabelClass = map[task.Status]string{
	task.StatusStarting:  "label-default",
	task.StatusRetrying:  "label-warning",
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

	incTriggers, err := i.IncomingTriggers()
	if err != nil {
		panic(errors.Annotate(err, "failed to deserialize incoming triggers").Err())
	}
	outTriggers, err := i.OutgoingTriggers()
	if err != nil {
		panic(errors.Annotate(err, "failed to deserialize outgoing triggers").Err())
	}

	return &invocation{
		ProjectID:        j.ProjectID,
		JobName:          j.JobName,
		InvID:            i.ID,
		Attempt:          i.RetryCount + 1,
		Revision:         i.Revision,
		RevisionURL:      i.RevisionURL,
		Definition:       taskToText(i.Task),
		TriggeredBy:      triggeredBy,
		Properties:       makeJSONFromProtoStruct(i.PropertiesRaw),
		Tags:             i.Tags,
		IncomingTriggers: makeTriggerList(j.now, incTriggers),
		OutgoingTriggers: makeTriggerList(j.now, outTriggers),
		Started:          humanize.RelTime(i.Started, j.now, "ago", "from now"),
		Duration:         duration,
		Status:           string(status),
		DebugLog:         i.DebugLog,
		RowClass:         statusToRowClass[status],
		LabelClass:       statusToLabelClass[status],
		ViewURL:          i.ViewURL,
	}
}

// trigger is UI representation of internal.Trigger struct.
type trigger struct {
	Title     string
	URL       string
	RelTime   string
	EmittedBy string
}

// makeTrigger builds UI presentation of some internal.Trigger.
func makeTrigger(t *internal.Trigger, now time.Time) trigger {
	out := trigger{
		Title:     t.Title,
		URL:       t.Url,
		EmittedBy: strings.TrimPrefix(t.EmittedByUser, "user:"),
	}
	if out.Title == "" {
		out.Title = t.Id
	}
	if t.Created != nil {
		out.RelTime = humanize.RelTime(google.TimeFromProto(t.Created), now, "ago", "from now")
	}
	return out
}

// makeTriggerList builds UI presentation of a bunch of triggers.
func makeTriggerList(now time.Time, list []*internal.Trigger) []trigger {
	out := make([]trigger, len(list))
	for i, t := range list {
		out[i] = makeTrigger(t, now)
	}
	return out
}

// makeJSONFromProtoStruct reformats serialized protobuf.Struct as JSON.
//
// If the blob is empty, returns empty string. If the blob is not valid proto
// message, returns a string with error message instead. This is exclusively for
// UI after all.
func makeJSONFromProtoStruct(blob []byte) string {
	if len(blob) == 0 {
		return ""
	}

	// Binary proto => internal representation.
	obj := structpb.Struct{}
	if err := proto.Unmarshal(blob, &obj); err != nil {
		return fmt.Sprintf("<not a valid protobuf.Struct - %s>", err)
	}

	// Internal representation => JSON. But JSONPB produces very ugly JSON when
	// using Ident. So we are not done yet...
	ugly, err := (&jsonpb.Marshaler{}).MarshalToString(&obj)
	if err != nil {
		return fmt.Sprintf("<failed to marshal to JSON - %s>", err)
	}

	// JSON => internal representation 2, sigh. Because there's no existing
	// structpb.Struct => map converter and writing one just for the sake of
	// JSON pretty printing is kind of annoying.
	var obj2 map[string]interface{}
	if err := json.Unmarshal([]byte(ugly), &obj2); err != nil {
		return fmt.Sprintf("<internal error when unmarshaling JSON - %s>", err)
	}

	// Internal representation 2 => pretty (well, prettier) JSON.
	pretty, err := json.MarshalIndent(obj2, "", "  ")
	if err != nil {
		return fmt.Sprintf("<internal error when marshaling JSON - %s>", err)
	}
	return string(pretty)
}
