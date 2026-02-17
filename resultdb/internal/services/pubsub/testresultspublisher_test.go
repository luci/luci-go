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
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/tq"
	"go.chromium.org/luci/server/tq/tqtesting"

	"go.chromium.org/luci/resultdb/internal/config"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/masking"
	"go.chromium.org/luci/resultdb/internal/permissions"
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
func runTasks(ctx context.Context, t *ftt.Test, sched *tqtesting.Scheduler, rdbHost string, pageSize int) {
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
	ftt.Run("HandlePublishTestResultsTask", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		ctx = caching.WithEmptyProcessCache(ctx) // For config in-process cache.
		ctx = memory.Use(ctx)                    // For config datastore cache.

		// Set up a placeholder service config.
		cfg := config.CreatePlaceholderServiceConfig()
		err := config.SetServiceConfigForTesting(ctx, cfg)
		assert.Loosely(t, err, should.BeNil)
		compiledCfg, err := config.NewCompiledServiceConfig(cfg, "")
		assert.Loosely(t, err, should.BeNil)

		ctx, sched := tq.TestingContext(ctx, nil)

		rootInvID := rootinvocations.ID("test-root-inv")
		rdbHost := "results.api.cr.dev"

		wuID1 := workunits.ID{RootInvocationID: rootInvID, WorkUnitID: "wu1"}
		wuID2 := workunits.ID{RootInvocationID: rootInvID, WorkUnitID: "wu2"}

		// Prepare the root invocation for this test case.
		var ms []*spanner.Mutation
		builder := rootinvocations.NewBuilder(rootInvID).WithStreamingExportState(pb.RootInvocation_METADATA_FINAL)
		ms = append(ms, insert.RootInvocationWithRootWorkUnit(builder.Build())...)

		// Prepare the work units.
		wu1 := workunits.NewBuilder(rootInvID, "wu1").WithState(pb.WorkUnit_FAILED).Build()
		wu2 := workunits.NewBuilder(rootInvID, "wu2").WithMinimalFields().WithState(pb.WorkUnit_SUCCEEDED).Build()
		ms = append(ms, workunits.InsertForTesting(wu1)...)
		ms = append(ms, workunits.InsertForTesting(wu2)...)

		// Prepare TestResults.
		tr1 := testresults.NewBuilder(wuID1.LegacyInvocationID(), "testA", "0").WithMinimalFields().WithStatus(pb.TestStatus_PASS).WithDuration(&durationpb.Duration{Seconds: 0})
		tr2 := testresults.NewBuilder(wuID2.LegacyInvocationID(), "testB", "0").WithMinimalFields().WithStatus(pb.TestStatus_FAIL).WithDuration(&durationpb.Duration{Seconds: 0})

		ms = append(ms, testresults.InsertForTesting(tr1.Build()))
		ms = append(ms, testresults.InsertForTesting(tr2.Build()))

		testutil.MustApply(ctx, t, ms...)

		type message struct {
			Attrs map[string]string
			Body  *pb.TestResultsNotification
		}

		runTaskAndContinuations := func(workUnits []string) ([]message, error) {
			task := &taskspb.PublishTestResultsTask{
				RootInvocationId: string(rootInvID),
				WorkUnitIds:      workUnits,
			}
			p := &testResultsPublisher{
				task:             task,
				resultDBHostname: rdbHost,
				pageSize:         100,
			}
			err := p.handleTestResultsPublisher(ctx)
			if err != nil {
				return nil, err
			}

			// Run all continuation tasks.
			runTasks(ctx, t, sched, rdbHost, 100)

			allTasks := sched.Tasks()
			var messages []message
			for _, task := range allTasks {
				if task.Class == "notify-test-results" {
					// Ignore TQ internal attribute.
					attrs := task.Message.GetAttributes()
					delete(attrs, "X-Luci-Tq-Reminder-Id")

					messages = append(messages, message{
						Attrs: attrs,
						Body:  task.Payload.(*taskspb.PublishTestResults).Message,
					})
				}
			}
			return messages, nil
		}

		expectedTRs1 := []*pb.TestResult{createExpectedTR(tr1, string(rootInvID), wuID1.WorkUnitID)}
		expectedTRs2 := []*pb.TestResult{createExpectedTR(tr2, string(rootInvID), wuID2.WorkUnitID)}
		expectedWU1 := masking.WorkUnit(wu1, permissions.FullAccess, pb.WorkUnitView_WORK_UNIT_VIEW_BASIC, compiledCfg)
		expectedWU2 := masking.WorkUnit(wu2, permissions.FullAccess, pb.WorkUnitView_WORK_UNIT_VIEW_BASIC, compiledCfg)

		expectedNotification := &pb.TestResultsNotification{
			ResultdbHost: rdbHost,
			TestResultsByWorkUnit: []*pb.TestResultsNotification_TestResultsByWorkUnit{
				{WorkUnit: expectedWU1, TestResults: expectedTRs1},
				{WorkUnit: expectedWU2, TestResults: expectedTRs2},
			},
			DeduplicationKey:       "d1c858729cc402d6b95b4b172de7de046e7010294d98558549d6c67a734bb3b9",
			RootInvocationMetadata: expectedInvocationMetadata(),
		}
		expectedAttrs := map[string]string{
			luciProjectFilter:    "testproject",
			androidBranchFilter:  "git_main",
			androidTargetFilter:  "some-target",
			definitionNameFilter: "project/bucket/builder",
		}

		finalizedWorkUnits := []string{wuID1.WorkUnitID, wuID2.WorkUnitID}

		t.Run("Baseline", func(t *ftt.Test) {
			messages, err := runTaskAndContinuations(finalizedWorkUnits)
			assert.NoErr(t, err)
			assert.Loosely(t, messages, should.HaveLength(1))
			assert.Loosely(t, messages[0].Body, should.Match(expectedNotification))
			assert.Loosely(t, messages[0].Attrs, should.Match(expectedAttrs))
		})
		t.Run("StreamingExportState not METADATA_FINAL", func(t *ftt.Test) {
			update := rootinvocations.NewMutationBuilder(rootInvID)
			update.UpdateStreamingExportState(pb.RootInvocation_WAIT_FOR_METADATA)
			testutil.MustApply(ctx, t, update.Build()...)

			messages, err := runTaskAndContinuations(finalizedWorkUnits)
			assert.NoErr(t, err)
			assert.Loosely(t, messages, should.HaveLength(0))
		})
		t.Run("Single result too large", func(t *ftt.Test) {
			// This test result is larger than the max size.
			tr1 := sizeLimitedTestResult(string(wuID1.LegacyInvocationID()), "testC", "res1", 10*1024*1024)
			testutil.MustApply(ctx, t, testresults.InsertForTesting(tr1.Build()))

			messages, err := runTaskAndContinuations([]string{wuID1.WorkUnitID})
			assert.Loosely(t, messages, should.HaveLength(0))
			assert.Loosely(t, err, should.ErrLike("exceeds Pub/Sub size limit"))
		})
		t.Run("Only one work unit", func(t *ftt.Test) {
			expectedNotification.TestResultsByWorkUnit = expectedNotification.TestResultsByWorkUnit[:1]

			messages, err := runTaskAndContinuations(finalizedWorkUnits[:1])
			assert.NoErr(t, err)
			assert.Loosely(t, messages, should.HaveLength(1))
			assert.Loosely(t, messages[0].Body, should.Match(expectedNotification))
			assert.Loosely(t, messages[0].Attrs, should.Match(expectedAttrs))
		})
		t.Run("Checkpoints", func(t *ftt.Test) {
			messages, err := runTaskAndContinuations(finalizedWorkUnits)
			assert.NoErr(t, err)
			assert.Loosely(t, messages, should.HaveLength(1))
			assert.Loosely(t, messages[0].Body, should.Match(expectedNotification))

			// Reset scheduler to clear tasks.
			ctx, sched = tq.TestingContext(ctx, nil)

			// Re-running the task has no effect.
			messages, err = runTaskAndContinuations(finalizedWorkUnits)
			assert.NoErr(t, err)
			assert.Loosely(t, messages, should.HaveLength(0))
		})
		t.Run("Batching", func(t *ftt.Test) {
			// WU1 has two results, each ~6MB. These should be in separate batches for this WU.
			tr1 := sizeLimitedTestResult(string(wuID1.LegacyInvocationID()), "testA", "res1", 6*1024*1024)
			tr2 := sizeLimitedTestResult(string(wuID1.LegacyInvocationID()), "testA", "res2", 6*1024*1024)

			// WU2 has two results, ~3MB each. These should be in the same batch.
			tr3 := sizeLimitedTestResult(string(wuID2.LegacyInvocationID()), "testB", "res1", 3*1024*1024)
			tr4 := sizeLimitedTestResult(string(wuID2.LegacyInvocationID()), "testB", "res2", 3*1024*1024)

			// Replace the test results stored.
			testutil.MustApply(ctx, t, spanner.Delete("TestResults", spanner.AllKeys()))
			testutil.MustApply(ctx, t, testresults.InsertForTesting(tr1.Build()))
			testutil.MustApply(ctx, t, testresults.InsertForTesting(tr2.Build()))
			testutil.MustApply(ctx, t, testresults.InsertForTesting(tr3.Build()))
			testutil.MustApply(ctx, t, testresults.InsertForTesting(tr4.Build()))

			// Prepare expectations for comparison.
			expectedTR1 := createExpectedTR(tr1, string(rootInvID), wuID1.WorkUnitID)
			expectedTR2 := createExpectedTR(tr2, string(rootInvID), wuID1.WorkUnitID)
			expectedTR3 := createExpectedTR(tr3, string(rootInvID), wuID2.WorkUnitID)
			expectedTR4 := createExpectedTR(tr4, string(rootInvID), wuID2.WorkUnitID)

			// Expected messages based on size and batching logic.
			// WU1's results are batched separately because each is > testMaxSize/2.
			// WU2's results fit in one batch.
			expectedWUBatch1 := &pb.TestResultsNotification{
				ResultdbHost:           rdbHost,
				RootInvocationMetadata: expectedInvocationMetadata(),
				TestResultsByWorkUnit: []*pb.TestResultsNotification_TestResultsByWorkUnit{
					{WorkUnit: expectedWU1, TestResults: []*pb.TestResult{expectedTR1}},
				},
			}
			expectedWUBatch1.DeduplicationKey = generateDeduplicationKey(expectedWUBatch1.TestResultsByWorkUnit)

			expectedWUBatch2 := &pb.TestResultsNotification{
				ResultdbHost:           rdbHost,
				RootInvocationMetadata: expectedInvocationMetadata(),
				TestResultsByWorkUnit: []*pb.TestResultsNotification_TestResultsByWorkUnit{
					{WorkUnit: expectedWU1, TestResults: []*pb.TestResult{expectedTR2}},
				},
			}
			expectedWUBatch2.DeduplicationKey = generateDeduplicationKey(expectedWUBatch2.TestResultsByWorkUnit)

			expectedWUBatch3 := &pb.TestResultsNotification{
				ResultdbHost:           rdbHost,
				RootInvocationMetadata: expectedInvocationMetadata(),
				TestResultsByWorkUnit: []*pb.TestResultsNotification_TestResultsByWorkUnit{
					{WorkUnit: expectedWU2, TestResults: []*pb.TestResult{expectedTR3, expectedTR4}},
				},
			}
			expectedWUBatch3.DeduplicationKey = generateDeduplicationKey(expectedWUBatch3.TestResultsByWorkUnit)

			messages, err := runTaskAndContinuations([]string{wuID1.WorkUnitID, wuID2.WorkUnitID})
			assert.NoErr(t, err)
			assert.Loosely(t, messages, should.HaveLength(3))
			assert.Loosely(t, messages[0].Body, should.Match(expectedWUBatch1))
			assert.Loosely(t, messages[1].Body, should.Match(expectedWUBatch2))
			assert.Loosely(t, messages[2].Body, should.Match(expectedWUBatch3))
		})
	})
}

