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
			insert.TestResults(t, "a", "A", nil, pb.TestStatus_PASS, pb.TestStatus_FAIL),
			insert.TestResultMessages(t, []*pb.TestResult{
				{
					Name:        "invocations/a/tests/B/results/maximalfields",
					Variant:     variant,
					Expected:    false,
					Status:      pb.TestStatus_FAIL,
					SummaryHtml: "SummaryHtml",
					FailureReason: &pb.FailureReason{
						PrimaryErrorMessage: "failure reason",
					},
					TestMetadata: &pb.TestMetadata{
						Name: "maximalfields test",
					},
					Tags:      []*pb.StringPair{{Key: "k1", Value: "v1"}, {Key: "k2", Value: "v2"}},
					StartTime: timestamppb.New(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)),
					Duration:  durationpb.New(1*time.Hour + 1*time.Microsecond),
					Properties: &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"f1": structpb.NewStringValue("v1"),
						},
					},
				},
			}),
			insert.TestResultMessages(t, []*pb.TestResult{
				{
					Name:     "invocations/a/tests/B/results/minimalfields",
					Expected: true,
					Status:   pb.TestStatus_PASS,
				},
			}),
			insert.TestResults(t, "a", "C", nil, pb.TestStatus_SKIP, pb.TestStatus_CRASH),
			// Should not be included in results for invocation 'a' because not
			// immediately inside invocation.
			insert.TestResults(t, "b", "A", nil, pb.TestStatus_CRASH),
		)...)

		properties := &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"key": structpb.NewStringValue("value"),
			},
		}
		expectedTestVerdicts := []*pb.RunTestVerdict{
			{
				TestId:      "A",
				Variant:     nil,
				VariantHash: "e3b0c44298fc1c14",
				Results: []*pb.TestResultBundle{
					{
						Result: &pb.TestResult{
							// Unexpeted results come first.
							Name:          "invocations/a/tests/A/results/1",
							ResultId:      "1",
							Duration:      &durationpb.Duration{Seconds: 1, Nanos: 234567000},
							Status:        pb.TestStatus_FAIL,
							SummaryHtml:   "SummaryHtml",
							FailureReason: &pb.FailureReason{PrimaryErrorMessage: "failure reason"},
							Properties:    properties,
						},
					}, {
						Result: &pb.TestResult{
							Name:        "invocations/a/tests/A/results/0",
							ResultId:    "0",
							Duration:    &durationpb.Duration{Seconds: 0, Nanos: 234567000},
							Expected:    true,
							Status:      pb.TestStatus_PASS,
							SummaryHtml: "SummaryHtml",
							Properties:  properties,
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
							Name:        "invocations/a/tests/B/results/maximalfields",
							ResultId:    "maximalfields",
							Expected:    false,
							Status:      pb.TestStatus_FAIL,
							SummaryHtml: "SummaryHtml",
							FailureReason: &pb.FailureReason{
								PrimaryErrorMessage: "failure reason",
							},
							Tags:      []*pb.StringPair{{Key: "k1", Value: "v1"}, {Key: "k2", Value: "v2"}},
							StartTime: timestamppb.New(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)),
							Duration:  durationpb.New(1*time.Hour + 1*time.Microsecond),
							Properties: &structpb.Struct{
								Fields: map[string]*structpb.Value{
									"f1": structpb.NewStringValue("v1"),
								},
							},
						},
					},
				},
				TestMetadata: &pb.TestMetadata{
					Name: "maximalfields test",
				},
			}, {
				TestId:      "B",
				Variant:     nil,
				VariantHash: "e3b0c44298fc1c14",
				Results: []*pb.TestResultBundle{
					{
						Result: &pb.TestResult{
							Name:     "invocations/a/tests/B/results/minimalfields",
							ResultId: "minimalfields",
							Expected: true,
							Status:   pb.TestStatus_PASS,
						},
					},
				},
			}, {
				TestId:      "C",
				Variant:     nil,
				VariantHash: "e3b0c44298fc1c14",
				Results: []*pb.TestResultBundle{
					{
						Result: &pb.TestResult{
							Name:        "invocations/a/tests/C/results/0",
							ResultId:    "0",
							Duration:    &durationpb.Duration{Seconds: 0, Nanos: 234567000},
							Status:      pb.TestStatus_SKIP,
							SummaryHtml: "SummaryHtml",
							Properties:  properties,
							SkipReason:  pb.SkipReason_AUTOMATICALLY_DISABLED_FOR_FLAKINESS,
						},
					},
					{
						Result: &pb.TestResult{
							Name:          "invocations/a/tests/C/results/1",
							ResultId:      "1",
							Duration:      &durationpb.Duration{Seconds: 1, Nanos: 234567000},
							Status:        pb.TestStatus_CRASH,
							SummaryHtml:   "SummaryHtml",
							FailureReason: &pb.FailureReason{PrimaryErrorMessage: "failure reason"},
							Properties:    properties,
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
			assert.Loosely(t, result, should.Resemble(QueryResult{}))
		})
		t.Run(`query all in one page`, func(t *ftt.Test) {
			result, err := query(ctx, q)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, result, should.Resemble(QueryResult{
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
			assert.Loosely(t, tvs, should.Resemble(expectedTestVerdicts))
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
			assert.Loosely(t, tvs, should.Resemble(expectedTestVerdicts))
		})
		t.Run(`result limit works`, func(t *ftt.Test) {
			q.ResultLimit = 1

			for _, tv := range expectedTestVerdicts {
				tv.Results = tv.Results[:1]
			}

			result, err := query(ctx, q)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, result, should.Resemble(QueryResult{
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
			assert.Loosely(t, tvs, should.Resemble(expectedTestVerdicts))
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
			assert.Loosely(t, tvs, should.Resemble(expectedTestVerdicts))
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
