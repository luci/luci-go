// Copyright 2025 The LUCI Authors.
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
	"time"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/tq"
	"go.chromium.org/luci/server/tq/tqtesting"

	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/tasks/taskspb"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/workunits"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestHandleWorkUnitPublisher(t *testing.T) {
	t.Run("HandleWorkUnitPublisher", func(t *testing.T) {
		rootInvID := rootinvocations.ID("test-root-inv")
		rdbHost := "results.api.cr.dev"

		wuID1 := workunits.ID{RootInvocationID: rootInvID, WorkUnitID: "wu1"}
		wuID2 := workunits.ID{RootInvocationID: rootInvID, WorkUnitID: "wu2"}

		testCases := []struct {
			name                 string
			rootInvBuilder       *rootinvocations.Builder
			workUnitIDs          []string
			extraMutations       []*spanner.Mutation
			expectedNotification *pb.WorkUnitsNotification
			expectedAttributes   map[string]string
		}{
			{
				name:           "StreamingExportState not METADATA_FINAL",
				rootInvBuilder: rootinvocations.NewBuilder(rootInvID).WithStreamingExportState(pb.RootInvocation_STREAMING_EXPORT_STATE_UNSPECIFIED),
				workUnitIDs:    []string{wuID1.WorkUnitID},
			},
			{
				name:           "Success",
				rootInvBuilder: rootinvocations.NewBuilder(rootInvID).WithStreamingExportState(pb.RootInvocation_METADATA_FINAL),
				workUnitIDs:    []string{wuID1.WorkUnitID, wuID2.WorkUnitID},
				extraMutations: []*spanner.Mutation{
					spanutil.InsertMap("Invocations", map[string]any{
						"InvocationId":             wuID1.LegacyInvocationID().RowID(),
						"ShardId":                  0,
						"State":                    pb.Invocation_FINALIZED,
						"Realm":                    "testproject:testrealm",
						"InvocationExpirationTime": time.Now().Add(time.Hour),
						"CreateTime":               spanner.CommitTimestamp,
						"Deadline":                 time.Now().Add(time.Hour),
					}),
					spanutil.InsertMap("Artifacts", map[string]any{
						"InvocationId": wuID1.LegacyInvocationID().RowID(),
						"ParentId":     "",
						"ArtifactId":   "a",
						"ContentType":  "text/plain",
						"Size":         100,
					}),
				},
				expectedNotification: &pb.WorkUnitsNotification{
					ResultdbHost: rdbHost,
					WorkUnits: []*pb.WorkUnitsNotification_WorkUnitDetails{
						{
							WorkUnitName: pbutil.WorkUnitName(string(rootInvID), wuID1.WorkUnitID),
							HasArtifacts: true,
						},
						{
							WorkUnitName: pbutil.WorkUnitName(string(rootInvID), wuID2.WorkUnitID),
							HasArtifacts: false,
						},
					},
				},
				expectedAttributes: map[string]string{
					"luci_project":                 "testproject",
					"definition_name":              "project/bucket/builder",
					"primary_build_android_branch": "git_main",
					"primary_build_android_target": "some-target",
				},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				ctx := testutil.SpannerTestContext(t)
				ctx, sched := tq.TestingContext(ctx, nil)

				// Insert the root invocation for this test case.
				muts := rootinvocations.InsertForTesting(tc.rootInvBuilder.Build())
				muts = append(muts, tc.extraMutations...)

				testutil.MustApply(ctx, t, muts...)

				task := &taskspb.PublishWorkUnitsTask{
					RootInvocationId: string(rootInvID),
					WorkUnitIds:      tc.workUnitIDs,
				}
				p := &workUnitPublisher{
					task:             task,
					resultDBHostname: rdbHost,
				}
				err := p.handleWorkUnitPublisher(ctx)
				assert.Loosely(t, err, should.BeNil)

				allTasks := sched.Tasks()
				var notifyTasks tqtesting.TaskList
				for _, task := range allTasks {
					if task.Class == "notify-work-units" {
						notifyTasks = append(notifyTasks, task)
					}
				}

				if tc.expectedNotification == nil {
					assert.Loosely(t, notifyTasks, should.HaveLength(0))
					return
				}

				assert.Loosely(t, notifyTasks, should.HaveLength(1))
				notifyTask := notifyTasks[0]
				payload := notifyTask.Payload.(*taskspb.PublishWorkUnits)
				assert.Loosely(t, payload.Message, should.Match(tc.expectedNotification))

				// Ignore TQ internal attribute.
				attrs := notifyTask.Message.GetAttributes()
				delete(attrs, "X-Luci-Tq-Reminder-Id")
				assert.Loosely(t, attrs, should.Match(tc.expectedAttributes))
			})
		}
	})
}
