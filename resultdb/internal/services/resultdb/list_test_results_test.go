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
	"google.golang.org/grpc/codes"
	durpb "google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
)

func TestValidateListTestResultsRequest(t *testing.T) {
	t.Parallel()
	ftt.Run(`Valid`, t, func(t *ftt.Test) {
		req := &pb.ListTestResultsRequest{Invocation: "invocations/valid_id_0", PageSize: 50}
		assert.Loosely(t, validateListTestResultsRequest(req), should.BeNil)
	})

	ftt.Run(`Invalid invocation`, t, func(t *ftt.Test) {
		req := &pb.ListTestResultsRequest{Invocation: "bad_name", PageSize: 50}
		assert.Loosely(t, validateListTestResultsRequest(req), should.ErrLike("invocation: does not match"))
	})

	ftt.Run(`Invalid page size`, t, func(t *ftt.Test) {
		req := &pb.ListTestResultsRequest{Invocation: "invocations/valid_id_0", PageSize: -50}
		assert.Loosely(t, validateListTestResultsRequest(req), should.ErrLike("page_size: negative"))
	})
}

func TestListTestResults(t *testing.T) {
	ftt.Run(`ListTestResults`, t, func(t *ftt.Test) {
		ctx := auth.WithState(testutil.SpannerTestContext(t), &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "testproject:testrealm", Permission: rdbperms.PermListTestResults},
			},
		})

		// Insert some TestResults.
		testutil.MustApply(ctx, t,
			insert.Invocation("req", pb.Invocation_ACTIVE, map[string]any{"Realm": "testproject:testrealm"}),
			insert.Invocation("reqx", pb.Invocation_ACTIVE, map[string]any{"Realm": "secretproject:testrealm"}),
		)
		trs := insertTestResults(ctx, t, "req", "DoBaz", 0,
			[]pb.TestStatus{pb.TestStatus_PASS, pb.TestStatus_FAIL})

		srv := newTestResultDBService()

		t.Run(`Permission denied`, func(t *ftt.Test) {
			req := &pb.ListTestResultsRequest{Invocation: "invocations/reqx"}
			_, err := srv.ListTestResults(ctx, req)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike("caller does not have permission resultdb.testResults.list in realm of invocation reqx"))
		})

		t.Run(`Works`, func(t *ftt.Test) {
			req := &pb.ListTestResultsRequest{Invocation: "invocations/req", PageSize: 1}
			res, err := srv.ListTestResults(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.NotBeNil)
			assert.Loosely(t, res.TestResults, should.Match(trs[:1]))
			assert.Loosely(t, res.NextPageToken, should.NotEqual(""))

			t.Run(`With pagination`, func(t *ftt.Test) {
				req.PageToken = res.NextPageToken
				req.PageSize = 2
				resp, err := srv.ListTestResults(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, resp, should.NotBeNil)
				assert.Loosely(t, resp.TestResults, should.Match(trs[1:]))
				assert.Loosely(t, resp.NextPageToken, should.BeEmpty)
			})

			t.Run(`With default page size`, func(t *ftt.Test) {
				req := &pb.ListTestResultsRequest{Invocation: "invocations/req"}
				res, err := srv.ListTestResults(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.NotBeNil)
				assert.Loosely(t, res.TestResults, should.Match(trs))
				assert.Loosely(t, res.NextPageToken, should.BeEmpty)
			})
		})
	})
}

// insertTestResults inserts some test results with the given statuses and returns them.
// A result is expected IFF it is PASS.
func insertTestResults(ctx context.Context, t testing.TB, invID invocations.ID, testName string, startID int, statuses []pb.TestStatus) []*pb.TestResult {
	t.Helper()
	trs := make([]*pb.TestResult, len(statuses))
	ms := make([]*spanner.Mutation, len(statuses))

	for i, status := range statuses {
		testID := "://chrome/test\\:foo_tests!gtest::BarTest#" + testName
		resultID := "result_id_within_inv" + strconv.Itoa(startID+i)

		v := pbutil.Variant("k1", "v1", "k2", "v2")
		trs[i] = &pb.TestResult{
			Name:     pbutil.TestResultName(string(invID), testID, resultID),
			TestId:   testID,
			ResultId: resultID,
			TestVariantIdentifier: &pb.TestVariantIdentifier{
				ModuleName:        "//chrome/test:foo_tests",
				ModuleScheme:      "gtest",
				ModuleVariant:     v,
				CoarseName:        "",
				FineName:          "BarTest",
				CaseName:          testName,
				ModuleVariantHash: pbutil.VariantHash(v),
			},
			Variant:     v,
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

	testutil.MustApply(ctx, t, ms...)
	return trs
}
