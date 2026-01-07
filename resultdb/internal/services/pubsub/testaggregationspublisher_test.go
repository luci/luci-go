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

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/tq"
	"go.chromium.org/luci/server/tq/tqtesting"

	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/tasks/taskspb"
	"go.chromium.org/luci/resultdb/internal/testutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestHandleTestAggregationsPublisher(t *testing.T) {
	t.Run("HandleTestAggregationsPublisher", func(t *testing.T) {
		rootInvID := rootinvocations.ID("test-root-inv-agg")
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

		testCases := []struct {
			name                 string
			rootInvBuilder       *rootinvocations.Builder
			expectedAttributes   map[string]string
			expectedNotification *pb.TestAggregationsNotification
		}{
			{
				name:           "StreamingExportState not METADATA_FINAL",
				rootInvBuilder: rootinvocations.NewBuilder(rootInvID).WithStreamingExportState(pb.RootInvocation_STREAMING_EXPORT_STATE_UNSPECIFIED),
			},
			{
				name:               "Ready for export",
				rootInvBuilder:     rootinvocations.NewBuilder(rootInvID).WithStreamingExportState(pb.RootInvocation_METADATA_FINAL).WithSources(gitilesSources).WithDefinition(rootInvDefinition).WithPrimaryBuild(nil),
				expectedAttributes: map[string]string{luciProjectFilter: "testproject", definitionNameFilter: "test-definition"},
				expectedNotification: &pb.TestAggregationsNotification{
					RootInvocation: rootInvID.Name(),
					ResultdbHost:   rdbHost,
				},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				ctx := testutil.SpannerTestContext(t)
				ctx, sched := tq.TestingContext(ctx, nil)

				// Insert the root invocation for this test case.
				muts := rootinvocations.InsertForTesting(tc.rootInvBuilder.Build())
				testutil.MustApply(ctx, t, muts...)

				task := &taskspb.PublishTestAggregationsTask{
					RootInvocationId: string(rootInvID),
				}
				p := &testAggregationsPublisher{
					task:             task,
					resultDBHostname: rdbHost,
				}
				err := p.handleTestAggregationsPublisher(ctx)
				assert.Loosely(t, err, should.BeNil)

				allTasks := sched.Tasks()
				var notifyTasks tqtesting.TaskList
				for _, task := range allTasks {
					if task.Class == "notify-test-aggregations" {
						notifyTasks = append(notifyTasks, task)
					}
				}

				if tc.expectedNotification == nil {
					assert.Loosely(t, notifyTasks, should.HaveLength(0))
					return
				}
				assert.Loosely(t, notifyTasks, should.HaveLength(1))
				notifyTask := notifyTasks[0]

				// Ignore TQ internal attribute.
				attrs := notifyTask.Message.GetAttributes()
				delete(attrs, "X-Luci-Tq-Reminder-Id")
				assert.Loosely(t, attrs, should.Match(tc.expectedAttributes))

				payload := notifyTask.Payload.(*taskspb.PublishTestAggregations)
				assert.Loosely(t, payload.Message, should.Match(tc.expectedNotification))
			})
		}
	})
}
