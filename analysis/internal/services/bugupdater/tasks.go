// Copyright 2023 The LUCI Authors.
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

// Package bugupdater defines a task for updating a project's LUCI
// Analysis bugs in response to the latest cluster metrics. Bugs
// are auto-opened, auto-verified and have their priorities adjusted
// according to the LUCI project's bug management policies.
package bugupdater

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/analysis/internal/analysis"
	"go.chromium.org/luci/analysis/internal/bugs/buganizer"
	"go.chromium.org/luci/analysis/internal/bugs/updater"
	"go.chromium.org/luci/analysis/internal/clustering/runs"
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
	"go.chromium.org/luci/analysis/pbutil"
)

const (
	taskClassID = "update-bugs"
	queueName   = "update-bugs"
)

var bugUpdater = tq.RegisterTaskClass(tq.TaskClass{
	ID:        taskClassID,
	Prototype: &taskspb.UpdateBugs{},
	Queue:     queueName,
	Kind:      tq.NonTransactional,
})

// RegisterTaskHandler registers the handler for bug update tasks.
// uiBaseURL is the base URL for the UI, without trailing slash, e.g.
// "https://luci-milo.appspot.com".
func RegisterTaskHandler(srv *server.Server, uiBaseURL string) error {
	h := &Handler{
		GCPProject: srv.Options.CloudProject,
		Simulate:   false,
		UIBaseURL:  uiBaseURL,
	}

	handler := func(ctx context.Context, payload proto.Message) error {
		task := payload.(*taskspb.UpdateBugs)
		err := h.UpdateBugs(ctx, task)
		if err != nil {
			// Tasks should never be retried as it may cause
			// us to exceed our bug-filing quota.
			//
			// No retries should be configured at the task queue level
			// as this is the only solution that will ensure no retries
			// even in the case of unexpected GAE instance shutdown.
			//
			// Nonetheless, as a precaution we also return a fatal error here.
			return tq.Fatal.Apply(err)
		}
		return nil
	}
	bugUpdater.AttachHandler(handler)
	return nil
}

// Schedule enqueues a task to update bugs for the given LUCI Project.
func Schedule(ctx context.Context, task *taskspb.UpdateBugs) error {
	if err := validateTask(task); err != nil {
		return errors.Fmt("validate task: %w", err)
	}

	title := fmt.Sprintf("update-bugs-%s-%v", task.Project, task.ReclusteringAttemptMinute.AsTime().Format("20060102-150405"))

	taskProto := &tq.Task{
		Title: title,
		// Copy the task to avoid the caller retaining an alias to
		// the task proto passed to tq.AddTask.
		Payload: proto.Clone(task).(*taskspb.UpdateBugs),
	}

	return tq.AddTask(ctx, taskProto)
}

// Handler provide a method to handle bug update tasks.
type Handler struct {
	// GCPProject is the GCP Cloud Project Name, e.g. luci-analysis.
	GCPProject string
	// UIBaseURL is the base URL for the UI, without trailing slash, e.g.
	// "https://luci-milo.appspot.com".
	UIBaseURL string
	// Whether bug updates should be simulated. Only used in local
	// development when UpdateBug(...) is called directly from the
	// cron handler.
	Simulate bool
}

// UpdateBugs handles the given bug update task.
func (h Handler) UpdateBugs(ctx context.Context, task *taskspb.UpdateBugs) (retErr error) {
	if err := validateTask(task); err != nil {
		return errors.Fmt("validate task: %w", err)
	}

	defer func() {
		status := "failure"
		if retErr == nil {
			status = "success"
		}
		statusGauge.Set(ctx, status, task.Project)
		runsCounter.Add(ctx, 1, task.Project, status)
	}()

	ctx, cancel := context.WithDeadline(ctx, task.Deadline.AsTime())
	defer cancel()
	if err := ctx.Err(); err != nil {
		return errors.Fmt("check deadline before start: %w", err)
	}

	buganizerClient, err := buganizer.CreateBuganizerClient(ctx)
	if err != nil {
		return errors.Fmt("creating a buganizer client: %w", err)
	}
	if buganizerClient != nil {
		defer buganizerClient.Close()
	}

	analysisClient, err := analysis.NewClient(ctx, h.GCPProject)
	if err != nil {
		return err
	}
	defer func() {
		if err := analysisClient.Close(); err != nil && retErr == nil {
			retErr = errors.Fmt("closing analysis client: %w", err)
		}
	}()

	progress, err := runs.ReadReclusteringProgressUpTo(ctx, task.Project, task.ReclusteringAttemptMinute.AsTime())
	if err != nil {
		return errors.Fmt("read re-clustering progress: %w", err)
	}

	runTimestamp := clock.Now(ctx).Truncate(time.Minute)

	opts := updater.UpdateOptions{
		UIBaseURL:            h.UIBaseURL,
		Project:              task.Project,
		AnalysisClient:       analysisClient,
		BuganizerClient:      buganizerClient,
		SimulateBugUpdates:   h.Simulate,
		MaxBugsFiledPerRun:   1,
		UpdateRuleBatchSize:  1000,
		ReclusteringProgress: progress,
		RunTimestamp:         runTimestamp,
	}

	start := time.Now()
	err = updater.UpdateBugsForProject(ctx, opts)
	if err != nil {
		err = errors.Fmt("in project %v: %w", task.Project, err)
		logging.Errorf(ctx, "Updating analysis and bugs: %s", err)
	}

	elapsed := time.Since(start)
	durationGauge.Set(ctx, elapsed.Seconds(), task.Project)

	return err
}

func validateTask(task *taskspb.UpdateBugs) error {
	if err := pbutil.ValidateProject(task.Project); err != nil {
		return errors.Fmt("project: %w", err)
	}
	if err := task.ReclusteringAttemptMinute.CheckValid(); err != nil {
		return errors.New("reclustering_attempt_minute: missing or invalid timestamp")
	}
	if err := task.Deadline.CheckValid(); err != nil {
		return errors.New("deadline: missing or invalid timestamp")
	}
	attemptMinute := task.ReclusteringAttemptMinute.AsTime()
	if attemptMinute.Truncate(time.Minute) != attemptMinute {
		return errors.New("reclustering_attempt_minute: must be aligned to the start of a minute")
	}
	return nil
}
