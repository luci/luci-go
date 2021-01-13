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

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	pb "go.chromium.org/luci/resultdb/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateQueryArtifactsRequest(t *testing.T) {
	t.Parallel()
	Convey(`TestValidateQueryArtifactsRequest`, t, func() {
		Convey(`Valid`, func() {
			err := validateQueryArtifactsRequest(&pb.QueryArtifactsRequest{
				Invocations: []string{"invocations/x"},
				PageSize:    50,
			})
			So(err, ShouldBeNil)
		})

		Convey(`Invalid invocation`, func() {
			err := validateQueryArtifactsRequest(&pb.QueryArtifactsRequest{
				Invocations: []string{"x"},
			})
			So(err, ShouldErrLike, `invocations: "x": does not match`)
		})

		Convey(`Invalid test result predicate`, func() {
			err := validateQueryArtifactsRequest(&pb.QueryArtifactsRequest{
				Invocations:         []string{"x"},
				TestResultPredicate: &pb.TestResultPredicate{TestIdRegexp: ")"},
			})
			So(err, ShouldErrLike, `test_result_predicate: test_id_regexp: error parsing regexp`)
		})
	})
}

func TestQueryArtifacts(t *testing.T) {
	Convey(`QueryArtifacts`, t, func() {
		ctx := auth.WithState(testutil.SpannerTestContext(t), &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "testproject:testrealm", Permission: permListArtifacts},
			},
		})

		testutil.MustApply(
			ctx,
			insert.Invocation("inv1", pb.Invocation_ACTIVE, map[string]interface{}{"Realm": "testproject:testrealm"}),
			insert.Invocation("invx", pb.Invocation_ACTIVE, map[string]interface{}{"Realm": "secretproject:testrealm"}),
		)
		req := &pb.QueryArtifactsRequest{
			Invocations:         []string{"invocations/inv1"},
			PageSize:            100,
			TestResultPredicate: &pb.TestResultPredicate{},
		}

		srv := newTestResultDBService()

		mustFetch := func(req *pb.QueryArtifactsRequest) (arts []*pb.Artifact, token string) {
			res, err := srv.QueryArtifacts(ctx, req)
			So(err, ShouldBeNil)
			return res.Artifacts, res.NextPageToken
		}

		mustFetchNames := func(req *pb.QueryArtifactsRequest) []string {
			arts, _ := mustFetch(req)
			names := make([]string, len(arts))
			for i, a := range arts {
				names[i] = a.Name
			}
			return names
		}

		Convey(`Permission denied`, func() {
			req.Invocations = []string{"invocations/invx"}
			_, err := srv.QueryArtifacts(ctx, req)
			So(err, ShouldHaveAppStatus, codes.PermissionDenied)
		})

		Convey(`Reads both invocation and test result artifacts`, func() {
			testutil.MustApply(ctx,
				insert.Artifact("inv1", "", "a", nil),
				insert.Artifact("inv1", "tr/t t/r", "a", nil),
			)
			actual := mustFetchNames(req)
			So(actual, ShouldResemble, []string{
				"invocations/inv1/artifacts/a",
				"invocations/inv1/tests/t%20t/results/r/artifacts/a",
			})
		})

		Convey(`Fetch URL`, func() {
			testutil.MustApply(ctx,
				insert.Artifact("inv1", "", "a", nil),
			)
			actual, _ := mustFetch(req)
			So(actual, ShouldHaveLength, 1)
			So(actual[0].FetchUrl, ShouldEqual, "https://signed-url.example.com/invocations/inv1/artifacts/a")
		})
	})
}
