// Copyright 2024 The LUCI Authors.
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

// Package join contains methods for joining buildbucket build completions
// with LUCI CV completions and ResultDB invocation finalizations.
package join

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/ingestion/control"
	ctlpb "go.chromium.org/luci/analysis/internal/ingestion/control/proto"
	"go.chromium.org/luci/analysis/internal/services/verdictingester"
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
)

const (
	// TODO: Removing the hosts after ResultDB PubSub, CVPubSub and GetRun RPC added them.
	// Host name of ResultDB.
	rdbHost = "results.api.cr.dev"
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

	rdbInvocationsInputCounter = metric.NewCounter(
		"analysis/ingestion/join/rdb_invocations_input",
		"The number of ResultDB invocation finalization was received."+
			" Broken down by project of the ResultDB invocation, and whether the invocation has buildbucket build.",
		nil,
		// The LUCI Project of the ResultDB invocation.
		field.String("project"),
		field.Bool("has_buildbucket_build"))

	rdbInvocationsOutputCounter = metric.NewCounter(
		"analysis/ingestion/join/rdb_invocations_output",
		"The number of ResultDBinvocation which were successfully joined and for which ingestion was queued."+
			" Broken down by project of the ResultDB invocation, and whether the invocation has buildbucket build.",
		nil,
		// The LUCI Project of the ResultDB invocation.
		field.String("project"),
		field.Bool("has_buildbucket_build"))
)

