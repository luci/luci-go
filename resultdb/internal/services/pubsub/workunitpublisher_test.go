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

	"cloud.google.com/go/spanner"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/tq"
	"go.chromium.org/luci/server/tq/tqtesting"

	"go.chromium.org/luci/resultdb/internal/config"
	"go.chromium.org/luci/resultdb/internal/masking"
	"go.chromium.org/luci/resultdb/internal/permissions"
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

		cfgProto := config.CreatePlaceholderServiceConfig()
		compiledCfg, err := config.NewCompiledServiceConfig(cfgProto, "")
		assert.Loosely(t, err, should.BeNil)

		wuID1 := workunits.ID{RootInvocationID: rootInvID, WorkUnitID: "wu1"}
		wuID2 := workunits.ID{RootInvocationID: rootInvID, WorkUnitID: "wu2"}

		rootProps := &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"root_key":   structpb.NewStringValue("root_val"),
				"shared_key": structpb.NewStringValue("root_shared_val"),
			},
		}
		wu1Props := &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"wu1_key":    structpb.NewStringValue("wu1_val"),
				"shared_key": structpb.NewStringValue("wu1_shared_val"),
			},
		}
		wu2Props := &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"wu2_key": structpb.NewStringValue("wu2_val"),
			},
		}

		rootWU := workunits.NewBuilder(rootInvID, "root").WithMinimalFields().WithFinalizationState(pb.WorkUnit_FINALIZED).WithInheritedProperties(rootProps).Build()
		wu1 := workunits.NewBuilder(rootInvID, "wu1").WithMinimalFields().WithParentWorkUnitID("root").WithFinalizationState(pb.WorkUnit_FINALIZED).WithInheritedProperties(wu1Props).Build()
		wu2 := workunits.NewBuilder(rootInvID, "wu2").WithMinimalFields().WithParentWorkUnitID("root").WithFinalizationState(pb.WorkUnit_FINALIZED).WithInheritedProperties(wu2Props).Build()

		successMutations := []*spanner.Mutation{}
		successMutations = append(successMutations, workunits.InsertForTesting(rootWU)...)
		successMutations = append(successMutations, workunits.InsertForTesting(wu1)...)
		successMutations = append(successMutations, workunits.InsertForTesting(wu2)...)
		successMutations = append(successMutations, spanutil.InsertMap("Artifacts", map[string]any{
			"InvocationId": wuID1.LegacyInvocationID().RowID(),
			"ParentId":     "",
			"ArtifactId":   "a",
			"ContentType":  "text/plain",
			"Size":         100,
		}))

		expectedWU1Props := &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"root_key":   structpb.NewStringValue("root_val"),
				"wu1_key":    structpb.NewStringValue("wu1_val"),
				"shared_key": structpb.NewStringValue("wu1_shared_val"),
			},
		}
		expectedWU2Props := &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"root_key":   structpb.NewStringValue("root_val"),
				"wu2_key":    structpb.NewStringValue("wu2_val"),
				"shared_key": structpb.NewStringValue("root_shared_val"),
			},
		}

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
				extraMutations: successMutations,
				expectedNotification: &pb.WorkUnitsNotification{
					ResultdbHost: rdbHost,
					WorkUnits: []*pb.WorkUnitsNotification_WorkUnitDetails{
						{
							WorkUnitName:              pbutil.WorkUnitName(string(rootInvID), wuID1.WorkUnitID),
							HasArtifacts:              true,
							MergedInheritedProperties: expectedWU1Props,
							WorkUnit:                  masking.WorkUnit(wu1, permissions.FullAccess, pb.WorkUnitView_WORK_UNIT_VIEW_BASIC, compiledCfg),
						},
						{
							WorkUnitName:              pbutil.WorkUnitName(string(rootInvID), wuID2.WorkUnitID),
							HasArtifacts:              false,
							MergedInheritedProperties: expectedWU2Props,
							WorkUnit:                  masking.WorkUnit(wu2, permissions.FullAccess, pb.WorkUnitView_WORK_UNIT_VIEW_BASIC, compiledCfg),
						},
					},
					RootInvocationMetadata: masking.RootInvocationMetadata(rootinvocations.NewBuilder(rootInvID).WithStreamingExportState(pb.RootInvocation_METADATA_FINAL).Build(), compiledCfg),
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
				ctx = caching.WithEmptyProcessCache(ctx) // For config in-process cache.
				ctx = memory.Use(ctx)                    // For config datastore cache.

				// Set up a placeholder service config in context.
				err := config.SetServiceConfigForTesting(ctx, cfgProto)
				assert.Loosely(t, err, should.BeNil)

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
				err = p.handleWorkUnitPublisher(ctx)
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

func TestHandleWorkUnitPublisher_SizeBasedSplitting(t *testing.T) {
	rootInvID := rootinvocations.ID("test-root-inv-split")
	rdbHost := "results.api.cr.dev"

	cfgProto := config.CreatePlaceholderServiceConfig()
	compiledCfg, err := config.NewCompiledServiceConfig(cfgProto, "")
	assert.Loosely(t, err, should.BeNil)

	rootWU := workunits.NewBuilder(rootInvID, "root").WithMinimalFields().WithFinalizationState(pb.WorkUnit_FINALIZED).Build()
	wu1 := workunits.NewBuilder(rootInvID, "wu1").WithMinimalFields().WithParentWorkUnitID("root").WithFinalizationState(pb.WorkUnit_FINALIZED).Build()
	wu2 := workunits.NewBuilder(rootInvID, "wu2").WithMinimalFields().WithParentWorkUnitID("root").WithFinalizationState(pb.WorkUnit_FINALIZED).Build()

	// Override maxWorkUnitPubSubMessageSize to a small value.
	oldLimit := maxWorkUnitPubSubMessageSize
	// Calculate size of one basic notification to choose a good limit.
	// We want to fit exactly one work unit per notification.
	baseNotification := &pb.WorkUnitsNotification{
		ResultdbHost:           rdbHost,
		RootInvocationMetadata: masking.RootInvocationMetadata(rootinvocations.NewBuilder(rootInvID).WithStreamingExportState(pb.RootInvocation_METADATA_FINAL).Build(), compiledCfg),
	}
	baseSize := proto.Size(baseNotification)

	// Make a dummy WorkUnitDetails to check size.
	dummyWUDetail := &pb.WorkUnitsNotification_WorkUnitDetails{
		WorkUnitName: pbutil.WorkUnitName(string(rootInvID), "wu1"),
		WorkUnit:     masking.WorkUnit(wu1, permissions.FullAccess, pb.WorkUnitView_WORK_UNIT_VIEW_BASIC, compiledCfg),
	}
	wuSize := proto.Size(dummyWUDetail)

	// Set limit to fit base + one work unit, but not two.
	maxWorkUnitPubSubMessageSize = baseSize + wuSize + 10 // buffer
	defer func() { maxWorkUnitPubSubMessageSize = oldLimit }()

	muts := []*spanner.Mutation{}
	muts = append(muts, workunits.InsertForTesting(rootWU)...)
	muts = append(muts, workunits.InsertForTesting(wu1)...)
	muts = append(muts, workunits.InsertForTesting(wu2)...)

	ctx := testutil.SpannerTestContext(t)
	ctx = caching.WithEmptyProcessCache(ctx)
	ctx = memory.Use(ctx)

	err = config.SetServiceConfigForTesting(ctx, cfgProto)
	assert.Loosely(t, err, should.BeNil)

	ctx, sched := tq.TestingContext(ctx, nil)

	// Insert the root invocation.
	rootInv := rootinvocations.NewBuilder(rootInvID).WithStreamingExportState(pb.RootInvocation_METADATA_FINAL).Build()
	testutil.MustApply(ctx, t, rootinvocations.InsertForTesting(rootInv)...)
	testutil.MustApply(ctx, t, muts...)

	task := &taskspb.PublishWorkUnitsTask{
		RootInvocationId: string(rootInvID),
		WorkUnitIds:      []string{"wu1", "wu2"},
	}
	p := &workUnitPublisher{
		task:             task,
		resultDBHostname: rdbHost,
	}
	err = p.handleWorkUnitPublisher(ctx)
	assert.Loosely(t, err, should.BeNil)

	allTasks := sched.Tasks()
	var notifyTasks tqtesting.TaskList
	for _, task := range allTasks {
		if task.Class == "notify-work-units" {
			notifyTasks = append(notifyTasks, task)
		}
	}

	// Should be split into 2 tasks.
	assert.Loosely(t, notifyTasks, should.HaveLength(2))

	// Verify content of tasks.
	// Order might not be guaranteed, so sort or identify by WorkUnitName.
	wuNames := []string{
		notifyTasks[0].Payload.(*taskspb.PublishWorkUnits).Message.WorkUnits[0].WorkUnitName,
		notifyTasks[1].Payload.(*taskspb.PublishWorkUnits).Message.WorkUnits[0].WorkUnitName,
	}
	expectedNames := []string{
		pbutil.WorkUnitName(string(rootInvID), "wu1"),
		pbutil.WorkUnitName(string(rootInvID), "wu2"),
	}

	// We expect one of each.
	assert.Loosely(t, wuNames, should.Contain(expectedNames[0]))
	assert.Loosely(t, wuNames, should.Contain(expectedNames[1]))

	// Both should have length 1.
	assert.Loosely(t, notifyTasks[0].Payload.(*taskspb.PublishWorkUnits).Message.WorkUnits, should.HaveLength(1))
	assert.Loosely(t, notifyTasks[1].Payload.(*taskspb.PublishWorkUnits).Message.WorkUnits, should.HaveLength(1))
}
