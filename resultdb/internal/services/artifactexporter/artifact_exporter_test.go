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
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"google.golang.org/genproto/googleapis/bytestream"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/span"

	artifactcontenttest "go.chromium.org/luci/resultdb/internal/artifactcontent/testutil"
	"go.chromium.org/luci/resultdb/internal/config"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	bqpb "go.chromium.org/luci/resultdb/proto/bq"
	configpb "go.chromium.org/luci/resultdb/proto/config"
	pb "go.chromium.org/luci/resultdb/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

type FakeByteStreamClient struct {
	ResponseData map[string][]byte
}

func (c *FakeByteStreamClient) Read(ctx context.Context, in *bytestream.ReadRequest, opts ...grpc.CallOption) (bytestream.ByteStream_ReadClient, error) {
	res := []*bytestream.ReadResponse{
		{Data: []byte("data")},
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

func (c *FakeByteStreamClient) Write(ctx context.Context, opts ...grpc.CallOption) (bytestream.ByteStream_WriteClient, error) {
	return nil, nil
}

func (c *FakeByteStreamClient) QueryWriteStatus(ctx context.Context, in *bytestream.QueryWriteStatusRequest, opts ...grpc.CallOption) (*bytestream.QueryWriteStatusResponse, error) {
	return nil, nil
}

func TestQueryTextArtifacts(t *testing.T) {
	Convey(`Query Text Artifacts`, t, func() {
		ctx := testutil.SpannerTestContext(t)
		ae := artifactExporter{}
		ctx, cancel := span.ReadOnlyTransaction(ctx)
		defer cancel()

		testutil.MustApply(ctx,
			insert.Invocation("inv1", pb.Invocation_FINALIZED, map[string]any{"Realm": "testproject:testrealm"}),
			insert.Artifact("inv1", "", "a0", map[string]any{"ContentType": "text/plain; encoding=utf-8", "Size": "100", "RBECASHash": "deadbeef"}),
			insert.Artifact("inv1", "tr/testid/0", "a1", map[string]any{"ContentType": "text/html", "Size": "100", "RBECASHash": "deadbeef"}),
			insert.Artifact("inv1", "tr/testid/0", "a2", map[string]any{"ContentType": "image/png", "Size": "100", "RBECASHash": "deadbeef"}),
		)

		testutil.MustApply(ctx, testutil.CombineMutations(
			insert.TestResults("inv1", "testid", nil, pb.TestStatus_PASS),
		)...)

		artifacts, err := ae.queryTextArtifacts(ctx, "inv1")
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
				InvocationID: "inv1",
				TestID:       "testid",
				ResultID:     "0",
				ArtifactID:   "a1",
				ContentType:  "text/html",
				Size:         100,
				RBECASHash:   "deadbeef",
				TestStatus:   pb.TestStatus_PASS,
			},
		})
	})
}

