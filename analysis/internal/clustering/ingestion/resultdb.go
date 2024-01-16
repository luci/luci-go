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

package ingestion

import (
	"fmt"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	rdbpb "go.chromium.org/luci/resultdb/proto/v1"

	cpb "go.chromium.org/luci/analysis/internal/clustering/proto"
	"go.chromium.org/luci/analysis/internal/ingestion/resultdb"
	"go.chromium.org/luci/analysis/pbutil"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

func failuresFromTestVariant(opts Options, tv TestVerdict) []*cpb.Failure {
	var failures []*cpb.Failure
	if tv.Verdict.Status == rdbpb.TestVariantStatus_EXPECTED {
		// Short circuit: There will be nothing in the test variant to
		// ingest, as everything is expected.
		return nil
	}

	// Whether there were any (non-skip) passed or expected results.
	var hasPass bool
	for _, tr := range tv.Verdict.Results {
		if tr.Result.Status != rdbpb.TestStatus_SKIP &&
			(tr.Result.Status == rdbpb.TestStatus_PASS ||
				tr.Result.Expected) {
			hasPass = true
		}
	}

	// Group test results by run and sort in order of start time.
	resultsByRun := resultdb.GroupAndOrderTestResults(tv.Verdict.Results)

	resultIndex := 0
	for _, run := range resultsByRun {
		// Whether there were any passed or expected results in the run.
		var testRunHasPass bool
		for _, tr := range run {
			if tr.Result.Status != rdbpb.TestStatus_SKIP &&
				(tr.Result.Status == rdbpb.TestStatus_PASS ||
					tr.Result.Expected) {
				testRunHasPass = true
			}
		}

		for i, tr := range run {
			if tr.Result.Expected || !isFailure(tr.Result.Status) {
				// Only unexpected failures are ingested for clustering.
				resultIndex++
				continue
			}

			failure := failureFromResult(tr.Result, tv, opts)
			failure.IngestedInvocationResultIndex = int64(resultIndex)
			failure.IngestedInvocationResultCount = int64(len(tv.Verdict.Results))
			failure.IsIngestedInvocationBlocked = !hasPass
			failure.TestRunResultIndex = int64(i)
			failure.TestRunResultCount = int64(len(run))
			failure.IsTestRunBlocked = !testRunHasPass
			failures = append(failures, failure)

			resultIndex++
		}
	}
	return failures
}

func isFailure(s rdbpb.TestStatus) bool {
	return (s == rdbpb.TestStatus_ABORT ||
		s == rdbpb.TestStatus_CRASH ||
		s == rdbpb.TestStatus_FAIL)
}

func failureFromResult(tr *rdbpb.TestResult, tv TestVerdict, opts Options) *cpb.Failure {
	exonerations := make([]*cpb.TestExoneration, 0, len(tv.Verdict.Exonerations))
	for _, e := range tv.Verdict.Exonerations {
		exonerations = append(exonerations, exonerationFromResultDB(e))
	}

	var presubmitRun *cpb.PresubmitRun
	var buildCritical *bool
	if opts.PresubmitRun != nil {
		presubmitRun = &cpb.PresubmitRun{
			PresubmitRunId: opts.PresubmitRun.ID,
			Owner:          opts.PresubmitRun.Owner,
			Mode:           opts.PresubmitRun.Mode,
			Status:         opts.PresubmitRun.Status,
		}
		buildCritical = &opts.BuildCritical
	}

	testRunID, err := resultdb.InvocationFromTestResultName(tr.Name)
	if err != nil {
		// Should never happen, as the result name from ResultDB
		// should be valid.
		panic(err)
	}

	result := &cpb.Failure{
		TestResultId:                  pbutil.TestResultIDFromResultDB(tr.Name),
		PartitionTime:                 timestamppb.New(opts.PartitionTime),
		ChunkIndex:                    0, // To be populated by chunking.
		Realm:                         opts.Realm,
		TestId:                        tv.Verdict.TestId,                              // Get from variant, as it is not populated on each result.
		Variant:                       pbutil.VariantFromResultDB(tv.Verdict.Variant), // Get from variant, as it is not populated on each result.
		Tags:                          pbutil.StringPairFromResultDB(tr.Tags),
		VariantHash:                   tv.Verdict.VariantHash, // Get from variant, as it is not populated on each result.
		FailureReason:                 pbutil.FailureReasonFromResultDB(tr.FailureReason),
		BugTrackingComponent:          extractBugTrackingComponent(tr.Tags, tv.Verdict.TestMetadata, opts.PreferBuganizerComponents),
		StartTime:                     tr.StartTime,
		Duration:                      tr.Duration,
		Exonerations:                  exonerations,
		PresubmitRun:                  presubmitRun,
		BuildStatus:                   opts.BuildStatus,
		BuildCritical:                 buildCritical,
		IngestedInvocationId:          opts.InvocationID,
		IngestedInvocationResultIndex: -1,    // To be populated by caller.
		IngestedInvocationResultCount: -1,    // To be populated by caller.
		IsIngestedInvocationBlocked:   false, // To be populated by caller.
		TestRunId:                     testRunID,
		TestRunResultIndex:            -1,    // To be populated by caller.
		TestRunResultCount:            -1,    // To be populated by caller.
		IsTestRunBlocked:              false, // To be populated by caller.
		Sources:                       tv.Sources,
		BuildGardenerRotations:        opts.BuildGardenerRotations,
		TestVariantBranch:             tv.TestVariantBranch,
	}

	// Copy the result to avoid the result aliasing any of the protos used as input.
	return proto.Clone(result).(*cpb.Failure)
}

func exonerationFromResultDB(e *rdbpb.TestExoneration) *cpb.TestExoneration {
	return &cpb.TestExoneration{
		Reason: pbutil.ExonerationReasonFromResultDB(e.Reason),
	}
}

func extractBugTrackingComponent(tags []*rdbpb.StringPair, testMetadata *rdbpb.TestMetadata, preferBuganizerComponents bool) *pb.BugTrackingComponent {
	if testMetadata != nil && testMetadata.BugComponent != nil {
		bc := testMetadata.BugComponent
		switch bcSystem := bc.System.(type) {
		case *rdbpb.BugComponent_IssueTracker:
			return &pb.BugTrackingComponent{
				System:    "buganizer",
				Component: fmt.Sprint(bcSystem.IssueTracker.ComponentId),
			}
		case *rdbpb.BugComponent_Monorail:
			return &pb.BugTrackingComponent{
				System:    "monorail",
				Component: bcSystem.Monorail.Value,
			}
		}
	} else {
		var buganizerComponent string
		for _, tag := range tags {
			if tag.Key == "public_buganizer_component" {
				buganizerComponent = tag.Value
				break
			}
		}
		var monorailComponent string
		for _, tag := range tags {
			if tag.Key == "monorail_component" {
				monorailComponent = tag.Value
				break
			}
		}

		if preferBuganizerComponents && buganizerComponent != "" {
			return &pb.BugTrackingComponent{
				System:    "buganizer",
				Component: buganizerComponent,
			}
		}

		if monorailComponent != "" {
			return &pb.BugTrackingComponent{
				System:    "monorail",
				Component: monorailComponent,
			}
		}

		if buganizerComponent != "" {
			return &pb.BugTrackingComponent{
				System:    "buganizer",
				Component: buganizerComponent,
			}
		}
	}

	return nil
}
