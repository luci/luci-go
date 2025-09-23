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
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"

	"go.chromium.org/luci/analysis/internal/bqutil"
	"go.chromium.org/luci/analysis/internal/checkpoints"
	"go.chromium.org/luci/analysis/internal/config"
	ctrlpb "go.chromium.org/luci/analysis/internal/ingestion/control/proto"
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
	"go.chromium.org/luci/analysis/internal/testutil"
	"go.chromium.org/luci/analysis/internal/testverdicts"
	"go.chromium.org/luci/analysis/pbutil"
	bqpb "go.chromium.org/luci/analysis/proto/bq"
	configpb "go.chromium.org/luci/analysis/proto/config"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

func TestExportTestVerdicts(t *testing.T) {
	ftt.Run("TestExportTestVerdicts", t, func(t *ftt.Test) {
		ctx := testutil.IntegrationTestContext(t)
		ctx = memory.Use(ctx)
		ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeLocal)
		testVerdicts := testverdicts.NewFakeClient()
		cfg := &configpb.Config{
			TestVerdictExport: &configpb.TestVerdictExport{
				Enabled: true,
			},
		}
		err := config.SetTestConfig(ctx, cfg)
		assert.Loosely(t, err, should.BeNil)

		bHost := "host"
		partitionTime := clock.Now(ctx).Add(-1 * time.Hour)
		invocationCreationTime := partitionTime.Add(-3 * time.Hour)

		payload := &taskspb.IngestTestVerdicts{
			IngestionId: "ingestion-id",
			Project:     "project",
			Invocation: &ctrlpb.InvocationResult{
				ResultdbHost: "rdb-host",
				InvocationId: invocationID,
				CreationTime: timestamppb.New(invocationCreationTime),
			},
			Build: &ctrlpb.BuildResult{
				Host:         bHost,
				Id:           testBuildID,
				CreationTime: timestamppb.New(time.Date(2020, time.April, 1, 2, 3, 4, 5, time.UTC)),
				Project:      "project",
				Bucket:       "bucket",
				Builder:      "builder",
				Status:       pb.BuildStatus_BUILD_STATUS_FAILURE,
				Changelists: []*pb.Changelist{
					{
						Host:      "anothergerrit.gerrit.instance",
						Change:    77788,
						Patchset:  19,
						OwnerKind: pb.ChangelistOwnerKind_HUMAN,
					},
					{
						Host:      "mygerrit-review.googlesource.com",
						Change:    12345,
						Patchset:  5,
						OwnerKind: pb.ChangelistOwnerKind_AUTOMATION,
					},
				},
				Commit: &bbpb.GitilesCommit{
					Host:     "myproject.googlesource.com",
					Project:  "someproject/src",
					Id:       strings.Repeat("0a", 20),
					Ref:      "refs/heads/mybranch",
					Position: 111888,
				},
				HasInvocation:     true,
				ResultdbHost:      "test.results.api.cr.dev",
				GardenerRotations: []string{"rotation1", "rotation2"},
			},
			PartitionTime: timestamppb.New(partitionTime),
			PresubmitRun: &ctrlpb.PresubmitResult{
				PresubmitRunId: &pb.PresubmitRunId{
					System: "luci-cv",
					Id:     "infra/12345",
				},
				Status:       pb.PresubmitRunStatus_PRESUBMIT_RUN_STATUS_SUCCEEDED,
				Mode:         pb.PresubmitRunMode_FULL_RUN,
				Owner:        "automation",
				CreationTime: timestamppb.New(time.Date(2021, time.April, 1, 2, 3, 4, 5, time.UTC)),
			},
			PageToken:            "expected_token",
			TaskIndex:            1,
			UseNewIngestionOrder: true,
		}
		input := Inputs{
			Invocation: &rdbpb.Invocation{
				Name:         testInvocation,
				Realm:        testRealm,
				IsExportRoot: true,
				FinalizeTime: timestamppb.New(time.Date(2025, 4, 8, 0, 0, 0, 0, time.UTC)),
			},
			Verdicts: mockedQueryTestVariantsRsp().TestVariants,
			SourcesByID: map[string]*pb.Sources{
				"sources1": {
					BaseSources: &pb.Sources_GitilesCommit{
						GitilesCommit: &pb.GitilesCommit{
							Host:       "project.googlesource.com",
							Project:    "myproject/src",
							Ref:        "refs/heads/main",
							CommitHash: "abcdefabcd1234567890abcdefabcd1234567890",
							Position:   16801,
						},
					},
					Changelists: []*pb.GerritChange{
						{
							Host:      "project-review.googlesource.com",
							Project:   "myproject/src2",
							Change:    9991,
							Patchset:  82,
							OwnerKind: pb.ChangelistOwnerKind_HUMAN,
						},
					},
					IsDirty: false,
				},
			},
			Payload: payload,
		}
		ingester := VerdictExporter{
			exporter: testverdicts.NewExporter(testVerdicts),
		}
		expectedChangepoint := []checkpoints.Checkpoint{{
			Key: checkpoints.Key{
				Project:    "project",
				ResourceID: fmt.Sprintf("rdb-host/%v", invocationID),
				ProcessID:  "verdict-ingestion/export-test-verdicts",
				Uniquifier: "1",
			},
		}}

		t.Run("without build", func(t *ftt.Test) {
			payload.Build = nil
			payload.PresubmitRun = nil

			err := ingester.Ingest(ctx, input)
			assert.NoErr(t, err)
			verifyTestVerdicts(t, testVerdicts, partitionTime, false)
			verifyCheckpoints(ctx, t, expectedChangepoint)
		})
		t.Run("with build", func(t *ftt.Test) {
			err := ingester.Ingest(ctx, input)
			assert.NoErr(t, err)
			verifyTestVerdicts(t, testVerdicts, partitionTime, true)
			verifyCheckpoints(ctx, t, expectedChangepoint)
		})
		t.Run("disabled in config", func(t *ftt.Test) {
			cfg.TestVerdictExport = &configpb.TestVerdictExport{
				Enabled: false,
			}
			err := config.SetTestConfig(ctx, cfg)
			assert.Loosely(t, err, should.BeNil)

			err = ingester.Ingest(ctx, input)
			assert.NoErr(t, err)
			assert.Loosely(t, testVerdicts.Insertions, should.BeEmpty)
			verifyCheckpoints(ctx, t, []checkpoints.Checkpoint{})
		})
	})
}

