// Copyright 2026 The LUCI Authors.
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

package pubsub

import (
	"testing"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"
	"go.chromium.org/luci/server/tq/tqtesting"

	"go.chromium.org/luci/resultdb/internal/checkpoints"
	"go.chromium.org/luci/resultdb/internal/config"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/tasks/taskspb"
	"go.chromium.org/luci/resultdb/internal/testaggregations"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestHandleTestAggregationsPublisher(t *testing.T) {
	t.Parallel()

	ctx := testutil.SpannerTestContext(t)
	ctx = caching.WithEmptyProcessCache(ctx)
	ctx = memory.Use(ctx)
	err := config.SetServiceConfigForTesting(ctx, config.CreatePlaceholderServiceConfig())
	assert.Loosely(t, err, should.BeNil)
	ctx, sched := tq.TestingContext(ctx, nil)

	rdbHost := "results.api.cr.dev"

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
	rootInvDefinition := &pb.RootInvocationDefinition{
		System: "atp",
		Name:   "test-definition",
	}

	t.Run("StreamingExportState not METADATA_FINAL", func(t *testing.T) {
		rootInvID := rootinvocations.ID("test-root-inv-agg-1")

		// Insert root invocation.
		testutil.MustApply(ctx, t, insert.RootInvocationWithRootWorkUnit(
			rootinvocations.NewBuilder(rootInvID).
				WithStreamingExportState(pb.RootInvocation_STREAMING_EXPORT_STATE_UNSPECIFIED).
				Build(),
		)...)

		task := &taskspb.PublishTestAggregationsTask{
			RootInvocationId: string(rootInvID),
		}
		p := &testAggregationsPublisher{
			task:             task,
			resultDBHostname: rdbHost,
		}
		err = p.handleTestAggregationsPublisher(ctx)
		assert.Loosely(t, err, should.BeNil)

		assert.Loosely(t, sched.Tasks(), should.HaveLength(0))
	})

	t.Run("Ready for export", func(t *testing.T) {
		rootInvID := rootinvocations.ID("test-root-inv-agg-2")

		// Insert root invocation.
		testutil.MustApply(ctx, t, insert.RootInvocationWithRootWorkUnit(
			rootinvocations.NewBuilder(rootInvID).
				WithStreamingExportState(pb.RootInvocation_METADATA_FINAL).
				WithSources(gitilesSources).
				WithDefinition(rootInvDefinition).
				WithPrimaryBuild(nil).
				Build(),
		)...)
		// Insert test data.
		testutil.MustApply(ctx, t, testaggregations.CreateTestData(rootInvID)...)

		task := &taskspb.PublishTestAggregationsTask{
			RootInvocationId: string(rootInvID),
		}
		p := &testAggregationsPublisher{
			task:             task,
			resultDBHostname: rdbHost,
		}
		err = p.handleTestAggregationsPublisher(ctx)
		assert.Loosely(t, err, should.BeNil)

		allTasks := sched.Tasks()
		var notifyTasks tqtesting.TaskList
		for _, task := range allTasks {
			if task.Class == "notify-test-aggregations" {
				notifyTasks = append(notifyTasks, task)
			}
		}

		expectedNotifications := []*pb.TestAggregationsNotification{
			{
				RootInvocation:   rootInvID.Name(),
				ResultdbHost:     rdbHost,
				TestAggregations: withoutMatchFields(testaggregations.ExpectedRootInvocationAggregation()),
				AggregationLevel: pb.AggregationLevel_INVOCATION,
			},
			{
				RootInvocation:   rootInvID.Name(),
				ResultdbHost:     rdbHost,
				TestAggregations: withoutMatchFields(testaggregations.ExpectedModuleAggregationsIDOrder()),
				AggregationLevel: pb.AggregationLevel_MODULE,
			},
			{
				RootInvocation:   rootInvID.Name(),
				ResultdbHost:     rdbHost,
				TestAggregations: withoutMatchFields(testaggregations.ExpectedCoarseAggregationsIDOrder()),
				AggregationLevel: pb.AggregationLevel_COARSE,
			},
			{
				RootInvocation:   rootInvID.Name(),
				ResultdbHost:     rdbHost,
				TestAggregations: withoutMatchFields(testaggregations.ExpectedFineAggregationsIDOrder()),
				AggregationLevel: pb.AggregationLevel_FINE,
			},
		}
		expectedAttributes := map[string]string{
			luciProjectFilter:    "testproject",
			definitionNameFilter: "test-definition",
		}

		assert.Loosely(t, notifyTasks, should.HaveLength(len(expectedNotifications)))

		for i, expected := range expectedNotifications {
			notifyTask := notifyTasks[i]

			// Ignore TQ internal attribute.
			attrs := notifyTask.Message.GetAttributes()
			delete(attrs, "X-Luci-Tq-Reminder-Id")

			// Check aggregation_level attribute.
			expectedLevel := expected.TestAggregations[0].Id.Level.String()
			assert.Loosely(t, attrs[aggregationLevelFilter], should.Equal(expectedLevel))
			delete(attrs, aggregationLevelFilter)

			assert.Loosely(t, attrs, should.Match(expectedAttributes))

			payload := notifyTask.Payload.(*taskspb.PublishTestAggregations)
			assert.Loosely(t, payload.Message, should.Match(expected))
		}

		// Check for the initial task's checkpoint.
		checkpointKey := checkpoints.Key{
			Project:    "testproject",
			ResourceID: string(rootInvID),
			ProcessID:  CheckpointProcessID,
			Uniquifier: "level/0/pageTokens/",
		}
		exists, err := checkpoints.Exists(span.Single(ctx), checkpointKey)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, exists, should.BeTrue)
	})

	t.Run("Checkpoints", func(t *testing.T) {
		ctx, sched := tq.TestingContext(ctx, nil)
		rootInvID := rootinvocations.ID("test-root-inv-agg-checkpoints")

		// Insert root invocation.
		testutil.MustApply(ctx, t, insert.RootInvocationWithRootWorkUnit(
			rootinvocations.NewBuilder(rootInvID).
				WithStreamingExportState(pb.RootInvocation_METADATA_FINAL).
				WithSources(gitilesSources).
				WithDefinition(rootInvDefinition).
				Build(),
		)...)

		// Create a checkpoint for the initial task.
		checkpointKey := checkpoints.Key{
			Project:    "testproject",
			ResourceID: string(rootInvID),
			ProcessID:  CheckpointProcessID,
			Uniquifier: "level/0/pageTokens/",
		}
		testutil.MustApply(ctx, t, checkpoints.Insert(ctx, checkpointKey, 10000))

		task := &taskspb.PublishTestAggregationsTask{
			RootInvocationId: string(rootInvID),
		}
		p := &testAggregationsPublisher{
			task:             task,
			resultDBHostname: rdbHost,
		}
		err = p.handleTestAggregationsPublisher(ctx)
		assert.Loosely(t, err, should.BeNil)

		// Should not have scheduled any tasks because checkpoint exists.
		assert.Loosely(t, sched.Tasks(), should.HaveLength(0))
	})

	t.Run("Continuation with a small message limit and multiple continuation tasks", func(t *testing.T) {
		ctx, sched := tq.TestingContext(ctx, nil)
		rootInvID := rootinvocations.ID("test-root-inv-agg-continuation")

		// Insert root invocation.
		testutil.MustApply(ctx, t, insert.RootInvocationWithRootWorkUnit(
			rootinvocations.NewBuilder(rootInvID).
				WithStreamingExportState(pb.RootInvocation_METADATA_FINAL).
				WithSources(gitilesSources).
				WithDefinition(rootInvDefinition).
				Build(),
		)...)
		testutil.MustApply(ctx, t, testaggregations.CreateTestData(rootInvID)...)

		// Initial task.
		task := &taskspb.PublishTestAggregationsTask{
			RootInvocationId: string(rootInvID),
		}

		// Set a small limit to force continuation.
		p := &testAggregationsPublisher{
			task:                 task,
			resultDBHostname:     rdbHost,
			maxPubSubMessageSize: 500,
		}

		// Run initial task.
		err = p.handleTestAggregationsPublisher(ctx)
		assert.Loosely(t, err, should.BeNil)

		// Process continuation tasks.
		processedTasks := 1
		taskIndex := 0

		for {
			currentTasks := sched.Tasks()
			foundContinuation := false
			// Check new tasks since last check.
			for i := taskIndex; i < len(currentTasks); i++ {
				tsk := currentTasks[i]
				if tsk.Class == "publish-test-aggregations" {
					// Found a continuation task.
					payload := tsk.Payload.(*taskspb.PublishTestAggregationsTask)
					p := &testAggregationsPublisher{
						task:                 payload,
						resultDBHostname:     rdbHost,
						maxPubSubMessageSize: 500,
					}
					err = p.handleTestAggregationsPublisher(ctx)
					assert.Loosely(t, err, should.BeNil)
					processedTasks++
					foundContinuation = true

					// Update taskIndex to the end of currently known tasks to avoid
					// re-processing. We break to refresh currentTasks because
					// handleTestAggregationsPublisher might have added more.
					taskIndex = len(currentTasks)
					break
				}
			}
			if !foundContinuation {
				// No new continuation task found in the new batch.
				break
			}
		}

		// Verify we processed multiple tasks.
		assert.Loosely(t, processedTasks, should.BeGreaterThan(1))

		// Verify notifications.
		allTasks := sched.Tasks()
		var notifyTasks tqtesting.TaskList
		for _, task := range allTasks {
			if task.Class == "notify-test-aggregations" {
				notifyTasks = append(notifyTasks, task)
			}
		}

		// We expect all aggregations to be published eventually.
		// Total aggregations: 17 (1 Invocation + 6 Module + 4 Coarse + 6 Fine).
		var allAggregations []*pb.TestAggregation
		for _, nt := range notifyTasks {
			payload := nt.Payload.(*taskspb.PublishTestAggregations)
			allAggregations = append(allAggregations, payload.Message.TestAggregations...)
		}
		assert.Loosely(t, len(allAggregations), should.Equal(18))
	})
}

func withoutMatchFields(aggs []*pb.TestAggregation) []*pb.TestAggregation {
	result := make([]*pb.TestAggregation, len(aggs))
	for i, agg := range aggs {
		copy := proto.Clone(agg).(*pb.TestAggregation)
		copy.MatchedModule = false
		copy.MatchedVerdictCounts = nil
		result[i] = copy
	}
	return result
}