func TestDownloadArtifactContent(t *testing.T) {
	ae := artifactExporter{
		rbecasClient: &FakeByteStreamClient{
			ResponseData: map[string][]byte{
				resourceName("hash1", 3): []byte("abc"),
				// Invalid UTF-8. It does not result in any row.
				resourceName("hash2", 2): {0xFF, 0xFE},
				// Need more than 1 shards.
				// each "€" is 3 bytes
				// We need 2 shards.
				resourceName("hash3", 150): []byte(strings.Repeat("€", 50)),
			},
		},
	}
	ctx := context.Background()
	Convey(`Download multiple artifact content`, t, func() {
		rowC := make(chan *bqpb.TextArtifactRow, 10)
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
				InvocationID: "inv1",
				TestID:       "testid",
				ResultID:     "0",
				ArtifactID:   "a2",
				ContentType:  "text/html",
				Size:         150,
				RBECASHash:   "hash3",
				TestStatus:   pb.TestStatus_FAIL,
			},
		}
		inv := &pb.Invocation{
			Realm:      "chromium:ci",
			CreateTime: timestamppb.New(time.Unix(10000, 0).UTC()),
		}
		err := ae.downloadMultipleArtifactContent(ctx, artifacts, inv, rowC, 100)
		So(err, ShouldBeNil)
		close(rowC)
		rows := []*bqpb.TextArtifactRow{}
		for r := range rowC {
			rows = append(rows, r)
		}
		So(len(rows), ShouldEqual, 3)
		// Sort the rows for deterministism.
		sort.Slice(rows, func(i, j int) bool {
			return (rows[i].ArtifactShard < rows[j].ArtifactShard)
		})
		So(rows, ShouldResembleProto, []*bqpb.TextArtifactRow{
			{
				Project:             "chromium",
				Realm:               "ci",
				InvocationId:        "inv1",
				TestId:              "",
				ResultId:            "",
				ArtifactId:          "a0",
				ShardId:             0,
				ContentType:         "text/plain; encoding=utf-8",
				Content:             "abc",
				ArtifactContentSize: int32(3),
				ShardContentSize:    int32(3),
				PartitionTime:       timestamppb.New(time.Unix(10000, 0).UTC()),
				ArtifactShard:       "a0:0",
				TestStatus:          "",
			},
			{
				Project:             "chromium",
				Realm:               "ci",
				InvocationId:        "inv1",
				TestId:              "testid",
				ResultId:            "0",
				ArtifactId:          "a2",
				ShardId:             0,
				ContentType:         "text/html",
				Content:             strings.Repeat("€", 33),
				ArtifactContentSize: int32(150),
				ShardContentSize:    int32(99),
				PartitionTime:       timestamppb.New(time.Unix(10000, 0).UTC()),
				ArtifactShard:       "a2:0",
				TestStatus:          "FAIL",
			},
			{
				Project:             "chromium",
				Realm:               "ci",
				InvocationId:        "inv1",
				TestId:              "testid",
				ResultId:            "0",
				ArtifactId:          "a2",
				ShardId:             1,
				ContentType:         "text/html",
				Content:             strings.Repeat("€", 17),
				ArtifactContentSize: int32(150),
				ShardContentSize:    int32(51),
				PartitionTime:       timestamppb.New(time.Unix(10000, 0).UTC()),
				ArtifactShard:       "a2:1",
				TestStatus:          "FAIL",
			},
		})
	})
}

func TestExportArtifacts(t *testing.T) {
	Convey("Export Artifacts", t, func() {
		ctx := testutil.SpannerTestContext(t)
		ctx = memory.Use(ctx)
		ae := artifactExporter{
			rbecasClient: &FakeByteStreamClient{},
		}

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

		testutil.MustApply(ctx,
			insert.Invocation("inv-1", pb.Invocation_FINALIZED, map[string]any{"Realm": "testproject:testrealm"}),
			insert.Invocation("inv-2", pb.Invocation_ACTIVE, map[string]any{"Realm": "testproject:testrealm"}),
			insert.Artifact("inv-1", "", "a0", map[string]any{"ContentType": "text/plain; encoding=utf-8", "Size": "100", "RBECASHash": "deadbeef"}),
			insert.Artifact("inv-1", "tr/testid/0", "a1", map[string]any{"ContentType": "text/html", "Size": "100", "RBECASHash": "deadbeef"}),
			insert.Artifact("inv-1", "tr/testid/0", "a2", map[string]any{"ContentType": "image/png", "Size": "100", "RBECASHash": "deadbeef"}),
		)

		testutil.MustApply(ctx, testutil.CombineMutations(
			insert.TestResults("inv-1", "testid", nil, pb.TestStatus_PASS),
		)...)

		Convey("Invocation not finalized", func() {
			err = ae.exportArtifacts(ctx, "inv-2")
			So(err, ShouldErrLike, "invocation not finalized")
		})

		Convey("Succeed", func() {
			err = ae.exportArtifacts(ctx, "inv-1")
			So(err, ShouldBeNil)
			// TODO (nqmtuan): Check Bigquery after implementing exporting to BQ.
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
