// Copyright 2025 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package pubsub

import (
	"context"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/tq"
	"go.chromium.org/luci/server/tq/tqtesting"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/tasks/taskspb"
	"go.chromium.org/luci/resultdb/internal/testresults"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/internal/workunits"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// sizeLimitedTestResult returns a TestResultRow builder with a SummaryHtml
// field padded to approximately the targetSizeInBytes.
func sizeLimitedTestResult(invID string, testID string, resultID string, targetSizeInBytes int) *testresults.Builder {
	b := testresults.NewBuilder(invocations.ID(invID), testID, resultID).WithMinimalFields().WithDuration(&durationpb.Duration{Seconds: 0})
	tr := b.Build().ToProto()

	// Adjust padding size to account for the base size of the proto.
	baseSize := proto.Size(tr)
	paddingSize := targetSizeInBytes - baseSize
	if paddingSize < 0 {
		paddingSize = 0
	}
	summaryHTML := strings.Repeat("a", paddingSize)
	b.WithSummaryHTML(summaryHTML)

	// Recalculate size to be sure.
	finalSize := proto.Size(b.Build().ToProto())
	if finalSize > targetSizeInBytes {
		// If we overshot, trim the summary. This is not perfect but good enough for testing.
		diff := finalSize - targetSizeInBytes
		if len(summaryHTML) > diff {
			b.WithSummaryHTML(summaryHTML[:len(summaryHTML)-diff])
		}
	}
	return b
}

// createExpectedTR creates an expected TestResult proto for comparison.
func createExpectedTR(b *testresults.Builder, rootInvID string, wuID string) *pb.TestResult {
	trRow := b.Build()
	tr := trRow.ToProto()
	expectedTR := proto.Clone(tr).(*pb.TestResult)
	expectedTR.Name = pbutil.TestResultName(rootInvID, wuID, trRow.TestID, trRow.ResultID)
	if expectedTR.Variant == nil {
		expectedTR.Variant = &pb.Variant{}
	}
	if expectedTR.TestIdStructured != nil && expectedTR.TestIdStructured.ModuleVariant == nil {
		expectedTR.TestIdStructured.ModuleVariant = &pb.Variant{}
	}
	if expectedTR.Duration == nil {
		expectedTR.Duration = &durationpb.Duration{Seconds: 0}
	}
	if expectedTR.VariantHash == "" {
		expectedTR.VariantHash = pbutil.VariantHash(expectedTR.Variant)
	}
	if expectedTR.TestIdStructured == nil {
		expectedTR.TestIdStructured, _ = pbutil.ParseStructuredTestIdentifierForOutput(expectedTR.TestId, expectedTR.Variant)
	}
	return expectedTR
}

// runTasks runs all pending publish-test-results tasks in the scheduler.
func runTasks(ctx context.Context, t *testing.T, sched *tqtesting.Scheduler, rdbHost string, pageSize int) {
	processed := 0
	for {
		allTasks := sched.Tasks()
		if processed >= len(allTasks) {
			break
		}
		toProcess := allTasks[processed:]
		processed = len(allTasks)

		for _, task := range toProcess {
			if task.Class != "publish-test-results" {
				continue
			}
			payload := task.Payload.(*taskspb.PublishTestResultsTask)
			p := &testResultsPublisher{
				task:             payload,
				resultDBHostname: rdbHost,
				pageSize:         pageSize,
			}
			err := p.handleTestResultsPublisher(ctx)
			assert.Loosely(t, err, should.BeNil)
		}
	}
}

// TestHandlePublishTestResultsTask tests the handlePublishTestResultsTask function.
func TestHandlePublishTestResultsTask(t *testing.T) {
	t.Run("HandlePublishTestResultsTask", func(t *testing.T) {
		ctx := testutil.SpannerTestContext(t)
		rootInvID := rootinvocations.ID("test-root-inv")
		rdbHost := "staging.results.api.cr.dev"

		wuID1 := workunits.ID{RootInvocationID: rootInvID, WorkUnitID: "wu1"}
		wuID2 := workunits.ID{RootInvocationID: rootInvID, WorkUnitID: "wu2"}

		// Insert legacy Invocations based on work unit IDs.
		testutil.MustApply(ctx, t, insert.Invocation(wuID1.LegacyInvocationID(), pb.Invocation_ACTIVE, nil))
		testutil.MustApply(ctx, t, insert.Invocation(wuID2.LegacyInvocationID(), pb.Invocation_ACTIVE, nil))

		// Insert TestResults.
		tr1 := testresults.NewBuilder(wuID1.LegacyInvocationID(), "testA", "0").WithMinimalFields().WithStatus(pb.TestStatus_PASS).WithDuration(&durationpb.Duration{Seconds: 0})
		tr2 := testresults.NewBuilder(wuID2.LegacyInvocationID(), "testB", "0").WithMinimalFields().WithStatus(pb.TestStatus_FAIL).WithDuration(&durationpb.Duration{Seconds: 0})

		expectedTRs1 := []*pb.TestResult{createExpectedTR(tr1, string(rootInvID), wuID1.WorkUnitID)}
		expectedTRs2 := []*pb.TestResult{createExpectedTR(tr2, string(rootInvID), wuID2.WorkUnitID)}

		gitilesSources := &pb.Sources{
			BaseSources: &pb.Sources_GitilesCommit{
				GitilesCommit: &pb.GitilesCommit{
					Host:       "test.googlesource.com",
					Project:    "test/project",
					Ref:        "refs/heads/main",
					CommitHash: "abcdef",
				},
			},
		}
		androidBuildDescriptor := &pb.BuildDescriptor{
			Definition: &pb.BuildDescriptor_AndroidBuild{
				AndroidBuild: &pb.AndroidBuildDescriptor{
					Branch:      "git_main",
					BuildTarget: "test-target",
				},
			},
		}
		rootInvDefinition := &pb.RootInvocationDefinition{
			System: "atp",
			Name:   "test-definition",
		}

		testCases := []struct {
			name                 string
			rootInvBuilder       *rootinvocations.Builder
			finalizedWUIDs       []string
			expectedAttributes   map[string]string
			expectedNotification *pb.TestResultsNotification
		}{
			{
				name:           "StreamingExportState not METADATA_FINAL",
				rootInvBuilder: rootinvocations.NewBuilder(rootInvID).WithStreamingExportState(pb.RootInvocation_STREAMING_EXPORT_STATE_UNSPECIFIED),
				finalizedWUIDs: []string{wuID1.WorkUnitID},
			},
			{
				name:               "No Sources or PrimaryBuild",
				rootInvBuilder:     rootinvocations.NewBuilder(rootInvID).WithStreamingExportState(pb.RootInvocation_METADATA_FINAL).WithSources(nil).WithPrimaryBuild(nil).WithDefinition(rootInvDefinition),
				finalizedWUIDs:     []string{wuID1.WorkUnitID},
				expectedAttributes: map[string]string{luciProjectFilter: "testproject", definitionNameFilter: "test-definition"},
				expectedNotification: &pb.TestResultsNotification{
					ResultdbHost: rdbHost,
					TestResultsByWorkUnit: []*pb.TestResultsNotification_TestResultsByWorkUnit{
						{WorkUnitName: wuID1.Name(), TestResults: expectedTRs1},
					},
				},
			},
			{
				name:               "Android Branch from PrimaryBuild",
				rootInvBuilder:     rootinvocations.NewBuilder(rootInvID).WithStreamingExportState(pb.RootInvocation_METADATA_FINAL).WithPrimaryBuild(androidBuildDescriptor).WithSources(gitilesSources).WithDefinition(rootInvDefinition),
				finalizedWUIDs:     []string{wuID1.WorkUnitID},
				expectedAttributes: map[string]string{luciProjectFilter: "testproject", androidBranchFilter: "git_main", androidTargetFilter: "test-target", definitionNameFilter: "test-definition"},
				expectedNotification: &pb.TestResultsNotification{
					ResultdbHost: rdbHost,
					TestResultsByWorkUnit: []*pb.TestResultsNotification_TestResultsByWorkUnit{
						{WorkUnitName: wuID1.Name(), TestResults: expectedTRs1},
					},
					Sources: gitilesSources,
				},
			},
			{
				name:               "Multiple Work Units",
				rootInvBuilder:     rootinvocations.NewBuilder(rootInvID).WithStreamingExportState(pb.RootInvocation_METADATA_FINAL).WithPrimaryBuild(androidBuildDescriptor).WithSources(gitilesSources).WithDefinition(rootInvDefinition),
				finalizedWUIDs:     []string{wuID1.WorkUnitID, wuID2.WorkUnitID},
				expectedAttributes: map[string]string{luciProjectFilter: "testproject", androidBranchFilter: "git_main", androidTargetFilter: "test-target", definitionNameFilter: "test-definition"},
				expectedNotification: &pb.TestResultsNotification{
					ResultdbHost: rdbHost,
					TestResultsByWorkUnit: []*pb.TestResultsNotification_TestResultsByWorkUnit{
						{WorkUnitName: wuID1.Name(), TestResults: expectedTRs1},
						{WorkUnitName: wuID2.Name(), TestResults: expectedTRs2},
					},
					Sources: gitilesSources,
				},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				ctx := testutil.SpannerTestContext(t)
				ctx, sched := tq.TestingContext(ctx, nil)
				t.Logf("Running test case: %s", tc.name)

				// Insert the root invocation for this test case.
				muts := rootinvocations.InsertForTesting(tc.rootInvBuilder.Build())
				testutil.MustApply(ctx, t, muts...)
				t.Logf("Inserted root invocation: %s", tc.rootInvBuilder.Build().RootInvocationID)

				// Insert legacy Invocations based on work unit IDs.
				testutil.MustApply(ctx, t, insert.Invocation(wuID1.LegacyInvocationID(), pb.Invocation_ACTIVE, nil))
				testutil.MustApply(ctx, t, insert.Invocation(wuID2.LegacyInvocationID(), pb.Invocation_ACTIVE, nil))

				// Insert TestResults.
				testutil.MustApply(ctx, t, testresults.InsertForTesting(tr1.Build()))
				testutil.MustApply(ctx, t, testresults.InsertForTesting(tr2.Build()))

				task := &taskspb.PublishTestResultsTask{
					RootInvocationId: string(rootInvID),
					WorkUnitIds:      tc.finalizedWUIDs,
				}
				p := &testResultsPublisher{
					task:             task,
					resultDBHostname: rdbHost,
					pageSize:         100,
				}
				err := p.handleTestResultsPublisher(ctx)
				if tc.name == "StreamingExportState not METADATA_FINAL" {
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, sched.Tasks(), should.HaveLength(0))
					return
				}
				assert.Loosely(t, err, should.BeNil)

				// Run all continuation tasks.
				runTasks(ctx, t, sched, rdbHost, 100)

				allTasks := sched.Tasks()
				var tasks tqtesting.TaskList
				for _, task := range allTasks {
					if task.Class == "notify-test-results" {
						tasks = append(tasks, task)
					}
				}

				if tc.expectedNotification == nil {
					assert.Loosely(t, tasks, should.HaveLength(0))
					return
				}
				assert.Loosely(t, tasks, should.HaveLength(1))
				notifyTask := tasks[0]

				// Ignore TQ internal attribute.
				attrs := notifyTask.Message.GetAttributes()
				delete(attrs, "X-Luci-Tq-Reminder-Id")
				assert.Loosely(t, attrs, should.Match(tc.expectedAttributes))

				payload := notifyTask.Payload.(*taskspb.PublishTestResults)

				// Calculate the expected deduplication key separately.
				tc.expectedNotification.DeduplicationKey = generateDeduplicationKey(payload.Message.TestResultsByWorkUnit)
				assert.Loosely(t, payload.Message, should.Match(tc.expectedNotification))
			})
		}
	})
}

