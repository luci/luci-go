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

package resultingester

import (
	"context"
	"strings"
	"time"

	"cloud.google.com/go/spanner"
	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/common/trace"
	rdbpbutil "go.chromium.org/luci/resultdb/pbutil"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/ingestion/resultdb"
	"go.chromium.org/luci/analysis/internal/perms"
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
	"go.chromium.org/luci/analysis/internal/testresults"
	"go.chromium.org/luci/analysis/internal/testresults/gitreferences"
	"go.chromium.org/luci/analysis/pbutil"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

// maximumCLs is the maximum number of changelists to capture from any
// buildbucket run, after which the CL list is truncated. This avoids builds
// with an excessive number of included CLs from storing an excessive amount
// of data per failure.
const maximumCLs = 10

// extractGitReference extracts the git reference used to number the commit
// tested by the given build.
func extractGitReference(project string, commit *bbpb.GitilesCommit) *gitreferences.GitReference {
	return &gitreferences.GitReference{
		Project:          project,
		GitReferenceHash: gitreferences.GitReferenceHash(commit.Host, commit.Project, commit.Ref),
		Hostname:         commit.Host,
		Repository:       commit.Project,
		Reference:        commit.Ref,
	}
}

// extractIngestionContext extracts the ingested invocation and
// the git reference tested (if any).
func extractIngestionContext(task *taskspb.IngestTestResults, inv *rdbpb.Invocation) (*testresults.IngestedInvocation, *gitreferences.GitReference, error) {
	invID, err := rdbpbutil.ParseInvocationName(inv.Name)
	if err != nil {
		// This should never happen. Inv was originated from ResultDB.
		panic(err)
	}

	proj, subRealm, err := perms.SplitRealm(inv.Realm)
	if err != nil {
		return nil, nil, errors.Annotate(err, "invocation has invalid realm: %q", inv.Realm).Err()
	}
	if proj != task.Build.Project {
		return nil, nil, errors.Reason("invocation project (%q) does not match build project (%q) for build %s-%d",
			proj, task.Build.Project, task.Build.Host, task.Build.Id).Err()
	}

	gerritChanges := task.Build.Changelists
	changelists := make([]testresults.Changelist, 0, len(gerritChanges))
	for _, change := range gerritChanges {
		if err := testresults.ValidateGerritHostname(change.Host); err != nil {
			return nil, nil, err
		}
		changelists = append(changelists, testresults.Changelist{
			Host:      change.Host,
			Change:    change.Change,
			Patchset:  change.Patchset,
			OwnerKind: change.OwnerKind,
		})
	}
	// TODO (b/258734241): This has been migrated to join-build task queue.
	// Remove when old build completions flushed out.
	// Store the tested changelists in sorted order. This ensures that for
	// the same combination of CLs tested, the arrays are identical.
	testresults.SortChangelists(changelists)

	// TODO (b/258734241): This has been migrated to join-build task queue.
	// Remove when old build completions flushed out.
	// Truncate the list of changelists to avoid storing an excessive number.
	// Apply truncation after sorting to ensure a stable set of changelists.
	if len(changelists) > maximumCLs {
		changelists = changelists[:maximumCLs]
	}

	var presubmitRun *testresults.PresubmitRun
	if task.PresubmitRun != nil {
		presubmitRun = &testresults.PresubmitRun{
			Mode: task.PresubmitRun.Mode,
		}
	}

	invocation := &testresults.IngestedInvocation{
		Project:              proj,
		IngestedInvocationID: invID,
		SubRealm:             subRealm,
		PartitionTime:        task.PartitionTime.AsTime(),
		BuildStatus:          task.Build.Status,
		PresubmitRun:         presubmitRun,
		Changelists:          changelists,
	}

	commit := task.Build.Commit
	var gitRef *gitreferences.GitReference
	if commit != nil {
		gitRef = extractGitReference(proj, commit)
		invocation.GitReferenceHash = gitRef.GitReferenceHash
		invocation.CommitPosition = int64(commit.Position)
		invocation.CommitHash = strings.ToLower(commit.Id)
	}
	return invocation, gitRef, nil
}

func recordIngestionContext(ctx context.Context, inv *testresults.IngestedInvocation, gitRef *gitreferences.GitReference) error {
	// Update the IngestedInvocations table.
	m := inv.SaveUnverified()

	_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		span.BufferWrite(ctx, m)

		if gitRef != nil {
			// Ensure the git reference (if any) exists in the GitReferences table.
			if err := gitreferences.EnsureExists(ctx, gitRef); err != nil {
				return errors.Annotate(err, "ensuring git reference").Err()
			}
		}
		return nil
	})
	return err
}

type batch struct {
	// The test realms to insert/update.
	// Test realms should be inserted before any test variant realms.
	testRealms []*spanner.Mutation
	// The test variant realms to insert/update.
	// Test variant realms should be inserted before any test results.
	testVariantRealms []*spanner.Mutation
	// Test results to insert. Already prepared as Spanner mutations.
	testResults []*spanner.Mutation
}

