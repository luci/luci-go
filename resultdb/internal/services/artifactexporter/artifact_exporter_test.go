// Copyright 2024 The LUCI Authors.
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

package artifactexporter

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	"google.golang.org/genproto/googleapis/bytestream"
	"google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/span"

	artifactcontenttest "go.chromium.org/luci/resultdb/internal/artifactcontent/testutil"
	"go.chromium.org/luci/resultdb/internal/checkpoints"
	"go.chromium.org/luci/resultdb/internal/config"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	bqpb "go.chromium.org/luci/resultdb/proto/bq"
	configpb "go.chromium.org/luci/resultdb/proto/config"
	pb "go.chromium.org/luci/resultdb/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

type fakeCASClient struct {
	ResponseData      map[string][]byte
	ResponseErrorCode map[string]int
	Err               error
	DigestHashes      [][]string
	m                 sync.Mutex
}

func (c *fakeCASClient) BatchReadBlobs(ctx context.Context, in *repb.BatchReadBlobsRequest, opts ...grpc.CallOption) (*repb.BatchReadBlobsResponse, error) {
	if c.Err != nil {
		return nil, c.Err
	}
	res := &repb.BatchReadBlobsResponse{}
	hashes := []string{}
	for _, digest := range in.Digests {
		hashes = append(hashes, digest.GetHash())
		if code, ok := c.ResponseErrorCode[digest.Hash]; ok {
			res.Responses = append(res.Responses, &repb.BatchReadBlobsResponse_Response{
				Digest: digest,
				Data:   []byte(""),
				Status: &status.Status{Code: int32(code)},
			})
		} else {
			data := []byte("batchdata")
			if val, ok := c.ResponseData[digest.Hash]; ok {
				data = []byte(val)
			}
			res.Responses = append(res.Responses, &repb.BatchReadBlobsResponse_Response{
				Digest: digest,
				Data:   data,
				Status: &status.Status{Code: int32(codes.OK)},
			})
		}
	}
	c.m.Lock()
	c.DigestHashes = append(c.DigestHashes, hashes)
	c.m.Unlock()
	return res, nil
}

func (c *fakeCASClient) BatchUpdateBlobs(ctx context.Context, in *repb.BatchUpdateBlobsRequest, opts ...grpc.CallOption) (*repb.BatchUpdateBlobsResponse, error) {
	return nil, nil
}

func (c *fakeCASClient) FindMissingBlobs(ctx context.Context, in *repb.FindMissingBlobsRequest, opts ...grpc.CallOption) (*repb.FindMissingBlobsResponse, error) {
	return nil, nil
}

func (c *fakeCASClient) GetTree(ctx context.Context, in *repb.GetTreeRequest, opts ...grpc.CallOption) (repb.ContentAddressableStorage_GetTreeClient, error) {
	return nil, nil
}

type fakeByteStreamClient struct {
	Err          error
	ResponseData map[string][]byte
}

func (c *fakeByteStreamClient) Read(ctx context.Context, in *bytestream.ReadRequest, opts ...grpc.CallOption) (bytestream.ByteStream_ReadClient, error) {
	if c.Err != nil {
		return nil, c.Err
	}

	res := []*bytestream.ReadResponse{
		{Data: []byte("streamdata")},
	}
	if val, ok := c.ResponseData[in.ResourceName]; ok {
		res = []*bytestream.ReadResponse{
			{Data: val},
		}
	}
	return &artifactcontenttest.FakeCASReader{
		Res: res,
	}, nil
}

func (c *fakeByteStreamClient) Write(ctx context.Context, opts ...grpc.CallOption) (bytestream.ByteStream_WriteClient, error) {
	return nil, nil
}

func (c *fakeByteStreamClient) QueryWriteStatus(ctx context.Context, in *bytestream.QueryWriteStatusRequest, opts ...grpc.CallOption) (*bytestream.QueryWriteStatusResponse, error) {
	return nil, nil
}

type fakeBQClient struct {
	Rows  [][]*bqpb.TextArtifactRow
	Error error
}

