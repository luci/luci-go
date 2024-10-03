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

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

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

		originalTmd := &analysispb.TestMetadata{
			Name: "original_name",
			Location: &analysispb.TestLocation{
				Repo:     "old_repo",
				FileName: "old_file_name",
				Line:     567,
			},
			BugComponent: &analysispb.BugComponent{
				System: &analysispb.BugComponent_Monorail{
					Monorail: &analysispb.MonorailComponent{
						Project: "chrome",
						Value:   "Blink>Component",
					},
				},
			},
		}
		updatedTmd := &analysispb.TestMetadata{
			Name: "updated_name",
			Location: &analysispb.TestLocation{
				Repo:     "repo",
				FileName: "file_name",
				Line:     456,
			},
			BugComponent: &analysispb.BugComponent{
				System: &analysispb.BugComponent_IssueTracker{
					IssueTracker: &analysispb.IssueTrackerComponent{
						ComponentId: 12345,
					},
				},
			},
		}

		// Expect the JSON serialisation format for this platform.
		properties, err := bqutil.MarshalStructPB(testProperties)
		assert.Loosely(t, err, should.BeNil)

		expectedResults := []*bqpb.TestResultRow{
			{
				Project:     "rootproject",
				TestId:      "ninja://test_expected",
				Variant:     `{}`,
				VariantHash: "hash",
				Invocation: &bqpb.TestResultRow_InvocationRecord{
					Id:    "test-root-invocation-name",
					Realm: "rootproject:root",
				},
				PartitionTime: timestamppb.New(time.Date(2020, 2, 3, 4, 5, 6, 7, time.UTC)),
				Parent:        parentInvocation,
				Name:          "invocations/test-invocation-name/tests/ninja%3A%2F%2Ftest_expected/results/one",
				ResultId:      "one",
				Expected:      true,
				Status:        analysispb.TestResultStatus_PASS,
				SummaryHtml:   "SummaryHTML for test_expected/one",
				StartTime:     timestamppb.New(time.Date(2010, time.March, 1, 0, 0, 0, 0, time.UTC)),
				DurationSecs:  3.000001,
				Tags:          []*analysispb.StringPair{{Key: "test-key", Value: "test-value"}},
				Properties:    "{}",
				Sources:       sources,
				SourceRef:     sourceRef,
				SourceRefHash: "5d47c679cf080cb5",
				TestMetadata:  updatedTmd,
				InsertTime:    timestamppb.New(testclock.TestRecentTimeLocal),
			},
			{
				Project:       "rootproject",
				TestId:        "ninja://test_flaky",
				Variant:       `{}`,
				VariantHash:   "hash",
				Invocation:    rootInvocation,
				PartitionTime: timestamppb.New(time.Date(2020, 2, 3, 4, 5, 6, 7, time.UTC)),
				Parent:        parentInvocation,
				Name:          "invocations/test-invocation-name/tests/ninja%3A%2F%2Ftest_flaky/results/one",
				ResultId:      "one",
				Expected:      false,
				Status:        analysispb.TestResultStatus_FAIL,
				SummaryHtml:   "SummaryHTML for test_flaky/one",
				StartTime:     timestamppb.New(time.Date(2010, time.February, 1, 0, 0, 10, 0, time.UTC)),
				FailureReason: &analysispb.FailureReason{
					PrimaryErrorMessage: "abc.def(123): unexpected nil-deference",
				},
				Properties:    "{}",
				Sources:       sources,
				SourceRef:     sourceRef,
				SourceRefHash: "5d47c679cf080cb5",
				InsertTime:    timestamppb.New(testclock.TestRecentTimeLocal),
			},
			{
				Project:       "rootproject",
				TestId:        "ninja://test_flaky",
				Variant:       `{}`,
				VariantHash:   "hash",
				Invocation:    rootInvocation,
				PartitionTime: timestamppb.New(time.Date(2020, 2, 3, 4, 5, 6, 7, time.UTC)),
				Parent:        parentInvocation,
				Name:          "invocations/test-invocation-name/tests/ninja%3A%2F%2Ftest_flaky/results/two",
				ResultId:      "two",
				Expected:      true,
				Status:        analysispb.TestResultStatus_PASS,
				SummaryHtml:   "SummaryHTML for test_flaky/two",
				StartTime:     timestamppb.New(time.Date(2010, time.February, 1, 0, 0, 20, 0, time.UTC)),
				Properties:    "{}",
				Sources:       sources,
				SourceRef:     sourceRef,
				SourceRefHash: "5d47c679cf080cb5",
				InsertTime:    timestamppb.New(testclock.TestRecentTimeLocal),
			},
			{
				Project:       "rootproject",
				TestId:        "ninja://test_skip",
				Variant:       `{"k1":"v1"}`,
				VariantHash:   "d70268c39e188014",
				Invocation:    rootInvocation,
				PartitionTime: timestamppb.New(time.Date(2020, 2, 3, 4, 5, 6, 7, time.UTC)),
				Parent:        parentInvocation,
				Name:          "invocations/test-invocation-name/tests/ninja%3A%2F%2Ftest_skip/results/one",
				ResultId:      "one",
				Expected:      true,
				Status:        analysispb.TestResultStatus_SKIP,
				SummaryHtml:   "SummaryHTML for test_skip/one",
				Properties:    properties,
				SkipReason:    "AUTOMATICALLY_DISABLED_FOR_FLAKINESS",
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
			assert.Loosely(t, exportClient.InsertionsByDestinationKey["partitioned-by-day"], should.Resemble(expectedResults))
			assert.Loosely(t, exportClient.InsertionsByDestinationKey["partitioned-by-month"], should.Resemble(expectedResults))
			assert.Loosely(t, verifyCheckpoints(ctx, expectedCheckpoints...), should.BeNil)

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
			assert.Loosely(t, exportClient.InsertionsByDestinationKey["partitioned-by-day"], should.Resemble(expectedResults))
			assert.Loosely(t, exportClient.InsertionsByDestinationKey["partitioned-by-month"], should.Resemble(expectedResults))
			assert.Loosely(t, verifyCheckpoints(ctx, expectedCheckpoints...), should.BeNil)
		})
	})
}