// JoinBuildResult sets the build result for the ingestion.
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
func JoinBuildResult(ctx context.Context, buildID int64, buildProject string, isPresubmit, hasInvocation bool, br *ctlpb.BuildResult) error {
	if br == nil {
		return errors.New("build result must be specified")
	}
	resultdbHost := br.ResultdbHost
	if resultdbHost == "" {
		// Assign a default rdbhost when the the resultdbHost from the build is empty.
		// This happens when the build doesn't have an invocation.
		resultdbHost = rdbHost
	}
	ingestionID := control.IngestionIDFromBuildID(resultdbHost, buildID)
	var saved bool
	var createdTask *taskDetails
	f := func(ctx context.Context) error {
		// Reset variables to clear out anything from a previous
		// (failed) try of this transaction to ensure nothing
		// leaks forward.
		saved = false
		createdTask = nil
		entries, err := control.Read(ctx, []control.IngestionID{ingestionID})
		if err != nil {
			return err
		}
		entry := entries[0]
		if entry == nil {
			// Record does not exist. Create it.
			entry = &control.Entry{
				IngestionID: ingestionID,
			}
		}
		// IsPresubmit should be populated and valid if either the build result
		// or presubmit result has been populated.
		if (entry.BuildResult != nil || entry.PresubmitResult != nil) && entry.IsPresubmit != isPresubmit {
			return fmt.Errorf("disagreement about whether build %d is part of presubmit run (got %v, want %v)", buildID, isPresubmit, entry.IsPresubmit)
		}
		// HasInvocation should be populated if either the build or invocation
		// result has already been populated.
		if (entry.BuildResult != nil || entry.InvocationResult != nil) && entry.HasInvocation != hasInvocation {
			return fmt.Errorf("disagreement about whether build %d has an invocation (got %v, want %v)", buildID, hasInvocation, entry.HasInvocation)
		}
		// HasBuildBucketBuild should be populated if InvocationResult, PresubmitResult or BuildResult has been populated.
		if (entry.InvocationResult != nil || entry.PresubmitResult != nil || entry.BuildResult != nil) && !entry.HasBuildBucketBuild {
			return fmt.Errorf("disagreement about whether build %d has an buildbucket build (got %v, want %v)", buildID, entry.HasBuildBucketBuild, true)
		}
		if entry.InvocationProject != "" && entry.InvocationProject != buildProject {
			return fmt.Errorf("disagreement about LUCI project between build %d and its invocation (invocation project %v, build project %v)", buildID, entry.InvocationProject, buildProject)
		}
		if entry.BuildResult != nil {
			// Build result already recorded. Do not modify and do not
			// create a duplicate ingestion.
			return nil
		}
		entry.HasBuildBucketBuild = true
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

		if err := control.InsertOrUpdate(ctx, entry); err != nil {
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
		logging.Warningf(ctx, "build result for build %d was dropped as one was already recorded", buildID)
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

// JoinPresubmitResult sets the presubmit result for an ingestion.
//
// Ingestion task(s) are created for invocations once all required data
// is available.
//
// If the presubmit result has already been provided for a build,
// this method has no effect.
func JoinPresubmitResult(ctx context.Context, presubmitResultByBuildID map[int64]*ctlpb.PresubmitResult, presubmitProject string) error {
	for id, result := range presubmitResultByBuildID {
		if result == nil {
			return errors.Reason("presubmit result for build %q must be specified", id).Err()
		}
	}

	var buildIDsSkipped []int64
	var tasksCreated []*taskDetails
	f := func(ctx context.Context) error {
		// Reset variables to clear out anything from a previous
		// (failed) try of this transaction to ensure nothing
		// leaks forward.
		buildIDsSkipped = nil
		tasksCreated = nil

		var ingestionIDs []control.IngestionID
		var buildIDs []int64
		for id := range presubmitResultByBuildID {
			ingestionID := control.IngestionIDFromBuildID(rdbHost, id)
			ingestionIDs = append(ingestionIDs, ingestionID)
			buildIDs = append(buildIDs, id)
		}

		entries, err := control.Read(ctx, ingestionIDs)
		if err != nil {
			return err
		}
		for i, entry := range entries {
			ingestionID := ingestionIDs[i]
			buildID := buildIDs[i]
			if entry == nil {
				// Record does not exist. Create it.
				entry = &control.Entry{
					IngestionID: ingestionID,
				}
			}
			// IsPresubmit should be populated and valid if either the build result
			// or presubmit result has been populated.
			if (entry.BuildResult != nil || entry.PresubmitResult != nil) && !entry.IsPresubmit {
				return errors.Reason("attempt to save presubmit result on build %d not marked as presubmit", buildID).Err()
			}
			// HasBuildBucketBuild should be populated if InvocationResult, PresubmitResult or BuildResult has been populated.
			if (entry.InvocationResult != nil || entry.PresubmitResult != nil || entry.BuildResult != nil) && !entry.HasBuildBucketBuild {
				return fmt.Errorf("disagreement about whether build %d has an buildbucket build (got %v, want %v)", buildID, entry.HasBuildBucketBuild, true)
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
			// Ingestion must has buildbucket build if it has a cv run.
			entry.HasBuildBucketBuild = true

			isTaskCreated := createTasksIfNeeded(ctx, entry)
			if isTaskCreated {
				tasksCreated = append(tasksCreated, newTaskDetails(entry))
			}
			if err := control.InsertOrUpdate(ctx, entry); err != nil {
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

// JoinInvocationResult sets the invocation result for the ingestion.
//
// Ingestion task(s) are created for builds once all required data
// is available.
//
// If the invocation result has already been provided for a build,
// this method has no effect.
func JoinInvocationResult(ctx context.Context, invocationID, invocationProject string, ir *ctlpb.InvocationResult) error {
	if ir == nil {
		return errors.New("invocation result must be specified")
	}
	buildBucketBuildInvocation := isBuildbucketBuildInvocation(invocationID)
	var saved bool
	var createdTask *taskDetails
	f := func(ctx context.Context) error {
		// Reset variables to clear out anything from a previous
		// (failed) try of this transaction to ensure nothing
		// leaks forward.
		saved = false
		createdTask = nil

		ingestionID := control.IngestionIDFromInvocationID(rdbHost, invocationID)
		entries, err := control.Read(ctx, []control.IngestionID{ingestionID})
		if err != nil {
			return err
		}
		entry := entries[0]
		if entry == nil {
			// Record does not exist. Create it.
			entry = &control.Entry{
				IngestionID: ingestionID,
			}
		}
		// HasInvocation should be populated if either the build or invocation
		// result has already been populated.
		if (entry.BuildResult != nil || entry.InvocationResult != nil) && !entry.HasInvocation {
			return fmt.Errorf("attempt to save invocation result on invocation %q marked as not having invocation", invocationID)
		}
		// HasBuildBucketBuild should be populated if InvocationResult, BuildResult or PresubmitResult has been populated.
		if (entry.InvocationResult != nil || entry.PresubmitResult != nil || entry.BuildResult != nil) && entry.HasBuildBucketBuild != buildBucketBuildInvocation {
			return fmt.Errorf("disagreement about whether invocation %s has an buildbucket build (got %v, want %v)", invocationID, buildBucketBuildInvocation, entry.HasBuildBucketBuild)
		}
		if entry.BuildProject != "" && entry.BuildProject != invocationProject {
			return fmt.Errorf("disagreement about LUCI project between invocation %s and its build (invocation project %v, build project %v)", invocationID, invocationProject, entry.BuildProject)
		}
		if entry.InvocationResult != nil {
			// Invocation result already recorded. Do not modify and
			// do not create a duplicate ingestion.
			return nil
		}
		entry.HasBuildBucketBuild = buildBucketBuildInvocation
		entry.HasInvocation = true
		entry.InvocationProject = invocationProject
		entry.InvocationResult = ir
		entry.InvocationJoinedTime = spanner.CommitTimestamp

		saved = true
		isTaskCreated := createTasksIfNeeded(ctx, entry)
		if isTaskCreated {
			createdTask = newTaskDetails(entry)
		}

		if err := control.InsertOrUpdate(ctx, entry); err != nil {
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
		logging.Warningf(ctx, "invocation result for invocatoin %q was dropped as one was already recorded", invocationID)
	}

	// Export metrics.
	if saved {
		rdbInvocationsInputCounter.Add(ctx, 1, invocationProject, buildBucketBuildInvocation)
	}
	if createdTask != nil {
		createdTask.reportMetrics(ctx)
	}
	return nil
}

// taskDetails contains information about a result-ingestion task
// that was created (if any).
type taskDetails struct {
	// HasInvocation is whether this ingestion has a Buildbucket build.
	HasBuildBucketBuild bool
	// BuildProject is the LUCI Project that contained the build.
	BuildProject string
	// IsPresubmit is whether this ingestion has a LUCI CV Run.
	IsPresubmit bool
	// PresubmitProject is the LUCI Project that contained the CV Run.
	// Only set if IsPresubmit is set.
	PresubmitProject string
	// HasInvocation is whether this ingestion has a ResultDB invocation.
	HasInvocation bool
	// BuildProject is the LUCI Project that contained the invocation
	// (should be the same as the build).
	InvocationProject string
}

func newTaskDetails(e *control.Entry) *taskDetails {
	return &taskDetails{
		HasBuildBucketBuild: e.HasBuildBucketBuild,
		BuildProject:        e.BuildProject,
		IsPresubmit:         e.IsPresubmit,
		PresubmitProject:    e.PresubmitProject,
		HasInvocation:       e.HasInvocation,
		InvocationProject:   e.InvocationProject,
	}
}

func (t *taskDetails) reportMetrics(ctx context.Context) {
	if t.HasBuildBucketBuild {
		bbBuildOutputCounter.Add(ctx, 1, t.BuildProject, t.IsPresubmit, t.HasInvocation)
	}
	if t.IsPresubmit {
		cvBuildOutputCounter.Add(ctx, 1, t.PresubmitProject)
	}
	if t.HasInvocation {
		rdbInvocationsOutputCounter.Add(ctx, 1, t.InvocationProject, t.HasBuildBucketBuild)
	}
}

// createTaskIfNeeded transactionally creates a result-ingestion task
// if all necessary data for the ingestion is available. Returns true if the
// task was created.
func createTasksIfNeeded(ctx context.Context, e *control.Entry) bool {
	// Join is completed if it satisfies all of the conditions.
	// * It is marked HasBuildBucketBuild and BuildResult is populated.
	// * It is marked HasInvocation and InvocationResult is populated.
	// * It is marked IsPresubmit and PresubmitResult is populated.
	joinComplete := (!e.HasBuildBucketBuild || e.BuildResult != nil) &&
		(!e.IsPresubmit || e.PresubmitResult != nil) &&
		(!e.HasInvocation || e.InvocationResult != nil)

	if !joinComplete {
		return false
	}
	var itvTask *taskspb.IngestTestVerdicts
	if e.IsPresubmit {
		// IsPresubmit is true implies that HasBuildBucketBuild is also true.
		itvTask = &taskspb.IngestTestVerdicts{
			IngestionId:   string(e.IngestionID),
			PartitionTime: e.PresubmitResult.CreationTime,
			Project:       e.BuildProject,
			Invocation:    e.InvocationResult,
			Build:         e.BuildResult,
			PresubmitRun:  e.PresubmitResult,
		}
	} else if e.HasInvocation {
		var partitionTime time.Time
		// TODO: remove if statement and always use InvocationResult.CreationTime
		// once protos without this field set have been flushed out.
		// If you are reading this in August 2024, this can be safely actioned
		// now.
		if e.InvocationResult.CreationTime != nil {
			partitionTime = e.InvocationResult.CreationTime.AsTime()
		} else if e.BuildResult != nil && e.BuildResult.CreationTime != nil {
			partitionTime = e.BuildResult.CreationTime.AsTime()
		} else {
			partitionTime = clock.Now(ctx)
		}

		itvTask = &taskspb.IngestTestVerdicts{
			IngestionId:   string(e.IngestionID),
			PartitionTime: timestamppb.New(partitionTime),
			Project:       e.InvocationProject,
			Invocation:    e.InvocationResult,
			Build:         e.BuildResult, // if any
		}
	} else if e.HasBuildBucketBuild {
		itvTask = &taskspb.IngestTestVerdicts{
			IngestionId:   string(e.IngestionID),
			PartitionTime: e.BuildResult.CreationTime,
			Project:       e.BuildProject,
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
