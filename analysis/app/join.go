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

package app

import (
	"context"
	"fmt"

	"cloud.google.com/go/spanner"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/server/span"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/analysis/internal/ingestion/control"
	ctlpb "go.chromium.org/luci/analysis/internal/ingestion/control/proto"
	"go.chromium.org/luci/analysis/internal/services/resultingester"
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
)

// For presubmit builds, proceeding to ingestion is conditional:
// we must wait for the both the CV run and Buildbucket build to complete.
// We define the following metrics to monitor the performance of that join.
var (
	cvPresubmitBuildInputCounter = metric.NewCounter(
		"analysis/ingestion/join/cv_presubmit_builds_input",
		"The number of unique presubmit builds for which CV Run Completion was received."+
			" Broken down by project of the CV run.",
		nil,
		// The LUCI Project of the CV run.
		field.String("project"))

	cvPresubmitBuildOutputCounter = metric.NewCounter(
		"analysis/ingestion/join/cv_presubmit_builds_output",
		"The number of presubmit builds which were successfully joined and for which ingestion was queued."+
			" Broken down by project of the CV run.",
		nil,
		// The LUCI Project of the CV run.
		field.String("project"))

	bbPresubmitBuildInputCounter = metric.NewCounter(
		"analysis/ingestion/join/bb_presubmit_builds_input",
		"The number of unique presubmit build for which buildbucket build completion was received."+
			" Broken down by project of the buildbucket build.",
		nil,
		// The LUCI Project of the buildbucket run.
		field.String("project"))

	bbPresubmitBuildOutputCounter = metric.NewCounter(
		"analysis/ingestion/join/bb_presubmit_builds_output",
		"The number of presubmit builds which were successfully joined and for which ingestion was queued."+
			" Broken down by project of the buildbucket build.",
		nil,
		// The LUCI Project of the buildbucket run.
		field.String("project"))
)

// For CI builds, no actual join needs to occur. So it is sufficient to
// monitor only the output flow (same as input flow).
var (
	outputCIBuildCounter = metric.NewCounter(
		"analysis/ingestion/join/ci_builds_output",
		"The number of CI builds for which ingestion was queued.",
		nil,
		// The LUCI Project.
		field.String("project"))
)

// JoinBuildResult sets the build result for the given build.
//
// An ingestion task is created if all required data for the
// ingestion is available (for builds part of a presubmit run,
// this is only after the presubmit result has joined, for
// all other builds, this is straight away).
//
// If the build result has already been provided for a build,
// this method has no effect.
func JoinBuildResult(ctx context.Context, buildID, buildProject string, isPresubmit bool, br *ctlpb.BuildResult) error {
	if br == nil {
		return errors.New("build result must be specified")
	}
	var saved bool
	var taskCreated bool
	var cvProject string
	f := func(ctx context.Context) error {
		// Clear variables to ensure nothing from a previous (failed)
		// try of this transaction leaks out to the outer context.
		saved = false
		taskCreated = false
		cvProject = ""

		entries, err := control.Read(ctx, []string{buildID})
		if err != nil {
			return err
		}
		entry := entries[0]
		if entry == nil {
			// Record does not exist. Create it.
			entry = &control.Entry{
				BuildID:     buildID,
				IsPresubmit: isPresubmit,
			}
		}
		if entry.IsPresubmit != isPresubmit {
			return fmt.Errorf("disagreement about whether ingestion is presubmit run (got %v, want %v)", isPresubmit, entry.IsPresubmit)
		}
		if entry.BuildResult != nil {
			// Build result already recorded. Do not modify and do not
			// create a duplicate ingestion.
			return nil
		}
		entry.BuildProject = buildProject
		entry.BuildResult = br
		entry.BuildJoinedTime = spanner.CommitTimestamp

		saved = true
		taskCreated = createTasksIfNeeded(ctx, entry)
		if taskCreated {
			entry.TaskCount = 1
		}

		if err := control.InsertOrUpdate(ctx, entry); err != nil {
			return err
		}
		// Will only populated if IsPresubmit is not empty.
		cvProject = entry.PresubmitProject
		return nil
	}
	if _, err := span.ReadWriteTransaction(ctx, f); err != nil {
		return err
	}
	if !saved {
		logging.Warningf(ctx, "build result for ingestion %q was dropped as one was already recorded", buildID)
	}

	// Export metrics.
	if saved && isPresubmit {
		bbPresubmitBuildInputCounter.Add(ctx, 1, buildProject)
	}
	if taskCreated {
		if isPresubmit {
			bbPresubmitBuildOutputCounter.Add(ctx, 1, buildProject)
			cvPresubmitBuildOutputCounter.Add(ctx, 1, cvProject)
		} else {
			outputCIBuildCounter.Add(ctx, 1, buildProject)
		}
	}
	return nil
}

