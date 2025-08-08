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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/resultdb/internal/artifacts"
	"go.chromium.org/luci/resultdb/internal/config"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	configpb "go.chromium.org/luci/resultdb/proto/config"
	pb "go.chromium.org/luci/resultdb/proto/v1"
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

func TestNewArtifactCreationRequestsFromProto(t *testing.T) {
	newLegacyArtReq := func(parent, artID, contentType string) *pb.CreateArtifactRequest {
		return &pb.CreateArtifactRequest{
			Parent:   parent,
			Artifact: &pb.Artifact{ArtifactId: artID, ContentType: contentType},
		}
	}

	newTestArtReq := func(artifactID string) *pb.CreateArtifactRequest {
		return &pb.CreateArtifactRequest{
			Artifact: &pb.Artifact{
				TestIdStructured: &pb.TestIdentifier{
					ModuleName:    "module",
					ModuleScheme:  "junit",
					ModuleVariant: pbutil.Variant("k1", "v1", "k2", "v2"),
					CoarseName:    "coarse",
					FineName:      "fine",
					CaseName:      "case",
				},
				ResultId:    "resultid",
				ArtifactId:  artifactID,
				ContentType: "image/png",
			},
		}
	}

	newInvArtReq := func(artifactID string) *pb.CreateArtifactRequest {
		return &pb.CreateArtifactRequest{
			Parent: "invocations/abc",
			Artifact: &pb.Artifact{
				ArtifactId:  artifactID,
				ContentType: "text/plain",
			},
		}
	}

	ftt.Run("newArtifactCreationRequestsFromProto", t, func(t *ftt.Test) {
		bReq := &pb.BatchCreateArtifactsRequest{Parent: "invocations/abc"}
		cfg, err := config.NewCompiledServiceConfig(config.CreatePlaceHolderServiceConfig(), "revision")
		assert.NoErr(t, err)
		t.Run("successes", func(t *ftt.Test) {
			t.Run("use the legacy parent format", func(t *ftt.Test) {
				invArt := newLegacyArtReq("invocations/inv1", "art1", "text/html")
				trArt := newLegacyArtReq("invocations/inv1/tests/t1/results/r1", "art2", "image/png")
				bReq.Requests = append(bReq.Requests, invArt)
				bReq.Requests = append(bReq.Requests, trArt)
				bReq.Parent = ""
				invID, arts, err := parseBatchCreateArtifactsRequest(bReq, cfg)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, invID, should.Equal(invocations.ID("inv1")))
				assert.Loosely(t, len(arts), should.Equal(len(bReq.Requests)))

				// invocation-level artifact
				assert.Loosely(t, arts[0].artifactID, should.Equal("art1"))
				assert.Loosely(t, arts[0].parentID(), should.Equal(artifacts.ParentID("", "")))
				assert.Loosely(t, arts[0].contentType, should.Equal("text/html"))

				// test-result-level artifact
				assert.Loosely(t, arts[1].artifactID, should.Equal("art2"))
				assert.Loosely(t, arts[1].parentID(), should.Equal(artifacts.ParentID("t1", "r1")))
				assert.Loosely(t, arts[1].contentType, should.Equal("image/png"))
				assert.Loosely(t, arts[1].variant, should.BeNil)
			})

			t.Run("use the top-level parent field", func(t *ftt.Test) {
				bReq.Requests = append(bReq.Requests, newInvArtReq("art1"))
				bReq.Requests = append(bReq.Requests, newTestArtReq("art2"))

				invID, arts, err := parseBatchCreateArtifactsRequest(bReq, cfg)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, invID, should.Equal(invocations.ID("abc")))
				assert.Loosely(t, len(arts), should.Equal(2))

				// invocation-level artifact
				assert.Loosely(t, arts[0].artifactID, should.Equal("art1"))
				assert.Loosely(t, arts[0].parentID(), should.Equal(artifacts.ParentID("", "")))
				assert.Loosely(t, arts[0].contentType, should.Equal("text/plain"))
				assert.Loosely(t, arts[0].variant, should.BeNil)

				// test-result-level artifact
				assert.Loosely(t, arts[1].artifactID, should.Equal("art2"))
				assert.Loosely(t, arts[1].parentID(), should.Equal(artifacts.ParentID(":module!junit:coarse:fine#case", "resultid")))
				assert.Loosely(t, arts[1].contentType, should.Equal("image/png"))
				assert.Loosely(t, arts[1].variant, should.Match(pbutil.Variant("k1", "v1", "k2", "v2")))
			})
			t.Run("module variant not specified when test_id_structured is specified", func(t *ftt.Test) {
				// We need this case to support legacy clients.
				// TODO(meiring): Delete when Tradefed uploader migrated.
				r := newTestArtReq("art1")
				r.Artifact.TestIdStructured.ModuleVariant = nil
				bReq.Requests = append(bReq.Requests, r)

				invID, arts, err := parseBatchCreateArtifactsRequest(bReq, cfg)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, invID, should.Equal(invocations.ID("abc")))
				assert.Loosely(t, len(arts), should.Equal(1))
				assert.Loosely(t, arts[0].variant, should.BeNil)
			})
		})

		t.Run("neither of nested parent nor top level parent is specified", func(t *ftt.Test) {
			r := newTestArtReq("art1")
			bReq.Requests = append(bReq.Requests, r)
			bReq.Parent = ""
			r.Parent = ""

			_, _, err := parseBatchCreateArtifactsRequest(bReq, cfg)
			assert.Loosely(t, err, should.ErrLike("requests[0]: parent: unspecified"))
		})

		t.Run("Different nested parent and top level parent", func(t *ftt.Test) {
			r := newTestArtReq("art1")
			r.Parent = "invocations/inv"
			bReq.Requests = append(bReq.Requests, r)

			_, _, err := parseBatchCreateArtifactsRequest(bReq, cfg)
			assert.Loosely(t, err, should.ErrLike("only one parent is allowed: \"invocations/abc\", \"invocations/inv\""))
		})

		t.Run("Use legacy parent format and test_id_structured are specified", func(t *ftt.Test) {
			r := newTestArtReq("art1")
			r.Parent = "invocations/abc/tests/testid/results/123"
			bReq.Requests = append(bReq.Requests, r)
			bReq.Parent = ""

			_, _, err := parseBatchCreateArtifactsRequest(bReq, cfg)
			assert.Loosely(t, err, should.ErrLike("test_id_structured must not be specified if parent is a test result name (legacy format)"))
		})

		t.Run("No top level parent and test_id_structured is specified", func(t *ftt.Test) {
			r := newTestArtReq("art1")
			r.Parent = "invocations/abc"
			bReq.Requests = append(bReq.Requests, r)
			bReq.Parent = ""

			_, _, err := parseBatchCreateArtifactsRequest(bReq, cfg)
			assert.Loosely(t, err, should.ErrLike("test_id_structured or result_id must not be specified if top-level invocation is not set (legacy uploader)"))
		})

		t.Run("test_id_structured not specified with result_id specified", func(t *ftt.Test) {
			r := newTestArtReq("art1")
			r.Artifact.TestIdStructured = nil
			bReq.Requests = append(bReq.Requests, r)

			_, _, err := parseBatchCreateArtifactsRequest(bReq, cfg)
			assert.Loosely(t, err, should.ErrLike("test_id_structured is required if result_id is specified"))
		})

		t.Run("result_id not specified with test_id_structured specified", func(t *ftt.Test) {
			r := newTestArtReq("art1")
			r.Artifact.ResultId = ""
			bReq.Requests = append(bReq.Requests, r)

			_, _, err := parseBatchCreateArtifactsRequest(bReq, cfg)
			assert.Loosely(t, err, should.ErrLike("result_id is required if test_id_structured is specified"))
		})

		t.Run("uploaded test_id_structured references unknown module scheme", func(t *ftt.Test) {
			r := newTestArtReq("art1")
			r.Artifact.TestIdStructured.ModuleScheme = "unknown"
			bReq.Requests = append(bReq.Requests, r)

			_, _, err := parseBatchCreateArtifactsRequest(bReq, cfg)
			assert.Loosely(t, err, should.ErrLike(`requests[0]: test_id_structured: module_scheme: scheme "unknown" is not a known scheme by the ResultDB deployment; see go/resultdb-schemes for instructions how to define a new scheme`))
		})

		t.Run("flat-form test_id references unknown module scheme", func(t *ftt.Test) {
			r := newTestArtReq("art1")
			r.Parent = "invocations/abc/tests/:module%21unknown::%23case/results/123"
			r.Artifact.TestIdStructured = nil
			bReq.Requests = append(bReq.Requests, r)

			_, _, err := parseBatchCreateArtifactsRequest(bReq, cfg)
			assert.Loosely(t, err, should.ErrLike(`requests[0]: parent: encoded test id: module_scheme: scheme "unknown" is not a known scheme by the ResultDB deployment; see go/resultdb-schemes for instructions how to define a new scheme`))
		})

		t.Run("mismatched size_bytes", func(t *ftt.Test) {
			trArt := newTestArtReq("art1")
			bReq.Requests = append(bReq.Requests, trArt)
			trArt.Artifact.SizeBytes = 123
			trArt.Artifact.Contents = make([]byte, 10249)
			_, _, err := parseBatchCreateArtifactsRequest(bReq, cfg)
			assert.Loosely(t, err, should.ErrLike(`sizeBytes and contents are specified but don't match`))
		})

		t.Run("ignored size_bytes", func(t *ftt.Test) {
			trArt := newTestArtReq("art1")
			bReq.Requests = append(bReq.Requests, trArt)
			trArt.Artifact.SizeBytes = 0
			trArt.Artifact.Contents = make([]byte, 10249)
			_, arts, err := parseBatchCreateArtifactsRequest(bReq, cfg)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, arts[0].size, should.Equal(10249))
		})

		t.Run("contents and gcs_uri both specified", func(t *ftt.Test) {
			trArt := newTestArtReq("art1")
			bReq.Requests = append(bReq.Requests, trArt)
			trArt.Artifact.SizeBytes = 0
			trArt.Artifact.Contents = make([]byte, 10249)
			trArt.Artifact.GcsUri = "gs://testbucket/testfile"
			_, _, err := parseBatchCreateArtifactsRequest(bReq, cfg)
			assert.Loosely(t, err, should.ErrLike(`only one of contents and gcs_uri can be given`))
		})

		t.Run("sum() of requests is too big", func(t *ftt.Test) {
			for i := range 11 {
				req := newTestArtReq(fmt.Sprintf("art%d", i))
				req.Artifact.Contents = make([]byte, 1024*1024)
				bReq.Requests = append(bReq.Requests, req)
			}
			_, _, err := parseBatchCreateArtifactsRequest(bReq, cfg)
			assert.Loosely(t, err, should.ErrLike("requests: the size of all requests is too large"))
		})

		t.Run("if more than one invocations - legacy", func(t *ftt.Test) {
			bReq.Requests = append(bReq.Requests, newLegacyArtReq("invocations/inv1", "art1", "text/html"))
			bReq.Requests = append(bReq.Requests, newLegacyArtReq("invocations/inv2", "art1", "text/html"))
			bReq.Parent = ""

			_, _, err := parseBatchCreateArtifactsRequest(bReq, cfg)
			assert.Loosely(t, err, should.ErrLike(`only one invocation is allowed: "inv1", "inv2"`))
		})
	})
}