func verifyTestVerdicts(t testing.TB, client *testverdicts.FakeClient, expectedPartitionTime time.Time, expectBuild bool) {
	t.Helper()
	actualRows := client.Insertions

	invocation := &bqpb.TestVerdictRow_InvocationRecord{
		Id:         "build-87654321",
		Realm:      "project:ci",
		Properties: "{}",
	}

	// Proto marshalling may not be the same on all platforms,
	// so find what we should expect on this platform.
	tmdProperties, err := bqutil.MarshalStructPB(&structpb.Struct{
		Fields: map[string]*structpb.Value{
			"string":  structpb.NewStringValue("value"),
			"number":  structpb.NewNumberValue(123),
			"boolean": structpb.NewBoolValue(true),
		},
	})
	assert.NoErr(t, err)

	testMetadata := &bqpb.TestMetadata{
		Name: "updated_name",
		Location: &rdbpb.TestLocation{
			Repo:     "repo",
			FileName: "file_name",
			Line:     456,
		},
		BugComponent: &rdbpb.BugComponent{
			System: &rdbpb.BugComponent_IssueTracker{
				IssueTracker: &rdbpb.IssueTrackerComponent{
					ComponentId: 12345,
				},
			},
		},
		PropertiesSchema: "myproject.MyMessage",
		Properties:       tmdProperties,
		PreviousTestId:   "another_previous_test_id",
	}

	var buildbucketBuild *bqpb.TestVerdictRow_BuildbucketBuild
	var cvRun *bqpb.TestVerdictRow_ChangeVerifierRun
	if expectBuild {
		buildbucketBuild = &bqpb.TestVerdictRow_BuildbucketBuild{
			Id: testBuildID,
			Builder: &bqpb.TestVerdictRow_BuildbucketBuild_Builder{
				Project: "project",
				Bucket:  "bucket",
				Builder: "builder",
			},
			Status:            "FAILURE",
			GardenerRotations: []string{"rotation1", "rotation2"},
		}
		cvRun = &bqpb.TestVerdictRow_ChangeVerifierRun{
			Id:              "infra/12345",
			Mode:            pb.PresubmitRunMode_FULL_RUN,
			Status:          "SUCCEEDED",
			IsBuildCritical: false,
		}
	}

	// Different platforms may use different spacing when serializing
	// JSONPB. Expect the spacing scheme used by this platform.
	expectedProperties, err := bqutil.MarshalStructPB(testProperties)
	assert.Loosely(t, err, should.BeNil, truth.LineContext())

	sr := &pb.SourceRef{
		System: &pb.SourceRef_Gitiles{
			Gitiles: &pb.GitilesRef{
				Host:    "project.googlesource.com",
				Project: "myproject/src",
				Ref:     "refs/heads/main",
			},
		},
	}
	expectedRows := []*bqpb.TestVerdictRow{
		{
			Project: "project",
			TestIdStructured: &bqpb.TestIdentifier{
				ModuleName:        "module",
				ModuleScheme:      "junit",
				ModuleVariant:     "{}",
				ModuleVariantHash: pbutil.VariantHash(pbutil.Variant()), // Hash of empty test variant.
				CoarseName:        "package",
				FineName:          "class",
				CaseName:          "test_consistent_failure",
			},
			TestId:         ":module!junit:package:class#test_consistent_failure",
			Variant:        "{}",
			VariantHash:    "hash",
			Invocation:     invocation,
			PartitionTime:  timestamppb.New(expectedPartitionTime),
			Status:         pb.TestVerdictStatus_EXONERATED,
			StatusV2:       pb.TestVerdict_FAILED,
			StatusOverride: pb.TestVerdict_EXONERATED,
			Results: []*bqpb.TestVerdictRow_TestResult{
				{
					Parent: &bqpb.TestVerdictRow_ParentInvocationRecord{
						Id: "build-1234",
					},
					Name:        "invocations/build-1234/tests/:module%21junit:package:class%23test_consistent_failure/results/one",
					ResultId:    "one",
					Expected:    false,
					Status:      pb.TestResultStatus_FAIL,
					StatusV2:    pb.TestResult_FAILED,
					SummaryHtml: "SummaryHTML",
					StartTime:   timestamppb.New(time.Date(2010, time.March, 1, 0, 0, 0, 0, time.UTC)),
					Duration:    3.000001,
					FailureReason: &rdbpb.FailureReason{
						PrimaryErrorMessage: "abc.def(123): unexpected nil-deference",
						Errors: []*rdbpb.FailureReason_Error{
							{
								Message: "abc.def(123): unexpected nil-deference",
							},
						},
					},
					Properties: expectedProperties,
				},
			},
			Exonerations: []*bqpb.TestVerdictRow_Exoneration{
				{
					ExplanationHtml: "LUCI Analysis reported this test as flaky.",
					Reason:          pb.ExonerationReason_OCCURS_ON_OTHER_CLS,
				},
				{
					ExplanationHtml: "Test is marked informational.",
					Reason:          pb.ExonerationReason_NOT_CRITICAL,
				},
				{
					Reason: pb.ExonerationReason_OCCURS_ON_MAINLINE,
				},
			},
			Counts: &bqpb.TestVerdictRow_Counts{
				Total: 1, TotalNonSkipped: 1, Unexpected: 1, UnexpectedNonSkipped: 1, UnexpectedNonSkippedNonPassed: 1,
			},
			CountsV2: &bqpb.TestVerdictRow_CountsV2{
				Failed: 1,
				Total:  1,
			},
			BuildbucketBuild:  buildbucketBuild,
			ChangeVerifierRun: cvRun,
			Sources: &pb.Sources{
				BaseSources: &pb.Sources_GitilesCommit{
					GitilesCommit: &pb.GitilesCommit{
						Host:       "project.googlesource.com",
						Project:    "myproject/src",
						Ref:        "refs/heads/main",
						CommitHash: "abcdefabcd1234567890abcdefabcd1234567890",
						Position:   16801,
					},
				},
				Changelists: []*pb.GerritChange{
					{
						Host:      "project-review.googlesource.com",
						Project:   "myproject/src2",
						Change:    9991,
						Patchset:  82,
						OwnerKind: pb.ChangelistOwnerKind_HUMAN,
					},
				},
				IsDirty: false,
			},
			SourceRef:     sr,
			SourceRefHash: hex.EncodeToString(pbutil.SourceRefHash(sr)),
			InsertTime:    timestamppb.New(testclock.TestRecentTimeLocal),
		},
		{
			Project: "project",
			TestIdStructured: &bqpb.TestIdentifier{
				ModuleName:        "module",
				ModuleScheme:      "junit",
				ModuleVariant:     "{}",
				ModuleVariantHash: pbutil.VariantHash(pbutil.Variant()), // Hash of empty test variant.
				CoarseName:        "package",
				FineName:          "class",
				CaseName:          "test_expected",
			},
			TestId:         ":module!junit:package:class#test_expected",
			Variant:        "{}",
			VariantHash:    "hash",
			Invocation:     invocation,
			PartitionTime:  timestamppb.New(expectedPartitionTime),
			Status:         pb.TestVerdictStatus_EXPECTED,
			StatusV2:       pb.TestVerdict_PASSED,
			StatusOverride: pb.TestVerdict_NOT_OVERRIDDEN,
			Results: []*bqpb.TestVerdictRow_TestResult{
				{
					Parent: &bqpb.TestVerdictRow_ParentInvocationRecord{
						Id: "build-1234",
					},
					Name:       "invocations/build-1234/tests/:module%21junit:package:class%23test_expected/results/one",
					ResultId:   "one",
					StartTime:  timestamppb.New(time.Date(2010, time.May, 1, 0, 0, 0, 0, time.UTC)),
					Status:     pb.TestResultStatus_PASS,
					StatusV2:   pb.TestResult_PASSED,
					Expected:   true,
					Duration:   5.0,
					Properties: "{}",
				},
			},
			Counts: &bqpb.TestVerdictRow_Counts{
				Total: 1, TotalNonSkipped: 1, Unexpected: 0, UnexpectedNonSkipped: 0, UnexpectedNonSkippedNonPassed: 0,
			},
			CountsV2: &bqpb.TestVerdictRow_CountsV2{
				Passed: 1,
				Total:  1,
			},
			BuildbucketBuild:  buildbucketBuild,
			ChangeVerifierRun: cvRun,
			InsertTime:        timestamppb.New(testclock.TestRecentTimeLocal),
		},
		{
			Project: "project",
			TestIdStructured: &bqpb.TestIdentifier{
				ModuleName:        "module",
				ModuleScheme:      "junit",
				ModuleVariant:     "{}",
				ModuleVariantHash: pbutil.VariantHash(pbutil.Variant()), // Hash of empty test variant.
				CoarseName:        "package",
				FineName:          "class",
				CaseName:          "test_filtering_event",
			},
			TestId:         ":module!junit:package:class#test_filtering_event",
			Variant:        "{}",
			VariantHash:    "hash",
			Invocation:     invocation,
			PartitionTime:  timestamppb.New(expectedPartitionTime),
			Status:         pb.TestVerdictStatus_EXPECTED,
			StatusV2:       pb.TestVerdict_SKIPPED,
			StatusOverride: pb.TestVerdict_NOT_OVERRIDDEN,
			Results: []*bqpb.TestVerdictRow_TestResult{
				{
					Parent: &bqpb.TestVerdictRow_ParentInvocationRecord{
						Id: "build-1234",
					},
					Name:       "invocations/build-1234/tests/:module%21junit:package:class%23test_filtering_event/results/one",
					ResultId:   "one",
					StartTime:  timestamppb.New(time.Date(2010, time.February, 2, 0, 0, 0, 0, time.UTC)),
					Status:     pb.TestResultStatus_SKIP,
					StatusV2:   pb.TestResult_SKIPPED,
					SkipReason: rdbpb.SkipReason_AUTOMATICALLY_DISABLED_FOR_FLAKINESS.String(),
					SkippedReason: &rdbpb.SkippedReason{
						Kind:          rdbpb.SkippedReason_DISABLED_AT_DECLARATION,
						ReasonMessage: "Test has @Ignored annotation.",
					},
					Expected:   true,
					Properties: "{}",
				},
			},
			Counts: &bqpb.TestVerdictRow_Counts{
				Total: 1, TotalNonSkipped: 0, Unexpected: 0, UnexpectedNonSkipped: 0, UnexpectedNonSkippedNonPassed: 0,
			},
			CountsV2: &bqpb.TestVerdictRow_CountsV2{
				Skipped: 1,
				Total:   1,
			},
			BuildbucketBuild:  buildbucketBuild,
			ChangeVerifierRun: cvRun,
			InsertTime:        timestamppb.New(testclock.TestRecentTimeLocal),
		},
		{
			Project: "project",
			TestIdStructured: &bqpb.TestIdentifier{
				ModuleName:        "module",
				ModuleScheme:      "junit",
				ModuleVariant:     "{}",
				ModuleVariantHash: pbutil.VariantHash(pbutil.Variant()), // Hash of empty test variant.
				CoarseName:        "package",
				FineName:          "class",
				CaseName:          "test_from_luci_bisection",
			},
			TestId:         ":module!junit:package:class#test_from_luci_bisection",
			Variant:        "{}",
			VariantHash:    "hash",
			Invocation:     invocation,
			PartitionTime:  timestamppb.New(expectedPartitionTime),
			Status:         pb.TestVerdictStatus_UNEXPECTED,
			StatusV2:       pb.TestVerdict_FAILED,
			StatusOverride: pb.TestVerdict_NOT_OVERRIDDEN,
			Results: []*bqpb.TestVerdictRow_TestResult{
				{
					Parent: &bqpb.TestVerdictRow_ParentInvocationRecord{
						Id: "build-1234",
					},
					Name:       "invocations/build-1234/tests/:module%21junit:package:class%23test_from_luci_bisection/results/one",
					ResultId:   "one",
					Status:     pb.TestResultStatus_PASS,
					StatusV2:   pb.TestResult_FAILED,
					Expected:   false,
					Properties: "{}",
					Tags:       []*pb.StringPair{{Key: "is_luci_bisection", Value: "true"}},
					FrameworkExtensions: &rdbpb.FrameworkExtensions{
						WebTest: &rdbpb.WebTest{
							Status:     rdbpb.WebTest_PASS,
							IsExpected: false,
						},
					},
				},
			},
			Counts: &bqpb.TestVerdictRow_Counts{
				Total: 1, TotalNonSkipped: 1, Unexpected: 1, UnexpectedNonSkipped: 1, UnexpectedNonSkippedNonPassed: 0,
			},
			CountsV2: &bqpb.TestVerdictRow_CountsV2{
				Failed: 1,
				Total:  1,
			},
			BuildbucketBuild:  buildbucketBuild,
			ChangeVerifierRun: cvRun,
			InsertTime:        timestamppb.New(testclock.TestRecentTimeLocal),
		},
		{
			Project: "project",
			TestIdStructured: &bqpb.TestIdentifier{
				ModuleName:        "module",
				ModuleScheme:      "junit",
				ModuleVariant:     "{}",
				ModuleVariantHash: pbutil.VariantHash(pbutil.Variant()), // Hash of empty test variant.
				CoarseName:        "package",
				FineName:          "class",
				CaseName:          "test_has_unexpected",
			},
			TestId:         ":module!junit:package:class#test_has_unexpected",
			VariantHash:    "hash",
			Variant:        "{}",
			Invocation:     invocation,
			PartitionTime:  timestamppb.New(expectedPartitionTime),
			Status:         pb.TestVerdictStatus_FLAKY,
			StatusV2:       pb.TestVerdict_FLAKY,
			StatusOverride: pb.TestVerdict_NOT_OVERRIDDEN,
			Results: []*bqpb.TestVerdictRow_TestResult{
				{
					Parent: &bqpb.TestVerdictRow_ParentInvocationRecord{
						Id: "invocation-0b",
					},
					Name:       "invocations/invocation-0b/tests/:module%21junit:package:class%23test_has_unexpected/results/one",
					ResultId:   "one",
					StartTime:  timestamppb.New(time.Date(2010, time.February, 1, 0, 0, 10, 0, time.UTC)),
					Status:     pb.TestResultStatus_FAIL,
					StatusV2:   pb.TestResult_FAILED,
					Expected:   false,
					Properties: "{}",
				},
				{
					Parent: &bqpb.TestVerdictRow_ParentInvocationRecord{
						Id: "invocation-0a",
					},
					Name:       "invocations/invocation-0a/tests/:module%21junit:package:class%23test_has_unexpected/results/two",
					ResultId:   "two",
					StartTime:  timestamppb.New(time.Date(2010, time.February, 1, 0, 0, 20, 0, time.UTC)),
					Status:     pb.TestResultStatus_PASS,
					StatusV2:   pb.TestResult_PASSED,
					Expected:   true,
					Properties: "{}",
				},
			},
			Counts: &bqpb.TestVerdictRow_Counts{
				Total: 2, TotalNonSkipped: 2, Unexpected: 1, UnexpectedNonSkipped: 1, UnexpectedNonSkippedNonPassed: 1,
			},
			CountsV2: &bqpb.TestVerdictRow_CountsV2{
				Passed: 1,
				Failed: 1,
				Total:  2,
			},
			BuildbucketBuild:  buildbucketBuild,
			ChangeVerifierRun: cvRun,
			InsertTime:        timestamppb.New(testclock.TestRecentTimeLocal),
		},
		{
			Project: "project",
			TestIdStructured: &bqpb.TestIdentifier{
				ModuleName:        "module",
				ModuleScheme:      "junit",
				ModuleVariant:     `{"k1":"v2"}`,
				ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("k1", "v2")),
				CoarseName:        "package",
				FineName:          "class",
				CaseName:          "test_known_flake",
			},
			TestId:         ":module!junit:package:class#test_known_flake",
			Variant:        `{"k1":"v2"}`,
			VariantHash:    "hash_2",
			Invocation:     invocation,
			PartitionTime:  timestamppb.New(expectedPartitionTime),
			Status:         pb.TestVerdictStatus_UNEXPECTED,
			StatusV2:       pb.TestVerdict_FAILED,
			StatusOverride: pb.TestVerdict_NOT_OVERRIDDEN,
			TestMetadata:   testMetadata,
			Results: []*bqpb.TestVerdictRow_TestResult{
				{
					Parent: &bqpb.TestVerdictRow_ParentInvocationRecord{
						Id: "build-1234",
					},
					Name:       "invocations/build-1234/tests/:module%21junit:package:class%23test_known_flake/results/one",
					ResultId:   "one",
					StartTime:  timestamppb.New(time.Date(2010, time.February, 1, 0, 0, 0, 0, time.UTC)),
					Status:     pb.TestResultStatus_FAIL,
					StatusV2:   pb.TestResult_FAILED,
					Expected:   false,
					Duration:   2.0,
					Tags:       pbutil.StringPairs("os", "Mac", "monorail_component", "Monorail>Component"),
					Properties: "{}",
				},
			},
			Counts: &bqpb.TestVerdictRow_Counts{
				Total: 1, TotalNonSkipped: 1, Unexpected: 1, UnexpectedNonSkipped: 1, UnexpectedNonSkippedNonPassed: 1,
			},
			CountsV2: &bqpb.TestVerdictRow_CountsV2{
				Failed: 1,
				Total:  1,
			},
			BuildbucketBuild:  buildbucketBuild,
			ChangeVerifierRun: cvRun,
			InsertTime:        timestamppb.New(testclock.TestRecentTimeLocal),
		},
		{
			Project: "project",
			TestIdStructured: &bqpb.TestIdentifier{
				ModuleName:        "module",
				ModuleScheme:      "junit",
				ModuleVariant:     `{"k1":"v1"}`,
				ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("k1", "v1")),
				CoarseName:        "package",
				FineName:          "class",
				CaseName:          "test_new_failure",
			},
			TestId:         ":module!junit:package:class#test_new_failure",
			Variant:        `{"k1":"v1"}`,
			VariantHash:    "hash_1",
			Invocation:     invocation,
			PartitionTime:  timestamppb.New(expectedPartitionTime),
			Status:         pb.TestVerdictStatus_UNEXPECTED,
			StatusV2:       pb.TestVerdict_FAILED,
			StatusOverride: pb.TestVerdict_NOT_OVERRIDDEN,
			TestMetadata:   testMetadata,
			Results: []*bqpb.TestVerdictRow_TestResult{
				{
					Parent: &bqpb.TestVerdictRow_ParentInvocationRecord{
						Id: "build-1234",
					},
					Name:       "invocations/build-1234/tests/:module%21junit:package:class%23test_new_failure/results/one",
					ResultId:   "one",
					StartTime:  timestamppb.New(time.Date(2010, time.January, 1, 0, 0, 0, 0, time.UTC)),
					Status:     pb.TestResultStatus_FAIL,
					StatusV2:   pb.TestResult_FAILED,
					Expected:   false,
					Duration:   1.0,
					Tags:       pbutil.StringPairs("random_tag", "random_tag_value", "public_buganizer_component", "951951951"),
					Properties: "{}",
				},
			},
			Counts: &bqpb.TestVerdictRow_Counts{
				Total: 1, TotalNonSkipped: 1, Unexpected: 1, UnexpectedNonSkipped: 1, UnexpectedNonSkippedNonPassed: 1,
			},
			CountsV2: &bqpb.TestVerdictRow_CountsV2{
				Failed: 1,
				Total:  1,
			},
			BuildbucketBuild:  buildbucketBuild,
			ChangeVerifierRun: cvRun,
			InsertTime:        timestamppb.New(testclock.TestRecentTimeLocal),
		},
		{
			Project: "project",
			TestIdStructured: &bqpb.TestIdentifier{
				ModuleName:        "module",
				ModuleScheme:      "junit",
				ModuleVariant:     `{}`,
				ModuleVariantHash: pbutil.VariantHash(pbutil.Variant()),
				CoarseName:        "package",
				FineName:          "class",
				CaseName:          "test_new_flake",
			},
			TestId:         ":module!junit:package:class#test_new_flake",
			Variant:        "{}",
			VariantHash:    "hash",
			Invocation:     invocation,
			PartitionTime:  timestamppb.New(expectedPartitionTime),
			Status:         pb.TestVerdictStatus_FLAKY,
			StatusV2:       pb.TestVerdict_FLAKY,
			StatusOverride: pb.TestVerdict_NOT_OVERRIDDEN,
			Results: []*bqpb.TestVerdictRow_TestResult{
				{
					Parent: &bqpb.TestVerdictRow_ParentInvocationRecord{
						Id: "invocation-1234",
					},
					Name:       "invocations/invocation-1234/tests/:module%21junit:package:class%23test_new_flake/results/two",
					ResultId:   "two",
					StartTime:  timestamppb.New(time.Date(2010, time.January, 1, 0, 0, 20, 0, time.UTC)),
					Status:     pb.TestResultStatus_FAIL,
					StatusV2:   pb.TestResult_FAILED,
					Expected:   false,
					Duration:   11.0,
					Properties: "{}",
				},
				{
					Parent: &bqpb.TestVerdictRow_ParentInvocationRecord{
						Id: "invocation-1234",
					},
					Name:       "invocations/invocation-1234/tests/:module%21junit:package:class%23test_new_flake/results/one",
					ResultId:   "one",
					StartTime:  timestamppb.New(time.Date(2010, time.January, 1, 0, 0, 10, 0, time.UTC)),
					Status:     pb.TestResultStatus_FAIL,
					StatusV2:   pb.TestResult_FAILED,
					Expected:   false,
					Duration:   10.0,
					Properties: "{}",
				},
				{
					Parent: &bqpb.TestVerdictRow_ParentInvocationRecord{
						Id: "invocation-4567",
					},
					Name:       "invocations/invocation-4567/tests/:module%21junit:package:class%23test_new_flake/results/three",
					ResultId:   "three",
					StartTime:  timestamppb.New(time.Date(2010, time.January, 1, 0, 0, 15, 0, time.UTC)),
					Status:     pb.TestResultStatus_PASS,
					StatusV2:   pb.TestResult_PASSED,
					Expected:   true,
					Duration:   12.0,
					Properties: "{}",
				},
			},
			Counts: &bqpb.TestVerdictRow_Counts{
				Total: 3, TotalNonSkipped: 3, Unexpected: 2, UnexpectedNonSkipped: 2, UnexpectedNonSkippedNonPassed: 2,
			},
			CountsV2: &bqpb.TestVerdictRow_CountsV2{
				Passed: 1,
				Failed: 2,
				Total:  3,
			},
			BuildbucketBuild:  buildbucketBuild,
			ChangeVerifierRun: cvRun,
			InsertTime:        timestamppb.New(testclock.TestRecentTimeLocal),
		},
		{
			Project: "project",
			TestIdStructured: &bqpb.TestIdentifier{
				ModuleName:        "module",
				ModuleScheme:      "junit",
				ModuleVariant:     `{}`,
				ModuleVariantHash: pbutil.VariantHash(pbutil.Variant()),
				CoarseName:        "package",
				FineName:          "class",
				CaseName:          "test_no_new_results",
			},
			TestId:         ":module!junit:package:class#test_no_new_results",
			Variant:        "{}",
			VariantHash:    "hash",
			Invocation:     invocation,
			PartitionTime:  timestamppb.New(expectedPartitionTime),
			Status:         pb.TestVerdictStatus_UNEXPECTED,
			StatusV2:       pb.TestVerdict_FAILED,
			StatusOverride: pb.TestVerdict_NOT_OVERRIDDEN,
			Results: []*bqpb.TestVerdictRow_TestResult{
				{
					Parent: &bqpb.TestVerdictRow_ParentInvocationRecord{
						Id: "build-1234",
					},
					Name:       "invocations/build-1234/tests/:module%21junit:package:class%23test_no_new_results/results/one",
					ResultId:   "one",
					StartTime:  timestamppb.New(time.Date(2010, time.April, 1, 0, 0, 0, 0, time.UTC)),
					Status:     pb.TestResultStatus_FAIL,
					StatusV2:   pb.TestResult_FAILED,
					Expected:   false,
					Duration:   4.0,
					Properties: "{}",
				},
			},
			Counts: &bqpb.TestVerdictRow_Counts{
				Total: 1, TotalNonSkipped: 1, Unexpected: 1, UnexpectedNonSkipped: 1, UnexpectedNonSkippedNonPassed: 1,
			},
			CountsV2: &bqpb.TestVerdictRow_CountsV2{
				Failed: 1,
				Total:  1,
			},
			BuildbucketBuild:  buildbucketBuild,
			ChangeVerifierRun: cvRun,
			InsertTime:        timestamppb.New(testclock.TestRecentTimeLocal),
		},
		{
			Project: "project",
			TestIdStructured: &bqpb.TestIdentifier{
				ModuleName:        "module",
				ModuleScheme:      "junit",
				ModuleVariant:     `{}`,
				ModuleVariantHash: pbutil.VariantHash(pbutil.Variant()),
				CoarseName:        "package",
				FineName:          "class",
				CaseName:          "test_skip",
			},
			TestId:         ":module!junit:package:class#test_skip",
			Variant:        "{}",
			VariantHash:    "hash",
			Invocation:     invocation,
			PartitionTime:  timestamppb.New(expectedPartitionTime),
			Status:         pb.TestVerdictStatus_UNEXPECTEDLY_SKIPPED,
			StatusV2:       pb.TestVerdict_EXECUTION_ERRORED,
			StatusOverride: pb.TestVerdict_NOT_OVERRIDDEN,
			Results: []*bqpb.TestVerdictRow_TestResult{
				{
					Parent: &bqpb.TestVerdictRow_ParentInvocationRecord{
						Id: "build-1234",
					},
					Name:       "invocations/build-1234/tests/:module%21junit:package:class%23test_skip/results/one",
					ResultId:   "one",
					StartTime:  timestamppb.New(time.Date(2010, time.February, 2, 0, 0, 0, 0, time.UTC)),
					Status:     pb.TestResultStatus_SKIP,
					StatusV2:   pb.TestResult_EXECUTION_ERRORED,
					Expected:   false,
					Properties: "{}",
				},
			},
			Counts: &bqpb.TestVerdictRow_Counts{
				Total: 1, TotalNonSkipped: 0, Unexpected: 1, UnexpectedNonSkipped: 0, UnexpectedNonSkippedNonPassed: 0,
			},
			CountsV2: &bqpb.TestVerdictRow_CountsV2{
				ExecutionErrored: 1,
				Total:            1,
			},
			BuildbucketBuild:  buildbucketBuild,
			ChangeVerifierRun: cvRun,
			InsertTime:        timestamppb.New(testclock.TestRecentTimeLocal),
		},
		{
			Project: "project",
			TestIdStructured: &bqpb.TestIdentifier{
				ModuleName:        "module",
				ModuleScheme:      "junit",
				ModuleVariant:     `{}`,
				ModuleVariantHash: pbutil.VariantHash(pbutil.Variant()),
				CoarseName:        "package",
				FineName:          "class",
				CaseName:          "test_unexpected_pass",
			},
			TestId:         ":module!junit:package:class#test_unexpected_pass",
			Variant:        "{}",
			VariantHash:    "hash",
			Invocation:     invocation,
			PartitionTime:  timestamppb.New(expectedPartitionTime),
			Status:         pb.TestVerdictStatus_UNEXPECTED,
			StatusV2:       pb.TestVerdict_FAILED,
			StatusOverride: pb.TestVerdict_NOT_OVERRIDDEN,
			Results: []*bqpb.TestVerdictRow_TestResult{
				{
					Parent: &bqpb.TestVerdictRow_ParentInvocationRecord{
						Id: "build-1234",
					},
					Name:       "invocations/build-1234/tests/:module%21junit:package:class%23test_unexpected_pass/results/one",
					ResultId:   "one",
					Status:     pb.TestResultStatus_PASS,
					StatusV2:   pb.TestResult_FAILED,
					Expected:   false,
					Properties: "{}",
				},
			},
			Counts: &bqpb.TestVerdictRow_Counts{
				Total: 1, TotalNonSkipped: 1, Unexpected: 1, UnexpectedNonSkipped: 1, UnexpectedNonSkippedNonPassed: 0,
			},
			CountsV2: &bqpb.TestVerdictRow_CountsV2{
				Failed: 1,
				Total:  1,
			},
			BuildbucketBuild:  buildbucketBuild,
			ChangeVerifierRun: cvRun,
			InsertTime:        timestamppb.New(testclock.TestRecentTimeLocal),
		},
	}
	assert.Loosely(t, actualRows, should.HaveLength(len(expectedRows)), truth.LineContext())
	for i, row := range actualRows {
		assert.Loosely(t, row, should.Match(expectedRows[i]), truth.LineContext())
	}
}