// TestHandlePublishTestResultsTask_Batching tests the batching logic in handlePublishTestResultsTask.
func TestHandlePublishTestResultsTask_Batching(t *testing.T) {
	t.Run("HandlePublishTestResultsTask_Batching", func(t *testing.T) {
		ctx := testutil.SpannerTestContext(t)
		rootInvID := rootinvocations.ID("test-root-inv-batch")
		rdbHost := "staging.results.api.cr.dev"

		wuID1 := workunits.ID{RootInvocationID: rootInvID, WorkUnitID: "wu1"}
		wuID2 := workunits.ID{RootInvocationID: rootInvID, WorkUnitID: "wu2"}

		testutil.MustApply(ctx, t, insert.Invocation(wuID1.LegacyInvocationID(), pb.Invocation_ACTIVE, nil))
		testutil.MustApply(ctx, t, insert.Invocation(wuID2.LegacyInvocationID(), pb.Invocation_ACTIVE, nil))

		// WU1 has two results, each ~6MB. These should be in separate batches for this WU.
		tr1 := sizeLimitedTestResult(string(wuID1.LegacyInvocationID()), "testA", "res1", 6*1024*1024)
		tr2 := sizeLimitedTestResult(string(wuID1.LegacyInvocationID()), "testA", "res2", 6*1024*1024)

		// WU2 has two results, ~3MB each. These should be in the same batch.
		tr3 := sizeLimitedTestResult(string(wuID2.LegacyInvocationID()), "testB", "res1", 3*1024*1024)
		tr4 := sizeLimitedTestResult(string(wuID2.LegacyInvocationID()), "testB", "res2", 3*1024*1024)

		testutil.MustApply(ctx, t, testresults.InsertForTesting(tr1.Build()))
		testutil.MustApply(ctx, t, testresults.InsertForTesting(tr2.Build()))
		testutil.MustApply(ctx, t, testresults.InsertForTesting(tr3.Build()))
		testutil.MustApply(ctx, t, testresults.InsertForTesting(tr4.Build()))

		gitilesSources := &pb.Sources{
			BaseSources: &pb.Sources_GitilesCommit{
				GitilesCommit: &pb.GitilesCommit{
					Host:       "test.googlesource.com",
					Project:    "test/project",
					Ref:        "refs/heads/main",
					CommitHash: "abcdef",
				},
			},
		}
		rootInvBuilder := rootinvocations.NewBuilder(rootInvID).WithStreamingExportState(pb.RootInvocation_METADATA_FINAL).WithSources(gitilesSources)
		testutil.MustApply(ctx, t, rootinvocations.InsertForTesting(rootInvBuilder.Build())...)
		ctx, sched := tq.TestingContext(ctx, nil)
		task := &taskspb.PublishTestResultsTask{
			RootInvocationId: string(rootInvID),
			WorkUnitIds:      []string{wuID1.WorkUnitID, wuID2.WorkUnitID},
		}

		p := &testResultsPublisher{
			task:             task,
			resultDBHostname: rdbHost,
			pageSize:         100,
		}
		err := p.handleTestResultsPublisher(ctx)
		assert.Loosely(t, err, should.BeNil)

		// Run all continuation tasks.
		runTasks(ctx, t, sched, rdbHost, 100)

		allTasks := sched.Tasks()
		var actualMessages []*pb.TestResultsNotification

		for _, task := range allTasks {
			if task.Class == "notify-test-results" {
				payload := task.Payload.(*taskspb.PublishTestResults)
				actualMessages = append(actualMessages, payload.Message)
			}
		}

		// Prepare expected results for comparison
		expectedTR1 := createExpectedTR(tr1, string(rootInvID), wuID1.WorkUnitID)
		expectedTR2 := createExpectedTR(tr2, string(rootInvID), wuID1.WorkUnitID)
		expectedTR3 := createExpectedTR(tr3, string(rootInvID), wuID2.WorkUnitID)
		expectedTR4 := createExpectedTR(tr4, string(rootInvID), wuID2.WorkUnitID)

		// Expected messages based on size and batching logic.
		// WU1's results are batched separately because each is > testMaxSize/2.
		// WU2's results fit in one batch.
		expectedWUBatch1 := &pb.TestResultsNotification{
			ResultdbHost: rdbHost,
			Sources:      gitilesSources,
			TestResultsByWorkUnit: []*pb.TestResultsNotification_TestResultsByWorkUnit{
				{WorkUnitName: wuID1.Name(), TestResults: []*pb.TestResult{expectedTR1}},
			},
		}
		expectedWUBatch1.DeduplicationKey = generateDeduplicationKey(expectedWUBatch1.TestResultsByWorkUnit)

		expectedWUBatch2 := &pb.TestResultsNotification{
			ResultdbHost: rdbHost,
			Sources:      gitilesSources,
			TestResultsByWorkUnit: []*pb.TestResultsNotification_TestResultsByWorkUnit{
				{WorkUnitName: wuID1.Name(), TestResults: []*pb.TestResult{expectedTR2}},
			},
		}
		expectedWUBatch2.DeduplicationKey = generateDeduplicationKey(expectedWUBatch2.TestResultsByWorkUnit)

		expectedWUBatch3 := &pb.TestResultsNotification{
			ResultdbHost: rdbHost,
			Sources:      gitilesSources,
			TestResultsByWorkUnit: []*pb.TestResultsNotification_TestResultsByWorkUnit{
				{WorkUnitName: wuID2.Name(), TestResults: []*pb.TestResult{expectedTR3, expectedTR4}},
			},
		}
		expectedWUBatch3.DeduplicationKey = generateDeduplicationKey(expectedWUBatch3.TestResultsByWorkUnit)

		assert.Loosely(t, actualMessages, should.HaveLength(3))
		assert.Loosely(t, actualMessages, should.Match([]*pb.TestResultsNotification{expectedWUBatch1, expectedWUBatch2, expectedWUBatch3}))
	})
}

