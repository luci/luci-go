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

	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
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
		ctx := testutil.SpannerTestContext(t)

		testutil.MustApply(ctx, insert.Invocation("inv1", pb.Invocation_ACTIVE, nil))
		req := &pb.ListArtifactsRequest{
			Parent:   "invocations/inv1",
			PageSize: 100,
		}

		srv := newTestResultDBService()

		mustFetch := func(req *pb.ListArtifactsRequest) (arts []*pb.Artifact, token string) {
			res, err := srv.ListArtifacts(ctx, req)
			So(err, ShouldBeNil)
			return res.Artifacts, res.NextPageToken
		}

		mustFetchNames := func(req *pb.ListArtifactsRequest) []string {
			arts, _ := mustFetch(req)
			names := make([]string, len(arts))
			for i, a := range arts {
				names[i] = a.Name
			}
			return names
		}

		Convey(`With both invocation and test result artifacts`, func() {
			testutil.MustApply(ctx,
				insert.Artifact("inv1", "", "a", nil),
				span.InsertMap("Artifacts", map[string]interface{}{
					"InvocationId": span.InvocationID("inv1"),
					"ParentID":     "tr/t t/r",
					"ArtifactId":   "a",
				}),
			)

			Convey(`Reads only invocation artifacts`, func() {
				req.Parent = "invocations/inv1"
				actual := mustFetchNames(req)
				So(actual, ShouldResemble, []string{
					"invocations/inv1/artifacts/a",
				})
			})

			Convey(`Reads only test result artifacts`, func() {
				req.Parent = "invocations/inv1/tests/t%20t/results/r"
				actual := mustFetchNames(req)
				So(actual, ShouldResemble, []string{
					"invocations/inv1/tests/t%20t/results/r/artifacts/a",
				})
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
