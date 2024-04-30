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

package resultdb

import (
	"context"
	"strconv"
	"testing"

	"cloud.google.com/go/spanner"
	durpb "google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"

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
		req := &pb.ListTestResultsRequest{Invocation: "bad_name", PageSize: 50}
		So(validateListTestResultsRequest(req), ShouldErrLike, "invocation: does not match")
	})

	Convey(`Invalid page size`, t, func() {
		req := &pb.ListTestResultsRequest{Invocation: "invocations/valid_id_0", PageSize: -50}
		So(validateListTestResultsRequest(req), ShouldErrLike, "page_size: negative")
	})
}

func TestListTestResults(t *testing.T) {
	Convey(`ListTestResults`, t, func() {
		ctx := auth.WithState(testutil.SpannerTestContext(t), &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "testproject:testrealm", Permission: rdbperms.PermListTestResults},
			},
		})

		// Insert some TestResults.
		testutil.MustApply(ctx,
			insert.Invocation("req", pb.Invocation_ACTIVE, map[string]any{"Realm": "testproject:testrealm"}),
			insert.Invocation("reqx", pb.Invocation_ACTIVE, map[string]any{"Realm": "secretproject:testrealm"}),
		)
		trs := insertTestResults(ctx, "req", "DoBaz", 0,
			[]pb.TestStatus{pb.TestStatus_PASS, pb.TestStatus_FAIL})

		srv := newTestResultDBService()

		Convey(`Permission denied`, func() {
			req := &pb.ListTestResultsRequest{Invocation: "invocations/reqx"}
			_, err := srv.ListTestResults(ctx, req)
			So(err, ShouldBeRPCPermissionDenied, "caller does not have permission resultdb.testResults.list in realm of invocation reqx")
		})

		Convey(`Works`, func() {
			req := &pb.ListTestResultsRequest{Invocation: "invocations/req", PageSize: 1}
			res, err := srv.ListTestResults(ctx, req)
			So(err, ShouldBeNil)
			So(res, ShouldNotBeNil)
			So(res.TestResults, ShouldResembleProto, trs[:1])
			So(res.NextPageToken, ShouldNotEqual, "")

			Convey(`With pagination`, func() {
				req.PageToken = res.NextPageToken
				req.PageSize = 2
				resp, err := srv.ListTestResults(ctx, req)
				So(err, ShouldBeNil)
				So(resp, ShouldNotBeNil)
				So(resp.TestResults, ShouldResembleProto, trs[1:])
				So(resp.NextPageToken, ShouldEqual, "")
			})

			Convey(`With default page size`, func() {
				req := &pb.ListTestResultsRequest{Invocation: "invocations/req"}
				res, err := srv.ListTestResults(ctx, req)
				So(err, ShouldBeNil)
				So(res, ShouldNotBeNil)
				So(res.TestResults, ShouldResembleProto, trs)
				So(res.NextPageToken, ShouldEqual, "")
			})
		})
	})
}

// insertTestResults inserts some test results with the given statuses and returns them.
// A result is expected IFF it is PASS.
func insertTestResults(ctx context.Context, invID invocations.ID, testName string, startID int, statuses []pb.TestStatus) []*pb.TestResult {
	trs := make([]*pb.TestResult, len(statuses))
	ms := make([]*spanner.Mutation, len(statuses))

	for i, status := range statuses {
		testID := "ninja:%2F%2Fchrome%2Ftest:foo_tests%2FBarTest." + testName
		resultID := "result_id_within_inv" + strconv.Itoa(startID+i)

		trs[i] = &pb.TestResult{
			Name:        pbutil.TestResultName(string(invID), testID, resultID),
			TestId:      testID,
			ResultId:    resultID,
			Variant:     pbutil.Variant("k1", "v1", "k2", "v2"),
			VariantHash: pbutil.VariantHash(pbutil.Variant("k1", "v1", "k2", "v2")),
			Expected:    status == pb.TestStatus_PASS,
			Status:      status,
			Duration:    &durpb.Duration{Seconds: int64(i), Nanos: 234567000},
		}

		mutMap := map[string]any{
			"InvocationId":    invID,
			"TestId":          testID,
			"ResultId":        resultID,
			"Variant":         trs[i].Variant,
			"VariantHash":     pbutil.VariantHash(trs[i].Variant),
			"CommitTimestamp": spanner.CommitTimestamp,
			"Status":          status,
			"RunDurationUsec": 1e6*i + 234567,
		}
		if !trs[i].Expected {
			mutMap["IsUnexpected"] = true
		}

		ms[i] = spanutil.InsertMap("TestResults", mutMap)
	}

	testutil.MustApply(ctx, ms...)
	return trs
}
