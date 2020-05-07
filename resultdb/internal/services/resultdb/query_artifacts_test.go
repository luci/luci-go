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
	"go.chromium.org/luci/resultdb/pbutil"
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

		insInv := testutil.InsertInvocation
		insInvArt := testutil.InsertInvocationArtifact
		insTRArt := testutil.InsertTestResultArtifact
		insTRs := testutil.InsertTestResults

		testutil.MustApply(ctx, insInv("inv1", pb.Invocation_ACTIVE, nil))
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

		Convey(`Populates fields correctly`, func() {
			testutil.MustApply(ctx,
				insInvArt("inv1", "a", map[string]interface{}{
					"ContentType": "text/plain",
					"Size":        64,
				}),
			)
			actual, _ := mustQuery(req)
			So(actual, ShouldHaveLength, 1)
			So(actual[0].ContentType, ShouldEqual, "text/plain")
			So(actual[0].SizeBytes, ShouldEqual, 64)
			So(actual[0].FetchUrl, ShouldEqual, "https://signed-url.example.com/invocations/inv1/artifacts/a")
		})

		Convey(`Reads both invocation and test result artifacts`, func() {
			testutil.MustApply(ctx,
				insInvArt("inv1", "a", nil),
				insTRArt("inv1", "t t", "r", "a", nil),
			)
			actual := mustQueryNames(req)
			So(actual, ShouldResemble, []string{
				"invocations/inv1/artifacts/a",
				"invocations/inv1/tests/t%20t/results/r/artifacts/a",
			})
		})

		Convey(`Does not fetch artifacts of other invocations`, func() {
			testutil.MustApply(ctx,
				insInv("inv0", pb.Invocation_ACTIVE, nil),
				insInv("inv2", pb.Invocation_ACTIVE, nil),
				insInvArt("inv0", "a", nil),
				insInvArt("inv1", "a", nil),
				insInvArt("inv2", "a", nil),
			)
			actual := mustQueryNames(req)
			So(actual, ShouldResemble, []string{"invocations/inv1/artifacts/a"})
		})

		Convey(`Test ID regexp`, func() {
			testutil.MustApply(ctx,
				insInvArt("inv1", "a", nil),
				insTRArt("inv1", "t00", "r", "a", nil),
				insTRArt("inv1", "t10", "r", "a", nil),
				insTRArt("inv1", "t11", "r", "a", nil),
				insTRArt("inv1", "t20", "r", "a", nil),
			)
			req.TestResultPredicate.TestIdRegexp = "t1."
			actual := mustQueryNames(req)
			So(actual, ShouldResemble, []string{
				"invocations/inv1/artifacts/a",
				"invocations/inv1/tests/t10/results/r/artifacts/a",
				"invocations/inv1/tests/t11/results/r/artifacts/a",
			})
		})

		Convey(`Follow edges`, func() {
			testutil.MustApply(ctx,
				insInvArt("inv1", "a0", nil),
				insInvArt("inv1", "a1", nil),
				insTRArt("inv1", "t", "r", "a0", nil),
				insTRArt("inv1", "t", "r", "a1", nil),
			)

			Convey(`Unspecified`, func() {
				actual := mustQueryNames(req)
				So(actual, ShouldResemble, []string{
					"invocations/inv1/artifacts/a0",
					"invocations/inv1/artifacts/a1",
					"invocations/inv1/tests/t/results/r/artifacts/a0",
					"invocations/inv1/tests/t/results/r/artifacts/a1",
				})
			})

			Convey(`Only invocations`, func() {
				req.FollowEdges = &pb.QueryArtifactsRequest_EdgeTypeSet{
					IncludedInvocations: true,
				}
				actual := mustQueryNames(req)
				So(actual, ShouldResemble, []string{
					"invocations/inv1/artifacts/a0",
					"invocations/inv1/artifacts/a1",
				})

				Convey(`Test result predicate is ignored`, func() {
					req.TestResultPredicate.TestIdRegexp = "t."
					actual := mustQueryNames(req)
					So(actual, ShouldResemble, []string{
						"invocations/inv1/artifacts/a0",
						"invocations/inv1/artifacts/a1",
					})
				})
			})

			Convey(`Only test results`, func() {
				req.FollowEdges = &pb.QueryArtifactsRequest_EdgeTypeSet{
					TestResults: true,
				}
				actual := mustQueryNames(req)
				So(actual, ShouldResemble, []string{
					"invocations/inv1/tests/t/results/r/artifacts/a0",
					"invocations/inv1/tests/t/results/r/artifacts/a1",
				})
			})

			Convey(`Both`, func() {
				req.FollowEdges = &pb.QueryArtifactsRequest_EdgeTypeSet{
					IncludedInvocations: true,
					TestResults:         true,
				}
				actual := mustQueryNames(req)
				So(actual, ShouldResemble, []string{
					"invocations/inv1/artifacts/a0",
					"invocations/inv1/artifacts/a1",
					"invocations/inv1/tests/t/results/r/artifacts/a0",
					"invocations/inv1/tests/t/results/r/artifacts/a1",
				})
			})
		})

		Convey(`Artifacts of interesting test results`, func() {
			testutil.MustApply(ctx,
				insInvArt("inv1", "a", nil),
				insTRArt("inv1", "t0", "0", "a", nil),
				insTRArt("inv1", "t1", "0", "a", nil),
				insTRArt("inv1", "t1", "1", "a", nil),
				insTRArt("inv1", "t1", "1", "b", nil),
				insTRArt("inv1", "t2", "0", "a", nil),
			)
			testutil.MustApply(ctx, testutil.CombineMutations(
				insTRs("inv1", "t0", nil, pb.TestStatus_PASS),
				insTRs("inv1", "t1", nil, pb.TestStatus_PASS, pb.TestStatus_FAIL),
				insTRs("inv1", "t2", nil, pb.TestStatus_FAIL),
			)...)

			req.TestResultPredicate.Expectancy = pb.TestResultPredicate_VARIANTS_WITH_UNEXPECTED_RESULTS
			actual := mustQueryNames(req)
			So(actual, ShouldResemble, []string{
				"invocations/inv1/artifacts/a",
				"invocations/inv1/tests/t1/results/0/artifacts/a",
				"invocations/inv1/tests/t1/results/1/artifacts/a",
				"invocations/inv1/tests/t1/results/1/artifacts/b",
				"invocations/inv1/tests/t2/results/0/artifacts/a",
			})

			Convey(`Without invocation artifacts`, func() {
				req.FollowEdges = &pb.QueryArtifactsRequest_EdgeTypeSet{TestResults: true}
				actual := mustQueryNames(req)
				So(actual, ShouldNotContain, "invocations/inv1/artifacts/a")
			})
		})

		Convey(`Variant equals`, func() {
			testutil.MustApply(ctx,
				insInvArt("inv1", "a", nil),
				insTRArt("inv1", "t0", "0", "a", nil),
				insTRArt("inv1", "t1", "0", "a", nil),
				insTRArt("inv1", "t1", "0", "b", nil),
				insTRArt("inv1", "t2", "0", "a", nil),
			)
			v1 := pbutil.Variant("k", "1")
			v2 := pbutil.Variant("k", "2")
			testutil.MustApply(ctx, testutil.CombineMutations(
				insTRs("inv1", "t0", v1, pb.TestStatus_PASS),
				insTRs("inv1", "t1", v2, pb.TestStatus_PASS),
				insTRs("inv1", "t2", v1, pb.TestStatus_PASS),
			)...)

			req.TestResultPredicate.Variant = &pb.VariantPredicate{
				Predicate: &pb.VariantPredicate_Equals{Equals: v2},
			}
			So(mustQueryNames(req), ShouldResemble, []string{
				"invocations/inv1/artifacts/a",
				"invocations/inv1/tests/t1/results/0/artifacts/a",
				"invocations/inv1/tests/t1/results/0/artifacts/b",
			})

			Convey(`Without invocation artifacts`, func() {
				req.FollowEdges = &pb.QueryArtifactsRequest_EdgeTypeSet{TestResults: true}
				actual := mustQueryNames(req)
				So(actual, ShouldNotContain, "invocations/inv1/artifacts/a")
			})
		})

		Convey(`Variant contains`, func() {
			testutil.MustApply(ctx,
				insInvArt("inv1", "a", nil),
				insTRArt("inv1", "t0", "0", "a", nil),
				insTRArt("inv1", "t1", "0", "a", nil),
				insTRArt("inv1", "t1", "0", "b", nil),
				insTRArt("inv1", "t2", "0", "a", nil),
			)
			v00 := pbutil.Variant("k0", "0")
			v01 := pbutil.Variant("k0", "0", "k1", "1")
			v10 := pbutil.Variant("k0", "1")
			testutil.MustApply(ctx, testutil.CombineMutations(
				insTRs("inv1", "t0", v00, pb.TestStatus_PASS),
				insTRs("inv1", "t1", v01, pb.TestStatus_PASS),
				insTRs("inv1", "t2", v10, pb.TestStatus_PASS),
			)...)

			req.TestResultPredicate.Variant = &pb.VariantPredicate{
				Predicate: &pb.VariantPredicate_Contains{Contains: v00},
			}
			So(mustQueryNames(req), ShouldResemble, []string{
				"invocations/inv1/artifacts/a",
				"invocations/inv1/tests/t0/results/0/artifacts/a",
				"invocations/inv1/tests/t1/results/0/artifacts/a",
				"invocations/inv1/tests/t1/results/0/artifacts/b",
			})

			Convey(`Without invocation artifacts`, func() {
				req.FollowEdges = &pb.QueryArtifactsRequest_EdgeTypeSet{TestResults: true}
				actual := mustQueryNames(req)
				So(actual, ShouldNotContain, "invocations/inv1/artifacts/a")
			})
		})

		Convey(`Paging`, func() {
			testutil.MustApply(ctx,
				insInvArt("inv1", "a0", nil),
				insInvArt("inv1", "a1", nil),
				insInvArt("inv1", "a2", nil),
				insInvArt("inv1", "a3", nil),
				insInvArt("inv1", "a4", nil),
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
