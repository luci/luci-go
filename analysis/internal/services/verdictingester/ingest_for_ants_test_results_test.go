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
	"fmt"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/resultdb/pbutil"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"

	antstestresultexporter "go.chromium.org/luci/analysis/internal/ants/testresults/exporter"
	"go.chromium.org/luci/analysis/internal/checkpoints"
	ctrlpb "go.chromium.org/luci/analysis/internal/ingestion/control/proto"
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
	"go.chromium.org/luci/analysis/internal/testutil"
	bqpblegacy "go.chromium.org/luci/analysis/proto/bq/legacy"
)

func TestExportAntsTestResults(t *testing.T) {
	ftt.Run("TestExportTestVerdicts", t, func(t *ftt.Test) {
		ctx := testutil.IntegrationTestContext(t)
		partitionTime := clock.Now(ctx).Add(-1 * time.Hour)

		verdicts := []*rdbpb.TestVariant{
			{
				TestId:   ":module!junit:package:class#test1",
				Variant:  pbutil.Variant("module_abi", "v1", "atp_test", "test"),
				StatusV2: rdbpb.TestVerdict_PASSED,
				Results: []*rdbpb.TestResultBundle{
					{
						Result: &rdbpb.TestResult{
							ResultId:  "result-1",
							Name:      "rootInvocations/ants-i123456789/workUnits/wu-1/tests/:module%21junit:package:class%23test1/results/result-1",
							StatusV2:  rdbpb.TestResult_PASSED,
							StartTime: timestamppb.New(time.Date(2025, 1, 1, 1, 0, 0, 0, time.UTC)),
							Duration:  durationpb.New(1 * time.Second),
						},
					},
				},
			},
			{
				TestId:   ":module!junit:package:class#test2",
				Variant:  pbutil.Variant("k1", "v2"),
				StatusV2: rdbpb.TestVerdict_FAILED,
				Results: []*rdbpb.TestResultBundle{
					{
						Result: &rdbpb.TestResult{
							ResultId:  "result-2",
							Name:      "rootInvocations/ants-i123456789/workUnits/wu-1/tests/:module%21junit:package:class%23test2/results/result-2",
							StatusV2:  rdbpb.TestResult_SKIPPED,
							StartTime: timestamppb.New(time.Date(2025, 1, 1, 2, 0, 0, 0, time.UTC)),
							Duration:  durationpb.New(1 * time.Second),
							SkippedReason: &rdbpb.SkippedReason{
								ReasonMessage: "assumption failure message",
								Trace:         "assumption failure trace",
								Kind:          rdbpb.SkippedReason_SKIPPED_BY_TEST_BODY,
							},
						},
					},
					{
						Result: &rdbpb.TestResult{
							ResultId:  "result-3",
							Name:      "rootInvocations/ants-i123456789/workUnits/wu-1/tests/:module%21junit:package:class%23test2/results/result-3",
							StatusV2:  rdbpb.TestResult_FAILED,
							StartTime: timestamppb.New(time.Date(2025, 1, 1, 3, 0, 0, 0, time.UTC)),
							Duration:  durationpb.New(1 * time.Second),
							FailureReason: &rdbpb.FailureReason{
								PrimaryErrorMessage: "failure message",
								Errors: []*rdbpb.FailureReason_Error{
									{Message: "failure message", Trace: "failure trace"},
								},
							},
							Tags: []*rdbpb.StringPair{
								{Key: "primary_error_code", Value: "123"},
								{Key: "primary_error_type", Value: "INFRA_ERROR"},
								{Key: "primary_error_name", Value: "test error name"},
								{Key: "primary_error_origin", Value: "test error origin"},
								{Key: "tag1", Value: "tag1-val"},
							},
						},
					},
				},
			},
		}

		payload := &taskspb.IngestTestVerdicts{
			IngestionId:          "ingestion-id",
			Project:              "android",
			PartitionTime:        timestamppb.New(partitionTime),
			PageToken:            "expected_token",
			TaskIndex:            1,
			UseNewIngestionOrder: true,
		}
		input := Inputs{
			Verdicts: verdicts,
			Payload:  payload,
		}
		antsTestResultsClient := antstestresultexporter.NewFakeClient()
		ingester := AnTSTestResultExporter{exporter: antstestresultexporter.NewExporter(antsTestResultsClient)}

		expectedRowsLegacy := []*bqpblegacy.AntsTestResultRow{
			{
				TestResultId: "result-1",
				InvocationId: invocationID,
				TestIdentifier: &bqpblegacy.AntsTestResultRow_TestIdentifier{
					Module:               "module",
					ModuleParameters:     []*bqpblegacy.StringPair{{Name: "module-abi", Value: "v1"}},
					ModuleParametersHash: "a97a3e62fb4fa57e2d30c1d1ace158eca3c87afb9f3e2d097587c6dc993b44c6",
					TestClass:            "package.class",
					ClassName:            "class",
					PackageName:          "package",
					Method:               "test1",
				},
				TestIdentifierHash: "d064bad880712fc5b11b048f5075f52d23d7a470a770f4fb914c03560cfe4c16",
				TestStatus:         bqpblegacy.AntsTestResultRow_PASS,
				Timing: &bqpblegacy.Timing{
					CreationTimestamp: 1735693200000, // 2025-01-01 01:00:00 UTC
					CompleteTimestamp: 1735693201000,
					CreationMonth:     "2025-01",
				},
				TestId:         ":module!junit:package:class#test1",
				CompletionTime: timestamppb.New(time.Date(2025, 4, 8, 0, 0, 0, 0, time.UTC)),
			},
			{
				TestResultId: "result-2",
				InvocationId: invocationID,
				TestIdentifier: &bqpblegacy.AntsTestResultRow_TestIdentifier{
					Module:               "module",
					ModuleParameters:     []*bqpblegacy.StringPair{{Name: "k1", Value: "v2"}},
					ModuleParametersHash: "f0adb88aadb2678fe69caf81773cd3a19a28d7fd41f15ff4bd3f7592a08a79ae",
					TestClass:            "package.class",
					ClassName:            "class",
					PackageName:          "package",
					Method:               "test2",
				},
				TestIdentifierHash: "49a5f3b27583ccf99c7c32e4d69420b77c9480f1687a4f9325f6a7220bf01836",
				TestStatus:         bqpblegacy.AntsTestResultRow_ASSUMPTION_FAILURE,
				DebugInfo: &bqpblegacy.DebugInfo{
					ErrorMessage: "assumption failure message",
					Trace:        "assumption failure trace",
				},
				Timing: &bqpblegacy.Timing{
					CreationTimestamp: 1735696800000, // 2025-01-01 02:00:00 UTC
					CompleteTimestamp: 1735696801000,
					CreationMonth:     "2025-01",
				},
				TestId:         ":module!junit:package:class#test2",
				CompletionTime: timestamppb.New(time.Date(2025, 4, 8, 0, 0, 0, 0, time.UTC)),
			},
			{
				TestResultId: "result-3",
				InvocationId: invocationID,
				TestIdentifier: &bqpblegacy.AntsTestResultRow_TestIdentifier{
					Module:               "module",
					ModuleParameters:     []*bqpblegacy.StringPair{{Name: "k1", Value: "v2"}},
					ModuleParametersHash: "f0adb88aadb2678fe69caf81773cd3a19a28d7fd41f15ff4bd3f7592a08a79ae",
					TestClass:            "package.class",
					ClassName:            "class",
					PackageName:          "package",
					Method:               "test2",
				},
				TestIdentifierHash: "49a5f3b27583ccf99c7c32e4d69420b77c9480f1687a4f9325f6a7220bf01836",
				TestStatus:         bqpblegacy.AntsTestResultRow_FAIL,
				DebugInfo: &bqpblegacy.DebugInfo{
					ErrorMessage: "failure message",
					Trace:        "failure trace",
					ErrorType:    bqpblegacy.ErrorType_INFRA_ERROR,
					ErrorCode:    123,
					ErrorName:    "test error name",
					ErrorOrigin:  "test error origin",
				},
				Timing: &bqpblegacy.Timing{
					CreationTimestamp: 1735700400000, // 2025-01-01 03:00:00 UTC
					CompleteTimestamp: 1735700401000,
					CreationMonth:     "2025-01",
				},
				Properties: []*bqpblegacy.StringPair{
					{Name: "primary_error_code", Value: "123"},
					{Name: "primary_error_type", Value: "INFRA_ERROR"},
					{Name: "primary_error_name", Value: "test error name"},
					{Name: "primary_error_origin", Value: "test error origin"},
					{Name: "tag1", Value: "tag1-val"},
				},
				TestId:         ":module!junit:package:class#test2",
				CompletionTime: timestamppb.New(time.Date(2025, 4, 8, 0, 0, 0, 0, time.UTC)),
			},
		}

		t.Run("legacy invocation", func(t *ftt.Test) {
			payload.Invocation = &ctrlpb.InvocationResult{
				ResultdbHost: "rdb-host",
				InvocationId: invocationID,
				CreationTime: timestamppb.New(partitionTime.Add(-3 * time.Hour)),
			}
			input.Invocation = &rdbpb.Invocation{
				Name:         testInvocation,
				Realm:        testRealm,
				IsExportRoot: true,
				FinalizeTime: timestamppb.New(time.Date(2025, 4, 8, 0, 0, 0, 0, time.UTC)),
			}

			err := ingester.Ingest(ctx, input)
			assert.NoErr(t, err)
			assert.Loosely(t, antsTestResultsClient.Insertions, should.Match(expectedRowsLegacy))
			expectedChangepoint := []checkpoints.Checkpoint{{
				Key: checkpoints.Key{
					Project:    "android",
					ResourceID: fmt.Sprintf("rdb-host/%v", invocationID),
					ProcessID:  "verdict-ingestion/export-ants-test-results",
					Uniquifier: "1",
				},
			}}
			verifyCheckpoints(ctx, t, expectedChangepoint)
		})

		t.Run("root invocation", func(t *ftt.Test) {
			input.RootInvocation = &rdbpb.RootInvocation{
				RootInvocationId: "ants-i123456789",
				FinalizeTime:     timestamppb.New(time.Date(2025, 4, 8, 0, 0, 0, 0, time.UTC)),
				PrimaryBuild: &rdbpb.BuildDescriptor{
					Definition: &rdbpb.BuildDescriptor_AndroidBuild{
						AndroidBuild: &rdbpb.AndroidBuildDescriptor{
							BuildId:     "P81983588",
							Branch:      "git_master",
							BuildTarget: "cf_x86_64_phone-userdebug",
						},
					},
				},
				Definition: &rdbpb.RootInvocationDefinition{
					Name: "test-definition-name",
					Properties: &rdbpb.RootInvocationDefinition_Properties{
						Def: map[string]string{
							"test_prop_1": "test_val_1",
						},
					},
				},
			}
			payload.Invocation = nil
			payload.RootInvocation = &ctrlpb.RootInvocationResult{
				ResultdbHost:     "rdb-host",
				RootInvocationId: "ants-i123456789",
			}

			err := ingester.Ingest(ctx, input)
			assert.NoErr(t, err)
			// Construct expected rows for root invocation
			expectedRowsRootInvocation := make([]*bqpblegacy.AntsTestResultRow, len(expectedRowsLegacy))
			for i, row := range expectedRowsLegacy {
				// Clone the row
				newRow := proto.Clone(row).(*bqpblegacy.AntsTestResultRow)
				newRow.InvocationId = "ants-i123456789"
				newRow.WorkUnitId = "wu-1"
				newRow.BuildId = "P81983588"
				newRow.BuildType = bqpblegacy.BuildType_PENDING
				newRow.BuildTarget = "cf_x86_64_phone-userdebug"
				newRow.BuildProvider = "androidbuild"
				newRow.Branch = "git_master"
				newRow.Test = &bqpblegacy.Test{
					Name:       "test-definition-name",
					Properties: []*bqpblegacy.StringPair{{Name: "test_prop_1", Value: "test_val_1"}},
				}
				expectedRowsRootInvocation[i] = newRow
			}

			assert.Loosely(t, antsTestResultsClient.Insertions, should.Match(expectedRowsRootInvocation))
			expectedChangepoint := []checkpoints.Checkpoint{
				{
					Key: checkpoints.Key{
						Project:    "android",
						ResourceID: fmt.Sprintf("rootInvocation/rdb-host/%v", "ants-i123456789"),
						ProcessID:  "verdict-ingestion/export-ants-test-results",
						Uniquifier: "1",
					},
				},
			}
			verifyCheckpoints(ctx, t, expectedChangepoint)
		})

		t.Run("not android project", func(t *ftt.Test) {
			payload.Project = "other"

			err := ingester.Ingest(ctx, input)
			assert.NoErr(t, err)
			assert.Loosely(t, antsTestResultsClient.Insertions, should.BeEmpty)
			verifyCheckpoints(ctx, t, []checkpoints.Checkpoint{})
		})
	})
}
