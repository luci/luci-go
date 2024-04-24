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
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	spb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/resultdb/internal/artifacts"
	"go.chromium.org/luci/resultdb/internal/config"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	bqpb "go.chromium.org/luci/resultdb/proto/bq"
	configpb "go.chromium.org/luci/resultdb/proto/config"
	pb "go.chromium.org/luci/resultdb/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

// fakeRBEClient mocks BatchUpdateBlobs.
type fakeRBEClient struct {
	repb.ContentAddressableStorageClient
	req  *repb.BatchUpdateBlobsRequest
	resp *repb.BatchUpdateBlobsResponse
	err  error
}

func (c *fakeRBEClient) BatchUpdateBlobs(ctx context.Context, in *repb.BatchUpdateBlobsRequest, opts ...grpc.CallOption) (*repb.BatchUpdateBlobsResponse, error) {
	c.req = in
	return c.resp, c.err
}

func (c *fakeRBEClient) mockResp(err error, cds ...codes.Code) {
	c.err = err
	c.resp = &repb.BatchUpdateBlobsResponse{}
	for _, cd := range cds {
		c.resp.Responses = append(c.resp.Responses, &repb.BatchUpdateBlobsResponse_Response{
			Status: &spb.Status{Code: int32(cd)},
		})
	}
}

type fakeBQClient struct {
	Rows  []*bqpb.TextArtifactRow
	Error error
}

func (c *fakeBQClient) InsertArtifactRows(ctx context.Context, rows []*bqpb.TextArtifactRow) error {
	if c.Error != nil {
		return c.Error
	}
	c.Rows = append(c.Rows, rows...)
	return nil
}

func TestNewArtifactCreationRequestsFromProto(t *testing.T) {
	newArtReq := func(parent, artID, contentType string) *pb.CreateArtifactRequest {
		return &pb.CreateArtifactRequest{
			Parent:   parent,
			Artifact: &pb.Artifact{ArtifactId: artID, ContentType: contentType},
		}
	}

	Convey("newArtifactCreationRequestsFromProto", t, func() {
		bReq := &pb.BatchCreateArtifactsRequest{}
		invArt := newArtReq("invocations/inv1", "art1", "text/html")
		trArt := newArtReq("invocations/inv1/tests/t1/results/r1", "art2", "image/png")

		Convey("successes", func() {
			bReq.Requests = append(bReq.Requests, invArt)
			bReq.Requests = append(bReq.Requests, trArt)
			invID, arts, err := parseBatchCreateArtifactsRequest(bReq)
			So(err, ShouldBeNil)
			So(invID, ShouldEqual, invocations.ID("inv1"))
			So(len(arts), ShouldEqual, len(bReq.Requests))

			// invocation-level artifact
			So(arts[0].artifactID, ShouldEqual, "art1")
			So(arts[0].parentID(), ShouldEqual, artifacts.ParentID("", ""))
			So(arts[0].contentType, ShouldEqual, "text/html")

			// test-result-level artifact
			So(arts[1].artifactID, ShouldEqual, "art2")
			So(arts[1].parentID(), ShouldEqual, artifacts.ParentID("t1", "r1"))
			So(arts[1].contentType, ShouldEqual, "image/png")
		})

		Convey("mismatched size_bytes", func() {
			bReq.Requests = append(bReq.Requests, trArt)
			trArt.Artifact.SizeBytes = 123
			trArt.Artifact.Contents = make([]byte, 10249)
			_, _, err := parseBatchCreateArtifactsRequest(bReq)
			So(err, ShouldErrLike, `sizeBytes and contents are specified but don't match`)
		})

		Convey("ignored size_bytes", func() {
			bReq.Requests = append(bReq.Requests, trArt)
			trArt.Artifact.SizeBytes = 0
			trArt.Artifact.Contents = make([]byte, 10249)
			_, arts, err := parseBatchCreateArtifactsRequest(bReq)
			So(err, ShouldBeNil)
			So(arts[0].size, ShouldEqual, 10249)
		})

		Convey("contents and gcs_uri both specified", func() {
			bReq.Requests = append(bReq.Requests, trArt)
			trArt.Artifact.SizeBytes = 0
			trArt.Artifact.Contents = make([]byte, 10249)
			trArt.Artifact.GcsUri = "gs://testbucket/testfile"
			_, _, err := parseBatchCreateArtifactsRequest(bReq)
			So(err, ShouldErrLike, `only one of contents and gcs_uri can be given`)
		})

		Convey("sum() of artifact.Contents is too big", func() {
			for i := 0; i < 11; i++ {
				req := newArtReq("invocations/inv1", fmt.Sprintf("art%d", i), "text/html")
				req.Artifact.Contents = make([]byte, 1024*1024)
				bReq.Requests = append(bReq.Requests, req)
			}
			_, _, err := parseBatchCreateArtifactsRequest(bReq)
			So(err, ShouldErrLike, "the total size of artifact contents exceeded")
		})

		Convey("if more than one invocations", func() {
			bReq.Requests = append(bReq.Requests, newArtReq("invocations/inv1", "art1", "text/html"))
			bReq.Requests = append(bReq.Requests, newArtReq("invocations/inv2", "art1", "text/html"))
			_, _, err := parseBatchCreateArtifactsRequest(bReq)
			So(err, ShouldErrLike, `only one invocation is allowed: "inv1", "inv2"`)
		})
	})
}

