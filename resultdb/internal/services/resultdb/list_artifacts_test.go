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
	"google.golang.org/grpc/codes"

	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
	. "go.chromium.org/luci/resultdb/internal/testutil"
)

func TestValidateListArtifactsRequest(t *testing.T) {
	t.Parallel()
	Convey(`TestValidateListArtifactsRequest`, t, func() {
		Convey(`Valid, invocation level`, func() {
			err := validateListArtifactsRequest(&pb.ListArtifactsRequest{
				Parent:   "invocations/x",
				PageSize: 50,
			})
			So(err, ShouldBeNil)
		})

		Convey(`Valid, test result level`, func() {
			err := validateListArtifactsRequest(&pb.ListArtifactsRequest{
				Parent:   "invocations/x/tests/t%20t/results/r",
				PageSize: 50,
			})
			So(err, ShouldBeNil)
		})

		Convey(`Invalid parent`, func() {
			err := validateListArtifactsRequest(&pb.ListArtifactsRequest{
				Parent: "x",
			})
			So(err, ShouldErrLike, `parent: neither valid invocation name nor valid test result name`)
		})

		Convey(`Invalid page size`, func() {
			err := validateListArtifactsRequest(&pb.ListArtifactsRequest{
				Parent:   "invocations/x",
				PageSize: -1,
			})
			So(err, ShouldErrLike, `page_size: negative`)
		})
	})
}

func TestListArtifacts(t *testing.T) {
	Convey(`ListArtifacts`, t, func() {
		ctx := SpannerTestContext(t)

		MustApply(ctx, InsertInvocation("inv1", pb.Invocation_ACTIVE, nil))
		req := &pb.ListArtifactsRequest{
			Parent:   "invocations/inv1",
			PageSize: 100,
		}

		srv := newTestResultDBService()

		mustList := func(req *pb.ListArtifactsRequest) (arts []*pb.Artifact, token string) {
			res, err := srv.ListArtifacts(ctx, req)
			So(err, ShouldBeNil)
			return res.Artifacts, res.NextPageToken
		}

		mustListNames := func(req *pb.ListArtifactsRequest) []string {
			arts, _ := mustList(req)
			names := make([]string, len(arts))
			for i, a := range arts {
				names[i] = a.Name
			}
			return names
		}

		Convey(`Reads fields correctly`, func() {
			MustApply(ctx,
				InsertInvocationArtifact("inv1", "a", map[string]interface{}{
					"ContentType": "text/plain",
					"Size":        54,
				}),
			)
			actual, _ := mustList(req)
			So(actual, ShouldHaveLength, 1)
			So(actual[0].ContentType, ShouldEqual, "text/plain")
			So(actual[0].SizeBytes, ShouldEqual, 54)
		})

		Convey(`With both invocation and test result artifacts`, func() {
			MustApply(ctx,
				InsertInvocationArtifact("inv1", "a", nil),
				InsertTestResultArtifact("inv1", "t t", "r", "a", nil),
			)

			Convey(`Reads only invocation artifacts`, func() {
				req.Parent = "invocations/inv1"
				actual := mustListNames(req)
				So(actual, ShouldResemble, []string{
					"invocations/inv1/artifacts/a",
				})
			})

			Convey(`Reads only test result artifacts`, func() {
				req.Parent = "invocations/inv1/tests/t%20t/results/r"
				actual := mustListNames(req)
				So(actual, ShouldResemble, []string{
					"invocations/inv1/tests/t%20t/results/r/artifacts/a",
				})
			})
		})

		Convey(`Does not fetch artifacts of other invocations`, func() {
			MustApply(ctx,
				InsertInvocation("inv0", pb.Invocation_ACTIVE, nil),
				InsertInvocation("inv2", pb.Invocation_ACTIVE, nil),
				InsertInvocationArtifact("inv0", "a", nil),
				InsertInvocationArtifact("inv1", "a", nil),
				InsertInvocationArtifact("inv2", "a", nil),
			)
			actual := mustListNames(req)
			So(actual, ShouldResemble, []string{"invocations/inv1/artifacts/a"})
		})

		Convey(`Paging`, func() {
			MustApply(ctx,
				InsertInvocationArtifact("inv1", "a0", nil),
				InsertInvocationArtifact("inv1", "a1", nil),
				InsertInvocationArtifact("inv1", "a2", nil),
				InsertInvocationArtifact("inv1", "a3", nil),
				InsertInvocationArtifact("inv1", "a4", nil),
			)

			mustReadPage := func(pageToken string, pageSize int, expectedArtifactIDs ...string) string {
				req2 := proto.Clone(req).(*pb.ListArtifactsRequest)
				req2.PageToken = pageToken
				req2.PageSize = int32(pageSize)
				arts, token := mustList(req2)

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
				_, err := srv.ListArtifacts(ctx, req)
				So(err, ShouldHaveAppStatus, codes.InvalidArgument, "invalid page_token")
			})
		})
	})
}
