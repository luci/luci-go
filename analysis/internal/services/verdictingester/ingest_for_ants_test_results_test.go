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

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
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
			Invocation: &rdbpb.Invocation{
				Name:         testInvocation,
				Realm:        testRealm,
				IsExportRoot: true,
				FinalizeTime: timestamppb.New(time.Date(2025, 4, 8, 0, 0, 0, 0, time.UTC)),
			},
			Verdicts: mockedQueryTestVariantsRsp().TestVariants,
			Payload:  payload,
		}
		antsTestResultsClient := antstestresultexporter.NewFakeClient()
		ingester := AnTSTestResultExporter{exporter: antstestresultexporter.NewExporter(antsTestResultsClient)}
		t.Run("baseline", func(t *ftt.Test) {
			err := ingester.Ingest(ctx, input)
			assert.NoErr(t, err)
			verifyAnTSTestResultExport(t, antsTestResultsClient, true)
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

		t.Run("not android project", func(t *ftt.Test) {
			payload.Project = "other"

			err := ingester.Ingest(ctx, input)
			assert.NoErr(t, err)
			verifyAnTSTestResultExport(t, antsTestResultsClient, false)
			verifyCheckpoints(ctx, t, []checkpoints.Checkpoint{})
		})
	})
}

func verifyAnTSTestResultExport(t testing.TB, client *antstestresultexporter.FakeClient, shouldExport bool) {
	t.Helper()
	actualRows := client.Insertions
	var expectedRows []*bqpblegacy.AntsTestResultRow
	if shouldExport {
		expectedRows = []*bqpblegacy.AntsTestResultRow{
			{
				TestResultId: "one",
				InvocationId: invocationID,
				TestIdentifier: &bqpblegacy.AntsTestResultRow_TestIdentifier{
					Module:      "module",
					TestClass:   "package.class",
					ClassName:   "class",
					PackageName: "package",
					Method:      "test_consistent_failure",
				},
				TestIdentifierHash: "439715a66ee549703a54f6ba7fc6883bb29bc6485ea04ddeace09a3675e8c188",
				TestStatus:         bqpblegacy.AntsTestResultRow_FAIL,
				DebugInfo: &bqpblegacy.AntsTestResultRow_DebugInfo{
					ErrorMessage: "abc.def(123): unexpected nil-deference",
				},
				Timing: &bqpblegacy.AntsTestResultRow_Timing{
					CreationTimestamp: 1267401600000, // 2010-03-01 00:00:00 UTC
					CompleteTimestamp: 1267401603000, // Start + 3s
					CreationMonth:     "2010-03",     // From StartTime
				},
				TestId:         ":module!junit:package:class#test_consistent_failure",
				CompletionTime: timestamppb.New(time.Date(2025, 4, 8, 0, 0, 0, 0, time.UTC)),
			},
			{
				TestResultId: "one",
				InvocationId: invocationID,
				TestIdentifier: &bqpblegacy.AntsTestResultRow_TestIdentifier{
					Module:      "module",
					TestClass:   "package.class",
					ClassName:   "class",
					PackageName: "package",
					Method:      "test_expected",
				},
				TestIdentifierHash: "c9b736b4fa95a9e2726cc1d54407a4b3cfef716b41932bf0b8764aa060d4cc30",
				TestStatus:         bqpblegacy.AntsTestResultRow_PASS,
				Timing: &bqpblegacy.AntsTestResultRow_Timing{
					CreationTimestamp: 1272672000000, // 2010-05-01 00:00:00 UTC
					CompleteTimestamp: 1272672005000, // Start + 5s
					CreationMonth:     "2010-05",
				},
				TestId:         ":module!junit:package:class#test_expected",
				CompletionTime: timestamppb.New(time.Date(2025, 4, 8, 0, 0, 0, 0, time.UTC)),
			},
			{
				TestResultId: "one",
				InvocationId: invocationID,
				TestIdentifier: &bqpblegacy.AntsTestResultRow_TestIdentifier{
					Module:      "module",
					TestClass:   "package.class",
					ClassName:   "class",
					PackageName: "package",
					Method:      "test_filtering_event",
				},
				TestIdentifierHash: "e0c64cde6e07ab7e6056eab6904ac2dd7572955c2fa46f905df6240077821f1a",
				TestStatus:         bqpblegacy.AntsTestResultRow_TEST_SKIPPED,
				Timing: &bqpblegacy.AntsTestResultRow_Timing{
					CreationTimestamp: 1265068800000, // 2010-02-02 00:00:00 UTC
					CompleteTimestamp: 1265068800000, // Start + 0s
					CreationMonth:     "2010-02",
				},
				TestId:         ":module!junit:package:class#test_filtering_event",
				CompletionTime: timestamppb.New(time.Date(2025, 4, 8, 0, 0, 0, 0, time.UTC)),
			},
			{
				TestResultId: "one",
				InvocationId: invocationID,
				TestIdentifier: &bqpblegacy.AntsTestResultRow_TestIdentifier{
					Module:      "module",
					TestClass:   "package.class",
					ClassName:   "class",
					PackageName: "package",
					Method:      "test_from_luci_bisection",
				},
				TestIdentifierHash: "7fbf857cd5260ec91627cd5b8558f57ed2079db286f75a94a73e279b777b372a",
				TestStatus:         bqpblegacy.AntsTestResultRow_PASS,
				Timing: &bqpblegacy.AntsTestResultRow_Timing{
					CreationTimestamp: 0,         // StartTime is nil in input
					CompleteTimestamp: 0,         // Start + 0s
					CreationMonth:     "1970-01", // Default from zero time
				},
				Properties: []*bqpblegacy.StringPair{
					{Name: "is_luci_bisection", Value: "true"},
				},
				TestId:         ":module!junit:package:class#test_from_luci_bisection",
				CompletionTime: timestamppb.New(time.Date(2025, 4, 8, 0, 0, 0, 0, time.UTC)),
			},
			{
				TestResultId: "one",
				InvocationId: invocationID,
				TestIdentifier: &bqpblegacy.AntsTestResultRow_TestIdentifier{
					Module:      "module",
					TestClass:   "package.class",
					ClassName:   "class",
					PackageName: "package",
					Method:      "test_has_unexpected",
				},
				TestIdentifierHash: "02a1ce2dd211366074c1456e549acfe211c957090f2f55071c09c550c0a1e9d0",
				TestStatus:         bqpblegacy.AntsTestResultRow_FAIL,
				Timing: &bqpblegacy.AntsTestResultRow_Timing{
					CreationTimestamp: 1264982410000, // 2010-02-01 00:00:10 UTC
					CompleteTimestamp: 1264982410000, // Start + 0s
					CreationMonth:     "2010-02",
				},
				TestId:         ":module!junit:package:class#test_has_unexpected",
				CompletionTime: timestamppb.New(time.Date(2025, 4, 8, 0, 0, 0, 0, time.UTC)),
			},
			{
				TestResultId: "two",
				InvocationId: invocationID,
				TestIdentifier: &bqpblegacy.AntsTestResultRow_TestIdentifier{
					Module:      "module",
					TestClass:   "package.class",
					ClassName:   "class",
					PackageName: "package",
					Method:      "test_has_unexpected",
				},
				TestIdentifierHash: "02a1ce2dd211366074c1456e549acfe211c957090f2f55071c09c550c0a1e9d0",
				TestStatus:         bqpblegacy.AntsTestResultRow_PASS,
				Timing: &bqpblegacy.AntsTestResultRow_Timing{
					CreationTimestamp: 1264982420000, // 2010-02-01 00:00:20 UTC
					CompleteTimestamp: 1264982420000, // Start + 0s
					CreationMonth:     "2010-02",
				},
				TestId:         ":module!junit:package:class#test_has_unexpected",
				CompletionTime: timestamppb.New(time.Date(2025, 4, 8, 0, 0, 0, 0, time.UTC)),
			},
			{
				TestResultId: "one",
				InvocationId: invocationID,
				TestIdentifier: &bqpblegacy.AntsTestResultRow_TestIdentifier{
					Module:      "module",
					TestClass:   "package.class",
					ClassName:   "class",
					PackageName: "package",
					Method:      "test_known_flake",
				},
				TestIdentifierHash: "f244f7948ccfb0ff95f037103d6f1d935145a7a92277e3e0e12a77f4ad344e87",
				TestStatus:         bqpblegacy.AntsTestResultRow_FAIL,
				Timing: &bqpblegacy.AntsTestResultRow_Timing{
					CreationTimestamp: 1264982400000, // 2010-02-01 00:00:00 UTC
					CompleteTimestamp: 1264982402000, // Start + 2s
					CreationMonth:     "2010-02",
				},
				Properties: []*bqpblegacy.StringPair{
					{Name: "os", Value: "Mac"},
					{Name: "monorail_component", Value: "Monorail>Component"},
				},
				TestId:         ":module!junit:package:class#test_known_flake",
				CompletionTime: timestamppb.New(time.Date(2025, 4, 8, 0, 0, 0, 0, time.UTC)),
			},
			{
				TestResultId: "one",
				InvocationId: invocationID,
				TestIdentifier: &bqpblegacy.AntsTestResultRow_TestIdentifier{
					Module:      "module",
					TestClass:   "package.class",
					ClassName:   "class",
					PackageName: "package",
					Method:      "test_new_failure",
				},
				TestIdentifierHash: "e121e340a729e3bc31f66b747e96577028c0c87a4c8c36f950a6e12b5d57bfe3",
				TestStatus:         bqpblegacy.AntsTestResultRow_FAIL,
				Timing: &bqpblegacy.AntsTestResultRow_Timing{
					CreationTimestamp: 1262304000000, // 2010-01-01 00:00:00 UTC
					CompleteTimestamp: 1262304001000, // Start + 1s
					CreationMonth:     "2010-01",
				},
				Properties: []*bqpblegacy.StringPair{
					{Name: "random_tag", Value: "random_tag_value"},
					{Name: "public_buganizer_component", Value: "951951951"},
				},
				TestId:         ":module!junit:package:class#test_new_failure",
				CompletionTime: timestamppb.New(time.Date(2025, 4, 8, 0, 0, 0, 0, time.UTC)),
			},
			{
				TestResultId: "two",
				InvocationId: invocationID,
				TestIdentifier: &bqpblegacy.AntsTestResultRow_TestIdentifier{
					Module:      "module",
					TestClass:   "package.class",
					ClassName:   "class",
					PackageName: "package",
					Method:      "test_new_flake",
				},
				TestIdentifierHash: "c6b22f8616e1403c797c923d867e0ab9a970915218a2f04a58f058809aa1a180",
				TestStatus:         bqpblegacy.AntsTestResultRow_FAIL,
				Timing: &bqpblegacy.AntsTestResultRow_Timing{
					CreationTimestamp: 1262304020000, // 2010-01-01 00:00:20 UTC
					CompleteTimestamp: 1262304031000, // Start + 11s
					CreationMonth:     "2010-01",
				},
				TestId:         ":module!junit:package:class#test_new_flake",
				CompletionTime: timestamppb.New(time.Date(2025, 4, 8, 0, 0, 0, 0, time.UTC)),
			},
			{
				TestResultId: "one",
				InvocationId: invocationID,
				TestIdentifier: &bqpblegacy.AntsTestResultRow_TestIdentifier{
					Module:      "module",
					TestClass:   "package.class",
					ClassName:   "class",
					PackageName: "package",
					Method:      "test_new_flake",
				},
				TestIdentifierHash: "c6b22f8616e1403c797c923d867e0ab9a970915218a2f04a58f058809aa1a180",
				TestStatus:         bqpblegacy.AntsTestResultRow_FAIL,
				Timing: &bqpblegacy.AntsTestResultRow_Timing{
					CreationTimestamp: 1262304010000, // 2010-01-01 00:00:10 UTC
					CompleteTimestamp: 1262304020000, // Start + 10s
					CreationMonth:     "2010-01",
				},
				TestId:         ":module!junit:package:class#test_new_flake",
				CompletionTime: timestamppb.New(time.Date(2025, 4, 8, 0, 0, 0, 0, time.UTC)),
			},
			{
				TestResultId: "three",
				InvocationId: invocationID,
				TestIdentifier: &bqpblegacy.AntsTestResultRow_TestIdentifier{
					Module:      "module",
					TestClass:   "package.class",
					ClassName:   "class",
					PackageName: "package",
					Method:      "test_new_flake",
				},
				TestIdentifierHash: "c6b22f8616e1403c797c923d867e0ab9a970915218a2f04a58f058809aa1a180",
				TestStatus:         bqpblegacy.AntsTestResultRow_PASS,
				Timing: &bqpblegacy.AntsTestResultRow_Timing{
					CreationTimestamp: 1262304015000, // 2010-01-01 00:00:15 UTC
					CompleteTimestamp: 1262304027000, // Start + 12s
					CreationMonth:     "2010-01",
				},
				TestId:         ":module!junit:package:class#test_new_flake",
				CompletionTime: timestamppb.New(time.Date(2025, 4, 8, 0, 0, 0, 0, time.UTC)),
			},
			{
				TestResultId: "one",
				InvocationId: invocationID,
				TestIdentifier: &bqpblegacy.AntsTestResultRow_TestIdentifier{
					Module:      "module",
					TestClass:   "package.class",
					ClassName:   "class",
					PackageName: "package",
					Method:      "test_no_new_results",
				},
				TestIdentifierHash: "c7129eb417ecbf4dff711bda0ef118759b474c8acac81dfc221fdcc19aabdebd",
				TestStatus:         bqpblegacy.AntsTestResultRow_FAIL,
				Timing: &bqpblegacy.AntsTestResultRow_Timing{
					CreationTimestamp: 1270080000000, // 2010-04-01 00:00:00 UTC
					CompleteTimestamp: 1270080004000, // Start + 4s
					CreationMonth:     "2010-04",
				},
				TestId:         ":module!junit:package:class#test_no_new_results",
				CompletionTime: timestamppb.New(time.Date(2025, 4, 8, 0, 0, 0, 0, time.UTC)),
			},
			{
				TestResultId: "one",
				InvocationId: invocationID,
				TestIdentifier: &bqpblegacy.AntsTestResultRow_TestIdentifier{
					Module:      "module",
					TestClass:   "package.class",
					ClassName:   "class",
					PackageName: "package",
					Method:      "test_skip",
				},
				TestIdentifierHash: "bf352f379fffe06da2f8c24ae6ddc8bac9d2d8ac18238ab25ee7771d59ff230e",
				TestStatus:         bqpblegacy.AntsTestResultRow_TEST_SKIPPED,
				Timing: &bqpblegacy.AntsTestResultRow_Timing{
					CreationTimestamp: 1265068800000, // 2010-02-02 00:00:00 UTC
					CompleteTimestamp: 1265068800000, // Start + 0s
					CreationMonth:     "2010-02",
				},
				TestId:         ":module!junit:package:class#test_skip",
				CompletionTime: timestamppb.New(time.Date(2025, 4, 8, 0, 0, 0, 0, time.UTC)),
			},
			{
				TestResultId: "one",
				InvocationId: invocationID,
				TestIdentifier: &bqpblegacy.AntsTestResultRow_TestIdentifier{
					Module:      "module",
					TestClass:   "package.class",
					ClassName:   "class",
					PackageName: "package",
					Method:      "test_unexpected_pass",
				},
				TestIdentifierHash: "336d86b97fda98f485e7a94fa47e4ef03672bf3dcec192371c9eb2b4ee313ac4",
				TestStatus:         bqpblegacy.AntsTestResultRow_PASS,
				Timing: &bqpblegacy.AntsTestResultRow_Timing{
					CreationTimestamp: 0,         // StartTime is nil in input
					CompleteTimestamp: 0,         // Start + 0s
					CreationMonth:     "1970-01", // Default from zero time
				},
				TestId:         ":module!junit:package:class#test_unexpected_pass",
				CompletionTime: timestamppb.New(time.Date(2025, 4, 8, 0, 0, 0, 0, time.UTC)),
			},
		}
		assert.Loosely(t, actualRows, should.HaveLength(len(expectedRows)), truth.LineContext())
		for i, row := range actualRows {
			assert.Loosely(t, row, should.Match(expectedRows[i]), truth.LineContext())
		}
	}
}
