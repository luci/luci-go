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

package resultdb

import (
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
)

func TestQueryRunTestVerdicts(t *testing.T) {
	ftt.Run(`QueryRunTestVerdicts`, t, func(t *ftt.Test) {
		ctx := auth.WithState(testutil.SpannerTestContext(t), &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "testproject:testrealm", Permission: rdbperms.PermListTestResults},
			},
		})
		ctx, _ = tsmon.WithDummyInMemory(ctx)

		testutil.MustApply(ctx, t, testutil.CombineMutations(
			insert.FinalizedInvocationWithInclusions("a", map[string]any{"Realm": "testproject:testrealm"}, "b"),
			insert.FinalizedInvocationWithInclusions("b", map[string]any{"Realm": "testproject:testrealm"}),
			insert.TestResults(t, "a", "A", nil, pb.TestResult_FAILED, pb.TestResult_PASSED),
			insert.TestResultsLegacy(t, "a", "B", nil, pb.TestStatus_PASS, pb.TestStatus_CRASH),
			insert.TestResults(t, "a", "C", nil, pb.TestResult_PASSED),
			insert.TestResults(t, "b", "A", nil, pb.TestResult_FAILED),
		)...)

		srv := newTestResultDBService()
		properties := &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"key": structpb.NewStringValue("value"),
			},
		}
		tags := []*pb.StringPair{
			{Key: "k1", Value: "v1"},
			{Key: "k2", Value: "v2"},
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
							Name:        "invocations/a/tests/A/results/0",
							ResultId:    "0",
							StartTime:   timestamppb.New(time.Date(2025, 4, 27, 1, 2, 3, 4000, time.UTC)),
							Duration:    &durationpb.Duration{Seconds: 0, Nanos: 234567000},
							Status:      pb.TestStatus_FAIL,
							StatusV2:    pb.TestResult_FAILED,
							SummaryHtml: "SummaryHtml",
							Tags:        tags,
							FailureReason: &pb.FailureReason{
								Kind:                pb.FailureReason_ORDINARY,
								PrimaryErrorMessage: "failure reason",
								Errors:              []*pb.FailureReason_Error{{Message: "failure reason"}},
							},
							Properties:          properties,
							FrameworkExtensions: fx,
						},
					}, {
						Result: &pb.TestResult{
							Name:                "invocations/a/tests/A/results/1",
							ResultId:            "1",
							StartTime:           timestamppb.New(time.Date(2025, 4, 27, 1, 2, 3, 4000, time.UTC)),
							Duration:            &durationpb.Duration{Seconds: 1, Nanos: 234567000},
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
				Variant:     &pb.Variant{},
				VariantHash: "e3b0c44298fc1c14",
				Results: []*pb.TestResultBundle{
					{
						Result: &pb.TestResult{
							Name:        "invocations/a/tests/B/results/1",
							ResultId:    "1",
							Duration:    &durationpb.Duration{Seconds: 1, Nanos: 234567000},
							Status:      pb.TestStatus_CRASH,
							StatusV2:    pb.TestResult_FAILED,
							SummaryHtml: "SummaryHtml",
							FailureReason: &pb.FailureReason{
								// Legacy test result: no Kind set.
								PrimaryErrorMessage: "failure reason",
								Errors:              []*pb.FailureReason_Error{{Message: "failure reason"}},
							},
							Properties: properties,
						},
					},
					{
						Result: &pb.TestResult{
							Name:        "invocations/a/tests/B/results/0",
							ResultId:    "0",
							Duration:    &durationpb.Duration{Seconds: 0, Nanos: 234567000},
							Expected:    true,
							Status:      pb.TestStatus_PASS,
							StatusV2:    pb.TestResult_PASSED,
							SummaryHtml: "SummaryHtml",
							Properties:  properties,
						},
					},
				},
				TestMetadata: &pb.TestMetadata{Name: "testname"},
			}, {
				TestId:      "C",
				Variant:     &pb.Variant{},
				VariantHash: "e3b0c44298fc1c14",
				Results: []*pb.TestResultBundle{
					{
						Result: &pb.TestResult{
							Name:                "invocations/a/tests/C/results/0",
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
			},
		}

		t.Run(`Permission denied`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t, insert.Invocation("y", pb.Invocation_ACTIVE, map[string]any{"Realm": "secretproject:secret"}))

			_, err := srv.QueryRunTestVerdicts(ctx, &pb.QueryRunTestVerdictsRequest{
				Invocation: "invocations/y",
			})

			assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike("caller does not have permission resultdb.testResults.list in realm of invocation y"))
		})
		t.Run(`Invalid argument`, func(t *ftt.Test) {
			t.Run(`Empty request`, func(t *ftt.Test) {
				_, err := srv.QueryRunTestVerdicts(ctx, &pb.QueryRunTestVerdictsRequest{})

				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike(`unspecified`))
			})
			t.Run(`Invalid invocation name`, func(t *ftt.Test) {
				_, err := srv.QueryRunTestVerdicts(ctx, &pb.QueryRunTestVerdictsRequest{
					Invocation: "x",
				})

				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike(`does not match pattern "^invocations/([a-z][a-z0-9_\\-:.]{0,99})$"`))
			})
			t.Run(`Invalid result limit`, func(t *ftt.Test) {
				_, err := srv.QueryRunTestVerdicts(ctx, &pb.QueryRunTestVerdictsRequest{
					Invocation:  "invocations/a",
					ResultLimit: -1,
				})

				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike(`result_limit: negative`))
			})
			t.Run(`Invalid page size`, func(t *ftt.Test) {
				_, err := srv.QueryRunTestVerdicts(ctx, &pb.QueryRunTestVerdictsRequest{
					Invocation: "invocations/a",
					PageSize:   -1,
				})

				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike(`page_size: negative`))
			})
			t.Run(`Invalid page token`, func(t *ftt.Test) {
				_, err := srv.QueryRunTestVerdicts(ctx, &pb.QueryRunTestVerdictsRequest{
					Invocation: "invocations/a",
					PageToken:  "aaaa",
				})

				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike(`page_token`))
			})
		})
		t.Run(`Invocation not found`, func(t *ftt.Test) {
			_, err := srv.QueryRunTestVerdicts(ctx, &pb.QueryRunTestVerdictsRequest{
				Invocation: "invocations/notexists",
			})
			assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
		})
		t.Run(`Valid, no pagination`, func(t *ftt.Test) {
			result, err := srv.QueryRunTestVerdicts(ctx, &pb.QueryRunTestVerdictsRequest{
				Invocation: "invocations/a",
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, result, should.Match(&pb.QueryRunTestVerdictsResponse{
				RunTestVerdicts: expectedTestVerdicts,
			}))
		})
		t.Run(`Valid, pagination`, func(t *ftt.Test) {
			response, err := srv.QueryRunTestVerdicts(ctx, &pb.QueryRunTestVerdictsRequest{
				Invocation: "invocations/a",
				PageSize:   1,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, response, should.Match(&pb.QueryRunTestVerdictsResponse{
				RunTestVerdicts: expectedTestVerdicts[:1],
				NextPageToken:   "CgFBChBlM2IwYzQ0Mjk4ZmMxYzE0",
			}))

			response, err = srv.QueryRunTestVerdicts(ctx, &pb.QueryRunTestVerdictsRequest{
				Invocation: "invocations/a",
				PageSize:   1,
				PageToken:  "CgFBChBlM2IwYzQ0Mjk4ZmMxYzE0",
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, response, should.Match(&pb.QueryRunTestVerdictsResponse{
				RunTestVerdicts: expectedTestVerdicts[1:2],
				NextPageToken:   "CgFCChBlM2IwYzQ0Mjk4ZmMxYzE0",
			}))

			response, err = srv.QueryRunTestVerdicts(ctx, &pb.QueryRunTestVerdictsRequest{
				Invocation: "invocations/a",
				PageSize:   1,
				PageToken:  "CgFCChBlM2IwYzQ0Mjk4ZmMxYzE0",
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, response, should.Match(&pb.QueryRunTestVerdictsResponse{
				RunTestVerdicts: expectedTestVerdicts[2:],
				NextPageToken:   "",
			}))
		})
	})
}