func (c *fakeBQClient) InsertArtifactRows(ctx context.Context, rows []*bqpb.TextArtifactRow) error {
	if c.Error != nil {
		return c.Error
	}
	c.Rows = append(c.Rows, rows)
	return nil
}

func TestQueryTextArtifacts(t *testing.T) {
	Convey(`Query Text Artifacts`, t, func() {
		ctx := testutil.SpannerTestContext(t)
		ae := artifactExporter{}
		ctx, cancel := span.ReadOnlyTransaction(ctx)
		defer cancel()
		ctx, _ = tsmon.WithDummyInMemory(ctx)

		Convey("query artifacts", func() {
			testutil.MustApply(ctx,
				insert.Invocation("inv1", pb.Invocation_FINALIZED, map[string]any{"Realm": "testproject:testrealm"}),
				insert.Artifact("inv1", "", "a0", map[string]any{"ContentType": "text/plain; encoding=utf-8", "Size": "100", "RBECASHash": "deadbeef"}),
				insert.Artifact("inv1", "tr/testid/0", "a1", map[string]any{"ContentType": "text/html", "Size": "100", "RBECASHash": "deadbeef"}),
				insert.Artifact("inv1", "tr/testid/0", "a2", map[string]any{"ContentType": "image/png", "Size": "100", "RBECASHash": "deadbeef"}),
				insert.Artifact("inv1", "", "a3", map[string]any{"Size": "100"}),
				insert.Artifact("inv1", "tr/testid/0", "a4", map[string]any{"ContentType": "text/html", "Size": "200000000", "RBECASHash": "deadbeef"}),
			)

			testutil.MustApply(ctx, testutil.CombineMutations(
				insert.TestResults("inv1", "testid", &pb.Variant{Def: map[string]string{"os": "linux"}}, pb.TestStatus_PASS),
			)...)

			artifacts, err := ae.queryTextArtifacts(ctx, "inv1", "testproject", 5*1024*1024*1024, map[string]bool{})
			So(err, ShouldBeNil)
			So(len(artifacts), ShouldEqual, 2)
			So(artifacts, ShouldResemble, []*Artifact{
				{
					InvocationID: "inv1",
					TestID:       "",
					ResultID:     "",
					ArtifactID:   "a0",
					ContentType:  "text/plain; encoding=utf-8",
					Size:         100,
					RBECASHash:   "deadbeef",
					TestStatus:   pb.TestStatus_STATUS_UNSPECIFIED,
				},
				{
					InvocationID:    "inv1",
					TestID:          "testid",
					ResultID:        "0",
					ArtifactID:      "a1",
					ContentType:     "text/html",
					Size:            100,
					RBECASHash:      "deadbeef",
					TestStatus:      pb.TestStatus_PASS,
					TestVariant:     &pb.Variant{Def: map[string]string{"os": "linux"}},
					TestVariantHash: "f334f047f88eb721",
				},
			})
			So(artifactContentCounter.Get(ctx, "testproject", "text"), ShouldEqual, 3)
			So(artifactContentCounter.Get(ctx, "testproject", "nontext"), ShouldEqual, 1)
			So(artifactContentCounter.Get(ctx, "testproject", "empty"), ShouldEqual, 1)
			So(artifactExportCounter.Get(ctx, "testproject", "skipped_over_limit"), ShouldEqual, 1)
		})

		Convey("total invocation size exceeds limit", func() {
			testutil.MustApply(ctx,
				insert.Invocation("inv1", pb.Invocation_FINALIZED, map[string]any{"Realm": "testproject:testrealm"}),
				insert.Artifact("inv1", "", "a0", map[string]any{"ContentType": "text/plain; encoding=utf-8", "Size": "100", "RBECASHash": "deadbeef"}),
				insert.Artifact("inv1", "tr/testid/0", "a1", map[string]any{"ContentType": "text/html", "Size": "5000000", "RBECASHash": "deadbeef"}),
				insert.Artifact("inv1", "tr/testid/0", "a2", map[string]any{"ContentType": "image/png", "Size": "100", "RBECASHash": "deadbeef"}),
				insert.Artifact("inv1", "", "a3", map[string]any{"Size": "100"}),
				insert.Artifact("inv1", "tr/testid/0", "a4", map[string]any{"ContentType": "text/html", "Size": "6000000", "RBECASHash": "deadbeef"}),
			)

			testutil.MustApply(ctx, testutil.CombineMutations(
				insert.TestResults("inv1", "testid", &pb.Variant{Def: map[string]string{"os": "linux"}}, pb.TestStatus_PASS),
			)...)

			artifacts, err := ae.queryTextArtifacts(ctx, "inv1", "test project", 10_000_000, map[string]bool{})
			So(err, ShouldBeNil)
			So(len(artifacts), ShouldEqual, 0)
			So(artifactInvocationCounter.Get(ctx, "test project", "skipped_over_limit"), ShouldEqual, 1)
		})

		Convey("With checkpoint", func() {
			testutil.MustApply(ctx,
				insert.Invocation("inv1", pb.Invocation_FINALIZED, map[string]any{"Realm": "testproject:testrealm"}),
				insert.Artifact("inv1", "", "a0", map[string]any{"ContentType": "text/plain; encoding=utf-8", "Size": "100", "RBECASHash": "deadbeef"}),
				insert.Artifact("inv1", "", "a1", map[string]any{"ContentType": "text/html", "Size": "100", "RBECASHash": "deadbeef"}),
			)

			artifacts, err := ae.queryTextArtifacts(ctx, "inv1", "testproject", 5*1024*1024*1024, map[string]bool{"//a1": true})
			So(err, ShouldBeNil)
			So(len(artifacts), ShouldEqual, 1)
			So(artifacts, ShouldResemble, []*Artifact{
				{
					InvocationID: "inv1",
					TestID:       "",
					ResultID:     "",
					ArtifactID:   "a0",
					ContentType:  "text/plain; encoding=utf-8",
					Size:         100,
					RBECASHash:   "deadbeef",
					TestStatus:   pb.TestStatus_STATUS_UNSPECIFIED,
				},
			})
			So(artifactContentCounter.Get(ctx, "testproject", "text"), ShouldEqual, 1)
		})
	})
}