func expectedInvocationMetadata() *pb.RootInvocationMetadata {
	// This is based on the default values used in rootinvocations.NewBuilder().
	return &pb.RootInvocationMetadata{
		Name:             "rootInvocations/test-root-inv",
		RootInvocationId: "test-root-inv",
		Realm:            "testproject:testrealm",
		CreateTime:       timestamppb.New(time.Date(2025, 4, 25, 1, 2, 3, 4000, time.UTC)),
		ProducerResource: &pb.ProducerResource{
			System:    "buildbucket",
			DataRealm: "prod",
			Name:      "builds/654",
			Url:       "https://milo-prod/ui/b/654",
		},
		Definition: &pb.RootInvocationDefinition{
			System:         "buildbucket",
			Name:           "project/bucket/builder",
			Properties:     pbutil.DefinitionProperties("key", "value"),
			PropertiesHash: "5d8482c3056d8635",
		},
		Sources: &pb.Sources{
			BaseSources: &pb.Sources_GitilesCommit{
				GitilesCommit: &pb.GitilesCommit{
					Host:       "chromium.googlesource.com",
					Project:    "chromium/src",
					Ref:        "refs/heads/main",
					CommitHash: "1234567890abcdef1234567890abcdef12345678",
					Position:   12345,
				},
			},
		},
		PrimaryBuild: &pb.BuildDescriptor{
			Definition: &pb.BuildDescriptor_AndroidBuild{
				AndroidBuild: &pb.AndroidBuildDescriptor{
					DataRealm:   "prod",
					Branch:      "git_main",
					BuildTarget: "some-target",
					BuildId:     "P1234567890",
				},
			},
			Url: "https://android-build.googleplex.com/build_explorer/build_details/P1234567890/some-target/",
		},
		ExtraBuilds: []*pb.BuildDescriptor{
			{
				Definition: &pb.BuildDescriptor_AndroidBuild{
					AndroidBuild: &pb.AndroidBuildDescriptor{
						DataRealm:   "prod",
						Branch:      "git_main",
						BuildTarget: "second-target",
						BuildId:     "9876543210",
					},
				},
				Url: "https://android-build.googleplex.com/build_explorer/build_details/9876543210/second-target/",
			},
		},
	}
}

