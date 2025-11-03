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
package finalizer

import (
	"testing"

	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/tq"
	"go.chromium.org/luci/server/tq/tqtesting"

	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/tasks/taskspb"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/workunits"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestHandlePublishTestResultsTask(t *testing.T) {
	ftt.Run("HandlePublishTestResultsTask", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		ctx, sched := tq.TestingContext(ctx, nil)
		rootInvID := rootinvocations.ID("test-root-inv")
		rdbHost := "rdb-host"

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
					ResultdbHost:          rdbHost,
					TestResultsByWorkUnit: []*pb.TestResultsNotification_TestResultsByWorkUnit{{WorkUnitName: wuID1.Name()}},
				},
			},
			{
				name:               "Android Branch from Properties",
				rootInvBuilder:     rootinvocations.NewBuilder(rootInvID).WithStreamingExportState(pb.RootInvocation_METADATA_FINAL).WithProperties(androidProperties).WithSources(gitilesSources),
				finalizedWUIDs:     []string{wuID1.WorkUnitID},
				expectedAttributes: map[string]string{"branch": "git_main"},
				expectedNotification: &pb.TestResultsNotification{
					ResultdbHost:          rdbHost,
					TestResultsByWorkUnit: []*pb.TestResultsNotification_TestResultsByWorkUnit{{WorkUnitName: wuID1.Name()}},
					Sources:               gitilesSources,
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
						{WorkUnitName: wuID1.Name()},
						{WorkUnitName: wuID2.Name()},
					},
					Sources: gitilesSources,
				},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *ftt.Test) {
				// Reset tasks.
				sched.Tasks()
				testutil.MustApply(ctx, t, rootinvocations.InsertForTesting(tc.rootInvBuilder.Build())...)

				task := &taskspb.PublishTestResultsTask{
					RootInvocationId: string(rootInvID),
					WorkUnitIds:      tc.finalizedWUIDs,
				}
				err := handlePublishTestResultsTask(ctx, task, rdbHost)
				assert.Loosely(t, err, should.BeNil)

				allTasks := sched.Tasks()
				var tasks tqtesting.TaskList
				for _, task := range allTasks {
					if task.Class == "notify-test-results" {
						tasks = append(tasks, task)
					}
				}

				for _, task := range tasks {
					attrs := task.Message.GetAttributes()
					delete(attrs, "X-Luci-Tq-Reminder-Id") // Ignore TQ internal attribute
					assert.Loosely(t, attrs, should.Match(tc.expectedAttributes))

					payload := task.Payload.(*taskspb.PublishTestResults)
					// Clear DeduplicationKey as it contains timestamp.                                                              │
					payload.Message.DeduplicationKey = ""
					assert.Loosely(t, payload.Message, should.Match(tc.expectedNotification))
				}
			})
		}
	})
}
