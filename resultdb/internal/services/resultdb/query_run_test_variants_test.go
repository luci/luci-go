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

	"google.golang.org/grpc/codes"

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

func TestValidateQueryPartialTestVariantsRequest(t *testing.T) {
	t.Parallel()
	Convey(`Validate`, t, func() {
		Convey(`no invocation`, func() {
			err := validateQueryRunTestVariantsRequest(&pb.QueryRunTestVariantsRequest{})
			So(err, ShouldErrLike, `invocation: unspecified`)
		})

		Convey(`invalid invocation`, func() {
			err := validateQueryRunTestVariantsRequest(&pb.QueryRunTestVariantsRequest{
				Invocation: "x",
			})
			So(err, ShouldErrLike, `invocation: does not match`)
		})

		Convey(`invalid page size`, func() {
			err := validateQueryRunTestVariantsRequest(&pb.QueryRunTestVariantsRequest{
				Invocation: "invocations/x",
				PageSize:   -1,
			})
			So(err, ShouldErrLike, `page_size: negative`)
		})

		Convey(`invalid result limit`, func() {
			err := validateQueryRunTestVariantsRequest(&pb.QueryRunTestVariantsRequest{
				Invocation:  "invocations/x",
				ResultLimit: -1,
			})
			So(err, ShouldErrLike, `result_limit: negative`)
		})
	})
}

func TestQueryPartialTestVariants(t *testing.T) {
	Convey(`QueryPartialTestVariants`, t, func() {
		ctx := auth.WithState(testutil.SpannerTestContext(t), &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "testproject:testrealm", Permission: rdbperms.PermListTestResults},
			},
		})
		ctx, _ = tsmon.WithDummyInMemory(ctx)

		insertInv := insert.FinalizedInvocationWithInclusions
		insertTRs := insert.TestResults
		testutil.MustApply(ctx, testutil.CombineMutations(
			insertInv("a", map[string]any{"Realm": "testproject:testrealm"}, "b"),
			insertInv("b", map[string]any{"Realm": "testproject:testrealm"}),
			insertTRs("a", "A", nil, pb.TestStatus_FAIL, pb.TestStatus_PASS),
			insertTRs("a", "B", nil, pb.TestStatus_CRASH, pb.TestStatus_PASS),
			insertTRs("a", "C", nil, pb.TestStatus_PASS),
			insertTRs("b", "A", nil, pb.TestStatus_FAIL),
		)...)

		srv := newTestResultDBService()

		Convey(`Permission denied`, func() {
			testutil.MustApply(ctx, insertInv("y", map[string]any{"Realm": "secretproject:secret"})...)

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

			Convey(`Invalid result limit`, func() {
				_, err := srv.QueryRunTestVariants(ctx, &pb.QueryRunTestVariantsRequest{
					Invocation:  "invocations/a",
					ResultLimit: -1,
				})

				So(err, ShouldBeRPCInvalidArgument, `result_limit: negative`)
			})
		})
		Convey(`Not found`, func() {
			_, err := srv.QueryRunTestVariants(ctx, &pb.QueryRunTestVariantsRequest{
				Invocation: "invocations/notexists",
			})
			So(err, ShouldBeRPCNotFound)
		})
		Convey(`Valid`, func() {
			_, err := srv.QueryRunTestVariants(ctx, &pb.QueryRunTestVariantsRequest{
				Invocation: "invocations/a",
			})
			So(err, ShouldHaveRPCCode, codes.Unimplemented)
		})
	})
}