// TestHandlePublishTestResultsTask_FlushPriorCollectedResultsIfNextPageTooLarge tests that if a single page
// of results is too large, we flush previously collected results first.
func TestHandlePublishTestResultsTask_FlushPriorCollectedResultsIfNextPageTooLarge(t *testing.T) {
	ftt.Run("HandlePublishTestResultsTask_Scenario1_Flush", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		ctx = caching.WithEmptyProcessCache(ctx) // For config in-process cache.
		ctx = memory.Use(ctx)                    // For config datastore cache.

		// Set up a placeholder service config.
		cfg := config.CreatePlaceholderServiceConfig()
		err := config.SetServiceConfigForTesting(ctx, cfg)
		assert.Loosely(t, err, should.BeNil)
		compiledCfg, err := config.NewCompiledServiceConfig(cfg, "")
		assert.Loosely(t, err, should.BeNil)

		rootInvID := rootinvocations.ID("test-root-inv-flush")
		rdbHost := "results.api.cr.dev"

		wuID1 := workunits.ID{RootInvocationID: rootInvID, WorkUnitID: "wu1"}
		wuID2 := workunits.ID{RootInvocationID: rootInvID, WorkUnitID: "wu2"}

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

		// Prepare root invocation.
		var ms []*spanner.Mutation
		rootInvBuilder := rootinvocations.NewBuilder(rootInvID).WithStreamingExportState(pb.RootInvocation_METADATA_FINAL).WithSources(gitilesSources)
		ms = append(ms, insert.RootInvocationWithRootWorkUnit(rootInvBuilder.Build())...)

		// Prepare work units.
		wu1 := workunits.NewBuilder(rootInvID, "wu1").WithState(pb.WorkUnit_RUNNING).Build()
		wu2 := workunits.NewBuilder(rootInvID, "wu2").WithMinimalFields().WithState(pb.WorkUnit_RUNNING).Build()
		ms = append(ms, workunits.InsertForTesting(wu1)...)
		ms = append(ms, workunits.InsertForTesting(wu2)...)
		testutil.MustApply(ctx, t, ms...)

		expectedWU1 := masking.WorkUnit(wu1, permissions.FullAccess, pb.WorkUnitView_WORK_UNIT_VIEW_BASIC, compiledCfg)
		expectedWU2 := masking.WorkUnit(wu2, permissions.FullAccess, pb.WorkUnitView_WORK_UNIT_VIEW_BASIC, compiledCfg)

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
		err = p.handleTestResultsPublisher(ctx)
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
		assert.Loosely(t, notifyPayload.Message.TestResultsByWorkUnit[0].WorkUnit, should.Match(expectedWU1))

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
				assert.Loosely(t, wu.WorkUnit, should.Match(expectedWU2))
				totalResults += len(wu.TestResults)
			}
		}
		assert.Loosely(t, totalResults, should.Equal(5))
	})
}

