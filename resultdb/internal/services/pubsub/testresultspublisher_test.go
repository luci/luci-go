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
	"sort"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/testing/ftt"
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

func createExpectedTR(tr *pb.TestResult, rootInvID string, wuID string) *pb.TestResult {
	expectedTR := proto.Clone(tr).(*pb.TestResult)
	expectedTR.Name = pbutil.TestResultName(rootInvID, wuID, tr.TestId, tr.ResultId)
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

func TestHandlePublishTestResultsTask(t *testing.T) {
	ftt.Run("HandlePublishTestResultsTask", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		rootInvID := rootinvocations.ID("test-root-inv")
		rdbHost := "staging.results.api.cr.dev"

		wuID1 := workunits.ID{RootInvocationID: rootInvID, WorkUnitID: "wu1"}
		wuID2 := workunits.ID{RootInvocationID: rootInvID, WorkUnitID: "wu2"}

		// Insert legacy Invocations based on work unit IDs.
		testutil.MustApply(ctx, t, insert.Invocation(wuID1.LegacyInvocationID(), pb.Invocation_ACTIVE, nil))
		testutil.MustApply(ctx, t, insert.Invocation(wuID2.LegacyInvocationID(), pb.Invocation_ACTIVE, nil))

		// Insert TestResults.
		trs1 := insert.MakeTestResults(wuID1.LegacyInvocationID(), "testA", nil, pb.TestResult_PASSED)
		trs2 := insert.MakeTestResults(wuID2.LegacyInvocationID(), "testB", nil, pb.TestResult_FAILED)
		testutil.MustApply(ctx, t, insert.TestResultMessages(t, trs1)...)
		testutil.MustApply(ctx, t, insert.TestResultMessages(t, trs2)...)

		// Expected TestResults in notification (names are
		// reconstructed).
		expectedTRs1 := make([]*pb.TestResult, len(trs1))
		for i, tr := range trs1 {
			expectedTRs1[i] = createExpectedTR(tr, string(rootInvID), wuID1.WorkUnitID)
		}
		expectedTRs2 := make([]*pb.TestResult, len(trs2))
		for i, tr := range trs2 {
			expectedTRs2[i] = createExpectedTR(tr, string(rootInvID), wuID2.WorkUnitID)
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
		androidProperties := &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"primary_build": structpb.NewStructValue(&structpb.Struct{
					Fields: map[string]*structpb.Value{
						"branch": structpb.NewStringValue("git_main"),
					},
				}),
			},
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
				name:           "No Sources or Properties",
				rootInvBuilder: rootinvocations.NewBuilder(rootInvID).WithStreamingExportState(pb.RootInvocation_METADATA_FINAL).WithSources(nil).WithProperties(nil),
				finalizedWUIDs: []string{wuID1.WorkUnitID},
				expectedNotification: &pb.TestResultsNotification{
					ResultdbHost: rdbHost,
					TestResultsByWorkUnit: []*pb.TestResultsNotification_TestResultsByWorkUnit{
						{WorkUnitName: wuID1.Name(), TestResults: expectedTRs1},
					},
				},
			},
			{
				name:               "Android Branch from Properties",
				rootInvBuilder:     rootinvocations.NewBuilder(rootInvID).WithStreamingExportState(pb.RootInvocation_METADATA_FINAL).WithProperties(androidProperties).WithSources(gitilesSources),
				finalizedWUIDs:     []string{wuID1.WorkUnitID},
				expectedAttributes: map[string]string{"branch": "git_main"},
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
				rootInvBuilder:     rootinvocations.NewBuilder(rootInvID).WithStreamingExportState(pb.RootInvocation_METADATA_FINAL).WithProperties(androidProperties).WithSources(gitilesSources),
				finalizedWUIDs:     []string{wuID1.WorkUnitID, wuID2.WorkUnitID},
				expectedAttributes: map[string]string{"branch": "git_main"},
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
			t.Run(tc.name, func(t *ftt.Test) {
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
				testutil.MustApply(ctx, t, insert.TestResultMessages(t, trs1)...)
				testutil.MustApply(ctx, t, insert.TestResultMessages(t, trs2)...)

				// Reset tasks.
				sched.Tasks()

				task := &taskspb.PublishTestResultsTask{
					RootInvocationId: string(rootInvID),
					WorkUnitIds:      tc.finalizedWUIDs,
				}
				err := handlePublishTestResultsTask(ctx, task, rdbHost)
				if tc.name == "StreamingExportState not METADATA_FINAL" {
					assert.Loosely(t, err, should.BeNil)
					return
				}
				assert.Loosely(t, err, should.BeNil)

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

				// Sort TestResultsByWorkUnit to make the test deterministic.
				sort.Slice(payload.Message.TestResultsByWorkUnit, func(i, j int) bool {
					return payload.Message.TestResultsByWorkUnit[i].WorkUnitName < payload.Message.TestResultsByWorkUnit[j].WorkUnitName
				})

				// Calculate the expected deduplication key separately.
				expectedDeduplicationKey := generateDeduplicationKey(payload.Message.TestResultsByWorkUnit)
				assert.Loosely(t, payload.Message.DeduplicationKey, should.Equal(expectedDeduplicationKey))

				// Clear the key for the rest of the proto comparison.
				payload.Message.DeduplicationKey = ""
				assert.Loosely(t, payload.Message, should.Match(tc.expectedNotification))
			})
		}
	})
}

