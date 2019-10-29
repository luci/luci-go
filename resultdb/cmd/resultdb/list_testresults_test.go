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

	"go.chromium.org/luci/resultdb/internal"
	"go.chromium.org/luci/resultdb/internal/proto"
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
			So(validateListTestResultsRequest(req), ShouldErrLike, "invocation missing")
		})

		Convey(`, invalid format`, func() {
			req := &pb.ListTestResultsRequest{Invocation: "bad_name", PageSize: 50}
			So(validateListTestResultsRequest(req), ShouldErrLike, "does not match")
		})
	})

	Convey(`Invalid page size`, t, func() {
		req := &pb.ListTestResultsRequest{Invocation: "invocations/valid_id_0", PageSize: -50}
		So(validateListTestResultsRequest(req), ShouldErrLike, "negative page size")
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
			So(resp.TestResults, ShouldResembleProto, trs[0:1])
			So(resp.NextPageToken, ShouldNotEqual, "")

			Convey(`with pagination`, func() {
				req.PageToken = resp.NextPageToken
				resp, err := srv.ListTestResults(ctx, req)
				So(err, ShouldBeNil)
				So(resp, ShouldNotBeNil)
				So(resp.TestResults, ShouldResembleProto, trs[1:2])
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
			[]pb.TestStatus{pb.TestStatus_PASS, pb.TestStatus_FAIL})

		Convey(`all results`, func() {
			cursor := testRead(ctx, "invocations/inv", false, internal.NewCursor(nil), 5, trs)
			tok, err := internal.Token(cursor)
			So(err, ShouldBeNil)
			So(tok, ShouldEqual, "")
		})

		Convey(`unexpected results`, func() {
			testRead(ctx, "invocations/inv", true, internal.NewCursor(nil), 5, trs[1:2])
		})

		Convey(`limit results`, func() {
			cursor := testRead(ctx, "invocations/inv", false, internal.NewCursor(nil), 1, trs[0:1])
			tok, err := internal.Token(cursor)
			So(err, ShouldBeNil)
			So(tok, ShouldNotEqual, "")

			Convey(`and paginate`, func() {
				cursor = testRead(ctx, "invocations/inv", false, cursor, 1, trs[1:2])
				tok, err := internal.Token(cursor)
				So(err, ShouldBeNil)
				So(tok, ShouldEqual, "")
			})

			Convey(`span test path boundaries`, func() {
				trs = append(trs, insertTestResults(ctx, "inv", "DoQux", 0,
					[]pb.TestStatus{pb.TestStatus_PASS, pb.TestStatus_FAIL})...)
				cursor := testRead(ctx, "invocations/inv", false, internal.NewCursor(nil), 3, trs[0:3])
				tok, err := internal.Token(cursor)
				So(err, ShouldBeNil)
				So(tok, ShouldNotEqual, "")

				Convey(`append results from previous test path`, func() {
					trs = append(trs, insertTestResults(ctx, "inv", "DoBar", 2,
						[]pb.TestStatus{pb.TestStatus_PASS})...)
					cursor = testRead(ctx, "invocations/inv", false, cursor, 3, trs[3:4])
					tok, err := internal.Token(cursor)
					So(err, ShouldBeNil)
					So(tok, ShouldEqual, "")
				})
			})
		})
	})
}

func testRead(ctx context.Context, invName string, excludeExpected bool, cursor *internalpb.Cursor, pageSize int, expected []*pb.TestResult) *internalpb.Cursor {
	txn, err := span.Client(ctx).BatchReadOnlyTransaction(ctx, spanner.StrongRead())
	So(err, ShouldBeNil)
	defer txn.Close()

	trs, cursor, err := span.ReadTestResults(ctx, txn, invName, excludeExpected, cursor, pageSize)
	So(err, ShouldBeNil)
	So(trs, ShouldResembleProto, expected)

	return cursor
}

// insertTestResults inserts some test results with the given statuses and returns them.
// A result is expected IFF it is PASS.
func insertTestResults(ctx context.Context, invID, testName string, startID int, statuses []pb.TestStatus) []*pb.TestResult {
	trs := make([]*pb.TestResult, len(statuses))
	muts := make([]*spanner.Mutation, len(statuses))

	for i, status := range statuses {
		testPath := "gn:%2F%2Fchrome%2Ftest:foo_tests%2FBarTest." + testName
		resultID := "result_id_within_inv" + strconv.Itoa(startID+i)

		trs[i] = &pb.TestResult{
			Name:     pbutil.TestResultName(invID, testPath, resultID),
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

		muts[i] = spanner.InsertMap("TestResults", span.ToSpannerMap(map[string]interface{}{
			"InvocationId":      invID,
			"TestPath":          testPath,
			"ResultId":          resultID,
			"ExtraVariantPairs": trs[i].ExtraVariantPairs,
			"CommitTimestamp":   spanner.CommitTimestamp,
			"IsUnexpected":      !trs[i].Expected,
			"Status":            status,
			"RunDurationUsec":   1e6*i + 234567,
		}))
	}

	testutil.MustApply(ctx, muts...)
	return trs
}