// TestHandlePublishTestResultsTask_FlushPriorCollectedResultsIfNextPageTooLarge tests that if a single page
// of results is too large, we flush previously collected results first.
func TestHandlePublishTestResultsTask_FlushPriorCollectedResultsIfNextPageTooLarge(t *testing.T) {
	t.Run("HandlePublishTestResultsTask_Scenario1_Flush", func(t *testing.T) {
		ctx := testutil.SpannerTestContext(t)
		rootInvID := rootinvocations.ID("test-root-inv-flush")
		rdbHost := "staging.results.api.cr.dev"

		wuID1 := workunits.ID{RootInvocationID: rootInvID, WorkUnitID: "wu1"}
		wuID2 := workunits.ID{RootInvocationID: rootInvID, WorkUnitID: "wu2"}

		testutil.MustApply(ctx, t, insert.Invocation(wuID1.LegacyInvocationID(), pb.Invocation_ACTIVE, nil))
		testutil.MustApply(ctx, t, insert.Invocation(wuID2.LegacyInvocationID(), pb.Invocation_ACTIVE, nil))

		// WU1: Small result.
		tr1 := sizeLimitedTestResult(string(wuID1.LegacyInvocationID()), "testA", "res1", 100)
		testutil.MustApply(ctx, t, testresults.InsertForTesting(tr1.Build()))

		// WU2: Large page. 5 results, 2MB each. Total 10MB > 9MB limit.
		// Since pageSize will be large enough, they come in one page.
		var largeResults []*testresults.Builder
		for i := 0; i < 5; i++ {
			tr := sizeLimitedTestResult(string(wuID2.LegacyInvocationID()), "testB", string(rune('0'+i)), 2*1024*1024)
			largeResults = append(largeResults, tr)
			testutil.MustApply(ctx, t, testresults.InsertForTesting(tr.Build()))
		}

		gitilesSources := &pb.Sources{
			BaseSources: &pb.Sources_GitilesCommit{
				GitilesCommit: &pb.GitilesCommit{
					Host:       "test.googlesource.com",
					Project:    "test/project",
					Ref:        "refs/heads/main",
					CommitHash: "abcdef",
				},
			},
		}
		rootInvBuilder := rootinvocations.NewBuilder(rootInvID).WithStreamingExportState(pb.RootInvocation_METADATA_FINAL).WithSources(gitilesSources)
		testutil.MustApply(ctx, t, rootinvocations.InsertForTesting(rootInvBuilder.Build())...)
		ctx, sched := tq.TestingContext(ctx, nil)

		task := &taskspb.PublishTestResultsTask{
			RootInvocationId: string(rootInvID),
			WorkUnitIds:      []string{wuID1.WorkUnitID, wuID2.WorkUnitID},
		}

		p := &testResultsPublisher{
			task:             task,
			resultDBHostname: rdbHost,
			pageSize:         100, // Large enough to get all WU2 results in one page.
		}

		// Run the first task.
		err := p.handleTestResultsPublisher(ctx)
		assert.Loosely(t, err, should.BeNil)

		// Check tasks in queue.
		// Expectation:
		// 1. notify-test-results for WU1 (flushed).
		// 2. publish-test-results (continuation) for WU2 (because WU2 page was too large).
		allTasks := sched.Tasks()
		notifyTasks := allTasks.Filter(func(t *tqtesting.Task) bool { return t.Class == "notify-test-results" })
		pubTasks := allTasks.Filter(func(t *tqtesting.Task) bool { return t.Class == "publish-test-results" })

		assert.Loosely(t, notifyTasks, should.HaveLength(1))
		assert.Loosely(t, pubTasks, should.HaveLength(1))

		// Verify WU1 notification.
		notifyPayload := notifyTasks[0].Payload.(*taskspb.PublishTestResults)
		assert.Loosely(t, notifyPayload.Message.TestResultsByWorkUnit, should.HaveLength(1))
		assert.Loosely(t, notifyPayload.Message.TestResultsByWorkUnit[0].WorkUnitName, should.Equal(wuID1.Name()))

		// Verify Continuation Task.
		pubPayload := pubTasks[0].Payload.(*taskspb.PublishTestResultsTask)
		assert.Loosely(t, pubPayload.WorkUnitIds, should.Match([]string{wuID1.WorkUnitID, wuID2.WorkUnitID}))
		assert.Loosely(t, pubPayload.CurrentWorkUnitIndex, should.Equal(int32(1))) // Pointing to WU2
		assert.Loosely(t, pubPayload.PageToken, should.BeEmpty)                    // Retrying start of WU2

		// Run the continuation task.
		// This time, collectedResults is empty.
		// It fetches WU2 page (10MB). Scenario 1 logic returns it (and sets token for next).
		// Then partitionTestResults splits it into chunks.
		p2 := &testResultsPublisher{
			task:             pubPayload,
			resultDBHostname: rdbHost,
			pageSize:         100,
		}
		// Reset scheduler to capture new tasks
		ctx, sched = tq.TestingContext(ctx, nil)
		err = p2.handleTestResultsPublisher(ctx)
		assert.Loosely(t, err, should.BeNil)

		// Check tasks.
		// Expectation:
		// Multiple notify-test-results for WU2 (split).
		// No more continuation tasks (assuming 1 page covered all WU2).
		allTasks = sched.Tasks()
		notifyTasks = allTasks.Filter(func(t *tqtesting.Task) bool { return t.Class == "notify-test-results" })
		pubTasks = allTasks.Filter(func(t *tqtesting.Task) bool { return t.Class == "publish-test-results" })

		// 10MB split into 9MB chunks -> 2 chunks
		// Batch 1: 4 results (8MB).
		// Batch 2: 1 result (2MB).
		// So 2 notifications.
		assert.Loosely(t, notifyTasks, should.HaveLength(2))
		assert.Loosely(t, pubTasks, should.HaveLength(0))

		// Verify results in notifications.
		var totalResults int
		for _, task := range notifyTasks {
			p := task.Payload.(*taskspb.PublishTestResults)
			for _, wu := range p.Message.TestResultsByWorkUnit {
				assert.Loosely(t, wu.WorkUnitName, should.Equal(wuID2.Name()))
				totalResults += len(wu.TestResults)
			}
		}
		assert.Loosely(t, totalResults, should.Equal(5))
	})
}

