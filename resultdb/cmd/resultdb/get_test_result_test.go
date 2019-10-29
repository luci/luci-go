// Copyright 2019 The LUCI Authors.
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

package main

import (
	"context"
	"testing"

	"cloud.google.com/go/spanner"
	durpb "github.com/golang/protobuf/ptypes/duration"

	"go.chromium.org/luci/common/clock/testclock"

	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/internal/testutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateGetTestResultRequest(t *testing.T) {
	t.Parallel()
	Convey(`ValidateGetTestResultRequest`, t, func() {
		Convey(`Valid`, func() {
			req := &pb.GetTestResultRequest{Name: "invocations/a/tests/gn:%2F%2Fchrome%2Ftest:foo_tests%2FBarTest.DoBaz/results/result5"}
			So(validateGetTestResultRequest(req), ShouldBeNil)
		})

		Convey(`Invalid name`, func() {
			Convey(`, missing`, func() {
				req := &pb.GetTestResultRequest{}
				So(validateGetTestResultRequest(req), ShouldErrLike, "unspecified")
			})

			Convey(`, invalid format`, func() {
				req := &pb.GetTestResultRequest{Name: "bad_name"}
				So(validateGetTestResultRequest(req), ShouldErrLike, "does not match")
			})
		})
	})
}

func TestGetTestResult(t *testing.T) {
	Convey(`GetTestResult`, t, func() {
		ctx := testutil.SpannerTestContext(t)

		ct := testclock.TestRecentTimeUTC
		ctx, _ = testclock.UseTime(ctx, ct)

		srv := NewResultDBServer()
		test := func(ctx context.Context, name string, expected *pb.TestResult) {
			req := &pb.GetTestResultRequest{Name: name}
			tr, err := srv.GetTestResult(ctx, req)
			So(err, ShouldBeNil)
			So(tr, ShouldResembleProto, expected)
		}

		// Insert a TestResult.
		testutil.MustApply(ctx,
			testutil.InsertInvocation("inv_0", pb.Invocation_ACTIVE, "", ct),
			spanner.InsertMap("TestResults", span.ToSpannerMap(map[string]interface{}{
				"InvocationId": "inv_0",
				"TestPath":     "gn://chrome/test:foo_tests/BarTest.DoBaz",
				"ResultId":     "result_id_within_inv_0",
				"ExtraVariantPairs": &pb.VariantDef{Def: map[string]string{
					"k1": "v1",
					"k2": "v2",
				}},
				"CommitTimestamp": spanner.CommitTimestamp,
				"IsUnexpected":    true,
				"Status":          pb.TestStatus_FAIL,
				"RunDurationUsec": 1234567,
			},
			)))

		// Fetch back the TestResult.
		test(ctx, "invocations/inv_0/tests/gn:%2F%2Fchrome%2Ftest:foo_tests%2FBarTest.DoBaz/results/result_id_within_inv_0",
			&pb.TestResult{
				Name:     "invocations/inv_0/tests/gn:%2F%2Fchrome%2Ftest:foo_tests%2FBarTest.DoBaz/results/result_id_within_inv_0",
				TestPath: "gn://chrome/test:foo_tests/BarTest.DoBaz",
				ResultId: "result_id_within_inv_0",
				ExtraVariantPairs: &pb.VariantDef{Def: map[string]string{
					"k1": "v1",
					"k2": "v2",
				}},
				Expected: false,
				Status:   pb.TestStatus_FAIL,
				Duration: &durpb.Duration{Seconds: 1, Nanos: 234567000},
			},
		)

		Convey(`works with expected result`, func() {
			testutil.MustApply(ctx, spanner.InsertMap("TestResults", span.ToSpannerMap(map[string]interface{}{
				"InvocationId": "inv_0",
				"TestPath":     "gn://chrome/test:foo_tests/BarTest.DoBaz",
				"ResultId":     "result_id_within_inv_1",
				"ExtraVariantPairs": &pb.VariantDef{Def: map[string]string{
					"k1": "v1",
					"k2": "v2",
				}},
				"CommitTimestamp": spanner.CommitTimestamp,
				"Status":          pb.TestStatus_PASS,
				"RunDurationUsec": 1534567,
			})))

			// Fetch back the TestResult.
			test(ctx, "invocations/inv_0/tests/gn:%2F%2Fchrome%2Ftest:foo_tests%2FBarTest.DoBaz/results/result_id_within_inv_1",
				&pb.TestResult{
					Name:     "invocations/inv_0/tests/gn:%2F%2Fchrome%2Ftest:foo_tests%2FBarTest.DoBaz/results/result_id_within_inv_1",
					TestPath: "gn://chrome/test:foo_tests/BarTest.DoBaz",
					ResultId: "result_id_within_inv_1",
					ExtraVariantPairs: &pb.VariantDef{Def: map[string]string{
						"k1": "v1",
						"k2": "v2",
					}},
					Expected: true,
					Status:   pb.TestStatus_PASS,
					Duration: &durpb.Duration{Seconds: 1, Nanos: 534567000},
				},
			)
		})
	})
}
