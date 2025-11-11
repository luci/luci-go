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
	"testing"

	"cloud.google.com/go/spanner"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/tq"
	"go.chromium.org/luci/server/tq/tqtesting"

	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/tasks/taskspb"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/internal/workunits"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

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
			expectedTRs1[i] = proto.Clone(tr).(*pb.TestResult)
			expectedTRs1[i].Name = pbutil.TestResultName(string(rootInvID), wuID1.WorkUnitID, tr.TestId, tr.ResultId)

			// Add default empty variant if it's nil.
			if expectedTRs1[i].Variant == nil {
				expectedTRs1[i].Variant = &pb.Variant{}
			}
			if expectedTRs1[i].TestIdStructured != nil && expectedTRs1[i].TestIdStructured.ModuleVariant == nil {
				expectedTRs1[i].TestIdStructured.ModuleVariant = &pb.Variant{}
			}
		}
		expectedTRs2 := make([]*pb.TestResult, len(trs2))
		for i, tr := range trs2 {
			expectedTRs2[i] = proto.Clone(tr).(*pb.TestResult)
			expectedTRs2[i].Name = pbutil.TestResultName(string(rootInvID), wuID2.WorkUnitID, tr.TestId, tr.ResultId)
			// Add default empty variant if it's nil.
			if expectedTRs2[i].Variant == nil {
				expectedTRs2[i].Variant = &pb.Variant{}
			}
			if expectedTRs2[i].TestIdStructured != nil && expectedTRs2[i].TestIdStructured.ModuleVariant == nil {
				expectedTRs2[i].TestIdStructured.ModuleVariant = &pb.Variant{}
			}
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
				ctx, sched := tq.TestingContext(ctx, nil)

				// Insert the root invocation for this test case.
				muts := rootinvocations.InsertForTesting(tc.rootInvBuilder.Build())
				testutil.MustApply(ctx, t, muts...)
				defer testutil.MustApply(ctx, t, spanner.Delete("RootInvocations", rootInvID.Key()))

				// Reset tasks.
				sched.Tasks()

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

				// Clear DeduplicationKey as it contains timestamp.
				payload := notifyTask.Payload.(*taskspb.PublishTestResults)
				payload.Message.DeduplicationKey = ""

				// Sort TestResultsByWorkUnit to make the test deterministic.
				sort.Slice(payload.Message.TestResultsByWorkUnit, func(i, j int) bool {
					return payload.Message.TestResultsByWorkUnit[i].WorkUnitName < payload.Message.TestResultsByWorkUnit[j].WorkUnitName
				})
				assert.Loosely(t, payload.Message, should.Match(tc.expectedNotification))
			})
		}
	})
}
