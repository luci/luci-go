// Copyright 2022 The LUCI Authors.
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

// Package joinlegacy contains methods for joining buildbucket build completions
// with LUCI CV completions and ResultDB invocation finalizations.
package joinlegacy

import (
	"context"
	"fmt"
	"regexp"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/server/span"

	ctlpb "go.chromium.org/luci/analysis/internal/ingestion/control/proto"
	"go.chromium.org/luci/analysis/internal/ingestion/controllegacy"
	"go.chromium.org/luci/analysis/internal/services/verdictingester"
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
)

var (
	// buildInvocationRE extracts the buildbucket build number from invocations
	// named after a buildbucket build ID.
	buildInvocationRE = regexp.MustCompile(`^build-([0-9]+)$`)
)

var (
	cvBuildInputCounter = metric.NewCounter(
		"analysis/ingestion/join/cv_builds_input",
		"The number of builds for which CV Run completion was received."+
			" Broken down by project of the CV run.",
		nil,
		// The LUCI Project of the CV run.
		field.String("project"))

	cvBuildOutputCounter = metric.NewCounter(
		"analysis/ingestion/join/cv_builds_output",
		"The number of builds associated with a presubmit run which were successfully joined and for which ingestion was queued."+
			" Broken down by project of the CV run.",
		nil,
		// The LUCI Project of the CV run.
		field.String("project"))

	bbBuildInputCounter = metric.NewCounter(
		"analysis/ingestion/join/bb_builds_input",
		"The number of builds for which buildbucket build completion was received."+
			" Broken down by project of the buildbucket build, whether the build"+
			" is part of a presubmit run, and whether the build has a ResultDB invocation.",
		nil,
		// The LUCI Project of the buildbucket run.
		field.String("project"),
		field.Bool("is_presubmit"),
		field.Bool("has_invocation"))

	bbBuildOutputCounter = metric.NewCounter(
		"analysis/ingestion/join/bb_builds_output",
		"The number of builds which were successfully joined and for which ingestion was queued."+
			" Broken down by project of the buildbucket build, whether the build"+
			" is part of a presubmit run, and whether the build has a ResultDB invocation.",
		nil,
		// The LUCI Project of the buildbucket run.
		field.String("project"),
		field.Bool("is_presubmit"),
		field.Bool("has_invocation"))

	rdbBuildInputCounter = metric.NewCounter(
		"analysis/ingestion/join/rdb_builds_input",
		"The number of builds for which ResultDB invocation finalization was received."+
			" Broken down by project of the ResultDB invocation.",
		nil,
		// The LUCI Project of the ResultDB invocation.
		field.String("project"))

	rdbBuildOutputCounter = metric.NewCounter(
		"analysis/ingestion/join/rdb_builds_output",
		"The number of builds associated with an invocation which were successfully joined and for which ingestion was queued."+
			" Broken down by project of the ResultDB invocation.",
		nil,
		// The LUCI Project of the ResultDB invocation.
		field.String("project"))
)

// JoinBuildResult sets the build result for the given build.
//
// An ingestion task is created if all required data for the
// ingestion is available.
//   - for builds part of a presubmit run, we will need to wait
//     for the presubmit run.
//   - for builds with a ResultDB invocation (note this is not
//     mutually exclusive to the above), we will need to wait
//     for the ResultDB invocation.
//
// If none of the above applies, we will start an ingestion
// straight away.
//
// If the build result has already been provided for a build,
// this method has no effect.
func JoinBuildResult(ctx context.Context, buildID, buildProject string, isPresubmit, hasInvocation bool, br *ctlpb.BuildResult) error {
	if br == nil {
		return errors.New("build result must be specified")
	}
	var saved bool
	var createdTask *taskDetails
	f := func(ctx context.Context) error {
		// Reset variables to clear out anything from a previous
		// (failed) try of this transaction to ensure nothing
		// leaks forward.
		saved = false
		createdTask = nil

		entries, err := controllegacy.Read(ctx, []string{buildID})
		if err != nil {
			return err
		}
		entry := entries[0]
		if entry == nil {
			// Record does not exist. Create it.
			entry = &controllegacy.Entry{
				BuildID: buildID,
			}
		}
		// IsPresubmit should be populated and valid if either the build result
		// or presubmit result has been populated.
		if (entry.BuildResult != nil || entry.PresubmitResult != nil) && entry.IsPresubmit != isPresubmit {
			return fmt.Errorf("disagreement about whether build %q is part of presubmit run (got %v, want %v)", buildID, isPresubmit, entry.IsPresubmit)
		}
		// HasInvocation should be populated if either the build or invocation
		// result has already been populated.
		if (entry.BuildResult != nil || entry.InvocationResult != nil) && entry.HasInvocation != hasInvocation {
			return fmt.Errorf("disagreement about whether build %q has an invocation (got %v, want %v)", buildID, hasInvocation, entry.HasInvocation)
		}
		if entry.BuildResult != nil {
			// Build result already recorded. Do not modify and do not
			// create a duplicate ingestion.
			return nil
		}
		entry.IsPresubmit = isPresubmit
		entry.HasInvocation = hasInvocation
		entry.BuildProject = buildProject
		entry.BuildResult = br
		entry.BuildJoinedTime = spanner.CommitTimestamp

		saved = true
		isTaskCreated := createTasksIfNeeded(ctx, entry)
		if isTaskCreated {
			createdTask = newTaskDetails(entry)
		}

		if err := controllegacy.InsertOrUpdate(ctx, entry); err != nil {
			return err
		}
		return nil
	}
	if _, err := span.ReadWriteTransaction(ctx, f); err != nil {
		if _, ok := status.FromError(errors.Unwrap(err)); ok {
			// Spanner gRPC error.
			return transient.Tag.Apply(err)
		}
		return err
	}
	if !saved {
		logging.Warningf(ctx, "build result for build %q was dropped as one was already recorded", buildID)
	}

	// Export metrics.
	if saved {
		bbBuildInputCounter.Add(ctx, 1, buildProject, isPresubmit, hasInvocation)
	}
	if createdTask != nil {
		createdTask.reportMetrics(ctx)
	}
	return nil
}

