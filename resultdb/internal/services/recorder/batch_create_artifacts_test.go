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
	"strings"
	"testing"

	"cloud.google.com/go/spanner"
	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	spb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/artifacts"
	"go.chromium.org/luci/resultdb/internal/config"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/internal/workunits"
	"go.chromium.org/luci/resultdb/pbutil"
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

func TestBatchCreateArtifacts(t *testing.T) {
	ftt.Run("TestBatchCreateArtifacts", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		ctx = caching.WithEmptyProcessCache(ctx) // For config in-process cache.
		ctx = memory.Use(ctx)                    // For config datastore cache.
		ctx, _ = tsmon.WithDummyInMemory(ctx)
		err := config.SetServiceConfigForTesting(ctx, config.CreatePlaceHolderServiceConfig())
		assert.NoErr(t, err)

		// Mock the CAS client.
		casClient := &fakeRBEClient{}
		recorder := newTestRecorderServerWithClients(casClient)

		rootInvID := rootinvocations.ID("root-inv-id")
		wuID1 := workunits.ID{
			RootInvocationID: "root-inv-id",
			WorkUnitID:       "work-unit:child-1",
		}
		wuID2 := workunits.ID{
			RootInvocationID: "root-inv-id",
			WorkUnitID:       "work-unit:child-2",
		}
		junitModuleID := &pb.ModuleIdentifier{
			ModuleName:   "//infra/junit_tests",
			ModuleScheme: "junit",
			ModuleVariant: pbutil.Variant(
				"a/b", "1",
				"c", "2",
			),
			ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("a/b", "1", "c", "2")),
		}
		legacyModuleID := &pb.ModuleIdentifier{
			ModuleName:    "legacy",
			ModuleScheme:  "legacy",
			ModuleVariant: &pb.Variant{},
		}

		// Create some sample work units and a sample root invocation.
		var ms []*spanner.Mutation
		ms = append(ms, insert.RootInvocationWithRootWorkUnit(rootinvocations.NewBuilder(rootInvID).Build())...)
		ms = append(ms, insert.WorkUnit(workunits.NewBuilder(rootInvID, "work-unit:child-1").WithState(pb.WorkUnit_ACTIVE).WithModuleID(junitModuleID).Build())...)
		ms = append(ms, insert.WorkUnit(workunits.NewBuilder(rootInvID, "work-unit:child-2").WithState(pb.WorkUnit_ACTIVE).WithModuleID(legacyModuleID).Build())...)
		testutil.MustApply(ctx, t, ms...)

		tvID := &pb.TestIdentifier{
			ModuleName:    junitModuleID.ModuleName,
			ModuleScheme:  junitModuleID.ModuleScheme,
			ModuleVariant: junitModuleID.ModuleVariant,
			CoarseName:    "org.chromium.go.luci",
			FineName:      "ValidationTests",
			CaseName:      "FooBar",
		}
		tvIDWithOutputOnlyFields := proto.Clone(tvID).(*pb.TestIdentifier)
		pbutil.PopulateStructuredTestIdentifierHashes(tvIDWithOutputOnlyFields)

		// Artifact for a new test result.
		reqItem1TestResult := &pb.CreateArtifactRequest{
			Parent: wuID1.Name(),
			Artifact: &pb.Artifact{
				TestIdStructured: proto.Clone(tvID).(*pb.TestIdentifier),
				ResultId:         "result-id-0",
				ArtifactId:       "artifact-id-0",
				ContentType:      "text/plain",
				Contents:         []byte("artifact content 0"),
			},
		}
		// GCS Artifact.
		reqItem2GCS := &pb.CreateArtifactRequest{
			Parent: wuID1.Name(),
			Artifact: &pb.Artifact{
				TestIdStructured: proto.Clone(tvID).(*pb.TestIdentifier),
				ResultId:         "result-id-2",
				ArtifactId:       "artifact-id-2",
				ContentType:      "image/png",
				SizeBytes:        50_000_000, // It's good practice to provide a size hint for GCS artifacts.
				GcsUri:           "gs://testbucket/art3",
			},
		}
		// External RBE Artifact.
		reqItem3RBE := &pb.CreateArtifactRequest{
			Parent: wuID1.Name(),
			Artifact: &pb.Artifact{
				TestIdStructured: proto.Clone(tvID).(*pb.TestIdentifier),
				ResultId:         "result-id-3",
				ArtifactId:       "artifact-id-3",
				ContentType:      "image/png",
				RbeUri:           "bytestream://remotebuildexecution.googleapis.com/projects/testproject/instances/default_instance/blobs/deadbeef/10",
			},
		}
		// Work unit-level artifact.
		reqItem4ContainerLevel := &pb.CreateArtifactRequest{
			Parent: wuID1.Name(),
			Artifact: &pb.Artifact{
				ArtifactId:  "artifact-4",
				ContentType: "text/richtext",
				Contents:    []byte("artifact content 4"),
			},
		}
		// Artifact for a legacy test result.
		reqItem5LegacyTestResult := &pb.CreateArtifactRequest{
			Parent: wuID2.Name(),
			Artifact: &pb.Artifact{
				TestIdStructured: &pb.TestIdentifier{
					ModuleName:   "legacy",
					ModuleScheme: "legacy",
					CaseName:     "some-legacy-test-id",
				},
				ResultId:    "result-id-1",
				ArtifactId:  "artifact-id-1",
				ContentType: "text/richtext",
				Contents:    []byte("artifact content 5"),
			},
		}

		req := &pb.BatchCreateArtifactsRequest{
			Requests:  []*pb.CreateArtifactRequest{reqItem1TestResult, reqItem2GCS, reqItem3RBE, reqItem4ContainerLevel, reqItem5LegacyTestResult},
			RequestId: "request-id",
		}

		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: "user:test@test.com",
		})
		ctx = testutil.SetGCSAllowedBuckets(ctx, t, "testproject", "user:test@test.com", "testbucket")
		ctx = testutil.SetRBEAllowedInstances(ctx, t, "testproject", "user:test@test.com", "default_instance")

		token, err := generateWorkUnitUpdateToken(ctx, wuID1)
		assert.Loosely(t, err, should.BeNil)
		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, token))

		t.Run(`request validation`, func(t *ftt.Test) {
			t.Run(`parent`, func(t *ftt.Test) {
				t.Run(`invalid`, func(t *ftt.Test) {
					req.Parent = `invalid`
					_, err := recorder.BatchCreateArtifacts(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike(`parent: does not match pattern`))
				})
				t.Run(`valid`, func(t *ftt.Test) {
					req.Parent = wuID1.Name()
					// Truncate the requests to only include those for wuID1, otherwise we
					// will get an error about the parent at the child request level
					// not matching the batch parent.
					req.Requests = req.Requests[:3]
					// Parent may be set or elided at the individual request level
					// if set at the parent level. Let's try a mix of both.
					req.Requests[0].Parent = ""
					_, err := recorder.BatchCreateArtifacts(ctx, req)
					assert.Loosely(t, err, should.BeNil)
				})
			})
			t.Run(`too many requests`, func(t *ftt.Test) {
				req.Requests = make([]*pb.CreateArtifactRequest, 1000)
				_, err := recorder.BatchCreateArtifacts(ctx, req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike(`requests: the number of requests in the batch (1000) exceeds 500`))
			})
			t.Run(`requests too large`, func(t *ftt.Test) {
				req.Requests[0].Parent = strings.Repeat("a", pbutil.MaxBatchRequestSize)
				_, err := recorder.BatchCreateArtifacts(ctx, req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike(`requests: the size of all requests is too large`))
			})
			t.Run(`sub-requests`, func(t *ftt.Test) {
				t.Run(`parent`, func(t *ftt.Test) {
					t.Run(`unspecified`, func(t *ftt.Test) {
						req.Requests[1].Parent = ""
						_, err := recorder.BatchCreateArtifacts(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike(`requests[1]: parent: unspecified`))
					})
					t.Run(`invalid`, func(t *ftt.Test) {
						req.Requests[1].Parent = "invalid"
						_, err := recorder.BatchCreateArtifacts(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike(`requests[1]: parent: does not match pattern`))
					})
					t.Run(`mismatches batch request parent`, func(t *ftt.Test) {
						req.Parent = wuID1.Name()
						_, err := recorder.BatchCreateArtifacts(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike(`requests[4]: parent: must be empty or equal to the batch-level parent; got "rootInvocations/root-inv-id/workUnits/work-unit:child-2", want "rootInvocations/root-inv-id/workUnits/work-unit:child-1"`))
					})
					t.Run(`mix of work units and invocations - first request is work unit`, func(t *ftt.Test) {
						req.Requests[1].Parent = "invocations/test-inv"
						_, err := recorder.BatchCreateArtifacts(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike(`requests[1]: parent: cannot create artifacts in mix of invocations and work units in the same request, expected another work unit`))
					})
					t.Run(`mix of work units and invocations - first request is invocation`, func(t *ftt.Test) {
						req.Requests[0].Parent = "invocations/test-inv"
						_, err := recorder.BatchCreateArtifacts(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike(`requests[1]: parent: cannot create artifacts in mix of invocations and work units in the same request, expected "invocations/test-inv"`))
					})
					t.Run(`legacy behaviour: test ID and result ID encoded in parent`, func(t *ftt.Test) {
						// Limit to one request so we don't have mixed invocations/work units in the same request.
						req.Requests = req.Requests[:1]
						req.Requests[0].Parent = pbutil.LegacyTestResultName(string("test-inv"), pbutil.EncodeTestID(pbutil.ExtractBaseTestIdentifier(tvID)), "result-id-0")
						req.Requests[0].Artifact.TestIdStructured = nil
						req.Requests[0].Artifact.ResultId = ""
						t.Run(`test ID references invalid scheme`, func(t *ftt.Test) {
							req.Requests[0].Parent = "invocations/abc/tests/:module%21unknown::%23case/results/123"
							_, err := recorder.BatchCreateArtifacts(ctx, req)
							assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
							assert.Loosely(t, err, should.ErrLike(`requests[0]: parent: encoded test id: module_scheme: scheme "unknown" is not a known scheme by the ResultDB deployment; see go/resultdb-schemes for instructions how to define a new scheme`))
						})
						t.Run(`test ID encoded in parent with test ID structured also specified`, func(t *ftt.Test) {
							req.Requests[0].Artifact.TestIdStructured = tvID
							req.Requests[0].Artifact.ResultId = "result-id"
							_, err := recorder.BatchCreateArtifacts(ctx, req)
							assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
							assert.Loosely(t, err, should.ErrLike(`requests[0]: artifact: test_id_structured must not be specified if parent is a test result name (legacy format`))
						})
					})
				})
				t.Run(`artifact`, func(t *ftt.Test) {
					t.Run(`unspecified`, func(t *ftt.Test) {
						req.Requests[1].Artifact = nil
						_, err := recorder.BatchCreateArtifacts(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike(`requests[1]: artifact: unspecified`))
					})
					t.Run(`test_id_structured`, func(t *ftt.Test) {
						t.Run(`unspecified`, func(t *ftt.Test) {
							// May not be left unspecified if result_id is specified.
							req.Requests[1].Artifact.TestIdStructured = nil
							req.Requests[1].Artifact.ResultId = "result_id"
							_, err := recorder.BatchCreateArtifacts(ctx, req)
							assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
							assert.Loosely(t, err, should.ErrLike(`requests[1]: artifact: test_id_structured is required if result_id is specified`))
						})
						t.Run(`invalid`, func(t *ftt.Test) {
							// ValidateStructuredTestIdentifierForStorage has various test cases already, just check it is called.
							req.Requests[1].Artifact.TestIdStructured.ModuleScheme = ""
							_, err := recorder.BatchCreateArtifacts(ctx, req)
							assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
							assert.Loosely(t, err, should.ErrLike(`requests[1]: artifact: test_id_structured: module_scheme: unspecified`))
						})
						t.Run(`unrecognised scheme`, func(t *ftt.Test) {
							req.Requests[1].Artifact.TestIdStructured.ModuleScheme = "unknown"
							_, err := recorder.BatchCreateArtifacts(ctx, req)
							assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
							assert.Loosely(t, err, should.ErrLike(`requests[1]: artifact: test_id_structured: module_scheme: scheme "unknown" is not a known scheme by the ResultDB deployment; see go/resultdb-schemes for instructions how to define a new scheme`))
						})
						t.Run(`does not meet scheme requirements`, func(t *ftt.Test) {
							req.Requests[1].Artifact.TestIdStructured.CoarseName = ""
							_, err := recorder.BatchCreateArtifacts(ctx, req)
							assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
							assert.Loosely(t, err, should.ErrLike(`requests[1]: artifact: test_id_structured: coarse_name: required, please set a Package (scheme "junit")`))
						})
						t.Run("does not match work unit module", func(t *ftt.Test) {
							req.Requests[1].Artifact.TestIdStructured.ModuleName = "other"
							_, err := recorder.BatchCreateArtifacts(ctx, req)
							assert.Loosely(t, err, grpccode.ShouldBe(codes.FailedPrecondition))
							assert.Loosely(t, err, should.ErrLike(`requests[1]: artifact: test_id_structured: module_name: does not match parent work unit module_id.module_name; got "other", want "//infra/junit_tests"`))
						})
					})
					t.Run(`result_id`, func(t *ftt.Test) {
						t.Run(`unspecified`, func(t *ftt.Test) {
							// May not be left unspecified if test_id_structured is specified.
							req.Requests[1].Artifact.TestIdStructured = tvID
							req.Requests[1].Artifact.ResultId = ""
							_, err := recorder.BatchCreateArtifacts(ctx, req)
							assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
							assert.Loosely(t, err, should.ErrLike(`requests[1]: artifact: result_id is required if test_id_structured is specified`))
						})
						t.Run(`invalid`, func(t *ftt.Test) {
							req.Requests[1].Artifact.ResultId = "\x00"
							_, err := recorder.BatchCreateArtifacts(ctx, req)
							assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
							assert.Loosely(t, err, should.ErrLike(`requests[1]: artifact: result_id: does not match pattern`))
						})
						t.Run(`too long`, func(t *ftt.Test) {
							req.Requests[1].Artifact.ResultId = strings.Repeat("a", 100)
							_, err := recorder.BatchCreateArtifacts(ctx, req)
							assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
							assert.Loosely(t, err, should.ErrLike(`requests[1]: artifact: result_id: does not match pattern`))
						})
					})
					t.Run(`artifact_id`, func(t *ftt.Test) {
						t.Run(`unspecified`, func(t *ftt.Test) {
							req.Requests[1].Artifact.ArtifactId = ""
							_, err := recorder.BatchCreateArtifacts(ctx, req)
							assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
							assert.Loosely(t, err, should.ErrLike(`requests[1]: artifact: artifact_id: unspecified`))
						})
						t.Run(`invalid`, func(t *ftt.Test) {
							req.Requests[1].Artifact.ArtifactId = "\x00"
							_, err := recorder.BatchCreateArtifacts(ctx, req)
							assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
							assert.Loosely(t, err, should.ErrLike(`requests[1]: artifact: artifact_id: does not match pattern`))
						})
						t.Run(`too long`, func(t *ftt.Test) {
							req.Requests[1].Artifact.ArtifactId = strings.Repeat("a", 513)
							_, err := recorder.BatchCreateArtifacts(ctx, req)
							assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
							assert.Loosely(t, err, should.ErrLike(`requests[1]: artifact: artifact_id: does not match pattern`))
						})
					})
					t.Run(`content_type`, func(t *ftt.Test) {
						t.Run(`empty`, func(t *ftt.Test) {
							// Empty content type is valid as it is an optional field.
							req.Requests[1].Artifact.ContentType = ""
							_, err := recorder.BatchCreateArtifacts(ctx, req)
							assert.Loosely(t, err, should.BeNil)
						})
						t.Run(`invalid`, func(t *ftt.Test) {
							req.Requests[1].Artifact.ContentType = "\x00"
							_, err := recorder.BatchCreateArtifacts(ctx, req)
							assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
							assert.Loosely(t, err, should.ErrLike(`requests[1]: artifact: content_type: mime: no media type`))
						})
					})
					t.Run(`size_bytes`, func(t *ftt.Test) {
						t.Run(`invalid`, func(t *ftt.Test) {
							req.Requests[1].Artifact.SizeBytes = -1
							_, err := recorder.BatchCreateArtifacts(ctx, req)
							assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
							assert.Loosely(t, err, should.ErrLike(`requests[1]: artifact: size_bytes: must be non-negative`))
						})
						t.Run(`set and mismatches with request content`, func(t *ftt.Test) {
							req.Requests[0].Artifact.SizeBytes = 1
							req.Requests[0].Artifact.Contents = make([]byte, 0)
							_, err := recorder.BatchCreateArtifacts(ctx, req)
							assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
							assert.Loosely(t, err, should.ErrLike(`requests[0]: artifact: size_bytes: does not match the size of contents (and the artifact is not a GCS or RBE reference)`))
						})
						t.Run(`set and mismatches with RBE content`, func(t *ftt.Test) {
							// 10 bytes
							req.Requests[2].Artifact.RbeUri = `bytestream://remotebuildexecution.googleapis.com/projects/testproject/instances/default_instance/blobs/deadbeef/10`
							req.Requests[2].Artifact.SizeBytes = 12
							_, err := recorder.BatchCreateArtifacts(ctx, req)
							assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
							assert.Loosely(t, err, should.ErrLike(`requests[2]: artifact: size_bytes: does not match the size of external RBE artifact; got 12, want 10`))
						})
					})
					t.Run(`gcs_uri`, func(t *ftt.Test) {
						t.Run(`bucket name too long`, func(t *ftt.Test) {
							req.Requests[1].Artifact.GcsUri = "gs://" + strings.Repeat("a", 223) + "/object"
							_, err := recorder.BatchCreateArtifacts(ctx, req)
							assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
							assert.Loosely(t, err, should.ErrLike(`requests[1]: artifact: gcs_uri: bucket name component exceeds 222 bytes`))
						})
						t.Run(`object name too long`, func(t *ftt.Test) {
							req.Requests[1].Artifact.GcsUri = "gs://bucket/" + strings.Repeat("a", 1025)
							_, err := recorder.BatchCreateArtifacts(ctx, req)
							assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
							assert.Loosely(t, err, should.ErrLike(`requests[1]: artifact: gcs_uri: object name component exceeds 1024 bytes`))
						})
						t.Run(`bucket name empty`, func(t *ftt.Test) {
							req.Requests[1].Artifact.GcsUri = "object name"
							_, err := recorder.BatchCreateArtifacts(ctx, req)
							assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
							assert.Loosely(t, err, should.ErrLike(`requests[1]: artifact: gcs_uri: missing bucket name; got "object name"`))
						})
						t.Run(`object name empty`, func(t *ftt.Test) {
							req.Requests[1].Artifact.GcsUri = "gs://bucket/"
							_, err := recorder.BatchCreateArtifacts(ctx, req)
							assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
							assert.Loosely(t, err, should.ErrLike(`requests[1]: artifact: gcs_uri: missing object name; got "gs://bucket/"`))
						})
					})
					t.Run(`rbe_uri`, func(t *ftt.Test) {
						t.Run(`invalid`, func(t *ftt.Test) {
							req.Requests[2].Artifact.RbeUri = "bytestream://"
							_, err := recorder.BatchCreateArtifacts(ctx, req)
							assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
							assert.Loosely(t, err, should.ErrLike(`requests[2]: artifact: rbe_uri: invalid RBE URI format: does not match 'bytestream://<HOSTNAME>/projects/<PROJECT_ID>/instances/<INSTANCE_ID>/blobs/<HASH>/<SIZE_BYTES>'`))
						})
					})
					t.Run(`mixed artifact types are rejected`, func(t *ftt.Test) {
						t.Run(`mixed RBE/contents`, func(t *ftt.Test) {
							req.Requests[2].Artifact.RbeUri = "bytestream://remotebuildexecution.googleapis.com/projects/testproject/instances/default_instance/blobs/deadbeef/10"
							req.Requests[2].Artifact.Contents = []byte("some content")
							_, err := recorder.BatchCreateArtifacts(ctx, req)
							assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
							assert.Loosely(t, err, should.ErrLike(`requests[2]: artifact: only one of contents, gcs_uri and rbe_uri can be given`))
						})
						t.Run(`mixed GCS/contents`, func(t *ftt.Test) {
							req.Requests[1].Artifact.GcsUri = "gs://testbucket/art3"
							req.Requests[1].Artifact.Contents = []byte("some content")
							_, err := recorder.BatchCreateArtifacts(ctx, req)
							assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
							assert.Loosely(t, err, should.ErrLike(`requests[1]: artifact: only one of contents, gcs_uri and rbe_uri can be given`))
						})
						t.Run(`mixed RBE/GCS`, func(t *ftt.Test) {
							req.Requests[1].Artifact.GcsUri = "gs://testbucket/art3"
							req.Requests[1].Artifact.RbeUri = "bytestream://remotebuildexecution.googleapis.com/projects/testproject/instances/default_instance/blobs/deadbeef/10"
							_, err := recorder.BatchCreateArtifacts(ctx, req)
							assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
							assert.Loosely(t, err, should.ErrLike(`requests[1]: artifact: only one of contents, gcs_uri and rbe_uri can be given`))
						})
					})
				})
				// Duplicate artifact creation requests should be detected and rejected.
				t.Run(`duplicate request - test result artifact`, func(t *ftt.Test) {
					req.Requests = append(req.Requests, reqItem1TestResult)
					_, err := recorder.BatchCreateArtifacts(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike(`requests[5]: same parent, test_id_structured, result_id and artifact_id as requests[0]`))
				})
				t.Run(`duplicate request - invocation-level artifact`, func(t *ftt.Test) {
					req.Requests = append(req.Requests, reqItem4ContainerLevel)
					_, err := recorder.BatchCreateArtifacts(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike(`requests[5]: same parent, test_id_structured, result_id and artifact_id as requests[3]`))
				})
			})
			t.Run(`request_id`, func(t *ftt.Test) {
				t.Run(`empty`, func(t *ftt.Test) {
					req.RequestId = ""
					_, err := recorder.BatchCreateArtifacts(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike(`request_id: unspecified`))
				})
				t.Run(`invalid`, func(t *ftt.Test) {
					// non-ascii character
					req.RequestId = string(rune(244))
					_, err = recorder.BatchCreateArtifacts(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike(`request_id: does not match pattern`))
				})
				t.Run(`unmatched request_id in requests`, func(t *ftt.Test) {
					req.RequestId = "foo"
					req.Requests[0].RequestId = "bar"
					_, err := recorder.BatchCreateArtifacts(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("requests[0]: request_id: must be either empty or equal"))
				})
			})
		})
		t.Run(`request authorization`, func(t *ftt.Test) {
			t.Run(`request cannot be authorised with single update token`, func(t *ftt.Test) {
				req.Requests[0].Parent = "rootInvocations/root-inv/workUnits/work-unit-1"
				req.Requests[1].Parent = "rootInvocations/root-inv/workUnits/work-unit-2"

				ctx := metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, "invalid-token"))
				_, err := recorder.BatchCreateArtifacts(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.That(t, err, should.ErrLike(`requests[1]: parent: work unit "rootInvocations/root-inv/workUnits/work-unit-2" requires a different update token to request[0]'s "rootInvocations/root-inv/workUnits/work-unit-1", but this RPC only accepts one update token`))
			})
			t.Run(`invalid update token`, func(t *ftt.Test) {
				ctx := metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, "invalid-token"))
				_, err := recorder.BatchCreateArtifacts(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.That(t, err, should.ErrLike(`invalid update token`))
			})
			t.Run(`missing update token`, func(t *ftt.Test) {
				ctx := metadata.NewIncomingContext(ctx, metadata.MD{})
				_, err := recorder.BatchCreateArtifacts(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.Unauthenticated))
				assert.That(t, err, should.ErrLike(`missing update-token metadata value in the request`))
			})
			t.Run(`with invocations`, func(t *ftt.Test) {
				req.Parent = "invocations/inv-1"
				for _, r := range req.Requests {
					r.Parent = ""
				}
				t.Run(`request cannot be authorized with a single update token`, func(t *ftt.Test) {
					req.Parent = ""
					req.Requests[0].Parent = "invocations/inv-1"
					for _, r := range req.Requests[1:] {
						r.Parent = "invocations/inv-2"
					}

					ctx := metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, "invalid-token"))
					_, err := recorder.BatchCreateArtifacts(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.That(t, err, should.ErrLike(`requests[1]: parent: only one invocation is allowed: got "invocations/inv-2", want "invocations/inv-1"`))
				})
				t.Run(`invalid update token`, func(t *ftt.Test) {
					ctx := metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, "invalid-token"))
					_, err := recorder.BatchCreateArtifacts(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
					assert.That(t, err, should.ErrLike(`invalid update token`))
				})
				t.Run(`missing update token`, func(t *ftt.Test) {
					ctx := metadata.NewIncomingContext(ctx, metadata.MD{})
					_, err := recorder.BatchCreateArtifacts(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.Unauthenticated))
					assert.That(t, err, should.ErrLike(`missing update-token metadata value in the request`))
				})
			})
			t.Run(`GCS artifact references`, func(t *ftt.Test) {
				t.Run(`missing GCS bucket ACL`, func(t *ftt.Test) {
					ctx := testutil.SetGCSAllowedBuckets(ctx, t, "testproject", "user:test@test.com", "otherbucket")
					_, err := recorder.BatchCreateArtifacts(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
					assert.That(t, err, should.ErrLike(`requests[1]: the user does not have permission to reference GCS objects in bucket "testbucket" in project "testproject"`))
				})
				t.Run(`missing GCS project ACL`, func(t *ftt.Test) {
					ctx = testutil.ClearProjectConfig(ctx, t, "testproject")
					_, err := recorder.BatchCreateArtifacts(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
					assert.That(t, err, should.ErrLike(`requests[1]: the user does not have permission to reference GCS objects in bucket "testbucket" in project "testproject"`))
				})
			})
			t.Run(`RBE artifact references`, func(t *ftt.Test) {
				t.Run(`missing RBE instance ACL`, func(t *ftt.Test) {
					ctx = testutil.SetRBEAllowedInstances(ctx, t, "testproject", "user:test@test.com", "otherbucket")
					_, err := recorder.BatchCreateArtifacts(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
					assert.That(t, err, should.ErrLike(`requests[2]: the user does not have permission to reference RBE objects in RBE instance "projects/testproject/instances/default_instance"`))
				})
				t.Run(`missing RBE project ACL`, func(t *ftt.Test) {
					// Put the RBE artifact first so that it attracts any errors in preference to the GCS artifact.
					req.Requests[2], req.Requests[0] = req.Requests[0], req.Requests[2]
					ctx = testutil.ClearProjectConfig(ctx, t, "testproject")
					_, err := recorder.BatchCreateArtifacts(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
					assert.That(t, err, should.ErrLike(`requests[0]: the user does not have permission to reference RBE objects in RBE instance "projects/testproject/instances/default_instance"`))
				})
			})
		})
		t.Run(`with work units`, func(t *ftt.Test) {
			expectedResponse := &pb.BatchCreateArtifactsResponse{
				Artifacts: []*pb.Artifact{
					{
						Name:             "rootInvocations/root-inv-id/workUnits/work-unit:child-1/tests/:%2F%2Finfra%2Fjunit_tests%21junit:org.chromium.go.luci:ValidationTests%23FooBar/results/result-id-0/artifacts/artifact-id-0",
						ArtifactId:       "artifact-id-0",
						ContentType:      "text/plain",
						SizeBytes:        18,
						TestId:           "://infra/junit_tests!junit:org.chromium.go.luci:ValidationTests#FooBar",
						TestIdStructured: tvIDWithOutputOnlyFields,
						ResultId:         "result-id-0",
						HasLines:         true,
					},
					{
						Name:             "rootInvocations/root-inv-id/workUnits/work-unit:child-1/tests/:%2F%2Finfra%2Fjunit_tests%21junit:org.chromium.go.luci:ValidationTests%23FooBar/results/result-id-2/artifacts/artifact-id-2",
						ArtifactId:       "artifact-id-2",
						ContentType:      "image/png",
						SizeBytes:        50_000_000,
						GcsUri:           "gs://testbucket/art3",
						TestId:           "://infra/junit_tests!junit:org.chromium.go.luci:ValidationTests#FooBar",
						TestIdStructured: tvIDWithOutputOnlyFields,
						ResultId:         "result-id-2",
					},
					{
						Name:             "rootInvocations/root-inv-id/workUnits/work-unit:child-1/tests/:%2F%2Finfra%2Fjunit_tests%21junit:org.chromium.go.luci:ValidationTests%23FooBar/results/result-id-3/artifacts/artifact-id-3",
						ArtifactId:       "artifact-id-3",
						ContentType:      "image/png",
						SizeBytes:        10,
						RbeUri:           "bytestream://remotebuildexecution.googleapis.com/projects/testproject/instances/default_instance/blobs/deadbeef/10",
						TestId:           "://infra/junit_tests!junit:org.chromium.go.luci:ValidationTests#FooBar",
						TestIdStructured: tvIDWithOutputOnlyFields,
						ResultId:         "result-id-3",
					},
					{
						Name:        "rootInvocations/root-inv-id/workUnits/work-unit:child-1/artifacts/artifact-4",
						ArtifactId:  "artifact-4",
						ContentType: "text/richtext",
						SizeBytes:   18,
						HasLines:    true,
					},
					{
						Name:        "rootInvocations/root-inv-id/workUnits/work-unit:child-2/tests/some-legacy-test-id/results/result-id-1/artifacts/artifact-id-1",
						ArtifactId:  "artifact-id-1",
						ContentType: "text/richtext",
						SizeBytes:   18,
						TestId:      "some-legacy-test-id",
						TestIdStructured: &pb.TestIdentifier{
							ModuleName:        "legacy",
							ModuleScheme:      "legacy",
							ModuleVariant:     &pb.Variant{},
							ModuleVariantHash: pbutil.VariantHash(&pb.Variant{}),
							CaseName:          "some-legacy-test-id",
						},
						ResultId: "result-id-1",
						HasLines: true,
					},
				},
			}

			t.Run(`base case (success)`, func(t *ftt.Test) {
				resp, err := recorder.BatchCreateArtifacts(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, resp, should.Match(expectedResponse))

				// Verify with the database.
				var rbeCASHashes []string
				for _, a := range expectedResponse.Artifacts {
					got, err := artifacts.Read(span.Single(ctx), a.Name)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, got.Artifact, should.Match(a))
					rbeCASHashes = append(rbeCASHashes, got.RBECASHash)
				}
				assert.Loosely(t, rbeCASHashes, should.Match([]string{
					artifacts.AddHashPrefix(sha256Hash("artifact content 0")),
					"", // GCS artifact
					"", // RBE artifact
					artifacts.AddHashPrefix(sha256Hash("artifact content 4")),
					artifacts.AddHashPrefix(sha256Hash("artifact content 5")),
				}))

				// Verify the RBECAS request
				assert.Loosely(t, casClient.req, should.Match(&repb.BatchUpdateBlobsRequest{
					InstanceName: "",
					Requests: []*repb.BatchUpdateBlobsRequest_Request{
						{
							Digest: &repb.Digest{
								Hash:      sha256Hash("artifact content 0"),
								SizeBytes: int64(len("artifact content 0")),
							},
							Data: []byte("artifact content 0"),
						},
						{
							Digest: &repb.Digest{
								Hash:      sha256Hash("artifact content 4"),
								SizeBytes: int64(len("artifact content 4")),
							},
							Data: []byte("artifact content 4"),
						},
						{
							Digest: &repb.Digest{
								Hash:      sha256Hash("artifact content 5"),
								SizeBytes: int64(len("artifact content 5")),
							},
							Data: []byte("artifact content 5"),
						},
					},
				}))

				t.Run(`idempotent`, func(t *ftt.Test) {
					// Verify retrying the same request produces the same result.
					casClient.req = nil

					resp, err := recorder.BatchCreateArtifacts(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, resp, should.Match(expectedResponse))

					// Verify with the database.
					for _, a := range expectedResponse.Artifacts {
						got, err := artifacts.Read(span.Single(ctx), a.Name)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, got.Artifact, should.Match(a))
					}

					// No further request made to RBE-CAS as the artifacts
					// already exist.
					assert.Loosely(t, casClient.req, should.BeNil)
				})
			})
			t.Run(`parent`, func(t *ftt.Test) {
				t.Run(`finalized`, func(t *ftt.Test) {
					testutil.MustApply(ctx, t, spanutil.UpdateMap("WorkUnits", map[string]any{
						"RootInvocationShardId": wuID1.RootInvocationShardID(),
						"WorkUnitId":            wuID1.WorkUnitID,
						"State":                 pb.WorkUnit_FINALIZED,
					}))
					_, err := recorder.BatchCreateArtifacts(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.FailedPrecondition))
					assert.Loosely(t, err, should.ErrLike(`"rootInvocations/root-inv-id/workUnits/work-unit:child-1" is not active`))
				})
				t.Run(`does not exist`, func(t *ftt.Test) {
					testutil.MustApply(ctx, t, spanner.Delete("WorkUnits", wuID1.Key()))
					_, err := recorder.BatchCreateArtifacts(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
					assert.Loosely(t, err, should.ErrLike(`"rootInvocations/root-inv-id/workUnits/work-unit:child-1" not found`))
				})
				t.Run(`no module ID set`, func(t *ftt.Test) {
					testutil.MustApply(ctx, t, spanutil.UpdateMap("WorkUnits", map[string]any{
						"RootInvocationShardId": wuID1.RootInvocationShardID(),
						"WorkUnitId":            wuID1.WorkUnitID,
						"ModuleName":            spanner.NullString{},
						"ModuleScheme":          spanner.NullString{},
						"ModuleVariant":         []string{},
						"ModuleVariantHash":     spanner.NullString{},
					}))

					_, err := recorder.BatchCreateArtifacts(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.FailedPrecondition))
					assert.Loosely(t, err, should.ErrLike(`requests[0]: artifact: to upload test results or test result artifacts, you must set the module_id on the parent work unit first`))
				})
			})
			t.Run(`artifact already exists`, func(t *ftt.Test) {
				// Create the artifacts.
				resp, err := recorder.BatchCreateArtifacts(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, resp, should.Match(expectedResponse))

				// Now create another request for the same artifacts but with
				// slightly different properties so it is not handled as an
				// idempotent request.
				t.Run(`RBE-CAS`, func(t *ftt.Test) {
					updatedRequestItem := proto.Clone(reqItem1TestResult).(*pb.CreateArtifactRequest)
					req := &pb.BatchCreateArtifactsRequest{
						Requests:  []*pb.CreateArtifactRequest{updatedRequestItem},
						RequestId: req.RequestId,
					}

					t.Run(`with different storage scheme`, func(t *ftt.Test) {
						updatedRequestItem.Artifact.RbeUri = "bytestream://remotebuildexution.googleapis.com/projects/testproject/instances/default_instance/blobs/deadbeef/10"
						updatedRequestItem.Artifact.Contents = nil
						_, err := recorder.BatchCreateArtifacts(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.AlreadyExists))
						assert.Loosely(t, err, should.ErrLike(`requests[0]: artifact "rootInvocations/root-inv-id/workUnits/work-unit:child-1/tests/:%2F%2Finfra%2Fjunit_tests%21junit:org.chromium.go.luci:ValidationTests%23FooBar/results/result-id-0/artifacts/artifact-id-0" already exists with a different storage scheme (RBE vs non-RBE)`))
					})
					t.Run(`with different content`, func(t *ftt.Test) {
						updatedRequestItem.Artifact.Contents = []byte("different content")
						_, err := recorder.BatchCreateArtifacts(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.AlreadyExists))
						assert.Loosely(t, err, should.ErrLike(`requests[0]: artifact "rootInvocations/root-inv-id/workUnits/work-unit:child-1/tests/:%2F%2Finfra%2Fjunit_tests%21junit:org.chromium.go.luci:ValidationTests%23FooBar/results/result-id-0/artifacts/artifact-id-0" already exists with a different hash`))
					})
				})
				t.Run(`GCS`, func(t *ftt.Test) {
					updatedRequestItem := proto.Clone(reqItem2GCS).(*pb.CreateArtifactRequest)
					req := &pb.BatchCreateArtifactsRequest{
						Requests:  []*pb.CreateArtifactRequest{updatedRequestItem},
						RequestId: req.RequestId,
					}

					t.Run(`with different content URL`, func(t *ftt.Test) {
						updatedRequestItem.Artifact.GcsUri = "gs://testbucket/other-path/file.txt"
						_, err := recorder.BatchCreateArtifacts(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.AlreadyExists))
						assert.Loosely(t, err, should.ErrLike(`requests[0]: artifact "rootInvocations/root-inv-id/workUnits/work-unit:child-1/tests/:%2F%2Finfra%2Fjunit_tests%21junit:org.chromium.go.luci:ValidationTests%23FooBar/results/result-id-2/artifacts/artifact-id-2" already exists with a different GCS URI: "gs://testbucket/other-path/file.txt" != "gs://testbucket/art3"`))
					})
					t.Run(`with different storage scheme`, func(t *ftt.Test) {
						updatedRequestItem.Artifact.GcsUri = ""
						updatedRequestItem.Artifact.Contents = []byte("different content")
						updatedRequestItem.Artifact.SizeBytes = 0
						_, err := recorder.BatchCreateArtifacts(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.AlreadyExists))
						assert.Loosely(t, err, should.ErrLike(`requests[0]: artifact "rootInvocations/root-inv-id/workUnits/work-unit:child-1/tests/:%2F%2Finfra%2Fjunit_tests%21junit:org.chromium.go.luci:ValidationTests%23FooBar/results/result-id-2/artifacts/artifact-id-2" already exists with a different storage scheme (GCS vs non-GCS)`))
					})
					t.Run(`with different size`, func(t *ftt.Test) {
						updatedRequestItem.Artifact.SizeBytes = 100
						_, err := recorder.BatchCreateArtifacts(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.AlreadyExists))
						assert.Loosely(t, err, should.ErrLike(`requests[0]: artifact "rootInvocations/root-inv-id/workUnits/work-unit:child-1/tests/:%2F%2Finfra%2Fjunit_tests%21junit:org.chromium.go.luci:ValidationTests%23FooBar/results/result-id-2/artifacts/artifact-id-2" already exists with a different size: 100 != 50000000`))
					})
				})
				t.Run(`RBE External`, func(t *ftt.Test) {
					updatedRequestItem := proto.Clone(reqItem3RBE).(*pb.CreateArtifactRequest)
					req := &pb.BatchCreateArtifactsRequest{
						Requests:  []*pb.CreateArtifactRequest{updatedRequestItem},
						RequestId: req.RequestId,
					}

					t.Run(`with different content URL`, func(t *ftt.Test) {
						updatedRequestItem.Artifact.RbeUri = `bytestream://remotebuildexecution.googleapis.com/projects/testproject/instances/default_instance/blobs/otherfile/10`
						_, err := recorder.BatchCreateArtifacts(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.AlreadyExists))
						assert.Loosely(t, err, should.ErrLike(`requests[0]: artifact "rootInvocations/root-inv-id/workUnits/work-unit:child-1/tests/:%2F%2Finfra%2Fjunit_tests%21junit:org.chromium.go.luci:ValidationTests%23FooBar/results/result-id-3/artifacts/artifact-id-3" already exists with a different RBE URI: "bytestream://remotebuildexecution.googleapis.com/projects/testproject/instances/default_instance/blobs/otherfile/10" != "bytestream://remotebuildexecution.googleapis.com/projects/testproject/instances/default_instance/blobs/deadbeef/10"`))
					})
					t.Run(`with different storage scheme`, func(t *ftt.Test) {
						updatedRequestItem.Artifact.RbeUri = ""
						updatedRequestItem.Artifact.Contents = []byte("different content")
						_, err := recorder.BatchCreateArtifacts(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.AlreadyExists))
						assert.Loosely(t, err, should.ErrLike(`requests[0]: artifact "rootInvocations/root-inv-id/workUnits/work-unit:child-1/tests/:%2F%2Finfra%2Fjunit_tests%21junit:org.chromium.go.luci:ValidationTests%23FooBar/results/result-id-3/artifacts/artifact-id-3" already exists with a different storage scheme (RBE vs non-RBE)`))
					})
				})
			})
			t.Run(`RBE-CAS write fails`, func(t *ftt.Test) {
				t.Run(`partly`, func(t *ftt.Test) {
					casClient.mockResp(nil, codes.OK, codes.InvalidArgument, codes.OK)

					// Call the implementation without postlude so that we can see the
					// internal error, and don't just get "Internal server error" back.
					_, err := recorder.Service.BatchCreateArtifacts(ctx, req)
					assert.Loosely(t, err, should.ErrLike(`artifact "rootInvocations/root-inv-id/workUnits/work-unit:child-1/artifacts/artifact-4": cas.BatchUpdateBlobs failed`))
				})
				t.Run(`entirely`, func(t *ftt.Test) {
					casClient.mockResp(errors.New("RBE-CAS write error"))

					// Call the implementation without postlude so that we can see the
					// internal error, and don't just get "Internal server error" back.
					_, err := recorder.Service.BatchCreateArtifacts(ctx, req)
					assert.Loosely(t, err, should.ErrLike(`cas.BatchUpdateBlobs failed`))
				})
			})
		})
		t.Run(`with invocations`, func(t *ftt.Test) {
			invID := invocations.ID("inv-id")
			// Create a sample invocation.
			testutil.MustApply(ctx, t, insert.Invocation(invID, pb.Invocation_ACTIVE, map[string]any{
				"ModuleName":        "//infra/junit_tests",
				"ModuleScheme":      "junit",
				"ModuleVariant":     pbutil.Variant("a/b", "1", "c", "2"),
				"ModuleVariantHash": pbutil.VariantHash(pbutil.Variant("a/b", "1", "c", "2")),
			}))

			// Artifact for a new test result.
			reqItem1TestResult := &pb.CreateArtifactRequest{
				Parent: pbutil.LegacyTestResultName(string(invID), pbutil.EncodeTestID(pbutil.ExtractBaseTestIdentifier(tvID)), "result-id-0"),
				Artifact: &pb.Artifact{
					ArtifactId:  "artifact-id-0",
					ContentType: "text/plain",
					Contents:    []byte("artifact content 0"),
				},
			}
			// GCS Artifact.
			reqItem2GCS := &pb.CreateArtifactRequest{
				Parent: invID.Name(),
				Artifact: &pb.Artifact{
					TestIdStructured: tvID,
					ResultId:         "result-id-2",
					ArtifactId:       "artifact-id-2",
					ContentType:      "image/png",
					SizeBytes:        50_000_000,
					GcsUri:           "gs://testbucket/art3",
				},
			}
			// Invocation-level artifact.
			reqItem3ContainerLevel := &pb.CreateArtifactRequest{
				Parent: invID.Name(),
				Artifact: &pb.Artifact{
					ArtifactId:  "artifact-4",
					ContentType: "text/richtext",
					Contents:    []byte("artifact content 4"),
				},
			}

			req := &pb.BatchCreateArtifactsRequest{
				Requests: []*pb.CreateArtifactRequest{reqItem1TestResult, reqItem2GCS, reqItem3ContainerLevel},
			}

			token, err := generateInvocationToken(ctx, invID)
			assert.Loosely(t, err, should.BeNil)
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, token))

			expectedResponse := &pb.BatchCreateArtifactsResponse{
				Artifacts: []*pb.Artifact{
					{
						Name:             "invocations/inv-id/tests/:%2F%2Finfra%2Fjunit_tests%21junit:org.chromium.go.luci:ValidationTests%23FooBar/results/result-id-0/artifacts/artifact-id-0",
						ArtifactId:       "artifact-id-0",
						ContentType:      "text/plain",
						SizeBytes:        18,
						TestId:           "://infra/junit_tests!junit:org.chromium.go.luci:ValidationTests#FooBar",
						TestIdStructured: tvIDWithOutputOnlyFields,
						ResultId:         "result-id-0",
						HasLines:         true,
					},
					{
						Name:             "invocations/inv-id/tests/:%2F%2Finfra%2Fjunit_tests%21junit:org.chromium.go.luci:ValidationTests%23FooBar/results/result-id-2/artifacts/artifact-id-2",
						ArtifactId:       "artifact-id-2",
						ContentType:      "image/png",
						SizeBytes:        50_000_000,
						GcsUri:           "gs://testbucket/art3",
						TestId:           "://infra/junit_tests!junit:org.chromium.go.luci:ValidationTests#FooBar",
						TestIdStructured: tvIDWithOutputOnlyFields,
						ResultId:         "result-id-2",
					},
					{
						Name:        "invocations/inv-id/artifacts/artifact-4",
						ArtifactId:  "artifact-4",
						ContentType: "text/richtext",
						SizeBytes:   18,
						HasLines:    true,
					},
				},
			}

			t.Run(`base case (success)`, func(t *ftt.Test) {
				resp, err := recorder.BatchCreateArtifacts(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, resp, should.Match(expectedResponse))

				// Verify with the database.
				var rbeCASHashes []string
				for _, a := range expectedResponse.Artifacts {
					got, err := artifacts.Read(span.Single(ctx), a.Name)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, got.Artifact, should.Match(a))
					rbeCASHashes = append(rbeCASHashes, got.RBECASHash)
				}
				assert.Loosely(t, rbeCASHashes, should.Match([]string{
					artifacts.AddHashPrefix(sha256Hash("artifact content 0")),
					"", // GCS artifact
					artifacts.AddHashPrefix(sha256Hash("artifact content 4")),
				}))

				t.Run(`idempotent`, func(t *ftt.Test) {
					resp, err := recorder.BatchCreateArtifacts(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, resp, should.Match(expectedResponse))

					// Verify with the database.
					for _, a := range expectedResponse.Artifacts {
						got, err := artifacts.Read(span.Single(ctx), a.Name)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, got.Artifact, should.Match(a))
					}
				})
			})
			t.Run(`parent`, func(t *ftt.Test) {
				t.Run(`finalized`, func(t *ftt.Test) {
					testutil.MustApply(ctx, t, spanutil.UpdateMap("Invocations", map[string]any{
						"InvocationId": invID,
						"State":        pb.Invocation_FINALIZED,
					}))
					_, err := recorder.BatchCreateArtifacts(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.FailedPrecondition))
					assert.Loosely(t, err, should.ErrLike(`"invocations/inv-id" is not active`))
				})
				t.Run(`not found`, func(t *ftt.Test) {
					testutil.MustApply(ctx, t, spanner.Delete("Invocations", invID.Key()))
					_, err := recorder.BatchCreateArtifacts(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
					assert.Loosely(t, err, should.ErrLike(`invocations/inv-id not found`))
				})
			})
			t.Run(`legacy use case: without module ID`, func(t *ftt.Test) {
				// Update expected module ID.
				testutil.MustApply(ctx, t, spanutil.UpdateMap("Invocations", map[string]any{
					"InvocationId":      invID,
					"ModuleName":        spanner.NullString{},
					"ModuleScheme":      spanner.NullString{},
					"ModuleVariant":     []string{},
					"ModuleVariantHash": spanner.NullString{},
				}))

				for _, e := range expectedResponse.Artifacts {
					if e.TestIdStructured != nil {
						e.TestIdStructured.ModuleVariant = nil
						e.TestIdStructured.ModuleVariantHash = ""
					}
				}

				resp, err := recorder.BatchCreateArtifacts(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, resp, should.Match(expectedResponse))
			})
			t.Run(`legacy use case: without module variant`, func(t *ftt.Test) {
				reqItem2GCS.Artifact.TestIdStructured.ModuleVariant = nil

				// Does not affect response as the variant is taken from the invocation module_id.
				resp, err := recorder.BatchCreateArtifacts(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, resp, should.Match(expectedResponse))
			})
			t.Run(`legacy use case: with legacy test ID`, func(t *ftt.Test) {
				// Update expected module ID.
				testutil.MustApply(ctx, t, spanutil.UpdateMap("Invocations", map[string]any{
					"InvocationId":      invID,
					"ModuleName":        "legacy",
					"ModuleScheme":      "legacy",
					"ModuleVariant":     pbutil.Variant("k", "v"),
					"ModuleVariantHash": pbutil.VariantHash(pbutil.Variant("k", "v")),
				}))

				// Artifact for a legacy test result.
				req.Requests = []*pb.CreateArtifactRequest{
					{
						Parent: pbutil.LegacyTestResultName(string(invID), "some-legacy-test-id", "result-id-1"),
						Artifact: &pb.Artifact{
							ArtifactId:  "artifact-id-1",
							ContentType: "text/richtext",
							Contents:    []byte("artifact content 1"),
						},
					},
				}
				expectedResponse.Artifacts = []*pb.Artifact{
					{
						Name:        "invocations/inv-id/tests/some-legacy-test-id/results/result-id-1/artifacts/artifact-id-1",
						ArtifactId:  "artifact-id-1",
						ContentType: "text/richtext",
						SizeBytes:   18,
						TestId:      "some-legacy-test-id",
						TestIdStructured: &pb.TestIdentifier{
							ModuleName:        "legacy",
							ModuleScheme:      "legacy",
							ModuleVariant:     pbutil.Variant("k", "v"),
							ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("k", "v")),
							CaseName:          "some-legacy-test-id",
						},
						ResultId: "result-id-1",
						HasLines: true,
					},
				}

				resp, err := recorder.BatchCreateArtifacts(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, resp, should.Match(expectedResponse))
			})
		})
	})
}

func sha256Hash(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:])
}