// JoinPresubmitResult sets the presubmit result for the given builds.
//
// Ingestion task(s) are created for builds where all required data
// is available (i.e. after the build result has also joined).
//
// If the presubmit result has already been provided for a build,
// this method has no effect.
func JoinPresubmitResult(ctx context.Context, presubmitResultByBuildID map[string]*ctlpb.PresubmitResult, presubmitProject string) error {
	for id, result := range presubmitResultByBuildID {
		if result == nil {
			return fmt.Errorf("presubmit result for build %v must be specified", id)
		}
	}

	var buildIDsSkipped []string
	var buildsOutputByBuildProject map[string]int64
	f := func(ctx context.Context) error {
		// Clear variables to ensure nothing from a previous (failed)
		// try of this transaction leaks out to the outer context.
		buildIDsSkipped = nil
		buildsOutputByBuildProject = make(map[string]int64)

		var buildIDs []string
		for id := range presubmitResultByBuildID {
			buildIDs = append(buildIDs, id)
		}

		entries, err := control.Read(ctx, buildIDs)
		if err != nil {
			return err
		}
		for i, entry := range entries {
			buildID := buildIDs[i]
			if entry == nil {
				// Record does not exist. Create it.
				entry = &control.Entry{
					BuildID:     buildID,
					IsPresubmit: true,
				}
			}
			if !entry.IsPresubmit {
				return fmt.Errorf("attempt to save presubmit result on build (%q) not marked as presubmit", buildID)
			}
			if entry.PresubmitResult != nil {
				// Presubmit result already recorded. Do not modify and do not
				// create a duplicate ingestion.
				buildIDsSkipped = append(buildIDsSkipped, buildID)
				continue
			}
			entry.PresubmitProject = presubmitProject
			entry.PresubmitResult = presubmitResultByBuildID[buildID]
			entry.PresubmitJoinedTime = spanner.CommitTimestamp

			taskCreated := createTasksIfNeeded(ctx, entry)
			if taskCreated {
				buildsOutputByBuildProject[entry.BuildProject]++
				entry.TaskCount = 1
			}
			if err := control.InsertOrUpdate(ctx, entry); err != nil {
				return err
			}
		}
		return nil
	}
	if _, err := span.ReadWriteTransaction(ctx, f); err != nil {
		return err
	}
	if len(buildIDsSkipped) > 0 {
		logging.Warningf(ctx, "presubmit result for builds %v were dropped as one was already recorded", buildIDsSkipped)
	}

	// Export metrics.
	cvPresubmitBuildInputCounter.Add(ctx, int64(len(presubmitResultByBuildID)-len(buildIDsSkipped)), presubmitProject)
	for buildProject, count := range buildsOutputByBuildProject {
		bbPresubmitBuildOutputCounter.Add(ctx, count, buildProject)
		cvPresubmitBuildOutputCounter.Add(ctx, count, presubmitProject)
	}
	return nil
}

// createTaskIfNeeded transactionally creates a test-result-ingestion task
// if all necessary data for the ingestion is available. Returns true if the
// task was created.
func createTasksIfNeeded(ctx context.Context, e *control.Entry) bool {
	if e.BuildResult == nil || (e.IsPresubmit && e.PresubmitResult == nil) {
		return false
	}

	var itrTask *taskspb.IngestTestResults
	if e.IsPresubmit {
		itrTask = &taskspb.IngestTestResults{
			PartitionTime: e.PresubmitResult.CreationTime,
			Build:         e.BuildResult,
			PresubmitRun:  e.PresubmitResult,
		}
	} else {
		itrTask = &taskspb.IngestTestResults{
			PartitionTime: e.BuildResult.CreationTime,
			Build:         e.BuildResult,
		}
	}

	// Copy the task to avoid aliasing issues if the caller ever
	// decides the modify e.PresubmitResult or e.BuildResult
	// after we return.
	itrTask = proto.Clone(itrTask).(*taskspb.IngestTestResults)
	resultingester.Schedule(ctx, itrTask)

	return true
}