func TestDownloadArtifactContent(t *testing.T) {
	ctx := context.Background()
	Convey(`Download multiple artifact content`, t, func() {
		ctx, _ = tsmon.WithDummyInMemory(ctx)
		casClient := &fakeCASClient{
			ResponseData: map[string][]byte{
				"hash1": []byte("abc"),
				// Invalid UTF-8. It does not result in any row.
				"hash2": {0xFF, 0xFE},
				"hash5": []byte(strings.Repeat("a", 99)),
			},
			ResponseErrorCode: map[string]int{
				"hash6": int(codes.InvalidArgument),
				"hash7": int(codes.NotFound),
				"hash8": int(codes.Internal),
			},
		}
		ae := artifactExporter{
			bytestreamClient: &fakeByteStreamClient{
				ResponseData: map[string][]byte{
					// Need more than 1 shards.
					// each "€" is 3 bytes
					// We need 2 shards.
					// This should be downloaded by stream.
					resourceName("hash3", 450): []byte(strings.Repeat("€", 150)),
					// Invalid data.
					resourceName("hash4", 400): {0xFF, 0xFE},
				},
			},
			casClient: casClient,
		}
		rowC := make(chan *Row, 10)
		artifacts := []*Artifact{
			{
				InvocationID: "inv1",
				TestID:       "",
				ResultID:     "",
				ArtifactID:   "a0",
				ContentType:  "text/plain; encoding=utf-8",
				Size:         3,
				RBECASHash:   "hash1",
				TestStatus:   pb.TestStatus_STATUS_UNSPECIFIED,
			},
			{
				InvocationID: "inv1",
				TestID:       "testid",
				ResultID:     "0",
				ArtifactID:   "a1",
				ContentType:  "text/html",
				Size:         2,
				RBECASHash:   "hash2",
				TestStatus:   pb.TestStatus_PASS,
			},
			{
				InvocationID:    "inv1",
				TestID:          "testid",
				ResultID:        "0",
				ArtifactID:      "a2",
				ContentType:     "text/html",
				Size:            450,
				RBECASHash:      "hash3",
				TestStatus:      pb.TestStatus_FAIL,
				TestVariant:     &pb.Variant{Def: map[string]string{"os": "linux"}},
				TestVariantHash: "f334f047f88eb721",
			},
			{
				InvocationID: "inv1",
				TestID:       "testid",
				ResultID:     "0",
				ArtifactID:   "a3",
				ContentType:  "text/html",
				Size:         400,
				RBECASHash:   "hash4",
				TestStatus:   pb.TestStatus_FAIL,
			},
			{
				InvocationID: "inv1",
				TestID:       "testid",
				ResultID:     "0",
				ArtifactID:   "a4",
				ContentType:  "text/html",
				Size:         97,
				RBECASHash:   "hash5",
				TestStatus:   pb.TestStatus_FAIL,
			},
			{
				InvocationID: "inv1",
				TestID:       "testid",
				ResultID:     "0",
				ArtifactID:   "a5",
				ContentType:  "text/html",
				Size:         98,
				RBECASHash:   "hash6",
				TestStatus:   pb.TestStatus_FAIL,
			},
			{
				InvocationID: "inv1",
				TestID:       "testid",
				ResultID:     "0",
				ArtifactID:   "a6",
				ContentType:  "text/html",
				Size:         99,
				RBECASHash:   "hash7",
				TestStatus:   pb.TestStatus_FAIL,
			},
		}
		inv := &pb.Invocation{
			Name:                   "invocations/inv1",
			Realm:                  "chromium:ci",
			CreateTime:             timestamppb.New(time.Unix(10000, 0).UTC()),
			TestResultVariantUnion: &pb.Variant{Def: map[string]string{"os": "linux", "runner": "test"}},
		}
		Convey("Batch failed", func() {
			ae.casClient = &fakeCASClient{
				Err: errors.New("batch failed"),
			}
			err := ae.downloadMultipleArtifactContent(ctx, artifacts, inv, rowC, 100, 1000, map[string]bool{})
			So(err, ShouldErrLike, "batch failed")
		})

		Convey("Invalid argument should not retry", func() {
			ae.casClient = &fakeCASClient{
				Err: grpcstatus.New(codes.InvalidArgument, "invalid argument").Err(),
			}
			err := ae.downloadMultipleArtifactContent(ctx, artifacts, inv, rowC, 100, 300, map[string]bool{})
			So(err, ShouldBeNil)
		})

		Convey("Resource Exhausted should not retry", func() {
			ae.casClient = &fakeCASClient{
				Err: grpcstatus.New(codes.ResourceExhausted, "resource exhausted").Err(),
			}
			err := ae.downloadMultipleArtifactContent(ctx, artifacts, inv, rowC, 100, 300, map[string]bool{})
			So(err, ShouldBeNil)
		})

		Convey("Stream failed", func() {
			ae.bytestreamClient = &fakeByteStreamClient{
				Err: errors.New("stream failed"),
			}
			err := ae.downloadMultipleArtifactContent(ctx, artifacts, inv, rowC, 100, 100, map[string]bool{})
			So(err, ShouldErrLike, "stream failed")
		})

		Convey("Succeed", func() {
			err := ae.downloadMultipleArtifactContent(ctx, artifacts, inv, rowC, 300, 300, map[string]bool{})
			So(err, ShouldBeNil)
			close(rowC)
			rows := []*Row{}
			for r := range rowC {
				rows = append(rows, r)
			}
			// Sort the rows for deterministism.
			sort.Slice(rows, func(i, j int) bool {
				return (rows[i].content.ArtifactId < rows[j].content.ArtifactId ||
					(rows[i].content.ArtifactId == rows[j].content.ArtifactId && rows[i].content.ShardId < rows[j].content.ShardId))
			})
			So(rows, ShouldResembleProto, []*Row{
				{
					content: &bqpb.TextArtifactRow{
						Project:                    "chromium",
						Realm:                      "ci",
						InvocationId:               "inv1",
						TestId:                     "",
						ResultId:                   "",
						ArtifactId:                 "a0",
						ShardId:                    0,
						ContentType:                "text/plain; encoding=utf-8",
						Content:                    "abc",
						ArtifactContentSize:        int32(3),
						ShardContentSize:           int32(3),
						PartitionTime:              timestamppb.New(time.Unix(10000, 0).UTC()),
						TestStatus:                 "",
						TestVariant:                "{}",
						InvocationVariantUnion:     `{"os":"linux","runner":"test"}`,
						InvocationVariantUnionHash: "a07aa2ca8acbfc88",
					},
					isLastShard: true,
				},
				{
					content: &bqpb.TextArtifactRow{
						Project:                "chromium",
						Realm:                  "ci",
						InvocationId:           "inv1",
						TestId:                 "testid",
						ResultId:               "0",
						ArtifactId:             "a2",
						ShardId:                0,
						ContentType:            "text/html",
						Content:                strings.Repeat("€", 100),
						ArtifactContentSize:    int32(450),
						ShardContentSize:       int32(300),
						PartitionTime:          timestamppb.New(time.Unix(10000, 0).UTC()),
						TestStatus:             "FAIL",
						TestVariant:            `{"os":"linux"}`,
						TestVariantHash:        "f334f047f88eb721",
						InvocationVariantUnion: "{}",
					},
					isLastShard: false,
				},
				{
					content: &bqpb.TextArtifactRow{
						Project:                "chromium",
						Realm:                  "ci",
						InvocationId:           "inv1",
						TestId:                 "testid",
						ResultId:               "0",
						ArtifactId:             "a2",
						ShardId:                1,
						ContentType:            "text/html",
						Content:                strings.Repeat("€", 50),
						ArtifactContentSize:    int32(450),
						ShardContentSize:       int32(150),
						PartitionTime:          timestamppb.New(time.Unix(10000, 0).UTC()),
						TestStatus:             "FAIL",
						TestVariant:            `{"os":"linux"}`,
						TestVariantHash:        "f334f047f88eb721",
						InvocationVariantUnion: "{}",
					},
					isLastShard: true,
				},
				{
					content: &bqpb.TextArtifactRow{
						Project:                "chromium",
						Realm:                  "ci",
						InvocationId:           "inv1",
						TestId:                 "testid",
						ResultId:               "0",
						ArtifactId:             "a4",
						ShardId:                0,
						ContentType:            "text/html",
						Content:                strings.Repeat("a", 99),
						ArtifactContentSize:    int32(97),
						ShardContentSize:       int32(97),
						PartitionTime:          timestamppb.New(time.Unix(10000, 0).UTC()),
						TestStatus:             "FAIL",
						TestVariant:            "{}",
						InvocationVariantUnion: "{}",
					},
					isLastShard: true,
				},
			})
			// Make sure we do the chunking properly.
			sort.Slice(casClient.DigestHashes, func(i, j int) bool {
				return (casClient.DigestHashes[i][0] < casClient.DigestHashes[j][0])
			})
			So(casClient.DigestHashes, ShouldResemble, [][]string{
				{"hash2", "hash1"},
				{"hash5"},
				{"hash6"},
				{"hash7"},
			})
			So(artifactExportCounter.Get(ctx, "chromium", "failure_input"), ShouldEqual, 4)
		})

		Convey("One batch error", func() {
			artifacts := []*Artifact{
				{
					InvocationID: "inv1",
					TestID:       "",
					ResultID:     "",
					ArtifactID:   "a0",
					ContentType:  "text/plain; encoding=utf-8",
					Size:         3,
					RBECASHash:   "hash8",
					TestStatus:   pb.TestStatus_PASS,
				},
			}
			err := ae.downloadMultipleArtifactContent(ctx, artifacts, inv, rowC, 100, 300, map[string]bool{})
			So(err, ShouldErrLike, "downloading artifact")
			close(rowC)
		})
	})
}