// TestHandlePublishTestResultsTask_Pagination tests the pagination logic in handlePublishTestResultsTask.
func TestHandlePublishTestResultsTask_Pagination(t *testing.T) {
	t.Run("HandlePublishTestResultsTask_Pagination", func(t *testing.T) {
		ctx := testutil.SpannerTestContext(t)

		rootInvID := rootinvocations.ID("test-root-inv-page")
		rdbHost := "staging.results.api.cr.dev"
		wuID1 := workunits.ID{RootInvocationID: rootInvID, WorkUnitID: "wu1"}

		testutil.MustApply(ctx, t, insert.Invocation(wuID1.LegacyInvocationID(), pb.Invocation_ACTIVE, nil))

		// Insert 3 test results. This should result in two pages.
		tr1 := testresults.NewBuilder(wuID1.LegacyInvocationID(), "testA", "res1").WithMinimalFields()
		tr2 := testresults.NewBuilder(wuID1.LegacyInvocationID(), "testA", "res2").WithMinimalFields()
		tr3 := testresults.NewBuilder(wuID1.LegacyInvocationID(), "testB", "res1").WithMinimalFields()
		testutil.MustApply(ctx, t, testresults.InsertForTesting(tr1.Build()))
		testutil.MustApply(ctx, t, testresults.InsertForTesting(tr2.Build()))

		testutil.MustApply(ctx, t, testresults.InsertForTesting(tr3.Build()))

		gitilesSources := &pb.Sources{
			BaseSources: &pb.Sources_GitilesCommit{
				GitilesCommit: &pb.GitilesCommit{
					Host:       "test.googlesource.com",
					Project:    "test/project",
					Ref:        "refs/heads/main",
					CommitHash: "abcdef",
				},
			},
		}
		rootInvBuilder := rootinvocations.NewBuilder(rootInvID).WithStreamingExportState(pb.RootInvocation_METADATA_FINAL).WithSources(gitilesSources)
		testutil.MustApply(ctx, t, rootinvocations.InsertForTesting(rootInvBuilder.Build())...)
		ctx, sched := tq.TestingContext(ctx, nil)

		task := &taskspb.PublishTestResultsTask{
			RootInvocationId: string(rootInvID),
			WorkUnitIds:      []string{wuID1.WorkUnitID},
		}

		// Run all tasks.
		p := &testResultsPublisher{
			task:             task,
			resultDBHostname: rdbHost,
			pageSize:         2, // Set pageSize to 2 to force pagination.
		}
		err := p.handleTestResultsPublisher(ctx)
		assert.Loosely(t, err, should.BeNil)
		runTasks(ctx, t, sched, rdbHost, 2)

		tasks := sched.Tasks()
		// No more continuation tasks.
		assert.Loosely(t, tasks.Filter(func(t *tqtesting.Task) bool { return t.Class == "publish-test-results" }), should.HaveLength(0))

		// One notify task should be enqueued with all results.
		notifyTasks := tasks.Filter(func(t *tqtesting.Task) bool { return t.Class == "notify-test-results" })
		assert.Loosely(t, notifyTasks, should.HaveLength(1))

		notifyPayload := notifyTasks[0].Payload.(*taskspb.PublishTestResults)
		expectedTR1 := createExpectedTR(tr1, string(rootInvID), wuID1.WorkUnitID)
		expectedTR2 := createExpectedTR(tr2, string(rootInvID), wuID1.WorkUnitID)
		expectedTR3 := createExpectedTR(tr3, string(rootInvID), wuID1.WorkUnitID)

		expectedMsg := &pb.TestResultsNotification{
			ResultdbHost: rdbHost,
			Sources:      gitilesSources,
			TestResultsByWorkUnit: []*pb.TestResultsNotification_TestResultsByWorkUnit{
				{
					WorkUnitName: wuID1.Name(),
					TestResults:  []*pb.TestResult{expectedTR1, expectedTR2, expectedTR3},
				},
			},
		}
		expectedMsg.DeduplicationKey = generateDeduplicationKey(expectedMsg.TestResultsByWorkUnit)
		assert.Loosely(t, notifyPayload.Message, should.Match(expectedMsg))
	})
}

