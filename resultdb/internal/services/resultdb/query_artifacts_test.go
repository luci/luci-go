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

	"github.com/golang/protobuf/proto"
	durpb "github.com/golang/protobuf/ptypes/duration"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/resultdb/internal/testutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateQueryArtifactsRequest(t *testing.T) {
	t.Parallel()
	Convey(`TestValidateQueryArtifactsRequest`, t, func() {
		Convey(`Valid`, func() {
			err := validateQueryArtifactsRequest(&pb.QueryArtifactsRequest{
				Invocations:  []string{"invocations/x"},
				PageSize:     50,
				MaxStaleness: &durpb.Duration{Seconds: 60},
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
		ctx := testutil.SpannerTestContext(t)

		testutil.MustApply(ctx, testutil.InsertInvocation("inv1", pb.Invocation_ACTIVE, nil))
		req := &pb.QueryArtifactsRequest{
			Invocations:         []string{"invocations/inv1"},
			PageSize:            100,
			TestResultPredicate: &pb.TestResultPredicate{},
		}

		srv := newTestResultDBService()

		mustQuery := func(req *pb.QueryArtifactsRequest) (arts []*pb.Artifact, token string) {
			res, err := srv.QueryArtifacts(ctx, req)
			So(err, ShouldBeNil)
			return res.Artifacts, res.NextPageToken
		}

		mustQueryNames := func(req *pb.QueryArtifactsRequest) []string {
			arts, _ := mustQuery(req)
			names := make([]string, len(arts))
			for i, a := range arts {
				names[i] = a.Name
			}
			return names
		}

		Convey(`Reads fields correctly`, func() {
			testutil.MustApply(ctx,
				testutil.InsertInvocationArtifact("inv1", "a", map[string]interface{}{
					"ContentType": "text/plain",
					"Size":        54,
				}),
			)
			actual, _ := mustQuery(req)
			So(actual, ShouldHaveLength, 1)
			So(actual[0].ContentType, ShouldEqual, "text/plain")
			So(actual[0].SizeBytes, ShouldEqual, 54)
		})

		Convey(`Reads both invocation and test result artifacts`, func() {
			testutil.MustApply(ctx,
				testutil.InsertInvocationArtifact("inv1", "a", nil),
				testutil.InsertTestResultArtifact("inv1", "t t", "r", "a", nil),
			)
			actual := mustQueryNames(req)
			So(actual, ShouldResemble, []string{
				"invocations/inv1/artifacts/a",
				"invocations/inv1/tests/t%20t/results/r/artifacts/a",
			})
		})

		Convey(`Does not fetch artifacts of other invocations`, func() {
			testutil.MustApply(ctx,
				testutil.InsertInvocation("inv0", pb.Invocation_ACTIVE, nil),
				testutil.InsertInvocation("inv2", pb.Invocation_ACTIVE, nil),
				testutil.InsertInvocationArtifact("inv0", "a", nil),
				testutil.InsertInvocationArtifact("inv1", "a", nil),
				testutil.InsertInvocationArtifact("inv2", "a", nil),
			)
			actual := mustQueryNames(req)
			So(actual, ShouldResemble, []string{"invocations/inv1/artifacts/a"})
		})

		Convey(`Paging`, func() {
			testutil.MustApply(ctx,
				testutil.InsertInvocationArtifact("inv1", "a0", nil),
				testutil.InsertInvocationArtifact("inv1", "a1", nil),
				testutil.InsertInvocationArtifact("inv1", "a2", nil),
				testutil.InsertInvocationArtifact("inv1", "a3", nil),
				testutil.InsertInvocationArtifact("inv1", "a4", nil),
			)

			mustReadPage := func(pageToken string, pageSize int, expectedArtifactIDs ...string) string {
				req2 := proto.Clone(req).(*pb.QueryArtifactsRequest)
				req2.PageToken = pageToken
				req2.PageSize = int32(pageSize)
				arts, token := mustQuery(req2)

				actualArtifactIDs := make([]string, len(arts))
				for i, a := range arts {
					actualArtifactIDs[i] = a.ArtifactId
				}
				So(actualArtifactIDs, ShouldResemble, expectedArtifactIDs)
				return token
			}

			Convey(`All results`, func() {
				token := mustReadPage("", 10, "a0", "a1", "a2", "a3", "a4")
				So(token, ShouldEqual, "")
			})

			Convey(`With pagination`, func() {
				token := mustReadPage("", 1, "a0")
				So(token, ShouldNotEqual, "")

				token = mustReadPage(token, 2, "a1", "a2")
				So(token, ShouldNotEqual, "")

				token = mustReadPage(token, 3, "a3", "a4")
				So(token, ShouldEqual, "")
			})

			Convey(`Bad token`, func() {
				req.PageToken = "CgVoZWxsbw=="
				_, err := srv.QueryArtifacts(ctx, req)
				So(err, ShouldHaveAppStatus, codes.InvalidArgument, "invalid page_token")
			})
		})
	})
}
