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
	"strconv"
	"testing"

	"cloud.google.com/go/spanner"
	durpb "github.com/golang/protobuf/ptypes/duration"

	"go.chromium.org/luci/common/clock/testclock"

	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateListTestResultsRequest(t *testing.T) {
	t.Parallel()
	Convey(`Valid`, t, func() {
		req := &pb.ListTestResultsRequest{Invocation: "invocations/valid_id_0", PageSize: 50}
		So(validateListTestResultsRequest(req), ShouldBeNil)
	})

	Convey(`Invalid invocation`, t, func() {
		Convey(`, missing`, func() {
			req := &pb.ListTestResultsRequest{PageSize: 50}
			So(validateListTestResultsRequest(req), ShouldErrLike, "unspecified")
		})

		Convey(`, invalid format`, func() {
			req := &pb.ListTestResultsRequest{Invocation: "bad_name", PageSize: 50}
			So(validateListTestResultsRequest(req), ShouldErrLike, "does not match")
		})
	})

	Convey(`Invalid page size`, t, func() {
		req := &pb.ListTestResultsRequest{Invocation: "invocations/valid_id_0", PageSize: -50}
		So(validateListTestResultsRequest(req), ShouldErrLike, "negative")
	})
}

func TestListTestResults(t *testing.T) {
	Convey(`ListTestResults`, t, func() {
		ctx := testutil.SpannerTestContext(t)

		ct := testclock.TestRecentTimeUTC
		ctx, _ = testclock.UseTime(ctx, ct)

		// Insert some TestResults.
		testutil.MustApply(ctx, testutil.InsertInvocation("req", pb.Invocation_ACTIVE, "", ct))
		trs := insertTestResults(ctx, "req", "DoBaz", 0,
			[]pb.TestStatus{pb.TestStatus_PASS, pb.TestStatus_FAIL})

		srv := NewResultDBServer()

		Convey(`works`, func() {
			req := &pb.ListTestResultsRequest{Invocation: "invocations/req", PageSize: 1}
			resp, err := srv.ListTestResults(ctx, req)
			So(err, ShouldBeNil)
			So(resp, ShouldNotBeNil)
			So(resp.TestResults, ShouldResembleProto, trs[:1])
			So(resp.NextPageToken, ShouldNotEqual, "")

			Convey(`with pagination`, func() {
				req.PageToken = resp.NextPageToken
				req.PageSize = 2
				resp, err := srv.ListTestResults(ctx, req)
				So(err, ShouldBeNil)
				So(resp, ShouldNotBeNil)
				So(resp.TestResults, ShouldResembleProto, trs[1:])
				So(resp.NextPageToken, ShouldEqual, "")
			})

			Convey(`with default page size`, func() {
				req := &pb.ListTestResultsRequest{Invocation: "invocations/req"}
				resp, err := srv.ListTestResults(ctx, req)
				So(err, ShouldBeNil)
				So(resp, ShouldNotBeNil)
				So(resp.TestResults, ShouldResembleProto, trs)
				So(resp.NextPageToken, ShouldEqual, "")
			})
		})
	})
}

func TestAdjustPageSize(t *testing.T) {
	t.Parallel()
	Convey(`OK`, t, func() {
		So(adjustPageSize(50), ShouldEqual, 50)
	})
	Convey(`too big`, t, func() {
		So(adjustPageSize(1e6), ShouldEqual, pageSizeMax)
	})
	Convey(`missing or 0`, t, func() {
		So(adjustPageSize(0), ShouldEqual, pageSizeDefault)
	})
}