// TestHandlePublishTestResultsTask_Pagination tests the pagination logic in handlePublishTestResultsTask.
func TestHandlePublishTestResultsTask_Pagination(t *testing.T) {
	ftt.Run("HandlePublishTestResultsTask_Pagination", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		ctx = caching.WithEmptyProcessCache(ctx) // For config in-process cache.
		ctx = memory.Use(ctx)                    // For config datastore cache.

		// Set up a placeholder service config.
		cfg := config.CreatePlaceholderServiceConfig()
		err := config.SetServiceConfigForTesting(ctx, cfg)
		assert.Loosely(t, err, should.BeNil)
		compiledCfg, err := config.NewCompiledServiceConfig(cfg, "")
		assert.Loosely(t, err, should.BeNil)

		rootInvID := rootinvocations.ID("test-root-inv")
		rdbHost := "results.api.cr.dev"
		wuID1 := workunits.ID{RootInvocationID: rootInvID, WorkUnitID: "wu1"}

		var ms []*spanner.Mutation
		rootInvBuilder := rootinvocations.NewBuilder(rootInvID).WithStreamingExportState(pb.RootInvocation_METADATA_FINAL)
		ms = append(ms, insert.RootInvocationWithRootWorkUnit(rootInvBuilder.Build())...)

		wu1 := workunits.NewBuilder(rootInvID, "wu1").WithState(pb.WorkUnit_SUCCEEDED).Build()
		ms = append(ms, workunits.InsertForTesting(wu1)...)

		expectedWU1 := masking.WorkUnit(wu1, permissions.FullAccess, pb.WorkUnitView_WORK_UNIT_VIEW_BASIC, compiledCfg)

		// Insert 3 test results. This should result in two pages.
		tr1 := testresults.NewBuilder(wuID1.LegacyInvocationID(), "testA", "res1").WithMinimalFields()
		tr2 := testresults.NewBuilder(wuID1.LegacyInvocationID(), "testA", "res2").WithMinimalFields()
		tr3 := testresults.NewBuilder(wuID1.LegacyInvocationID(), "testB", "res1").WithMinimalFields()
		ms = append(ms, testresults.InsertForTesting(tr1.Build()))
		ms = append(ms, testresults.InsertForTesting(tr2.Build()))
		ms = append(ms, testresults.InsertForTesting(tr3.Build()))
		testutil.MustApply(ctx, t, ms...)

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
		err = p.handleTestResultsPublisher(ctx)
		assert.Loosely(t, err, should.BeNil)
		runTasks(ctx, t, sched, rdbHost, 2)

		tasks := sched.Tasks()
		// No more continuation tasks.
		assert.Loosely(t, tasks.Filter(func(t *tqtesting.Task) bool { return t.Class == "publish-test-results" }), should.HaveLength(0))

		// One notify task should be enqueued with all results.
		notifyTasks := tasks.Filter(func(t *tqtesting.Task) bool { return t.Class == "notify-test-results" })
		assert.Loosely(t, notifyTasks, should.HaveLength(1))

		notifyPayload := notifyTasks[0].Payload.(*taskspb.PublishTestResults)

		// Prepare expectations for comparison.
		expectedTR1 := createExpectedTR(tr1, string(rootInvID), wuID1.WorkUnitID)
		expectedTR2 := createExpectedTR(tr2, string(rootInvID), wuID1.WorkUnitID)
		expectedTR3 := createExpectedTR(tr3, string(rootInvID), wuID1.WorkUnitID)

		expectedMsg := &pb.TestResultsNotification{
			ResultdbHost:           rdbHost,
			RootInvocationMetadata: expectedInvocationMetadata(),
			TestResultsByWorkUnit: []*pb.TestResultsNotification_TestResultsByWorkUnit{
				{
					WorkUnit:    expectedWU1,
					TestResults: []*pb.TestResult{expectedTR1, expectedTR2, expectedTR3},
				},
			},
		}
		expectedMsg.DeduplicationKey = generateDeduplicationKey(expectedMsg.TestResultsByWorkUnit)
		assert.Loosely(t, notifyPayload.Message, should.Match(expectedMsg))
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
					{WorkUnit: &pb.WorkUnit{Name: "wu1"}, TestResults: []*pb.TestResult{tr1}},
				},
				expected: "6148ca2bce03ca7754194e4a4ccd225e8aad492fccb1ea83dda239eae2cc2307",
			},
			{
				name: "Single work unit, multiple test results",
				input: []*pb.TestResultsNotification_TestResultsByWorkUnit{
					{WorkUnit: &pb.WorkUnit{Name: "wu1"}, TestResults: []*pb.TestResult{tr1, tr2}},
				},
				expected: "6148ca2bce03ca7754194e4a4ccd225e8aad492fccb1ea83dda239eae2cc2307",
			},
			{
				name: "Single work unit, multiple test results - different order",
				input: []*pb.TestResultsNotification_TestResultsByWorkUnit{
					{WorkUnit: &pb.WorkUnit{Name: "wu1"}, TestResults: []*pb.TestResult{tr2, tr1}},
				},
				expected: "dbc1036e16886afa80f0f0e6f4444d37d1490021738dce44803d2220b34a55ee",
			},
			{
				name: "Multiple work units",
				input: []*pb.TestResultsNotification_TestResultsByWorkUnit{
					{WorkUnit: &pb.WorkUnit{Name: "wu1"}, TestResults: []*pb.TestResult{tr1, tr2}},
					{WorkUnit: &pb.WorkUnit{Name: "wu2"}, TestResults: []*pb.TestResult{tr3}},
				},
				expected: "6148ca2bce03ca7754194e4a4ccd225e8aad492fccb1ea83dda239eae2cc2307",
			},
			{
				name: "Multiple work units - different order",
				input: []*pb.TestResultsNotification_TestResultsByWorkUnit{
					{WorkUnit: &pb.WorkUnit{Name: "wu2"}, TestResults: []*pb.TestResult{tr3}},
					{WorkUnit: &pb.WorkUnit{Name: "wu1"}, TestResults: []*pb.TestResult{tr1, tr2}},
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
