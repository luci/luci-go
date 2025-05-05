// Copyright 2024 The LUCI Authors.
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

package resultingester

import (
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"

	"go.chromium.org/luci/analysis/internal/bqutil"
	"go.chromium.org/luci/analysis/internal/checkpoints"
	"go.chromium.org/luci/analysis/internal/testresults/exporter"
	"go.chromium.org/luci/analysis/internal/testutil"
	bqpb "go.chromium.org/luci/analysis/proto/bq"
	analysispb "go.chromium.org/luci/analysis/proto/v1"

	_ "go.chromium.org/luci/server/tq/txn/spanner"
)

func TestExportTestResults(t *testing.T) {
	ftt.Run("TestExportTestResults", t, func(t *ftt.Test) {
		ctx := testutil.IntegrationTestContext(t)
		ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeLocal)

		inputs := testInputs()
		exportClient := exporter.NewFakeClient()
		ingester := NewTestResultsExporter(exportClient)

		rootInvocation := &bqpb.TestResultRow_InvocationRecord{
			Id:    "test-root-invocation-name",
			Realm: "rootproject:root",
		}
		parentInvocation := &bqpb.TestResultRow_ParentInvocationRecord{
			Id: "test-invocation-name",
			Tags: []*analysispb.StringPair{
				{
					Key:   "tag-key",
					Value: "tag-value",
				},
				{
					Key:   "tag-key2",
					Value: "tag-value2",
				},
			},
			Realm:      "invproject:inv",
			Properties: `{"prop-key":"prop-value"}`,
		}
		sources := resolvedSourcesForTesting()
		sourceRef := &analysispb.SourceRef{
			System: &analysispb.SourceRef_Gitiles{
				Gitiles: &analysispb.GitilesRef{
					Host:    "project.googlesource.com",
					Project: "myproject/src",
					Ref:     "refs/heads/main",
				},
			},
		}

		originalTmd := &bqpb.TestMetadata{
			Name: "original_name",
			Location: &rdbpb.TestLocation{
				Repo:     "old_repo",
				FileName: "old_file_name",
				Line:     567,
			},
			BugComponent: &rdbpb.BugComponent{
				System: &rdbpb.BugComponent_Monorail{
					Monorail: &rdbpb.MonorailComponent{
						Project: "chrome",
						Value:   "Blink>Component",
					},
				},
			},
			Properties:     "{}",
			PreviousTestId: "previous_test_id",
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

		updatedTmd := &bqpb.TestMetadata{
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

		// Expect the JSON serialisation format for this platform.
		properties, err := bqutil.MarshalStructPB(testProperties)
		assert.Loosely(t, err, should.BeNil)

		expectedResults := []*bqpb.TestResultRow{
			{
				Project: "rootproject",
				TestIdStructured: &bqpb.TestIdentifier{
					ModuleName:        "module",
					ModuleScheme:      "junit",
					ModuleVariant:     "{}",
					ModuleVariantHash: "e3b0c44298fc1c14", // Hash of the empty variant.
					CoarseName:        "package",
					FineName:          "class",
					CaseName:          "test_expected",
				},
				TestId:      ":module!junit:package:class#test_expected",
				Variant:     `{}`,
				VariantHash: "hash",
				Invocation: &bqpb.TestResultRow_InvocationRecord{
					Id:    "test-root-invocation-name",
					Realm: "rootproject:root",
				},
				PartitionTime: timestamppb.New(time.Date(2020, 2, 3, 4, 5, 6, 7, time.UTC)),
				Parent:        parentInvocation,
				Name:          "invocations/test-invocation-name/tests/:module%21junit:package:class%23test_expected/results/one",
				ResultId:      "one",
				Expected:      true,
				Status:        analysispb.TestResultStatus_PASS,
				SummaryHtml:   "SummaryHTML for test_expected/one",
				StatusV2:      analysispb.TestResult_PASSED,
				StartTime:     timestamppb.New(time.Date(2010, time.March, 1, 0, 0, 0, 0, time.UTC)),
				DurationSecs:  3.000001,
				Tags:          []*analysispb.StringPair{{Key: "test-key", Value: "test-value"}},
				Properties:    "{}",
				Sources:       sources,
				SourceRef:     sourceRef,
				SourceRefHash: "5d47c679cf080cb5",
				TestMetadata:  updatedTmd,
				FrameworkExtensions: &rdbpb.FrameworkExtensions{
					WebTest: &rdbpb.WebTest{
						Status:     rdbpb.WebTest_PASS,
						IsExpected: true,
					},
				},
				InsertTime: timestamppb.New(testclock.TestRecentTimeLocal),
			},
			{
				Project: "rootproject",
				TestIdStructured: &bqpb.TestIdentifier{
					ModuleName:        "module",
					ModuleScheme:      "junit",
					ModuleVariant:     "{}",
					ModuleVariantHash: "e3b0c44298fc1c14", // Hash of the empty variant.
					CoarseName:        "package",
					FineName:          "class",
					CaseName:          "test_flaky",
				},
				TestId:        ":module!junit:package:class#test_flaky",
				Variant:       `{}`,
				VariantHash:   "hash",
				Invocation:    rootInvocation,
				PartitionTime: timestamppb.New(time.Date(2020, 2, 3, 4, 5, 6, 7, time.UTC)),
				Parent:        parentInvocation,
				Name:          "invocations/test-invocation-name/tests/:module%21junit:package:class%23test_flaky/results/one",
				ResultId:      "one",
				Expected:      false,
				Status:        analysispb.TestResultStatus_FAIL,
				StatusV2:      analysispb.TestResult_FAILED,
				SummaryHtml:   "SummaryHTML for test_flaky/one",
				StartTime:     timestamppb.New(time.Date(2010, time.February, 1, 0, 0, 10, 0, time.UTC)),
				FailureReason: &rdbpb.FailureReason{
					PrimaryErrorMessage: "abc.def(123): unexpected nil-deference",
					Errors: []*rdbpb.FailureReason_Error{
						{
							Message: "abc.def(123): unexpected nil-deference",
						},
					},
				},
				Properties:    "{}",
				Sources:       sources,
				SourceRef:     sourceRef,
				SourceRefHash: "5d47c679cf080cb5",
				InsertTime:    timestamppb.New(testclock.TestRecentTimeLocal),
			},
			{
				Project: "rootproject",
				TestIdStructured: &bqpb.TestIdentifier{
					ModuleName:        "module",
					ModuleScheme:      "junit",
					ModuleVariant:     "{}",
					ModuleVariantHash: "e3b0c44298fc1c14", // Hash of the empty variant.
					CoarseName:        "package",
					FineName:          "class",
					CaseName:          "test_flaky",
				},
				TestId:        ":module!junit:package:class#test_flaky",
				Variant:       `{}`,
				VariantHash:   "hash",
				Invocation:    rootInvocation,
				PartitionTime: timestamppb.New(time.Date(2020, 2, 3, 4, 5, 6, 7, time.UTC)),
				Parent:        parentInvocation,
				Name:          "invocations/test-invocation-name/tests/:module%21junit:package:class%23test_flaky/results/two",
				ResultId:      "two",
				Expected:      true,
				Status:        analysispb.TestResultStatus_PASS,
				StatusV2:      analysispb.TestResult_PASSED,
				SummaryHtml:   "SummaryHTML for test_flaky/two",
				StartTime:     timestamppb.New(time.Date(2010, time.February, 1, 0, 0, 20, 0, time.UTC)),
				Properties:    "{}",
				Sources:       sources,
				SourceRef:     sourceRef,
				SourceRefHash: "5d47c679cf080cb5",
				InsertTime:    timestamppb.New(testclock.TestRecentTimeLocal),
			},
			{
				Project: "rootproject",
				TestIdStructured: &bqpb.TestIdentifier{
					ModuleName:        "module",
					ModuleScheme:      "junit",
					ModuleVariant:     `{"k1":"v1"}`,
					ModuleVariantHash: "d70268c39e188014", // Hash of the empty variant.
					CoarseName:        "package",
					FineName:          "class",
					CaseName:          "test_skip",
				},
				TestId:        ":module!junit:package:class#test_skip",
				Variant:       `{"k1":"v1"}`,
				VariantHash:   "d70268c39e188014",
				Invocation:    rootInvocation,
				PartitionTime: timestamppb.New(time.Date(2020, 2, 3, 4, 5, 6, 7, time.UTC)),
				Parent:        parentInvocation,
				Name:          "invocations/test-invocation-name/tests/:module%21junit:package:class%23test_skip/results/one",
				ResultId:      "one",
				Expected:      true,
				Status:        analysispb.TestResultStatus_SKIP,
				StatusV2:      analysispb.TestResult_SKIPPED,
				SummaryHtml:   "SummaryHTML for test_skip/one",
				Properties:    properties,
				SkipReason:    "AUTOMATICALLY_DISABLED_FOR_FLAKINESS",
				SkippedReason: &rdbpb.SkippedReason{
					Kind:          rdbpb.SkippedReason_DISABLED_AT_DECLARATION,
					ReasonMessage: "Test has @Ignored annotation.",
				},
				Sources:       sources,
				SourceRef:     sourceRef,
				SourceRefHash: "5d47c679cf080cb5",
				TestMetadata:  originalTmd,
				InsertTime:    timestamppb.New(testclock.TestRecentTimeLocal),
			},
		}

		expectedCheckpoints := []checkpoints.Checkpoint{
			{
				Key: checkpoints.Key{
					Project:    "rootproject",
					ResourceID: "fake.rdb.host/test-root-invocation-name/test-invocation-name",
					ProcessID:  "result-ingestion/export-test-results/partitioned-by-day",
					Uniquifier: "1",
				},
				// Creation and expiry time not validated.
			},
			{
				Key: checkpoints.Key{
					Project:    "rootproject",
					ResourceID: "fake.rdb.host/test-root-invocation-name/test-invocation-name",
					ProcessID:  "result-ingestion/export-test-results/partitioned-by-month",
					Uniquifier: "1",
				},
				// Creation and expiry time not validated.
			},
		}

		t.Run(`Baseline`, func(t *ftt.Test) {
			err := ingester.Ingest(ctx, inputs)
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, exportClient.InsertionsByDestinationKey, should.HaveLength(2))
			assert.Loosely(t, exportClient.InsertionsByDestinationKey["partitioned-by-day"], should.Match(expectedResults))
			assert.Loosely(t, exportClient.InsertionsByDestinationKey["partitioned-by-month"], should.Match(expectedResults))
			assert.Loosely(t, verifyCheckpoints(ctx, t, expectedCheckpoints...), should.BeNil)

			t.Run(`Results are not exported again if the process is re-run`, func(t *ftt.Test) {
				exportClient.InsertionsByDestinationKey = map[string][]*bqpb.TestResultRow{}

				err = ingester.Ingest(ctx, inputs)
				assert.Loosely(t, err, should.BeNil)

				// Nothing should be exported because the checkpoint already exists.
				assert.Loosely(t, exportClient.InsertionsByDestinationKey, should.BeEmpty)
			})
		})
		t.Run(`Without sources`, func(t *ftt.Test) {
			// Base case should already have sources set.
			assert.Loosely(t, inputs.Sources, should.NotBeNil)
			inputs.Sources = nil

			err := ingester.Ingest(ctx, inputs)
			assert.Loosely(t, err, should.BeNil)

			for _, r := range expectedResults {
				r.Sources = nil
				r.SourceRef = nil
				r.SourceRefHash = ""
			}
			assert.Loosely(t, exportClient.InsertionsByDestinationKey, should.HaveLength(2))
			assert.Loosely(t, exportClient.InsertionsByDestinationKey["partitioned-by-day"], should.Match(expectedResults))
			assert.Loosely(t, exportClient.InsertionsByDestinationKey["partitioned-by-month"], should.Match(expectedResults))
			assert.Loosely(t, verifyCheckpoints(ctx, t, expectedCheckpoints...), should.BeNil)
		})
	})
}