func TestBatchCreateArtifacts(t *testing.T) {
	// metric field values for Artifact table
	artMFVs := []any{string(spanutil.Artifacts), string(spanutil.Inserted), insert.TestRealm}

	ftt.Run("TestBatchCreateArtifacts", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		ctx = testutil.TestProjectConfigContext(ctx, t, "testproject", "user:test@test.com", "testbucket")
		token, err := generateInvocationToken(ctx, "inv")
		assert.Loosely(t, err, should.BeNil)
		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, token))
		ctx, _ = tsmon.WithDummyInMemory(ctx)
		store := tsmon.Store(ctx)
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: "user:test@test.com",
		})

		serviceCfg := config.CreatePlaceHolderServiceConfig()
		serviceCfg.BqArtifactExportConfig = &configpb.BqArtifactExportConfig{
			Enabled:       true,
			ExportPercent: 100,
		}
		err = config.SetServiceConfigForTesting(ctx, serviceCfg)
		assert.NoErr(t, err)

		casClient := &fakeRBEClient{}
		recorder := newTestRecorderServerWithClients(casClient)
		bReq := &pb.BatchCreateArtifactsRequest{
			Parent: "invocations/inv",
		}

		appendArtReq := func(aID, content, cType, caseName, resultID string) {
			var testID *pb.TestIdentifier
			if caseName != "" {
				testID = &pb.TestIdentifier{
					ModuleName:    "module",
					ModuleScheme:  "junit",
					ModuleVariant: pbutil.Variant("k1", "v1", "k2", "v2"),
					CoarseName:    "coarse",
					FineName:      "fine",
					CaseName:      caseName,
				}
			}
			bReq.Requests = append(bReq.Requests, &pb.CreateArtifactRequest{
				Artifact: &pb.Artifact{
					TestIdStructured: testID,
					ResultId:         resultID,
					ArtifactId:       aID,
					Contents:         []byte(content),
					ContentType:      cType,
				},
			})
		}
		appendGcsArtReq := func(aID string, cSize int64, cType string, gcsURI string) {
			bReq.Requests = append(bReq.Requests, &pb.CreateArtifactRequest{
				Artifact: &pb.Artifact{
					ArtifactId: aID, SizeBytes: cSize, ContentType: cType, GcsUri: gcsURI,
				},
			})
		}

		fetchState := func(parentID, aID string) (size int64, hash string, contentType string, gcsURI string, variant *pb.Variant) {
			testutil.MustReadRow(
				ctx, t, "Artifacts", invocations.ID("inv").Key(parentID, aID),
				map[string]any{
					"Size":          &size,
					"RBECASHash":    &hash,
					"ContentType":   &contentType,
					"GcsURI":        &gcsURI,
					"ModuleVariant": &variant,
				},
			)
			return
		}
		compHash := func(content string) string {
			h := sha256.Sum256([]byte(content))
			return hex.EncodeToString(h[:])
		}

		t.Run("GCS reference isAllowed", func(t *ftt.Test) {
			t.Run("reference is allowed", func(t *ftt.Test) {
				testutil.SetGCSAllowedBuckets(ctx, t, "testproject", "user:test@test.com", "testbucket")

				testutil.MustApply(ctx, t, insert.Invocation("inv", pb.Invocation_ACTIVE, nil))
				appendGcsArtReq("art1", 0, "text/plain", "gs://testbucket/art1")

				_, err := recorder.BatchCreateArtifacts(ctx, bReq)
				assert.Loosely(t, err, should.BeNil)
			})
			t.Run("project not configured", func(t *ftt.Test) {
				testutil.SetGCSAllowedBuckets(ctx, t, "otherproject", "user:test@test.com", "testbucket")

				testutil.MustApply(ctx, t, insert.Invocation("inv", pb.Invocation_ACTIVE, nil))
				appendGcsArtReq("art1", 0, "text/plain", "gs://testbucket/art1")

				_, err := recorder.BatchCreateArtifacts(ctx, bReq)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike("testproject"))
			})
			t.Run("user not configured", func(t *ftt.Test) {
				testutil.SetGCSAllowedBuckets(ctx, t, "testproject", "user:test@test.com", "testbucket")
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: "user:other@test.com",
				})
				testutil.MustApply(ctx, t, insert.Invocation("inv", pb.Invocation_ACTIVE, nil))
				appendGcsArtReq("art1", 0, "text/plain", "gs://testbucket/art1")

				_, err := recorder.BatchCreateArtifacts(ctx, bReq)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike("user:other@test.com"))
			})
			t.Run("bucket not listed", func(t *ftt.Test) {
				testutil.SetGCSAllowedBuckets(ctx, t, "testproject", "user:test@test.com", "otherbucket")

				testutil.MustApply(ctx, t, insert.Invocation("inv", pb.Invocation_ACTIVE, nil))
				appendGcsArtReq("art1", 0, "text/plain", "gs://testbucket/art1")

				_, err := recorder.BatchCreateArtifacts(ctx, bReq)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike("testbucket"))
			})
		})
		t.Run("works", func(t *ftt.Test) {
			testutil.MustApply(ctx, t, insert.Invocation("inv", pb.Invocation_ACTIVE, map[string]any{
				"CreateTime": time.Unix(10000, 0),
			}))

			appendArtReq("art1", "c0ntent", "text/plain", "", "")
			appendArtReq("art2", "c1ntent", "text/richtext", "test_id", "0")
			appendGcsArtReq("art3", 0, "text/plain", "gs://testbucket/art3")
			appendGcsArtReq("art4", 500, "text/richtext", "gs://testbucket/art4")
			appendArtReq("art5", "c5ntent", "text/richtext", "test_id_1", "0")

			casClient.mockResp(nil, codes.OK, codes.OK)

			resp, err := recorder.BatchCreateArtifacts(ctx, bReq)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp, should.Match(&pb.BatchCreateArtifactsResponse{
				Artifacts: []*pb.Artifact{
					{
						Name:        "invocations/inv/artifacts/art1",
						ArtifactId:  "art1",
						ContentType: "text/plain",
						SizeBytes:   7,
					},
					{
						Name: "invocations/inv/tests/:module%21junit:coarse:fine%23test_id/results/0/artifacts/art2",
						TestIdStructured: &pb.TestIdentifier{
							ModuleName:    "module",
							ModuleScheme:  "junit",
							ModuleVariant: pbutil.Variant("k1", "v1", "k2", "v2"),
							CoarseName:    "coarse",
							FineName:      "fine",
							CaseName:      "test_id",
						},
						TestId:      ":module!junit:coarse:fine#test_id",
						ResultId:    "0",
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
						Name: "invocations/inv/tests/:module%21junit:coarse:fine%23test_id_1/results/0/artifacts/art5",
						TestIdStructured: &pb.TestIdentifier{
							ModuleName:    "module",
							ModuleScheme:  "junit",
							ModuleVariant: pbutil.Variant("k1", "v1", "k2", "v2"),
							CoarseName:    "coarse",
							FineName:      "fine",
							CaseName:      "test_id_1",
						},
						TestId:      ":module!junit:coarse:fine#test_id_1",
						ResultId:    "0",
						ArtifactId:  "art5",
						ContentType: "text/richtext",
						SizeBytes:   7,
					},
				},
			}))
			// verify the RBECAS reqs
			assert.Loosely(t, casClient.req, should.Match(&repb.BatchUpdateBlobsRequest{
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
			}))
			// verify the Spanner states
			size, hash, cType, gcsURI, variant := fetchState("", "art1")
			assert.Loosely(t, size, should.Equal(int64(len("c0ntent"))))
			assert.Loosely(t, hash, should.Equal(artifacts.AddHashPrefix(compHash("c0ntent"))))
			assert.Loosely(t, cType, should.Equal("text/plain"))
			assert.Loosely(t, gcsURI, should.BeEmpty)
			assert.Loosely(t, variant, should.Match(&pb.Variant{}))

			size, hash, cType, gcsURI, variant = fetchState("tr/:module!junit:coarse:fine#test_id/0", "art2")
			assert.Loosely(t, size, should.Equal(int64(len("c1ntent"))))
			assert.Loosely(t, hash, should.Equal(artifacts.AddHashPrefix(compHash("c1ntent"))))
			assert.Loosely(t, cType, should.Equal("text/richtext"))
			assert.Loosely(t, gcsURI, should.BeEmpty)
			assert.Loosely(t, variant, should.Match(pbutil.Variant("k1", "v1", "k2", "v2")))

			size, hash, cType, gcsURI, variant = fetchState("tr/:module!junit:coarse:fine#test_id_1/0", "art5")
			assert.Loosely(t, size, should.Equal(int64(len("c5ntent"))))
			assert.Loosely(t, hash, should.Equal(artifacts.AddHashPrefix(compHash("c5ntent"))))
			assert.Loosely(t, cType, should.Equal("text/richtext"))
			assert.Loosely(t, gcsURI, should.BeEmpty)
			assert.Loosely(t, variant, should.Match(pbutil.Variant("k1", "v1", "k2", "v2")))

			size, hash, cType, gcsURI, variant = fetchState("", "art3")
			assert.Loosely(t, size, should.BeZero)
			assert.Loosely(t, hash, should.BeEmpty)
			assert.Loosely(t, cType, should.Equal("text/plain"))
			assert.Loosely(t, gcsURI, should.Equal("gs://testbucket/art3"))
			assert.Loosely(t, variant, should.Match(&pb.Variant{}))

			size, hash, cType, gcsURI, variant = fetchState("", "art4")
			assert.Loosely(t, size, should.Equal(500))
			assert.Loosely(t, hash, should.BeEmpty)
			assert.Loosely(t, cType, should.Equal("text/richtext"))
			assert.Loosely(t, gcsURI, should.Equal("gs://testbucket/art4"))
			assert.Loosely(t, variant, should.Match(&pb.Variant{}))

			// RowCount metric should be increased by 5.
			assert.Loosely(t, store.Get(ctx, spanutil.RowCounter, artMFVs), should.Equal(5))
		})

		t.Run("BatchUpdateBlobs fails", func(t *ftt.Test) {
			testutil.MustApply(ctx, t, insert.Invocation("inv", pb.Invocation_ACTIVE, nil))
			appendArtReq("art1", "c0ntent", "text/plain", "", "")
			appendArtReq("art2", "c1ntent", "text/richtext", "", "")

			t.Run("Partly", func(t *ftt.Test) {
				casClient.mockResp(nil, codes.OK, codes.InvalidArgument)
				// Call the implementation without postlude so that we can see the
				// internal error, and don't just get "Internal server error" back.
				_, err := recorder.Service.BatchCreateArtifacts(ctx, bReq)
				assert.Loosely(t, err, should.ErrLike(`artifact "invocations/inv/artifacts/art2": cas.BatchUpdateBlobs failed`))
			})

			t.Run("Entirely", func(t *ftt.Test) {
				// exceeded the maximum size limit is the only possible error that
				// can cause the entire request failed.
				casClient.mockResp(errors.New("err"), codes.OK, codes.OK)
				// Call the implementation without postlude so that we can see the
				// internal error, and don't just get "Internal server error" back.
				_, err := recorder.Service.BatchCreateArtifacts(ctx, bReq)
				assert.Loosely(t, err, should.ErrLike("cas.BatchUpdateBlobs failed"))
			})

			// RowCount metric should have no changes from any of the above Convey()s.
			assert.Loosely(t, store.Get(ctx, spanutil.RowCounter, artMFVs), should.BeNil)
		})

		t.Run("Token", func(t *ftt.Test) {
			appendArtReq("art1", "", "text/plain", "", "")
			testutil.MustApply(ctx, t, insert.Invocation("inv", pb.Invocation_ACTIVE, nil))

			t.Run("Missing", func(t *ftt.Test) {
				ctx = metadata.NewIncomingContext(ctx, metadata.Pairs())
				_, err = recorder.BatchCreateArtifacts(ctx, bReq)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.Unauthenticated))
				assert.Loosely(t, err, should.ErrLike(`missing update-token`))
			})
			t.Run("Wrong", func(t *ftt.Test) {
				ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, "rong"))
				_, err = recorder.BatchCreateArtifacts(ctx, bReq)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike(`invalid update token`))
			})
		})

		t.Run("Verify state", func(t *ftt.Test) {
			casClient.mockResp(nil, codes.OK, codes.OK)

			t.Run("Finalized invocation", func(t *ftt.Test) {
				testutil.MustApply(ctx, t, insert.Invocation("inv", pb.Invocation_FINALIZED, nil))
				appendArtReq("art1", "c0ntent", "text/plain", "", "")
				_, err = recorder.BatchCreateArtifacts(ctx, bReq)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.FailedPrecondition))
				assert.Loosely(t, err, should.ErrLike(`invocations/inv is not active`))
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

			t.Run("Same artifact exists", func(t *ftt.Test) {
				testutil.MustApply(ctx, t,
					insert.Invocation("inv", pb.Invocation_ACTIVE, nil),
					spanutil.InsertMap("Artifacts", art),
				)
				appendArtReq("art1", "c0ntent", "text/plain", "", "")
				resp, err := recorder.BatchCreateArtifacts(ctx, bReq)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, resp, should.Match(&pb.BatchCreateArtifactsResponse{}))
			})

			t.Run("Different artifact exists", func(t *ftt.Test) {
				testutil.SetGCSAllowedBuckets(ctx, t, "testproject", "user:test@test.com", "testbucket")
				testutil.MustApply(ctx, t,
					insert.Invocation("inv", pb.Invocation_ACTIVE, nil),
					spanutil.InsertMap("Artifacts", art),
				)

				appendArtReq("art1", "c0ntent", "text/plain", "", "")
				bReq.Requests[0].Artifact.Contents = []byte("loooong content")
				_, err := recorder.BatchCreateArtifacts(ctx, bReq)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.AlreadyExists))
				assert.Loosely(t, err, should.ErrLike("exists w/ different size"))

				bReq.Requests[0].Artifact.Contents = []byte("c1ntent")
				_, err = recorder.BatchCreateArtifacts(ctx, bReq)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.AlreadyExists))
				assert.Loosely(t, err, should.ErrLike("exists w/ different hash"))

				bReq.Requests[0].Artifact.Contents = []byte("")
				bReq.Requests[0].Artifact.GcsUri = "gs://testbucket/art1"
				_, err = recorder.BatchCreateArtifacts(ctx, bReq)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.AlreadyExists))
				assert.Loosely(t, err, should.ErrLike("exists w/ different storage scheme"))
			})

			t.Run("Different artifact exists GCS", func(t *ftt.Test) {
				testutil.SetGCSAllowedBuckets(ctx, t, "testproject", "user:test@test.com", "testbucket")
				testutil.MustApply(ctx, t,
					insert.Invocation("inv", pb.Invocation_ACTIVE, nil),
					spanutil.InsertMap("Artifacts", gcsArt),
				)

				appendArtReq("art1", "c0ntent", "text/plain", "", "")
				_, err := recorder.BatchCreateArtifacts(ctx, bReq)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.AlreadyExists))
				assert.Loosely(t, err, should.ErrLike("exists w/ different storage scheme"))

				bReq.Requests[0].Artifact.Contents = []byte("")
				bReq.Requests[0].Artifact.GcsUri = "gs://testbucket/art2"
				_, err = recorder.BatchCreateArtifacts(ctx, bReq)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.AlreadyExists))
				assert.Loosely(t, err, should.ErrLike("exists w/ different GCS URI"))

				appendArtReq("art1", "c0ntent", "text/plain", "", "")
				bReq.Requests[0].Artifact.SizeBytes = 42
				_, err = recorder.BatchCreateArtifacts(ctx, bReq)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.AlreadyExists))
				assert.Loosely(t, err, should.ErrLike("exists w/ different size"))
			})

			// RowCount metric should have no changes from any of the above Convey()s.
			assert.Loosely(t, store.Get(ctx, spanutil.RowCounter, artMFVs), should.BeNil)
		})

		t.Run("Too many requests", func(t *ftt.Test) {
			bReq.Requests = make([]*pb.CreateArtifactRequest, 1000)
			_, err := recorder.BatchCreateArtifacts(ctx, bReq)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("the number of requests in the batch"))
			assert.Loosely(t, err, should.ErrLike("exceeds 500"))
		})
	})
}
