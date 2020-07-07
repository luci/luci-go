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
	"testing"

	pb "go.chromium.org/luci/resultdb/proto/v1"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestQueryTestResultStatistics(t *testing.T) {
	Convey(`QueryTestResultStatistics`, t, func() {
		ctx := auth.WithState(testutil.SpannerTestContext(t), &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "testproject:testrealm", Permission: permListTestResults},
			},
		})

		testutil.MustApply(ctx,
			insert.Invocation(
				"x", pb.Invocation_FINALIZED, map[string]interface{}{
					"Realm": "secretproject:testrealm",
				}),
			insert.Invocation(
				"a", pb.Invocation_FINALIZED, map[string]interface{}{
					"Realm": "testproject:testrealm",
				}),
			insert.Invocation(
				"b", pb.Invocation_FINALIZED, map[string]interface{}{
					"TestResultCount": 5,
					"Realm":           "otherproject:testrealm",
				}),
			insert.Invocation(
				"c", pb.Invocation_FINALIZED, map[string]interface{}{
					"TestResultCount": 10,
					"Realm":           "otherproject:testrealm",
				}),
			insert.Invocation(
				"d", pb.Invocation_FINALIZED, map[string]interface{}{
					"TestResultCount": 20,
					"Realm":           "otherproject:testrealm",
				}),
			insert.Inclusion("a", "b"),
			insert.Inclusion("a", "c"),
			// To test no double count if an invocation is included by multiple
			// invocations.
			insert.Inclusion("b", "d"),
			insert.Inclusion("c", "d"),
		)

		srv := newTestResultDBService()

		Convey(`Permission denied`, func() {
			_, err := srv.QueryTestResultStatistics(ctx, &pb.QueryTestResultStatisticsRequest{
				Invocations: []string{"invocations/x"},
			})
			So(err, ShouldHaveAppStatus, codes.PermissionDenied)
		})

		Convey(`Valid`, func() {
			res, err := srv.QueryTestResultStatistics(ctx, &pb.QueryTestResultStatisticsRequest{
				Invocations: []string{"invocations/a"},
			})
			So(err, ShouldBeNil)
			So(res.TotalTestResults, ShouldEqual, 35)
		})
	})

}
