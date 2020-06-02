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

	durpb "github.com/golang/protobuf/ptypes/duration"

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

		insInv := testutil.InsertInvocation

		testutil.MustApply(ctx, insInv("inv1", pb.Invocation_ACTIVE, nil))
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

		Convey(`Reads both invocation and test result artifacts`, func() {
			testutil.MustApply(ctx,
				testutil.InsertArtifact("inv1", "", "a", nil),
				testutil.InsertArtifact("inv1", "tr/t t/r", "a", nil),
			)
			actual := mustFetchNames(req)
			So(actual, ShouldResemble, []string{
				"invocations/inv1/artifacts/a",
				"invocations/inv1/tests/t%20t/results/r/artifacts/a",
			})
		})

		Convey(`Fetch URL`, func() {
			testutil.MustApply(ctx,
				testutil.InsertArtifact("inv1", "", "a", nil),
			)
			actual, _ := mustFetch(req)
			So(actual, ShouldHaveLength, 1)
			So(actual[0].FetchUrl, ShouldEqual, "https://signed-url.example.com/invocations/inv1/artifacts/a")
		})
	})
}