func TestHandlePublishTestResultsTask_Batching(t *testing.T) {
	ftt.Run("HandlePublishTestResultsTask_Batching", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		rootInvID := rootinvocations.ID("test-root-inv-batch")
		rdbHost := "staging.results.api.cr.dev"

		wuID1 := workunits.ID{RootInvocationID: rootInvID, WorkUnitID: "wu1"}
		wuID2 := workunits.ID{RootInvocationID: rootInvID, WorkUnitID: "wu2"}

		testutil.MustApply(ctx, t, insert.Invocation(wuID1.LegacyInvocationID(), pb.Invocation_ACTIVE, nil))
		testutil.MustApply(ctx, t, insert.Invocation(wuID2.LegacyInvocationID(), pb.Invocation_ACTIVE, nil))

		// Set a small size for testing batching.
		const testMaxSize = 1000 // 1KB

		// WU1 has two results, each ~600 bytes. These should be in separate batches.
		tr1 := sizeLimitedTestResult(string(wuID1.LegacyInvocationID()), "testA", "res1", 600)
		tr2 := sizeLimitedTestResult(string(wuID1.LegacyInvocationID()), "testA", "res2", 600)

		// WU2 has two results, ~300 bytes each. These should be in the same batch.
		tr3 := sizeLimitedTestResult(string(wuID2.LegacyInvocationID()), "testB", "res1", 300)
		tr4 := sizeLimitedTestResult(string(wuID2.LegacyInvocationID()), "testB", "res2", 300)

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

		err := handlePublishTestResultsTask(ctx, task, rdbHost)
		assert.Loosely(t, err, should.BeNil)

		allTasks := sched.Tasks()
		var actualMessages []*taskspb.PublishTestResults

		for _, task := range allTasks {
			if task.Class == "notify-test-results" {
				payload := task.Payload.(*taskspb.PublishTestResults)

				// Calculate the expected deduplication key separately.
				expectedDeduplicationKey := generateDeduplicationKey(payload.Message.TestResultsByWorkUnit)
				assert.Loosely(t, payload.Message.DeduplicationKey, should.Equal(expectedDeduplicationKey))

				// Clear the key for the rest of the proto comparison.
				payload.Message.DeduplicationKey = ""
				actualMessages = append(actualMessages, payload)
			}
		}

		// Prepare expected results for comparison
		expectedTR1 := createExpectedTR(tr1.Build().ToProto(), string(rootInvID), wuID1.WorkUnitID)
		expectedTR2 := createExpectedTR(tr2.Build().ToProto(), string(rootInvID), wuID1.WorkUnitID)
		expectedTR3 := createExpectedTR(tr3.Build().ToProto(), string(rootInvID), wuID2.WorkUnitID)
		expectedTR4 := createExpectedTR(tr4.Build().ToProto(), string(rootInvID), wuID2.WorkUnitID)

		// Expected messages based on size and batching logic.
		expectedMessages := []*taskspb.PublishTestResults{
			{
				Message: &pb.TestResultsNotification{
					ResultdbHost: rdbHost,
					Sources:      gitilesSources,
					TestResultsByWorkUnit: []*pb.TestResultsNotification_TestResultsByWorkUnit{
						{WorkUnitName: wuID1.Name(), TestResults: []*pb.TestResult{expectedTR1, expectedTR2}},
						{WorkUnitName: wuID2.Name(), TestResults: []*pb.TestResult{expectedTR3, expectedTR4}},
					},
				},
			},
		}

		// Loosely compare the contents without checking the order of messages.
		for _, msg := range actualMessages {
			for _, tr := range msg.Message.TestResultsByWorkUnit {
				assert.Loosely(t, tr, should.MatchIn(expectedMessages[0].Message.TestResultsByWorkUnit))
			}
		}
	})
}
func TestHandlePublishTestResultsTask_SingleResultTooLarge(t *testing.T) {
	ftt.Run("HandlePublishTestResultsTask_SingleResultTooLarge", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		rootInvID := rootinvocations.ID("test-root-inv-large")
		rdbHost := "staging.results.api.cr.dev"
		wuID1 := workunits.ID{RootInvocationID: rootInvID, WorkUnitID: "wu1"}

		testutil.MustApply(ctx, t, insert.Invocation(wuID1.LegacyInvocationID(), pb.Invocation_ACTIVE, nil))

		// This test result is larger than the max size.
		tr1 := sizeLimitedTestResult(string(wuID1.LegacyInvocationID()), "testC", "res1", 1200)
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

		// Set a small size for testing batching.
		const testMaxSize = 1000 // 1KB

		err := enqueueBatchedNotifications(ctx, testResultsNotification(map[string][]*pb.TestResult{
			wuID1.Name(): {tr1.Build().ToProto()},
		}), rdbHost, gitilesSources, nil, testMaxSize)

		assert.Loosely(t, err, should.NotBeNil)
		assert.Loosely(t, strings.Contains(err.Error(), "exceeds Pub/Sub size limit"), should.BeTrue)

		// No tasks should be enqueued.
		assert.Loosely(t, sched.Tasks(), should.HaveLength(0))
	})
}