func TestExportArtifacts(t *testing.T) {
	Convey("Export Artifacts", t, func() {
		ctx := testutil.SpannerTestContext(t)
		ctx = memory.Use(ctx)
		bqClient := &fakeBQClient{}
		ae := artifactExporter{
			bytestreamClient: &fakeByteStreamClient{
				ResponseData: map[string][]byte{
					resourceName("hash4", 15000000): []byte(strings.Repeat("a", 15000000)),
				},
			},
			bqExportClient: bqClient,
			casClient:      &fakeCASClient{},
		}
		ctx, _ = tsmon.WithDummyInMemory(ctx)

		Convey("Export disabled", func() {
			err := config.SetServiceConfig(ctx, &configpb.Config{
				BqArtifactExporterServiceConfig: &configpb.BqArtifactExportConfig{
					Enabled: false,
				},
			})
			So(err, ShouldBeNil)
			err = ae.exportArtifacts(ctx, "inv1")
			So(err, ShouldBeNil)
		})

		err := config.SetServiceConfig(ctx, &configpb.Config{
			BqArtifactExporterServiceConfig: &configpb.BqArtifactExportConfig{
				Enabled:       true,
				ExportPercent: 100,
			},
		})
		So(err, ShouldBeNil)

		invocationVariantUnion := &pb.Variant{Def: map[string]string{"os": "linux", "runner": "test"}}
		commitTime := testutil.MustApply(ctx,
			insert.Invocation("inv-1", pb.Invocation_FINALIZED, map[string]any{"Realm": "testproject:testrealm", "TestResultVariantUnion": invocationVariantUnion}),
			insert.Invocation("inv-2", pb.Invocation_ACTIVE, map[string]any{"Realm": "testproject:testrealm"}),
			insert.Artifact("inv-1", "", "a0", map[string]any{"ContentType": "text/plain; encoding=utf-8", "Size": "4", "RBECASHash": "deadbeef"}),
			insert.Artifact("inv-1", "tr/testid/0", "a1", map[string]any{"ContentType": "text/html", "Size": "4", "RBECASHash": "deadbeef"}),
			insert.Artifact("inv-1", "tr/testid/0", "a2", map[string]any{"ContentType": "image/png", "Size": "100", "RBECASHash": "deadbeef"}),
			// a3 should not be exported again since it has checkpoint.
			insert.Artifact("inv-1", "tr/testid/0", "a3", map[string]any{"ContentType": "text/html", "Size": "4", "RBECASHash": "deadbeef"}),
			insert.Artifact("inv-1", "tr/testid/0", "a4", map[string]any{"ContentType": "text/html", "Size": "15000000", "RBECASHash": "hash4"}),
			insert.Checkpoint(ctx, "testproject", "inv-1", CheckpointProcessID, "testid/0/a3"),
			// Only part of the a4 should be exported.
			insert.Checkpoint(ctx, "testproject", "inv-1", CheckpointProcessID, "testid/0/a4/0"),
		)

		testutil.MustApply(ctx, testutil.CombineMutations(
			insert.TestResults("inv-1", "testid", &pb.Variant{Def: map[string]string{"os": "linux"}}, pb.TestStatus_PASS),
		)...)

		Convey("Invocation not finalized", func() {
			err = ae.exportArtifacts(ctx, "inv-2")
			So(err, ShouldErrLike, "invocation not finalized")
		})

		Convey("BQ Export failed", func() {
			ae.bqExportClient = &fakeBQClient{
				Error: errors.New("bq error"),
			}
			err = ae.exportArtifacts(ctx, "inv-1")
			So(err, ShouldErrLike, "bq error")
			So(artifactExportCounter.Get(ctx, "testproject", "failure_bq"), ShouldEqual, 3)
		})

		Convey("Succeed", func() {
			err = ae.exportArtifacts(ctx, "inv-1")
			So(err, ShouldBeNil)
			// Sort to make deterministic, as we parallelized the downloading,
			// so it is not certain which row will come first.
			rows := []*bqpb.TextArtifactRow{}
			for _, r := range bqClient.Rows {
				rows = append(rows, r...)
			}
			sort.Slice(rows, func(i, j int) bool {
				return (rows[i].ArtifactId < rows[j].ArtifactId)
			})

			So(rows, ShouldResembleProto, []*bqpb.TextArtifactRow{
				{
					Project:                    "testproject",
					Realm:                      "testrealm",
					InvocationId:               "inv-1",
					TestId:                     "",
					ResultId:                   "",
					ArtifactId:                 "a0",
					ShardId:                    0,
					ContentType:                "text/plain; encoding=utf-8",
					Content:                    "batchdata",
					ArtifactContentSize:        4,
					ShardContentSize:           4,
					PartitionTime:              timestamppb.New(commitTime),
					TestVariant:                "{}",
					InvocationVariantUnion:     `{"os":"linux","runner":"test"}`,
					InvocationVariantUnionHash: "a07aa2ca8acbfc88",
				},
				{
					Project:                    "testproject",
					Realm:                      "testrealm",
					InvocationId:               "inv-1",
					TestId:                     "testid",
					ResultId:                   "0",
					ArtifactId:                 "a1",
					ShardId:                    0,
					ContentType:                "text/html",
					Content:                    "batchdata",
					ArtifactContentSize:        4,
					TestStatus:                 "PASS",
					ShardContentSize:           4,
					PartitionTime:              timestamppb.New(commitTime),
					TestVariant:                `{"os":"linux"}`,
					TestVariantHash:            "f334f047f88eb721",
					InvocationVariantUnion:     "{}",
					InvocationVariantUnionHash: "",
				},
				{
					Project:                    "testproject",
					Realm:                      "testrealm",
					InvocationId:               "inv-1",
					TestId:                     "testid",
					ResultId:                   "0",
					ArtifactId:                 "a4",
					ShardId:                    1,
					ContentType:                "text/html",
					Content:                    strings.Repeat("a", 6010240),
					ArtifactContentSize:        15000000,
					TestStatus:                 "PASS",
					ShardContentSize:           6010240,
					PartitionTime:              timestamppb.New(commitTime),
					TestVariant:                `{"os":"linux"}`,
					TestVariantHash:            "f334f047f88eb721",
					InvocationVariantUnion:     "{}",
					InvocationVariantUnionHash: "",
				},
			})
			So(artifactExportCounter.Get(ctx, "testproject", "success"), ShouldEqual, 3)
			So(artifactInvocationCounter.Get(ctx, "testproject", "success"), ShouldEqual, 1)

			// Check that new checkpoints are created.
			uqs, err := checkpoints.ReadAllUniquifiers(span.Single(ctx), "testproject", "inv-1", CheckpointProcessID)
			So(err, ShouldBeNil)
			So(uqs, ShouldResemble, map[string]bool{
				"//a0":          true,
				"testid/0/a1":   true,
				"testid/0/a3":   true,
				"testid/0/a4/0": true,
				"testid/0/a4":   true,
			})
		})
	})
}