func TestReadTestResults(t *testing.T) {
	Convey(`ReadTestResults`, t, func() {
		ctx := testutil.SpannerTestContext(t)

		ct := testclock.TestRecentTimeUTC
		ctx, _ = testclock.UseTime(ctx, ct)

		// Insert some TestResults.
		testutil.MustApply(ctx, testutil.InsertInvocation("inv", pb.Invocation_ACTIVE, "", ct))
		trs := insertTestResults(ctx, "inv", "DoBaz", 0,
			[]pb.TestStatus{pb.TestStatus_PASS, pb.TestStatus_FAIL,
				pb.TestStatus_FAIL, pb.TestStatus_PASS, pb.TestStatus_FAIL,
			})

		Convey(`all results`, func() {
			token := testRead(ctx, "inv", true, "", 10, trs)
			So(token, ShouldEqual, "")

			Convey(`with pagination`, func() {
				token := testRead(ctx, "inv", true, "", 1, trs[:1])
				So(token, ShouldNotEqual, "")

				token = testRead(ctx, "inv", true, token, 4, trs[1:])
				So(token, ShouldNotEqual, "")

				token = testRead(ctx, "inv", true, token, 5, nil)
				So(token, ShouldEqual, "")
			})
		})

		Convey(`unexpected results`, func() {
			testRead(ctx, "inv", false, "", 10, append(trs[1:3], trs[4]))

			Convey(`with pagination`, func() {
				token := testRead(ctx, "inv", false, "", 1, trs[1:2])
				So(token, ShouldNotEqual, "")

				token = testRead(ctx, "inv", false, token, 1, trs[2:3])
				So(token, ShouldNotEqual, "")

				token = testRead(ctx, "inv", false, token, 10, trs[4:])
				So(token, ShouldEqual, "")
			})
		})

		Convey(`span test path boundaries`, func() {
			trs = append(trs, insertTestResults(ctx, "inv", "DoQux", 0,
				[]pb.TestStatus{pb.TestStatus_PASS, pb.TestStatus_FAIL})...)
			token := testRead(ctx, "inv", true, "", 6, trs[:6])
			So(token, ShouldNotEqual, "")

			testRead(ctx, "inv", true, token, 2, trs[6:])
		})

		Convey(`errors with bad token`, func() {
			txn, err := span.Client(ctx).BatchReadOnlyTransaction(ctx, spanner.StrongRead())
			So(err, ShouldBeNil)
			defer txn.Close()

			Convey(`from bad position`, func() {
				_, _, err = span.ReadTestResults(ctx, txn, "invocations/inv", true, "CgVoZWxsbw==", 5)
				So(err, ShouldErrLike, "invalid page_token")
			})

			Convey(`from decoding`, func() {
				_, _, err = span.ReadTestResults(ctx, txn, "invocations/inv", true, "%%%", 5)
				So(err, ShouldErrLike, "invalid page_token")
			})
		})
	})
}

func testRead(ctx context.Context, invID span.InvocationID, excludeExpected bool, token string, pageSize int, expected []*pb.TestResult) string {
	txn, err := span.Client(ctx).BatchReadOnlyTransaction(ctx, spanner.StrongRead())
	So(err, ShouldBeNil)
	defer txn.Close()

	trs, token, err := span.ReadTestResults(ctx, txn, invID, excludeExpected, token, pageSize)
	So(err, ShouldBeNil)
	So(trs, ShouldResembleProto, expected)

	return token
}

// insertTestResults inserts some test results with the given statuses and returns them.
// A result is expected IFF it is PASS.
func insertTestResults(ctx context.Context, invID span.InvocationID, testName string, startID int, statuses []pb.TestStatus) []*pb.TestResult {
	trs := make([]*pb.TestResult, len(statuses))
	muts := make([]*spanner.Mutation, len(statuses))

	for i, status := range statuses {
		testPath := "gn:%2F%2Fchrome%2Ftest:foo_tests%2FBarTest." + testName
		resultID := "result_id_within_inv" + strconv.Itoa(startID+i)

		trs[i] = &pb.TestResult{
			Name:     pbutil.TestResultName(string(invID), testPath, resultID),
			TestPath: testPath,
			ResultId: resultID,
			ExtraVariantPairs: &pb.VariantDef{Def: map[string]string{
				"k1": "v1",
				"k2": "v2",
			}},
			Expected: status == pb.TestStatus_PASS,
			Status:   status,
			Duration: &durpb.Duration{Seconds: int64(i), Nanos: 234567000},
		}

		mutMap := map[string]interface{}{
			"InvocationId":      invID,
			"TestPath":          testPath,
			"ResultId":          resultID,
			"ExtraVariantPairs": trs[i].ExtraVariantPairs,
			"CommitTimestamp":   spanner.CommitTimestamp,
			"Status":            status,
			"RunDurationUsec":   1e6*i + 234567,
		}
		if !trs[i].Expected {
			mutMap["IsUnexpected"] = true
		}

		muts[i] = span.InsertMap("TestResults", mutMap)
	}

	testutil.MustApply(ctx, muts...)
	return trs
}
