// Copyright 2021 The LUCI Authors.
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

package recorder

import (
	"fmt"
	"testing"

	"go.chromium.org/luci/resultdb/internal/artifacts"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestParseBatchCreateArtifactsRequest(t *testing.T) {
	newArtReq := func(parent, artID, contentType string) *pb.CreateArtifactRequest {
		return &pb.CreateArtifactRequest{
			Parent:   parent,
			Artifact: &pb.Artifact{ArtifactId: artID, ContentType: contentType},
		}
	}

	Convey("parseBatchCreateArtifactsRequest", t, func() {
		bReq := &pb.BatchCreateArtifactsRequest{}
		invArt := newArtReq("invocations/inv1", "art1", "text/html")
		trArt := newArtReq("invocations/inv1/tests/t1/results/r1", "art2", "image/png")

		Convey("successes", func() {
			bReq.Requests = append(bReq.Requests, invArt)
			bReq.Requests = append(bReq.Requests, trArt)
			arts, err := parseBatchCreateArtifactsRequest(bReq)
			So(err, ShouldBeNil)
			So(len(arts), ShouldEqual, len(bReq.Requests))

			// invocation-level artifact
			So(arts[0].invID, ShouldEqual, invocations.ID("inv1"))
			So(arts[0].artifactID, ShouldEqual, "art1")
			So(arts[0].localParentID, ShouldEqual, artifacts.ParentID("", ""))
			So(arts[0].contentType, ShouldEqual, "text/html")

			// test-result-level artifact
			So(arts[1].invID, ShouldEqual, invocations.ID("inv1"))
			So(arts[1].artifactID, ShouldEqual, "art2")
			So(arts[1].localParentID, ShouldEqual, artifacts.ParentID("t1", "r1"))
			So(arts[1].contentType, ShouldEqual, "image/png")
		})

		Convey("ignores size_bytes", func() {
			bReq.Requests = append(bReq.Requests, trArt)
			trArt.Artifact.SizeBytes = 123
			trArt.Artifact.Contents = make([]byte, 10249)
			arts, err := parseBatchCreateArtifactsRequest(bReq)
			So(err, ShouldBeNil)
			So(arts[0].size, ShouldEqual, 10249)
		})

		Convey("fails", func() {
			Convey("if sum() of artifact.Contents is too big", func() {
				for i := 0; i < 11; i++ {
					req := newArtReq("invocations/inv1", fmt.Sprintf("art%d", i), "text/html")
					req.Artifact.Contents = make([]byte, 1024*1024)
					bReq.Requests = append(bReq.Requests, req)
				}
				_, err := parseBatchCreateArtifactsRequest(bReq)
				So(err, ShouldErrLike, "the total size of artifact contents exceeded")
			})

			Convey("if more than one invocations", func() {
				bReq.Requests = append(bReq.Requests, newArtReq("invocations/inv1", "art1", "text/html"))
				bReq.Requests = append(bReq.Requests, newArtReq("invocations/inv2", "art1", "text/html"))
				_, err := parseBatchCreateArtifactsRequest(bReq)
				So(err, ShouldErrLike, `the invocation ID("inv2") is different to the previous`)
			})
		})
	})
}

func TestBatchCreateArtifacts(t *testing.T) {
	Convey("TestBatchCreateArtifacts", t, func() {
		ctx := testutil.SpannerTestContext(t)
		recorder := newTestRecorderServer()
		token, err := generateInvocationToken(ctx, "inv")
		So(err, ShouldBeNil)

		bReq := &pb.BatchCreateArtifactsRequest{
			Requests: []*pb.CreateArtifactRequest{
				&pb.CreateArtifactRequest{
					Parent:   "invocations/inv",
					Artifact: &pb.Artifact{ArtifactId: "art1"},
				},
			},
		}

		Convey("works", func() {
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(UpdateTokenMetadataKey, token))
			testutil.MustApply(ctx, insert.Invocation("inv", pb.Invocation_ACTIVE, nil))
			_, err = recorder.BatchCreateArtifacts(ctx, bReq)
			So(err, ShouldBeNil)
		})

		Convey("missing update-token", func() {
			testutil.MustApply(ctx, insert.Invocation("inv", pb.Invocation_ACTIVE, nil))
			_, err = recorder.BatchCreateArtifacts(ctx, bReq)
			So(err, ShouldHaveAppStatus, codes.Unauthenticated, `missing update-token`)
		})

		Convey("wrong update-token", func() {
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(UpdateTokenMetadataKey, "wrong token"))
			testutil.MustApply(ctx, insert.Invocation("inv", pb.Invocation_ACTIVE, nil))
			_, err = recorder.BatchCreateArtifacts(ctx, bReq)
			So(err, ShouldHaveAppStatus, codes.PermissionDenied, `invalid update token`)
		})

		Convey("finalized invocation", func() {
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(UpdateTokenMetadataKey, token))
			testutil.MustApply(ctx, insert.Invocation("inv", pb.Invocation_FINALIZED, nil))
			_, err = recorder.BatchCreateArtifacts(ctx, bReq)
			So(err, ShouldHaveAppStatus, codes.PermissionDenied, `inv is not active`)
		})
	})
}