func TestBatchCreateArtifacts(t *testing.T) {
	// metric field values for Artifact table
	artMFVs := []any{string(spanutil.Artifacts), string(spanutil.Inserted), insert.TestRealm}

	Convey("TestBatchCreateArtifacts", t, func() {
		ctx := testutil.SpannerTestContext(t)
		ctx = testutil.TestProjectConfigContext(ctx, "testproject", "user:test@test.com", "testbucket")
		token, err := generateInvocationToken(ctx, "inv")
		So(err, ShouldBeNil)
		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, token))
		ctx, _ = tsmon.WithDummyInMemory(ctx)
		store := tsmon.Store(ctx)
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: "user:test@test.com",
		})

		err = config.SetServiceConfig(ctx, &configpb.Config{
			BqArtifactExportConfig: &configpb.BqArtifactExportConfig{
				Enabled:       true,
				ExportPercent: 100,
			},
		})
		So(err, ShouldBeNil)

		casClient := &fakeRBEClient{}
		bqClient := &fakeBQClient{}
		recorder := newTestRecorderServerWithClients(casClient, bqClient)
		bReq := &pb.BatchCreateArtifactsRequest{}

		appendArtReq := func(aID, content, cType, parent string, testStatus pb.TestStatus) {
			bReq.Requests = append(bReq.Requests, &pb.CreateArtifactRequest{
				Parent: parent,
				Artifact: &pb.Artifact{
					ArtifactId: aID, Contents: []byte(content), ContentType: cType, TestStatus: testStatus,
				},
			})
		}
		appendGcsArtReq := func(aID string, cSize int64, cType string, gcsURI string) {
			bReq.Requests = append(bReq.Requests, &pb.CreateArtifactRequest{
				Parent: "invocations/inv",
				Artifact: &pb.Artifact{
					ArtifactId: aID, SizeBytes: cSize, ContentType: cType, GcsUri: gcsURI,
				},
			})
		}

		fetchState := func(parentID, aID string) (size int64, hash string, contentType string, gcsURI string) {
			testutil.MustReadRow(
				ctx, "Artifacts", invocations.ID("inv").Key(parentID, aID),
				map[string]any{
					"Size":        &size,
					"RBECASHash":  &hash,
					"ContentType": &contentType,
					"GcsURI":      &gcsURI,
				},
			)
			return
		}
		compHash := func(content string) string {
			h := sha256.Sum256([]byte(content))
			return hex.EncodeToString(h[:])
		}

		Convey("GCS reference isAllowed", func() {
			Convey("reference is allowed", func() {
				testutil.SetGCSAllowedBuckets(ctx, "testproject", "user:test@test.com", "testbucket")

				testutil.MustApply(ctx, insert.Invocation("inv", pb.Invocation_ACTIVE, nil))
				appendGcsArtReq("art1", 0, "text/plain", "gs://testbucket/art1")

				_, err := recorder.BatchCreateArtifacts(ctx, bReq)
				So(err, ShouldBeNil)
			})
			Convey("project not configured", func() {
				testutil.SetGCSAllowedBuckets(ctx, "otherproject", "user:test@test.com", "testbucket")

				testutil.MustApply(ctx, insert.Invocation("inv", pb.Invocation_ACTIVE, nil))
				appendGcsArtReq("art1", 0, "text/plain", "gs://testbucket/art1")

				_, err := recorder.BatchCreateArtifacts(ctx, bReq)
				So(err, ShouldBeRPCPermissionDenied, "testproject")
			})
			Convey("user not configured", func() {
				testutil.SetGCSAllowedBuckets(ctx, "testproject", "user:test@test.com", "testbucket")
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: "user:other@test.com",
				})
				testutil.MustApply(ctx, insert.Invocation("inv", pb.Invocation_ACTIVE, nil))
				appendGcsArtReq("art1", 0, "text/plain", "gs://testbucket/art1")

				_, err := recorder.BatchCreateArtifacts(ctx, bReq)
				So(err, ShouldBeRPCPermissionDenied, "user:other@test.com")
			})
			Convey("bucket not listed", func() {
				testutil.SetGCSAllowedBuckets(ctx, "testproject", "user:test@test.com", "otherbucket")

				testutil.MustApply(ctx, insert.Invocation("inv", pb.Invocation_ACTIVE, nil))
				appendGcsArtReq("art1", 0, "text/plain", "gs://testbucket/art1")

				_, err := recorder.BatchCreateArtifacts(ctx, bReq)
				So(err, ShouldBeRPCPermissionDenied, "testbucket")
			})
		})
		Convey("works", func() {
			testutil.MustApply(ctx, insert.Invocation("inv", pb.Invocation_ACTIVE, map[string]any{
				"CreateTime": time.Unix(10000, 0),
			}))

			appendArtReq("art1", "c0ntent", "text/plain", "invocations/inv", pb.TestStatus_STATUS_UNSPECIFIED)
			appendArtReq("art2", "c1ntent", "text/richtext", "invocations/inv/tests/test_id/results/0", pb.TestStatus_PASS)
			appendGcsArtReq("art3", 0, "text/plain", "gs://testbucket/art3")
			appendGcsArtReq("art4", 500, "text/richtext", "gs://testbucket/art4")
			appendArtReq("art5", "c5ntent", "text/richtext", "invocations/inv/tests/test_id_1/results/0", pb.TestStatus_FAIL)

			casClient.mockResp(nil, codes.OK, codes.OK)

			resp, err := recorder.BatchCreateArtifacts(ctx, bReq)
			So(err, ShouldBeNil)
			So(resp, ShouldResemble, &pb.BatchCreateArtifactsResponse{
				Artifacts: []*pb.Artifact{
					{
						Name:        "invocations/inv/artifacts/art1",
						ArtifactId:  "art1",
						ContentType: "text/plain",
						SizeBytes:   7,
					},
					{
						Name:        "invocations/inv/tests/test_id/results/0/artifacts/art2",
						ArtifactId:  "art2",
						ContentType: "text/richtext",
						SizeBytes:   7,
					},
					{
						Name:        "invocations/inv/artifacts/art3",
						ArtifactId:  "art3",
						ContentType: "text/plain",
						SizeBytes:   0,
					},
					{
						Name:        "invocations/inv/artifacts/art4",
						ArtifactId:  "art4",
						ContentType: "text/richtext",
						SizeBytes:   500,
					},
					{
						Name:        "invocations/inv/tests/test_id_1/results/0/artifacts/art5",
						ArtifactId:  "art5",
						ContentType: "text/richtext",
						SizeBytes:   7,
					},
				},
			})
			// verify the RBECAS reqs
			So(casClient.req, ShouldResemble, &repb.BatchUpdateBlobsRequest{
				InstanceName: "",
				Requests: []*repb.BatchUpdateBlobsRequest_Request{
					{
						Digest: &repb.Digest{
							Hash:      compHash("c0ntent"),
							SizeBytes: int64(len("c0ntent")),
						},
						Data: []byte("c0ntent"),
					},
					{
						Digest: &repb.Digest{
							Hash:      compHash("c1ntent"),
							SizeBytes: int64(len("c1ntent")),
						},
						Data: []byte("c1ntent"),
					},
					{
						Digest: &repb.Digest{
							Hash:      compHash("c5ntent"),
							SizeBytes: int64(len("c5ntent")),
						},
						Data: []byte("c5ntent"),
					},
				},
			})
			// verify the Spanner states
			size, hash, cType, gcsURI := fetchState("", "art1")
			So(size, ShouldEqual, int64(len("c0ntent")))
			So(hash, ShouldEqual, artifacts.AddHashPrefix(compHash("c0ntent")))
			So(cType, ShouldEqual, "text/plain")
			So(gcsURI, ShouldEqual, "")

			size, hash, cType, gcsURI = fetchState("tr/test_id/0", "art2")
			So(size, ShouldEqual, int64(len("c1ntent")))
			So(hash, ShouldEqual, artifacts.AddHashPrefix(compHash("c1ntent")))
			So(cType, ShouldEqual, "text/richtext")
			So(gcsURI, ShouldEqual, "")

			size, hash, cType, gcsURI = fetchState("tr/test_id_1/0", "art5")
			So(size, ShouldEqual, int64(len("c5ntent")))
			So(hash, ShouldEqual, artifacts.AddHashPrefix(compHash("c5ntent")))
			So(cType, ShouldEqual, "text/richtext")
			So(gcsURI, ShouldEqual, "")

			size, hash, cType, gcsURI = fetchState("", "art3")
			So(size, ShouldEqual, 0)
			So(hash, ShouldEqual, "")
			So(cType, ShouldEqual, "text/plain")
			So(gcsURI, ShouldEqual, "gs://testbucket/art3")

			size, hash, cType, gcsURI = fetchState("", "art4")
			So(size, ShouldEqual, 500)
			So(hash, ShouldEqual, "")
			So(cType, ShouldEqual, "text/richtext")
			So(gcsURI, ShouldEqual, "gs://testbucket/art4")

			// RowCount metric should be increased by 5.
			So(store.Get(ctx, spanutil.RowCounter, time.Time{}, artMFVs), ShouldEqual, 5)

			// Verify the bigquery rows.
			So(len(bqClient.Rows), ShouldEqual, 3)
			So(bqClient.Rows, ShouldResembleProto, []*bqpb.TextArtifactRow{
				{
					Project:             "testproject",
					Realm:               "testrealm",
					InvocationId:        "inv",
					ArtifactId:          "art1",
					ContentType:         "text/plain",
					Content:             "c0ntent",
					NumShards:           1,
					ShardId:             0,
					ShardContentSize:    7,
					ArtifactContentSize: 7,
					PartitionTime:       timestamppb.New(time.Unix(10000, 0)),
					ArtifactShard:       "art1:0",
					TestStatus:          "",
				},
				{
					Project:             "testproject",
					Realm:               "testrealm",
					InvocationId:        "inv",
					ArtifactId:          "art2",
					ContentType:         "text/richtext",
					Content:             "c1ntent",
					NumShards:           1,
					ShardId:             0,
					ShardContentSize:    7,
					ArtifactContentSize: 7,
					PartitionTime:       timestamppb.New(time.Unix(10000, 0)),
					TestId:              "test_id",
					ResultId:            "0",
					ArtifactShard:       "art2:0",
					TestStatus:          "PASS",
				},
				{
					Project:             "testproject",
					Realm:               "testrealm",
					InvocationId:        "inv",
					ArtifactId:          "art5",
					ContentType:         "text/richtext",
					Content:             "c5ntent",
					NumShards:           1,
					ShardId:             0,
					ShardContentSize:    7,
					ArtifactContentSize: 7,
					PartitionTime:       timestamppb.New(time.Unix(10000, 0)),
					TestId:              "test_id_1",
					ResultId:            "0",
					ArtifactShard:       "art5:0",
					TestStatus:          "FAIL",
				},
			})
		})

		Convey("BatchUpdateBlobs fails", func() {
			testutil.MustApply(ctx, insert.Invocation("inv", pb.Invocation_ACTIVE, nil))
			appendArtReq("art1", "c0ntent", "text/plain", "invocations/inv", pb.TestStatus_STATUS_UNSPECIFIED)
			appendArtReq("art2", "c1ntent", "text/richtext", "invocations/inv", pb.TestStatus_STATUS_UNSPECIFIED)

			Convey("Partly", func() {
				casClient.mockResp(nil, codes.OK, codes.InvalidArgument)
				// Call the implementation without postlude so that we can see the
				// internal error, and don't just get "Internal server error" back.
				_, err := recorder.Service.BatchCreateArtifacts(ctx, bReq)
				So(err, ShouldErrLike, `artifact "invocations/inv/artifacts/art2": cas.BatchUpdateBlobs failed`)
			})

			Convey("Entirely", func() {
				// exceeded the maximum size limit is the only possible error that
				// can cause the entire request failed.
				casClient.mockResp(errors.New("err"), codes.OK, codes.OK)
				// Call the implementation without postlude so that we can see the
				// internal error, and don't just get "Internal server error" back.
				_, err := recorder.Service.BatchCreateArtifacts(ctx, bReq)
				So(err, ShouldErrLike, "cas.BatchUpdateBlobs failed")
			})

			// RowCount metric should have no changes from any of the above Convey()s.
			So(store.Get(ctx, spanutil.RowCounter, time.Time{}, artMFVs), ShouldBeNil)
		})

		Convey("Token", func() {
			appendArtReq("art1", "", "text/plain", "invocations/inv", pb.TestStatus_STATUS_UNSPECIFIED)
			testutil.MustApply(ctx, insert.Invocation("inv", pb.Invocation_ACTIVE, nil))

			Convey("Missing", func() {
				ctx = metadata.NewIncomingContext(ctx, metadata.Pairs())
				_, err = recorder.BatchCreateArtifacts(ctx, bReq)
				So(err, ShouldBeRPCUnauthenticated, `missing update-token`)
			})
			Convey("Wrong", func() {
				ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, "rong"))
				_, err = recorder.BatchCreateArtifacts(ctx, bReq)
				So(err, ShouldBeRPCPermissionDenied, `invalid update token`)
			})
		})

		Convey("Verify state", func() {
			casClient.mockResp(nil, codes.OK, codes.OK)

			Convey("Finalized invocation", func() {
				testutil.MustApply(ctx, insert.Invocation("inv", pb.Invocation_FINALIZED, nil))
				appendArtReq("art1", "c0ntent", "text/plain", "invocations/inv", pb.TestStatus_STATUS_UNSPECIFIED)
				_, err = recorder.BatchCreateArtifacts(ctx, bReq)
				So(err, ShouldBeRPCFailedPrecondition, `invocations/inv is not active`)
			})

			art := map[string]any{
				"InvocationId": invocations.ID("inv"),
				"ParentId":     "",
				"ArtifactId":   "art1",
				"RBECASHash":   artifacts.AddHashPrefix(compHash("c0ntent")),
				"Size":         len("c0ntent"),
				"ContentType":  "text/plain",
			}

			gcsArt := map[string]any{
				"InvocationId": invocations.ID("inv"),
				"ParentId":     "",
				"ArtifactId":   "art1",
				"ContentType":  "text/plain",
				"GcsURI":       "gs://testbucket/art1",
			}

			Convey("Same artifact exists", func() {
				testutil.MustApply(ctx,
					insert.Invocation("inv", pb.Invocation_ACTIVE, nil),
					spanutil.InsertMap("Artifacts", art),
				)
				appendArtReq("art1", "c0ntent", "text/plain", "invocations/inv", pb.TestStatus_STATUS_UNSPECIFIED)
				resp, err := recorder.BatchCreateArtifacts(ctx, bReq)
				So(err, ShouldBeNil)
				So(resp, ShouldResemble, &pb.BatchCreateArtifactsResponse{})
			})

			Convey("Different artifact exists", func() {
				testutil.SetGCSAllowedBuckets(ctx, "testproject", "user:test@test.com", "testbucket")
				testutil.MustApply(ctx,
					insert.Invocation("inv", pb.Invocation_ACTIVE, nil),
					spanutil.InsertMap("Artifacts", art),
				)

				appendArtReq("art1", "c0ntent", "text/plain", "invocations/inv", pb.TestStatus_STATUS_UNSPECIFIED)
				bReq.Requests[0].Artifact.Contents = []byte("loooong content")
				_, err := recorder.BatchCreateArtifacts(ctx, bReq)
				So(err, ShouldBeRPCAlreadyExists, "exists w/ different size")

				bReq.Requests[0].Artifact.Contents = []byte("c1ntent")
				_, err = recorder.BatchCreateArtifacts(ctx, bReq)
				So(err, ShouldBeRPCAlreadyExists, "exists w/ different hash")

				bReq.Requests[0].Artifact.Contents = []byte("")
				bReq.Requests[0].Artifact.GcsUri = "gs://testbucket/art1"
				_, err = recorder.BatchCreateArtifacts(ctx, bReq)
				So(err, ShouldBeRPCAlreadyExists, "exists w/ different storage scheme")
			})

			Convey("Different artifact exists GCS", func() {
				testutil.SetGCSAllowedBuckets(ctx, "testproject", "user:test@test.com", "testbucket")
				testutil.MustApply(ctx,
					insert.Invocation("inv", pb.Invocation_ACTIVE, nil),
					spanutil.InsertMap("Artifacts", gcsArt),
				)

				appendArtReq("art1", "c0ntent", "text/plain", "invocations/inv", pb.TestStatus_STATUS_UNSPECIFIED)
				_, err := recorder.BatchCreateArtifacts(ctx, bReq)
				So(err, ShouldBeRPCAlreadyExists, "exists w/ different storage scheme")

				bReq.Requests[0].Artifact.Contents = []byte("")
				bReq.Requests[0].Artifact.GcsUri = "gs://testbucket/art2"
				_, err = recorder.BatchCreateArtifacts(ctx, bReq)
				So(err, ShouldBeRPCAlreadyExists, "exists w/ different GCS URI")

				appendArtReq("art1", "c0ntent", "text/plain", "invocations/inv", pb.TestStatus_STATUS_UNSPECIFIED)
				bReq.Requests[0].Artifact.SizeBytes = 42
				_, err = recorder.BatchCreateArtifacts(ctx, bReq)
				So(err, ShouldBeRPCAlreadyExists, "exists w/ different size")
			})

			// RowCount metric should have no changes from any of the above Convey()s.
			So(store.Get(ctx, spanutil.RowCounter, time.Time{}, artMFVs), ShouldBeNil)
		})

		Convey("Too many requests", func() {
			bReq.Requests = make([]*pb.CreateArtifactRequest, 1000)
			_, err := recorder.BatchCreateArtifacts(ctx, bReq)
			So(err, ShouldBeRPCInvalidArgument, "the number of requests in the batch exceeds 500")
		})
	})
}