// JoinPresubmitResult sets the presubmit result for the given builds.
//
// Ingestion task(s) are created for builds once all required data
// is available.
//
// If the presubmit result has already been provided for a build,
// this method has no effect.
func JoinPresubmitResult(ctx context.Context, presubmitResultByBuildID map[string]*ctlpb.PresubmitResult, presubmitProject string) error {
	for id, result := range presubmitResultByBuildID {
		if result == nil {
			return errors.Reason("presubmit result for build %q must be specified", id).Err()
		}
	}

	var buildIDsSkipped []string
	var tasksCreated []*taskDetails
	f := func(ctx context.Context) error {
		// Reset variables to clear out anything from a previous
		// (failed) try of this transaction to ensure nothing
		// leaks forward.
		buildIDsSkipped = nil
		tasksCreated = nil

		var buildIDs []string
		for id := range presubmitResultByBuildID {
			buildIDs = append(buildIDs, id)
		}

		entries, err := controllegacy.Read(ctx, buildIDs)
		if err != nil {
			return err
		}
		for i, entry := range entries {
			buildID := buildIDs[i]
			if entry == nil {
				// Record does not exist. Create it.
				entry = &controllegacy.Entry{
					BuildID: buildID,
				}
			}
			// IsPresubmit should be populated and valid if either the build result
			// or presubmit result has been populated.
			if (entry.BuildResult != nil || entry.PresubmitResult != nil) && !entry.IsPresubmit {
				return errors.Reason("attempt to save presubmit result on build %q not marked as presubmit", buildID).Err()
			}
			if entry.PresubmitResult != nil {
				// Presubmit result already recorded. Do not modify and do not
				// create a duplicate ingestion.
				buildIDsSkipped = append(buildIDsSkipped, buildID)
				continue
			}
			entry.IsPresubmit = true
			entry.PresubmitProject = presubmitProject
			entry.PresubmitResult = presubmitResultByBuildID[buildID]
			entry.PresubmitJoinedTime = spanner.CommitTimestamp

			isTaskCreated := createTasksIfNeeded(ctx, entry)
			if isTaskCreated {
				tasksCreated = append(tasksCreated, newTaskDetails(entry))
			}
			if err := controllegacy.InsertOrUpdate(ctx, entry); err != nil {
				return err
			}
		}
		return nil
	}
	if _, err := span.ReadWriteTransaction(ctx, f); err != nil {
		if _, ok := status.FromError(errors.Unwrap(err)); ok {
			// Spanner gRPC error.
			return transient.Tag.Apply(err)
		}
		return err
	}
	if len(buildIDsSkipped) > 0 {
		logging.Warningf(ctx, "presubmit result for builds %v were dropped as one was already recorded", buildIDsSkipped)
	}

	// Export metrics.
	cvBuildInputCounter.Add(ctx, int64(len(presubmitResultByBuildID)-len(buildIDsSkipped)), presubmitProject)
	for _, task := range tasksCreated {
		task.reportMetrics(ctx)
	}
	return nil
}

