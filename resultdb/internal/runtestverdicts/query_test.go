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

package runtestverdicts

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestQuery(t *testing.T) {
	ftt.Run(`Query`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)

		variant := &pb.Variant{
			Def: map[string]string{"k1": "v1"},
		}

		testutil.MustApply(ctx, t, testutil.CombineMutations(
			insert.FinalizedInvocationWithInclusions("a", map[string]any{}, "b"),
			insert.FinalizedInvocationWithInclusions("b", map[string]any{}),
			insert.TestResults(t, "a", "A", nil, pb.TestResult_PASSED, pb.TestResult_FAILED),
			insert.TestResults(t, "a", "B", variant, pb.TestResult_SKIPPED),
			insert.TestResultMessages(t, []*pb.TestResult{
				{
					Name:     "invocations/a/tests/B/results/minimalfields",
					Expected: true,
					Status:   pb.TestStatus_PASS,
					StatusV2: pb.TestResult_PASSED,
				},
			}),
			insert.TestResultsLegacy(t, "a", "C", nil, pb.TestStatus_SKIP, pb.TestStatus_CRASH),
			// Should not be included in results for invocation 'a' because not
			// immediately inside invocation.
			insert.TestResults(t, "b", "A", nil, pb.TestResult_FAILED),
		)...)

		tags := []*pb.StringPair{
			{Key: "k1", Value: "v1"},
			{Key: "k2", Value: "v2"},
		}
		properties := &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"key": structpb.NewStringValue("value"),
			},
		}
		fx := &pb.FrameworkExtensions{
			WebTest: &pb.WebTest{
				Status:     pb.WebTest_PASS,
				IsExpected: true,
			},
		}
		expectedTestVerdicts := []*pb.RunTestVerdict{
			{
				TestId:      "A",
				Variant:     &pb.Variant{},
				VariantHash: "e3b0c44298fc1c14",
				Results: []*pb.TestResultBundle{
					{
						Result: &pb.TestResult{
							// Unexpected results come first.
							Name:        "invocations/a/tests/A/results/1",
							ResultId:    "1",
							StartTime:   timestamppb.New(time.Date(2025, 4, 27, 1, 2, 3, 4000, time.UTC)),
							Duration:    &durationpb.Duration{Seconds: 1, Nanos: 234567000},
							Status:      pb.TestStatus_FAIL,
							StatusV2:    pb.TestResult_FAILED,
							SummaryHtml: "SummaryHtml",
							FailureReason: &pb.FailureReason{
								Kind:                pb.FailureReason_ORDINARY,
								PrimaryErrorMessage: "failure reason",
								Errors: []*pb.FailureReason_Error{{
									Message: "failure reason",
									Trace:   "trace",
								}},
							},
							Tags:                tags,
							Properties:          properties,
							FrameworkExtensions: fx,
						},
					}, {
						Result: &pb.TestResult{
							Name:                "invocations/a/tests/A/results/0",
							ResultId:            "0",
							StartTime:           timestamppb.New(time.Date(2025, 4, 27, 1, 2, 3, 4000, time.UTC)),
							Duration:            &durationpb.Duration{Seconds: 0, Nanos: 234567000},
							Expected:            true,
							Status:              pb.TestStatus_PASS,
							StatusV2:            pb.TestResult_PASSED,
							SummaryHtml:         "SummaryHtml",
							Tags:                tags,
							Properties:          properties,
							FrameworkExtensions: fx,
						},
					},
				},
				TestMetadata: &pb.TestMetadata{Name: "testname"},
			}, {
				TestId:      "B",
				Variant:     variant,
				VariantHash: "d70268c39e188014",
				Results: []*pb.TestResultBundle{
					{
						Result: &pb.TestResult{
							Name:        "invocations/a/tests/B/results/0",
							ResultId:    "0",
							Expected:    true,
							Status:      pb.TestStatus_SKIP,
							StatusV2:    pb.TestResult_SKIPPED,
							StartTime:   timestamppb.New(time.Date(2025, 4, 27, 1, 2, 3, 4000, time.UTC)),
							Duration:    &durationpb.Duration{Seconds: 0, Nanos: 234567000},
							SummaryHtml: "SummaryHtml",
							Tags:        tags,
							Properties:  properties,
							SkipReason:  pb.SkipReason_AUTOMATICALLY_DISABLED_FOR_FLAKINESS,
							SkippedReason: &pb.SkippedReason{
								Kind:          pb.SkippedReason_DEMOTED,
								ReasonMessage: "skip reason",
								Trace:         "trace",
							},
							FrameworkExtensions: fx,
						},
					},
				},
				TestMetadata: &pb.TestMetadata{Name: "testname"},
			}, {
				TestId:      "B",
				Variant:     &pb.Variant{},
				VariantHash: "e3b0c44298fc1c14",
				Results: []*pb.TestResultBundle{
					{
						Result: &pb.TestResult{
							Name:     "invocations/a/tests/B/results/minimalfields",
							ResultId: "minimalfields",
							Expected: true,
							Status:   pb.TestStatus_PASS,
							StatusV2: pb.TestResult_PASSED,
						},
					},
				},
			}, {
				TestId:      "C",
				Variant:     &pb.Variant{},
				VariantHash: "e3b0c44298fc1c14",
				Results: []*pb.TestResultBundle{
					{
						Result: &pb.TestResult{
							Name:        "invocations/a/tests/C/results/0",
							ResultId:    "0",
							Duration:    &durationpb.Duration{Seconds: 0, Nanos: 234567000},
							Status:      pb.TestStatus_SKIP,
							StatusV2:    pb.TestResult_EXECUTION_ERRORED,
							SummaryHtml: "SummaryHtml",
							Properties:  properties,
							SkipReason:  pb.SkipReason_AUTOMATICALLY_DISABLED_FOR_FLAKINESS,
						},
					},
					{
						Result: &pb.TestResult{
							Name:        "invocations/a/tests/C/results/1",
							ResultId:    "1",
							Duration:    &durationpb.Duration{Seconds: 1, Nanos: 234567000},
							Status:      pb.TestStatus_CRASH,
							StatusV2:    pb.TestResult_FAILED,
							SummaryHtml: "SummaryHtml",
							FailureReason: &pb.FailureReason{
								// Legacy test result: Kind is not set.
								PrimaryErrorMessage: "failure reason",
								Errors:              []*pb.FailureReason_Error{{Message: "failure reason"}},
							},
							Properties: properties,
						},
					},
				},
				TestMetadata: &pb.TestMetadata{Name: "testname"},
			},
		}

		q := &Query{
			InvocationID:       invocations.ID("a"),
			PageSize:           100,
			ResultLimit:        10,
			ResponseLimitBytes: 1_000_000,
		}

		t.Run(`empty invocation`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t, insert.FinalizedInvocationWithInclusions("empty", map[string]any{})...)
			q.InvocationID = invocations.ID("empty")

			result, err := query(ctx, q)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, result, should.Match(QueryResult{}))
		})
		t.Run(`query all in one page`, func(t *ftt.Test) {
			result, err := query(ctx, q)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, result, should.Match(QueryResult{
				RunTestVerdicts: expectedTestVerdicts,
			}))
		})
		t.Run(`page size works`, func(t *ftt.Test) {
			q.PageSize = 1

			var tvs []*pb.RunTestVerdict
			err := fetchAll(ctx, q, func(page QueryResult) {
				tvs = append(tvs, page.RunTestVerdicts...)
				assert.Loosely(t, page.RunTestVerdicts, should.HaveLength(1))
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tvs, should.Match(expectedTestVerdicts))
		})
		t.Run(`response limit bytes works`, func(t *ftt.Test) {
			q.ResponseLimitBytes = 1

			var tvs []*pb.RunTestVerdict
			err := fetchAll(ctx, q, func(page QueryResult) {
				if (page.NextPageToken == PageToken{} && len(page.RunTestVerdicts) == 0) {
					// Allowed to have a blank page as the final page.
					return
				}

				tvs = append(tvs, page.RunTestVerdicts...)
				assert.Loosely(t, page.RunTestVerdicts, should.HaveLength(1))
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tvs, should.Match(expectedTestVerdicts))
		})
		t.Run(`result limit works`, func(t *ftt.Test) {
			q.ResultLimit = 1

			for _, tv := range expectedTestVerdicts {
				tv.Results = tv.Results[:1]
			}

			result, err := query(ctx, q)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, result, should.Match(QueryResult{
				RunTestVerdicts: expectedTestVerdicts,
			}))
		})
		t.Run(`low result and page limit works #1`, func(t *ftt.Test) {
			q.ResultLimit = 1
			q.PageSize = 1

			for _, tv := range expectedTestVerdicts {
				tv.Results = tv.Results[:1]
			}

			var tvs []*pb.RunTestVerdict
			err := fetchAll(ctx, q, func(page QueryResult) {
				if (page.NextPageToken == PageToken{} && len(page.RunTestVerdicts) == 0) {
					// Allowed to have a blank page as the final page.
					return
				}

				tvs = append(tvs, page.RunTestVerdicts...)
				assert.Loosely(t, page.RunTestVerdicts, should.HaveLength(1))
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tvs, should.Match(expectedTestVerdicts))
		})
		t.Run(`low result and page limit works #2`, func(t *ftt.Test) {
			q.ResultLimit = 2
			q.PageSize = 1

			var tvs []*pb.RunTestVerdict
			err := fetchAll(ctx, q, func(page QueryResult) {
				if (page.NextPageToken == PageToken{} && len(page.RunTestVerdicts) == 0) {
					// Allowed to have a blank page as the final page.
					return
				}

				tvs = append(tvs, page.RunTestVerdicts...)
				assert.Loosely(t, page.RunTestVerdicts, should.HaveLength(1))
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tvs, should.Match(expectedTestVerdicts))
		})
	})
}

func query(ctx context.Context, q *Query) (QueryResult, error) {
	ctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()
	return q.Run(ctx)
}

func fetchAll(ctx context.Context, q *Query, f func(page QueryResult)) error {
	for {
		page, err := query(ctx, q)
		if err != nil {
			return err
		}
		f(page)
		if (page.NextPageToken == PageToken{}) {
			break
		}

		// The page token should always advance, it should
		// never remain the same.
		if page.NextPageToken == q.PageToken {
			return errors.New("page token did not advance")
		}
		q.PageToken = page.NextPageToken
	}
	return nil
}
