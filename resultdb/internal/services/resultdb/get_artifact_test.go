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
	"context"
	"net/url"
	"testing"
	"time"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/clock"

	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateGetArtifactRequest(t *testing.T) {
	t.Parallel()
	Convey(`ValidateGetArtifactRequest`, t, func() {
		Convey(`Valid`, func() {
			req := &pb.GetArtifactRequest{Name: "invocations/inv/artifacts/a"}
			So(validateGetArtifactRequest(req), ShouldBeNil)
		})

		Convey(`Invalid name`, func() {
			req := &pb.GetArtifactRequest{}
			So(validateGetArtifactRequest(req), ShouldErrLike, "unspecified")
		})
	})
}

func AssertFetchURLCorrectness(ctx context.Context, a *pb.Artifact) {
	fetchURL, err := url.Parse(a.FetchUrl)
	So(err, ShouldBeNil)
	So(fetchURL.Query().Get("token"), ShouldNotBeEmpty)
	So(fetchURL.RawPath, ShouldEqual, "/"+a.Name)

	So(a.FetchUrlExpiration, ShouldNotBeNil)
	So(pbutil.MustTimestamp(a.FetchUrlExpiration), ShouldHappenWithin, 10*time.Second, clock.Now(ctx))
}

func TestGetArtifact(t *testing.T) {
	Convey(`GetArtifact`, t, func() {
		ctx := testutil.SpannerTestContext(t)
		srv := newTestResultDBService()

		Convey(`Exists`, func() {
			// Insert a Artifact.
			testutil.MustApply(ctx,
				testutil.InsertInvocation("inv", pb.Invocation_ACTIVE, nil),
				testutil.InsertArtifact("inv", "", "a", nil),
			)
			const name = "invocations/inv/artifacts/a"
			req := &pb.GetArtifactRequest{Name: name}
			art, err := srv.GetArtifact(ctx, req)
			So(err, ShouldBeNil)
			So(art.Name, ShouldEqual, name)
			So(art.ArtifactId, ShouldEqual, "a")
			So(art.FetchUrl, ShouldEqual, "https://signed-url.example.com/invocations/inv/artifacts/a")
		})

		Convey(`Does not exist`, func() {
			req := &pb.GetArtifactRequest{Name: "invocations/inv/artifacts/a"}
			_, err := srv.GetArtifact(ctx, req)
			So(err, ShouldHaveAppStatus, codes.NotFound, "invocations/inv/artifacts/a not found")
		})
	})
}