func TestExportArtifactsToBigQuery(t *testing.T) {
	Convey("Export Artifacts To BigQuery", t, func() {
		ctx := testutil.SpannerTestContext(t)
		ctx = memory.Use(ctx)
		ctx, _ = tsmon.WithDummyInMemory(ctx)
		bqClient := &fakeBQClient{}
		ae := artifactExporter{
			bytestreamClient: &fakeByteStreamClient{},
			bqExportClient:   bqClient,
		}

		rowC := make(chan *Row, 10)
		rows := []*bqpb.TextArtifactRow{}
		// Insert 3 artifacts, each of size ~4MB to rowC.
		// With the batch size of ~10MB, we will need 2 batches.
		for i := 0; i < 3; i++ {
			row := &bqpb.TextArtifactRow{
				Project:             "project",
				Realm:               "realm",
				InvocationId:        "inv",
				TestId:              "test",
				ResultId:            "result",
				ArtifactId:          fmt.Sprintf("artifact%d", i),
				ShardId:             0,
				ContentType:         "text/plain",
				ArtifactContentSize: 4 * 1024 * 1024,
				Content:             strings.Repeat("a", 4*1024*1024),
				ShardContentSize:    4 * 1024 * 1024,
				TestStatus:          "PASS",
				PartitionTime:       timestamppb.New(time.Unix(10000, 0)),
			}
			rows = append(rows, row)
			rowC <- &Row{
				content:     row,
				isLastShard: i == 0,
			}
		}

		close(rowC)

		Convey("Grouping", func() {
			err := ae.exportToBigQuery(ctx, rowC)
			So(err, ShouldBeNil)

			So(bqClient.Rows, ShouldResembleProto, [][]*bqpb.TextArtifactRow{
				{
					rows[0], rows[1],
				},
				{
					rows[2],
				},
			})
		})
		So(artifactExportCounter.Get(ctx, "project", "success"), ShouldEqual, 3)
		// Check that new checkpoints are created.
		uqs, err := checkpoints.ReadAllUniquifiers(span.Single(ctx), "project", "inv", CheckpointProcessID)
		So(err, ShouldBeNil)
		So(uqs, ShouldResemble, map[string]bool{
			"test/result/artifact0":   true,
			"test/result/artifact1/0": true,
			"test/result/artifact2/0": true,
		})
	})
}