// JoinInvocationResult sets the invocation result for the given builds.
//
// Ingestion task(s) are created for builds once all required data
// is available.
//
// If the invocation result has already been provided for a build,
// this method has no effect.
func JoinInvocationResult(ctx context.Context, buildID, invocationProject string, ir *ctlpb.InvocationResult) error {
	if ir == nil {
		return errors.New("invocation result must be specified")
	}
	var saved bool
	var createdTask *taskDetails
	f := func(ctx context.Context) error {
		// Reset variables to clear out anything from a previous
		// (failed) try of this transaction to ensure nothing
		// leaks forward.
		saved = false
		createdTask = nil

		entries, err := controllegacy.Read(ctx, []string{buildID})
		if err != nil {
			return err
		}
		entry := entries[0]
		if entry == nil {
			// Record does not exist. Create it.
			entry = &controllegacy.Entry{
				BuildID: buildID,
			}
		}
		// HasInvocation should be populated if either the build or invocation
		// result has already been populated.
		if (entry.BuildResult != nil || entry.InvocationResult != nil) && !entry.HasInvocation {
			return fmt.Errorf("attempt to save invocation result on build %q marked as not having invocation", buildID)
		}
		if entry.InvocationResult != nil {
			// Invocation result already recorded. Do not modify and
			// do not create a duplicate ingestion.
			return nil
		}
		entry.HasInvocation = true
		entry.InvocationProject = invocationProject
		entry.InvocationResult = ir
		entry.InvocationJoinedTime = spanner.CommitTimestamp

		saved = true
		isTaskCreated := createTasksIfNeeded(ctx, entry)
		if isTaskCreated {
			createdTask = newTaskDetails(entry)
		}

		if err := controllegacy.InsertOrUpdate(ctx, entry); err != nil {
			return err
		}
		return nil
	}
	if _, err := span.ReadWriteTransaction(ctx, f); err != nil {
		if _, ok := status.FromError(errors.Unwrap(err)); ok {
			// Spanner gRPC error.
			return transient.Tag.Apply(err)
		}
		return err
	}
	if !saved {
		logging.Warningf(ctx, "invocation result for build %q was dropped as one was already recorded", buildID)
	}

	// Export metrics.
	if saved {
		rdbBuildInputCounter.Add(ctx, 1, invocationProject)
	}
	if createdTask != nil {
		createdTask.reportMetrics(ctx)
	}
	return nil
}

// taskDetails contains information about a result-ingestion task
// that was created (if any).
type taskDetails struct {
	// BuildProject is the LUCI Project that contained the build.
	BuildProject string
	// IsPresubmit is whether this build was part of a LUCI CV Run.
	IsPresubmit bool
	// PresubmitProject is the LUCI Project that contained the CV Run.
	// Only set if IsPresubmit is set.
	PresubmitProject string
	// HasInvocation is whether this build has a ResultDB invocation.
	HasInvocation bool
	// BuildProject is the LUCI Project that contained the invocation
	// (should be the same as the build).
	InvocationProject string
}

func newTaskDetails(e *controllegacy.Entry) *taskDetails {
	return &taskDetails{
		BuildProject:      e.BuildProject,
		IsPresubmit:       e.IsPresubmit,
		PresubmitProject:  e.PresubmitProject,
		HasInvocation:     e.HasInvocation,
		InvocationProject: e.InvocationProject,
	}
}

func (t *taskDetails) reportMetrics(ctx context.Context) {
	bbBuildOutputCounter.Add(ctx, 1, t.BuildProject, t.IsPresubmit, t.HasInvocation)
	if t.IsPresubmit {
		cvBuildOutputCounter.Add(ctx, 1, t.PresubmitProject)
	}
	if t.HasInvocation {
		rdbBuildOutputCounter.Add(ctx, 1, t.InvocationProject)
	}
}

// createTaskIfNeeded transactionally creates a result-ingestion task
// if all necessary data for the ingestion is available. Returns true if the
// task was created.
func createTasksIfNeeded(ctx context.Context, e *controllegacy.Entry) bool {
	joinComplete := e.BuildResult != nil &&
		(!e.IsPresubmit || e.PresubmitResult != nil) &&
		(!e.HasInvocation || e.InvocationResult != nil)
	if !joinComplete {
		return false
	}

	var itvTask *taskspb.IngestTestVerdicts
	if e.IsPresubmit {
		itvTask = &taskspb.IngestTestVerdicts{
			PartitionTime: e.PresubmitResult.CreationTime,
			Build:         e.BuildResult,
			PresubmitRun:  e.PresubmitResult,
		}
	} else {
		itvTask = &taskspb.IngestTestVerdicts{
			PartitionTime: e.BuildResult.CreationTime,
			Build:         e.BuildResult,
		}
	}

	// Copy the task to avoid aliasing issues if the caller ever
	// decides the modify e.PresubmitResult or e.BuildResult
	// after we return.
	itvTask = proto.Clone(itvTask).(*taskspb.IngestTestVerdicts)
	verdictingester.Schedule(ctx, itvTask)

	return true
}