// TestHandlePublishTestResultsTask_SingleResultTooLarge tests the case where a single test result exceeds the Pub/Sub size limit.
func TestHandlePublishTestResultsTask_SingleResultTooLarge(t *testing.T) {
	t.Run("HandlePublishTestResultsTask_SingleResultTooLarge", func(t *testing.T) {
		ctx := testutil.SpannerTestContext(t)
		rootInvID := rootinvocations.ID("test-root-inv-large")
		rdbHost := "staging.results.api.cr.dev"
		wuID1 := workunits.ID{RootInvocationID: rootInvID, WorkUnitID: "wu1"}

		testutil.MustApply(ctx, t, insert.Invocation(wuID1.LegacyInvocationID(), pb.Invocation_ACTIVE, nil))

		// This test result is larger than the max size.
		tr1 := sizeLimitedTestResult(string(wuID1.LegacyInvocationID()), "testC", "res1", 10*1024*1024)
		testutil.MustApply(ctx, t, testresults.InsertForTesting(tr1.Build()))

		gitilesSources := &pb.Sources{
			BaseSources: &pb.Sources_GitilesCommit{
				GitilesCommit: &pb.GitilesCommit{
					Host:       "test.googlesource.com",
					Project:    "test/project",
					Ref:        "refs/heads/main",
					CommitHash: "abcdef",
				},
			},
		}

		rootInvBuilder := rootinvocations.NewBuilder(rootInvID).WithStreamingExportState(pb.RootInvocation_METADATA_FINAL).WithSources(gitilesSources)
		testutil.MustApply(ctx, t, rootinvocations.InsertForTesting(rootInvBuilder.Build())...)
		ctx, sched := tq.TestingContext(ctx, nil)

		task := &taskspb.PublishTestResultsTask{
			RootInvocationId: string(rootInvID),
			WorkUnitIds:      []string{wuID1.WorkUnitID},
		}

		p := &testResultsPublisher{
			task:             task,
			resultDBHostname: rdbHost,
			pageSize:         100,
		}
		err := p.handleTestResultsPublisher(ctx)

		assert.Loosely(t, err, should.NotBeNil)
		assert.Loosely(t, strings.Contains(err.Error(), "exceeds Pub/Sub size limit"), should.BeTrue)

		// No tasks should be enqueued.
		assert.Loosely(t, sched.Tasks(), should.HaveLength(0))
	})
}

