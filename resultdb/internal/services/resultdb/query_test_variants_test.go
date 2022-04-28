// Copyright 2020 The LUCI Authors.
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
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"google.golang.org/grpc/codes"

	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/resultdb/internal/pagination"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
)

func TestQueryTestVariants(t *testing.T) {
	Convey(`QueryTestVariants`, t, func() {
		ctx := auth.WithState(testutil.SpannerTestContext(t), &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "testproject:testrealm", Permission: permListTestResults},
			},
		})

		testutil.MustApply(
			ctx,
			insert.InvocationWithInclusions("inv0", pb.Invocation_ACTIVE, map[string]interface{}{"Realm": "testproject:testrealm"}, "inv1")...,
		)
		testutil.MustApply(
			ctx,
			insert.Invocation("inv1", pb.Invocation_ACTIVE, map[string]interface{}{"Realm": "testproject:testrealm"}),
		)
		testutil.MustApply(ctx, testutil.CombineMutations(
			insert.TestResults("inv0", "T1", nil, pb.TestStatus_FAIL),
			insert.TestResults("inv0", "T2", nil, pb.TestStatus_FAIL),
			insert.TestResults("inv1", "T3", nil, pb.TestStatus_PASS),
			insert.TestResults("inv1", "T1", pbutil.Variant("a", "b"), pb.TestStatus_FAIL, pb.TestStatus_PASS),
			insert.TestExonerations("inv0", "T1", nil, pb.ExonerationReason_OCCURS_ON_OTHER_CLS),
			insert.TestExonerationsLegacy("inv0", "T1", nil, 1),
		)...)

		srv := &resultDBServer{}

		Convey(`Permission denied`, func() {
			testutil.MustApply(ctx, insert.Invocation("invx", pb.Invocation_ACTIVE, map[string]interface{}{"Realm": "randomproject:testrealm"}))
			_, err := srv.QueryTestVariants(ctx, &pb.QueryTestVariantsRequest{
				Invocations: []string{"invocations/invx"},
			})
			So(err, ShouldHaveAppStatus, codes.PermissionDenied)
		})

		Convey(`Valid with included invocation`, func() {
			res, err := srv.QueryTestVariants(ctx, &pb.QueryTestVariantsRequest{
				Invocations: []string{"invocations/inv0"},
			})
			So(err, ShouldBeNil)
			So(res.NextPageToken, ShouldEqual, pagination.Token("EXPECTED", "", ""))

			So(len(res.TestVariants), ShouldEqual, 3)
			getTVStrings := func(tvs []*pb.TestVariant) []string {
				tvStrings := make([]string, len(tvs))
				for i, tv := range tvs {
					tvStrings[i] = fmt.Sprintf("%d/%s/%s", int32(tv.Status), tv.TestId, tv.VariantHash)
				}
				return tvStrings
			}
			So(getTVStrings(res.TestVariants), ShouldResemble, []string{
				"10/T2/e3b0c44298fc1c14",
				"30/T1/c467ccce5a16dc72",
				"40/T1/e3b0c44298fc1c14",
			})
		})

		Convey(`Valid without included invocation`, func() {
			res, err := srv.QueryTestVariants(ctx, &pb.QueryTestVariantsRequest{
				Invocations: []string{"invocations/inv1"},
			})
			So(err, ShouldBeNil)
			So(res.NextPageToken, ShouldEqual, pagination.Token("EXPECTED", "", ""))

			So(len(res.TestVariants), ShouldEqual, 1)
			getTVStrings := func(tvs []*pb.TestVariant) []string {
				tvStrings := make([]string, len(tvs))
				for i, tv := range tvs {
					tvStrings[i] = fmt.Sprintf("%d/%s/%s", int32(tv.Status), tv.TestId, tv.VariantHash)
				}
				return tvStrings
			}
			So(getTVStrings(res.TestVariants), ShouldResemble, []string{
				"30/T1/c467ccce5a16dc72",
			})
		})

		Convey(`Valid with missing included invocation`, func() {
			testutil.MustApply(
				ctx,
				// The invocation missinginv is missing in Invocations table.
				insert.Inclusion("inv0", "missinginv"),
			)
			res, err := srv.QueryTestVariants(ctx, &pb.QueryTestVariantsRequest{
				Invocations: []string{"invocations/inv0", "invocations/inv1"},
			})
			So(err, ShouldBeNil)
			So(res.NextPageToken, ShouldEqual, pagination.Token("EXPECTED", "", ""))

			So(len(res.TestVariants), ShouldEqual, 3)
			getTVStrings := func(tvs []*pb.TestVariant) []string {
				tvStrings := make([]string, len(tvs))
				for i, tv := range tvs {
					tvStrings[i] = fmt.Sprintf("%d/%s/%s", int32(tv.Status), tv.TestId, tv.VariantHash)
				}
				return tvStrings
			}
			So(getTVStrings(res.TestVariants), ShouldResemble, []string{
				"10/T2/e3b0c44298fc1c14",
				"30/T1/c467ccce5a16dc72",
				"40/T1/e3b0c44298fc1c14",
			})
		})

		Convey(`Try next page`, func() {
			res, err := srv.QueryTestVariants(ctx, &pb.QueryTestVariantsRequest{
				Invocations: []string{"invocations/inv0", "invocations/inv1"},
				PageSize:    3,
				PageToken:   pagination.Token("EXPECTED", "", ""),
			})

			So(err, ShouldBeNil)
			So(len(res.TestVariants), ShouldEqual, 1)
		})
	})
}