func batchTestResults(inv *testresults.IngestedInvocation, tvs []*rdbpb.TestVariant, outputC chan batch) {
	// Must be selected such that no more than 20,000 mutations occur in
	// one transaction in the worst case.
	const batchSize = 900

	var trs []*spanner.Mutation
	var tvrs []*spanner.Mutation
	testRealmSet := make(map[testresults.TestRealm]bool, batchSize)
	startBatch := func() {
		trs = make([]*spanner.Mutation, 0, batchSize)
		tvrs = make([]*spanner.Mutation, 0, batchSize)
		testRealmSet = make(map[testresults.TestRealm]bool, batchSize)
	}
	outputBatch := func() {
		if len(trs) == 0 {
			// This should never happen.
			panic("Pushing empty batch")
		}
		testRealms := make([]*spanner.Mutation, 0, len(testRealmSet))
		for testRealm := range testRealmSet {
			testRealms = append(testRealms, testRealm.SaveUnverified())
		}

		outputC <- batch{
			testRealms:        testRealms,
			testVariantRealms: tvrs,
			testResults:       trs,
		}
	}

	startBatch()
	for _, tv := range tvs {
		// Limit batch size.
		// Keep all results for one test variant in one batch, so that the
		// TestVariantRealm record is kept together with the test results.
		if len(trs) > batchSize {
			outputBatch()
			startBatch()
		}

		testRealm := testresults.TestRealm{
			Project:           inv.Project,
			TestID:            tv.TestId,
			SubRealm:          inv.SubRealm,
			LastIngestionTime: spanner.CommitTimestamp,
		}
		testRealmSet[testRealm] = true

		tvr := testresults.TestVariantRealm{
			Project:           inv.Project,
			TestID:            tv.TestId,
			VariantHash:       tv.VariantHash,
			SubRealm:          inv.SubRealm,
			Variant:           pbutil.VariantFromResultDB(tv.Variant),
			LastIngestionTime: spanner.CommitTimestamp,
		}
		tvrs = append(tvrs, tvr.SaveUnverified())

		exonerationReasons := make([]pb.ExonerationReason, 0, len(tv.Exonerations))
		for _, ex := range tv.Exonerations {
			exonerationReasons = append(exonerationReasons, pbutil.ExonerationReasonFromResultDB(ex.Reason))
		}

		// Group results into test runs and order them by start time.
		resultsByRun := resultdb.GroupAndOrderTestResults(tv.Results)
		for runIndex, run := range resultsByRun {
			for resultIndex, inputTR := range run {
				tr := testresults.TestResult{
					Project:              inv.Project,
					TestID:               tv.TestId,
					PartitionTime:        inv.PartitionTime,
					VariantHash:          tv.VariantHash,
					IngestedInvocationID: inv.IngestedInvocationID,
					RunIndex:             int64(runIndex),
					ResultIndex:          int64(resultIndex),
					IsUnexpected:         !inputTR.Result.Expected,
					Status:               pbutil.TestResultStatusFromResultDB(inputTR.Result.Status),
					ExonerationReasons:   exonerationReasons,
					SubRealm:             inv.SubRealm,
					BuildStatus:          inv.BuildStatus,
					PresubmitRun:         inv.PresubmitRun,
					GitReferenceHash:     inv.GitReferenceHash,
					CommitPosition:       inv.CommitPosition,
					Changelists:          inv.Changelists,
				}
				if inputTR.Result.Duration != nil {
					d := new(time.Duration)
					*d = inputTR.Result.Duration.AsDuration()
					tr.RunDuration = d
				}
				// Convert the test result into a mutation immediately
				// to avoid storing both the TestResult object and
				// mutation object in memory until the transaction
				// commits.
				trs = append(trs, tr.SaveUnverified())
			}
		}
	}
	if len(trs) > 0 {
		outputBatch()
	}
}

// recordTestResults records test results from an test-verdict-ingestion task.
func recordTestResults(ctx context.Context, inv *testresults.IngestedInvocation, tvs []*rdbpb.TestVariant) (err error) {
	ctx, s := trace.StartSpan(ctx, "go.chromium.org/luci/analysis/internal/services/resultingester.recordTestResults")
	defer func() { s.End(err) }()

	const workerCount = 8

	return parallel.WorkPool(workerCount, func(c chan<- func() error) {
		batchC := make(chan batch)

		c <- func() error {
			defer close(batchC)
			batchTestResults(inv, tvs, batchC)
			return nil
		}

		for batch := range batchC {
			// Bind to a local variable so it can be used in a goroutine without being
			// overwritten. See https://go.dev/doc/faq#closures_and_goroutines
			batch := batch

			c <- func() error {
				// Write to different tables in different transactions to minimize the
				// number of splits involved in each transaction.
				_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
					span.BufferWrite(ctx, batch.testRealms...)
					return nil
				})
				if err != nil {
					return errors.Annotate(err, "inserting test realms").Err()
				}
				_, err = span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
					span.BufferWrite(ctx, batch.testVariantRealms...)
					return nil
				})
				if err != nil {
					return errors.Annotate(err, "inserting test variant realms").Err()
				}
				_, err = span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
					span.BufferWrite(ctx, batch.testResults...)
					return nil
				})
				if err != nil {
					return errors.Annotate(err, "inserting test results").Err()
				}
				return nil
			}
		}
	})
}
