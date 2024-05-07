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

	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestQueryRunTestVariants(t *testing.T) {
	Convey(`QueryRunTestVariants`, t, func() {
		ctx := auth.WithState(testutil.SpannerTestContext(t), &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "testproject:testrealm", Permission: rdbperms.PermListTestResults},
			},
		})
		ctx, _ = tsmon.WithDummyInMemory(ctx)

		testutil.MustApply(ctx, testutil.CombineMutations(
			insert.FinalizedInvocationWithInclusions("a", map[string]any{"Realm": "testproject:testrealm"}, "b"),
			insert.FinalizedInvocationWithInclusions("b", map[string]any{"Realm": "testproject:testrealm"}),
			insert.TestResults("a", "A", nil, pb.TestStatus_FAIL, pb.TestStatus_PASS),
			insert.TestResults("a", "B", nil, pb.TestStatus_PASS, pb.TestStatus_CRASH),
			insert.TestResults("a", "C", nil, pb.TestStatus_PASS),
			insert.TestResults("b", "A", nil, pb.TestStatus_CRASH),
		)...)

		srv := newTestResultDBService()
		properties := &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"key": structpb.NewStringValue("value"),
			},
		}
		expectedTestVariants := []*pb.TestVariant{
			{
				TestId:      "A",
				Variant:     nil,
				VariantHash: "e3b0c44298fc1c14",
				Results: []*pb.TestResultBundle{
					{
						Result: &pb.TestResult{
							Name:          "invocations/a/tests/A/results/0",
							ResultId:      "0",
							Duration:      &durationpb.Duration{Seconds: 0, Nanos: 234567000},
							Status:        pb.TestStatus_FAIL,
							SummaryHtml:   "SummaryHtml",
							FailureReason: &pb.FailureReason{PrimaryErrorMessage: "failure reason"},
							Properties:    properties,
						},
					}, {
						Result: &pb.TestResult{
							Name:        "invocations/a/tests/A/results/1",
							ResultId:    "1",
							Duration:    &durationpb.Duration{Seconds: 1, Nanos: 234567000},
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
				Variant:     nil,
				VariantHash: "e3b0c44298fc1c14",
				Results: []*pb.TestResultBundle{
					{
						Result: &pb.TestResult{
							Name:          "invocations/a/tests/B/results/1",
							ResultId:      "1",
							Duration:      &durationpb.Duration{Seconds: 1, Nanos: 234567000},
							Status:        pb.TestStatus_CRASH,
							SummaryHtml:   "SummaryHtml",
							FailureReason: &pb.FailureReason{PrimaryErrorMessage: "failure reason"},
							Properties:    properties,
						},
					},
					{
						Result: &pb.TestResult{
							Name:        "invocations/a/tests/B/results/0",
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
				TestId:      "C",
				Variant:     nil,
				VariantHash: "e3b0c44298fc1c14",
				Results: []*pb.TestResultBundle{
					{
						Result: &pb.TestResult{
							Name:        "invocations/a/tests/C/results/0",
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
			},
		}

		Convey(`Permission denied`, func() {
			testutil.MustApply(ctx, insert.Invocation("y", pb.Invocation_ACTIVE, map[string]any{"Realm": "secretproject:secret"}))

			_, err := srv.QueryRunTestVariants(ctx, &pb.QueryRunTestVariantsRequest{
				Invocation: "invocations/y",
			})

			So(err, ShouldBeRPCPermissionDenied, "caller does not have permission resultdb.testResults.list in realm of invocation y")
		})
		Convey(`Invalid argument`, func() {
			Convey(`Empty request`, func() {
				_, err := srv.QueryRunTestVariants(ctx, &pb.QueryRunTestVariantsRequest{})

				So(err, ShouldBeRPCInvalidArgument, `unspecified`)
			})
			Convey(`Invalid invocation name`, func() {
				_, err := srv.QueryRunTestVariants(ctx, &pb.QueryRunTestVariantsRequest{
					Invocation: "x",
				})

				So(err, ShouldBeRPCInvalidArgument, `does not match ^invocations/([a-z][a-z0-9_\-:.]{0,99})$`)
			})
			Convey(`Invalid result limit`, func() {
				_, err := srv.QueryRunTestVariants(ctx, &pb.QueryRunTestVariantsRequest{
					Invocation:  "invocations/a",
					ResultLimit: -1,
				})

				So(err, ShouldBeRPCInvalidArgument, `result_limit: negative`)
			})
			Convey(`Invalid page size`, func() {
				_, err := srv.QueryRunTestVariants(ctx, &pb.QueryRunTestVariantsRequest{
					Invocation: "invocations/a",
					PageSize:   -1,
				})

				So(err, ShouldBeRPCInvalidArgument, `page_size: negative`)
			})
			Convey(`Invalid page token`, func() {
				_, err := srv.QueryRunTestVariants(ctx, &pb.QueryRunTestVariantsRequest{
					Invocation: "invocations/a",
					PageToken:  "aaaa",
				})

				So(err, ShouldBeRPCInvalidArgument, `page_token`)
			})
		})
		Convey(`Invocation not found`, func() {
			_, err := srv.QueryRunTestVariants(ctx, &pb.QueryRunTestVariantsRequest{
				Invocation: "invocations/notexists",
			})
			So(err, ShouldBeRPCNotFound)
		})
		Convey(`Valid, no pagination`, func() {
			result, err := srv.QueryRunTestVariants(ctx, &pb.QueryRunTestVariantsRequest{
				Invocation: "invocations/a",
			})
			So(err, ShouldBeNil)
			So(result, ShouldResembleProto, &pb.QueryRunTestVariantsResponse{
				TestVariants: expectedTestVariants,
			})
		})
		Convey(`Valid, pagination`, func() {
			response, err := srv.QueryRunTestVariants(ctx, &pb.QueryRunTestVariantsRequest{
				Invocation: "invocations/a",
				PageSize:   1,
			})
			So(err, ShouldBeNil)
			So(response, ShouldResembleProto, &pb.QueryRunTestVariantsResponse{
				TestVariants:  expectedTestVariants[:1],
				NextPageToken: "CgFBChBlM2IwYzQ0Mjk4ZmMxYzE0",
			})

			response, err = srv.QueryRunTestVariants(ctx, &pb.QueryRunTestVariantsRequest{
				Invocation: "invocations/a",
				PageSize:   1,
				PageToken:  "CgFBChBlM2IwYzQ0Mjk4ZmMxYzE0",
			})
			So(err, ShouldBeNil)
			So(response, ShouldResembleProto, &pb.QueryRunTestVariantsResponse{
				TestVariants:  expectedTestVariants[1:2],
				NextPageToken: "CgFCChBlM2IwYzQ0Mjk4ZmMxYzE0",
			})

			response, err = srv.QueryRunTestVariants(ctx, &pb.QueryRunTestVariantsRequest{
				Invocation: "invocations/a",
				PageSize:   1,
				PageToken:  "CgFCChBlM2IwYzQ0Mjk4ZmMxYzE0",
			})
			So(err, ShouldBeNil)
			So(response, ShouldResembleProto, &pb.QueryRunTestVariantsResponse{
				TestVariants:  expectedTestVariants[2:],
				NextPageToken: "",
			})
		})
	})
}