// TestHandlePublishTestResultsTask_Checkpoints tests that checkpoints prevent redundant processing.
func TestHandlePublishTestResultsTask_Checkpoints(t *testing.T) {
	t.Run("HandlePublishTestResultsTask_Checkpoints", func(t *testing.T) {
		ctx := testutil.SpannerTestContext(t)
		rootInvID := rootinvocations.ID("test-root-inv-checkpoints")
		rdbHost := "staging.results.api.cr.dev"
		wuID1 := workunits.ID{RootInvocationID: rootInvID, WorkUnitID: "wu1"}
		testutil.MustApply(ctx, t, insert.Invocation(wuID1.LegacyInvocationID(), pb.Invocation_ACTIVE, nil))

		tr1 := testresults.NewBuilder(wuID1.LegacyInvocationID(), "testA", "res1").WithMinimalFields()
		testutil.MustApply(ctx, t, testresults.InsertForTesting(tr1.Build()))

		rootInvBuilder := rootinvocations.NewBuilder(rootInvID).WithStreamingExportState(pb.RootInvocation_METADATA_FINAL)
		testutil.MustApply(ctx, t, rootinvocations.InsertForTesting(rootInvBuilder.Build())...)
		ctx, sched := tq.TestingContext(ctx, nil)

		task := &taskspb.PublishTestResultsTask{
			RootInvocationId:     string(rootInvID),
			WorkUnitIds:          []string{wuID1.WorkUnitID},
			CurrentWorkUnitIndex: 0,
			PageToken:            "",
		}

		p := &testResultsPublisher{
			task:             task,
			resultDBHostname: rdbHost,
			pageSize:         100,
		}

		// First run should process and create checkpoint.
		err := p.handleTestResultsPublisher(ctx)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, sched.Tasks().Filter(func(t *tqtesting.Task) bool { return t.Class == "notify-test-results" }), should.HaveLength(1))

		// Reset scheduler to clear tasks.
		ctx, sched = tq.TestingContext(ctx, nil)

		// Second run with same task should be skipped by checkpoint.
		err = p.handleTestResultsPublisher(ctx)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, sched.Tasks().Filter(func(t *tqtesting.Task) bool { return t.Class == "notify-test-results" }), should.HaveLength(0))
	})
}

