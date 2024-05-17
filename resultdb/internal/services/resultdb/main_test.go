// Copyright 2019 The LUCI Authors.
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
	"testing"

	"google.golang.org/genproto/googleapis/bytestream"

	"go.chromium.org/luci/resultdb/internal"
	"go.chromium.org/luci/resultdb/internal/artifactcontent"
	artifactcontenttest "go.chromium.org/luci/resultdb/internal/artifactcontent/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestMain(m *testing.M) {
	testutil.SpannerTestMain(m)
}

func newTestResultDBService() pb.ResultDBServer {
	return newTestResultDBServiceWithArtifactContent("contents")
}

func newTestResultDBServiceWithArtifactContent(artifactContent string) pb.ResultDBServer {
	casReader := &artifactcontenttest.FakeCASReader{
		Res: []*bytestream.ReadResponse{
			{Data: []byte(artifactContent)},
		},
	}
	svr := &resultDBServer{
		contentServer: &artifactcontent.Server{
			HostnameProvider: func(string) string {
				return "signed-url.example.com"
			},
			RBECASInstanceName: "projects/example/instances/artifacts",
			ReadCASBlob: func(ctx context.Context, req *bytestream.ReadRequest) (bytestream.ByteStream_ReadClient, error) {
				casReader.ReadLimit = int(req.ReadLimit)
				return casReader, nil
			},
		},
	}
	return &pb.DecoratedResultDB{
		Service:  svr,
		Postlude: internal.CommonPostlude,
	}
}