func TestThrottleArtifacts(t *testing.T) {
	Convey("Throttle artifact", t, func() {
		artReqs := []*Artifact{
			{
				TestID:     "test",
				ArtifactID: "artifact38", // Hash value 0
			},
			{
				TestID:     "test",
				ArtifactID: "artifact158", // Hash value 99
			},
			{
				TestID:     "test",
				ArtifactID: "artifact230", // Hash value 32
			},
			{
				TestID:     "test",
				ArtifactID: "artifact232", // Hash value 54
			},
			{
				TestID:     "test",
				ArtifactID: "artifact341", // Hash value 91
			},
		}
		// 0%.
		results, err := throttleArtifactsForBQ(artReqs, 0)
		So(err, ShouldBeNil)
		So(results, ShouldResemble, []*Artifact{})

		// 1%.
		results, err = throttleArtifactsForBQ(artReqs, 1)
		So(err, ShouldBeNil)
		So(results, ShouldResemble, []*Artifact{
			{
				TestID:     "test",
				ArtifactID: "artifact158", // Hash value 99
			},
		})

		// 33%.
		results, err = throttleArtifactsForBQ(artReqs, 33)
		So(err, ShouldBeNil)
		So(results, ShouldResemble, []*Artifact{
			{
				TestID:     "test",
				ArtifactID: "artifact158", // Hash value 99
			},
			{
				TestID:     "test",
				ArtifactID: "artifact341", // Hash value 91
			},
		})

		// 90%.
		results, err = throttleArtifactsForBQ(artReqs, 90)
		So(err, ShouldBeNil)
		So(results, ShouldResemble, []*Artifact{
			{
				TestID:     "test",
				ArtifactID: "artifact158", // Hash value 0
			},
			{
				TestID:     "test",
				ArtifactID: "artifact230", // Hash value 32
			},
			{
				TestID:     "test",
				ArtifactID: "artifact232", // Hash value 54
			},
			{
				TestID:     "test",
				ArtifactID: "artifact341", // Hash value 91
			},
		})

		// 100%.
		results, err = throttleArtifactsForBQ(artReqs, 100)
		So(err, ShouldBeNil)
		So(results, ShouldResemble, []*Artifact{
			{
				TestID:     "test",
				ArtifactID: "artifact38", // Hash value 0
			},
			{
				TestID:     "test",
				ArtifactID: "artifact158", // Hash value 99
			},
			{
				TestID:     "test",
				ArtifactID: "artifact230", // Hash value 32
			},
			{
				TestID:     "test",
				ArtifactID: "artifact232", // Hash value 54
			},
			{
				TestID:     "test",
				ArtifactID: "artifact341", // Hash value 91
			},
		})
	})
}

func resourceName(hash string, size int) string {
	return fmt.Sprintf("/blobs/%s/%d", hash, size)
}