func TestFilterAndThrottle(t *testing.T) {
	ctx := context.Background()
	// Setup tsmon
	ctx, _ = tsmon.WithDummyInMemory(ctx)

	invInfo := &invocationInfo{realm: "chromium:ci"}
	Convey("Filter artifact", t, func() {
		artReqs := []*artifactCreationRequest{
			{
				testID:      "test1",
				contentType: "",
			},
			{
				testID:      "test2",
				contentType: "text/plain",
			},
			{
				testID:      "test3",
				contentType: "image/png",
			},
			{
				testID:      "test4",
				contentType: "text/html",
			},
		}
		results := filterTextArtifactRequests(ctx, artReqs, invInfo)
		So(results, ShouldResemble, []*artifactCreationRequest{
			{
				testID:      "test2",
				contentType: "text/plain",
			},
			{
				testID:      "test4",
				contentType: "text/html",
			},
		})
	})

	Convey("Throttle artifact", t, func() {
		artReqs := []*artifactCreationRequest{
			{
				testID:     "test",
				artifactID: "artifact38", // Hash value 0
			},
			{
				testID:     "test",
				artifactID: "artifact158", // Hash value 99
			},
			{
				testID:     "test",
				artifactID: "artifact230", // Hash value 32
			},
			{
				testID:     "test",
				artifactID: "artifact232", // Hash value 54
			},
			{
				testID:     "test",
				artifactID: "artifact341", // Hash value 91
			},
		}
		// 0%.
		results, err := throttleArtifactsForBQ(artReqs, 0)
		So(err, ShouldBeNil)
		So(results, ShouldResemble, []*artifactCreationRequest{})

		// 1%.
		results, err = throttleArtifactsForBQ(artReqs, 1)
		So(err, ShouldBeNil)
		So(results, ShouldResemble, []*artifactCreationRequest{
			{
				testID:     "test",
				artifactID: "artifact38", // Hash value 0
			},
		})

		// 33%.
		results, err = throttleArtifactsForBQ(artReqs, 33)
		So(err, ShouldBeNil)
		So(results, ShouldResemble, []*artifactCreationRequest{
			{
				testID:     "test",
				artifactID: "artifact38", // Hash value 0
			},
			{
				testID:     "test",
				artifactID: "artifact230", // Hash value 32
			},
		})

		// 90%.
		results, err = throttleArtifactsForBQ(artReqs, 90)
		So(err, ShouldBeNil)
		So(results, ShouldResemble, []*artifactCreationRequest{
			{
				testID:     "test",
				artifactID: "artifact38", // Hash value 0
			},
			{
				testID:     "test",
				artifactID: "artifact230", // Hash value 32
			},
			{
				testID:     "test",
				artifactID: "artifact232", // Hash value 54
			},
		})

		// 100%.
		results, err = throttleArtifactsForBQ(artReqs, 100)
		So(err, ShouldBeNil)
		So(results, ShouldResemble, []*artifactCreationRequest{
			{
				testID:     "test",
				artifactID: "artifact38", // Hash value 0
			},
			{
				testID:     "test",
				artifactID: "artifact158", // Hash value 99
			},
			{
				testID:     "test",
				artifactID: "artifact230", // Hash value 32
			},
			{
				testID:     "test",
				artifactID: "artifact232", // Hash value 54
			},
			{
				testID:     "test",
				artifactID: "artifact341", // Hash value 91
			},
		})
	})
}

