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

package verdictingester

import (
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"

	antsinvocationexporter "go.chromium.org/luci/analysis/internal/ants/invocations/exporter"
	ctrlpb "go.chromium.org/luci/analysis/internal/ingestion/control/proto"
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
	"go.chromium.org/luci/analysis/internal/testutil"
	bqpblegacy "go.chromium.org/luci/analysis/proto/bq/legacy"
)

func TestExportAntsInvocation(t *testing.T) {
	ftt.Run("TestExportTestVerdicts", t, func(t *ftt.Test) {
		ctx := testutil.IntegrationTestContext(t)
		partitionTime := clock.Now(ctx).Add(-1 * time.Hour)
		invocationCreationTime := partitionTime.Add(-3 * time.Hour)

		payload := &taskspb.IngestTestVerdicts{
			IngestionId: "ingestion-id",
			Project:     "android",
			Invocation: &ctrlpb.InvocationResult{
				ResultdbHost: "rdb-host",
				InvocationId: invocationID,
				CreationTime: timestamppb.New(invocationCreationTime),
			},
			PartitionTime:        timestamppb.New(partitionTime),
			PageToken:            "expected_token",
			TaskIndex:            1,
			UseNewIngestionOrder: true,
		}
		input := Inputs{
			Verdicts: mockedQueryTestVariantsRsp().TestVariants,
			Payload:  payload,
			LastPage: true,
		}
		antsInvocationClient := antsinvocationexporter.NewFakeClient()
		ingester := AnTSTInvocationExporter{exporter: antsinvocationexporter.NewExporter(antsInvocationClient)}
		t.Run("baseline", func(t *ftt.Test) {
			t.Run("root invocation", func(t *ftt.Test) {
				input.Invocation = nil
				input.RootInvocation = &rdbpb.RootInvocation{
					Name:            "rootInvocations/inv-123",
					State:           rdbpb.RootInvocation_SUCCEEDED,
					CreateTime:      timestamppb.New(time.Unix(1000, 0)),
					FinalizeTime:    timestamppb.New(time.Unix(2000, 0)),
					SummaryMarkdown: "summary",
					Creator:         "user:someone@example.com",
					Tags: []*rdbpb.StringPair{
						{Key: "key1", Value: "value1"},
						{Key: "runner", Value: "runner1"},
						{Key: "trigger", Value: "trigger1"},
						{Key: "test_label", Value: "label1"},
						{Key: "test_label", Value: "label2"},
						{Key: "user", Value: "user1"},
						{Key: "device_tracing_enable", Value: "true"},
					},
					PrimaryBuild: &rdbpb.BuildDescriptor{
						Definition: &rdbpb.BuildDescriptor_AndroidBuild{
							AndroidBuild: &rdbpb.AndroidBuildDescriptor{
								BuildId:     "12345",
								BuildTarget: "target",
								Branch:      "branch",
							},
						},
					},
					ExtraBuilds: []*rdbpb.BuildDescriptor{
						{
							Definition: &rdbpb.BuildDescriptor_AndroidBuild{
								AndroidBuild: &rdbpb.AndroidBuildDescriptor{
									BuildId:     "67890",
									BuildTarget: "target2",
									Branch:      "branch2",
								},
							},
						},
					},
					Definition: &rdbpb.RootInvocationDefinition{
						Name: "test-name",
						Properties: &rdbpb.RootInvocationDefinition_Properties{
							Def: map[string]string{"prop1": "val1"},
						},
					},
					ProducerResource: &rdbpb.ProducerResource{
						System: "tradefed",
					},
				}

				err := ingester.Ingest(ctx, input)
				assert.NoErr(t, err)
				assert.Loosely(t, antsInvocationClient.Insertion, should.Match(&bqpblegacy.AntsInvocationRow{
					InvocationId:   "inv-123",
					SchedulerState: bqpblegacy.SchedulerState_COMPLETED,
					Timing: &bqpblegacy.Timing{
						CreationTimestamp: 1000000,
						CompleteTimestamp: 2000000,
					},
					CompletionTime: timestamppb.New(time.Unix(2000, 0)),
					Properties: []*bqpblegacy.StringPair{
						{Name: "key1", Value: "value1"},
						{Name: "runner", Value: "runner1"},
						{Name: "trigger", Value: "trigger1"},
						{Name: "test_label", Value: "label1"},
						{Name: "test_label", Value: "label2"},
						{Name: "user", Value: "user1"},
						{Name: "device_tracing_enable", Value: "true"},
					},
					Summary:    "summary",
					Users:      []string{"user1", "user:someone@example.com"},
					Scheduler:  "tradefed",
					Runner:     "runner1",
					Trigger:    "trigger1",
					TestLabels: []string{"label1", "label2"},
					BuildId:    "12345",
					PrimaryBuild: &bqpblegacy.AntsInvocationRow_AndroidBuild{
						BuildProvider: "androidbuild",
						BuildId:       "12345",
						BuildTarget:   "target",
						Branch:        "branch",
					},
					ExtraBuilds: []*bqpblegacy.AntsInvocationRow_AndroidBuild{
						{
							BuildProvider: "androidbuild",
							BuildId:       "67890",
							BuildTarget:   "target2",
							Branch:        "branch2",
						},
					},
					Test: &bqpblegacy.Test{
						Name: "test-name",
						Properties: []*bqpblegacy.StringPair{
							{Name: "prop1", Value: "val1"},
						},
					},
					Tags: []string{"device_tracing_enable", "tf_invocation"},
				}))
			})

			t.Run("legacy invocation", func(t *ftt.Test) {
				input.RootInvocation = nil
				input.Invocation = &rdbpb.Invocation{
					Name:         testInvocation,
					Realm:        testRealm,
					IsExportRoot: true,
					FinalizeTime: timestamppb.New(time.Date(2025, 4, 8, 0, 0, 0, 0, time.UTC)),
				}

				err := ingester.Ingest(ctx, input)
				assert.NoErr(t, err)
				expectedRow := &bqpblegacy.AntsInvocationRow{
					InvocationId: "build-87654321",
					Timing: &bqpblegacy.Timing{
						CreationTimestamp: 0,
						CompleteTimestamp: 1744070400000,
					},
					CompletionTime: timestamppb.New(time.Date(2025, 4, 8, 0, 0, 0, 0, time.UTC)),
				}
				assert.Loosely(t, antsInvocationClient.Insertion, should.Match(expectedRow))
			})
		})

		t.Run("not android project", func(t *ftt.Test) {
			payload.Project = "other"

			err := ingester.Ingest(ctx, input)
			assert.NoErr(t, err)
			assert.Loosely(t, antsInvocationClient.Insertion, should.BeNil)
		})
	})
}