// TestGenerateDeduplicationKey tests the generateDeduplicationKey function.
func TestGenerateDeduplicationKey(t *testing.T) {
	t.Run("GenerateDeduplicationKey", func(t *testing.T) {
		tr1 := &pb.TestResult{TestId: "testA", ResultId: "res1"}
		tr2 := &pb.TestResult{TestId: "testA", ResultId: "res2"}
		tr3 := &pb.TestResult{TestId: "testB", ResultId: "res1"}

		testCases := []struct {
			name     string
			input    []*pb.TestResultsNotification_TestResultsByWorkUnit
			expected string
		}{
			{
				name:     "Empty input",
				input:    []*pb.TestResultsNotification_TestResultsByWorkUnit{},
				expected: "",
			},
			{
				name: "Single work unit, single test result",
				input: []*pb.TestResultsNotification_TestResultsByWorkUnit{
					{WorkUnitName: "wu1", TestResults: []*pb.TestResult{tr1}},
				},
				expected: "6148ca2bce03ca7754194e4a4ccd225e8aad492fccb1ea83dda239eae2cc2307",
			},
			{
				name: "Single work unit, multiple test results",
				input: []*pb.TestResultsNotification_TestResultsByWorkUnit{
					{WorkUnitName: "wu1", TestResults: []*pb.TestResult{tr1, tr2}},
				},
				expected: "6148ca2bce03ca7754194e4a4ccd225e8aad492fccb1ea83dda239eae2cc2307",
			},
			{
				name: "Single work unit, multiple test results - different order",
				input: []*pb.TestResultsNotification_TestResultsByWorkUnit{
					{WorkUnitName: "wu1", TestResults: []*pb.TestResult{tr2, tr1}},
				},
				expected: "dbc1036e16886afa80f0f0e6f4444d37d1490021738dce44803d2220b34a55ee",
			},
			{
				name: "Multiple work units",
				input: []*pb.TestResultsNotification_TestResultsByWorkUnit{
					{WorkUnitName: "wu1", TestResults: []*pb.TestResult{tr1, tr2}},
					{WorkUnitName: "wu2", TestResults: []*pb.TestResult{tr3}},
				},
				expected: "6148ca2bce03ca7754194e4a4ccd225e8aad492fccb1ea83dda239eae2cc2307",
			},
			{
				name: "Multiple work units - different order",
				input: []*pb.TestResultsNotification_TestResultsByWorkUnit{
					{WorkUnitName: "wu2", TestResults: []*pb.TestResult{tr3}},
					{WorkUnitName: "wu1", TestResults: []*pb.TestResult{tr1, tr2}},
				},
				expected: "624dd4c0b534a66e61ea7843f009de302c73ccb3da15ae95b514a9bc06c10970",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				actual := generateDeduplicationKey(tc.input)
				assert.Loosely(t, actual, should.Equal(tc.expected))
			})
		}
	})
}