func TestReqToProto(t *testing.T) {
	Convey("Artifact request to proto", t, func() {
		ctx := context.Background()
		req := &artifactCreationRequest{
			testID:      "testid",
			resultID:    "resultid",
			artifactID:  "artifactid",
			contentType: "text/plain",
			size:        20,
			data:        []byte("0123456789abcdefghij"),
		}
		invInfo := &invocationInfo{
			id:         "invid",
			realm:      "chromium:ci",
			createTime: time.Unix(1000, 0),
		}
		results, err := reqToProtos(ctx, req, invInfo, pb.TestStatus_PASS, 10, 10)
		So(err, ShouldBeNil)
		So(results, ShouldResembleProto, []*bqpb.TextArtifactRow{
			{
				Project:             "chromium",
				Realm:               "ci",
				InvocationId:        "invid",
				TestId:              "testid",
				ResultId:            "resultid",
				ArtifactId:          "artifactid",
				ContentType:         "text/plain",
				NumShards:           2,
				ShardId:             0,
				Content:             "0123456789",
				ShardContentSize:    10,
				ArtifactContentSize: 20,
				PartitionTime:       timestamppb.New(time.Unix(1000, 0)),
				ArtifactShard:       "artifactid:0",
				TestStatus:          "PASS",
			},
			{
				Project:             "chromium",
				Realm:               "ci",
				InvocationId:        "invid",
				TestId:              "testid",
				ResultId:            "resultid",
				ArtifactId:          "artifactid",
				ContentType:         "text/plain",
				NumShards:           2,
				ShardId:             1,
				Content:             "abcdefghij",
				ShardContentSize:    10,
				ArtifactContentSize: 20,
				PartitionTime:       timestamppb.New(time.Unix(1000, 0)),
				ArtifactShard:       "artifactid:1",
				TestStatus:          "PASS",
			},
		})
	})
}
