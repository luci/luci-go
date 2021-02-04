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

package artifactcontent

import (
	"bufio"
	"context"
	"io"
	"strings"
	"testing"

	"google.golang.org/genproto/googleapis/bytestream"
	"google.golang.org/grpc"

	"go.chromium.org/luci/resultdb/internal/testutil"

	. "github.com/smartystreets/goconvey/convey"
)

type fakeByteStreamClient struct{}

func (c *fakeByteStreamClient) Read(ctx context.Context, in *bytestream.ReadRequest, opts ...grpc.CallOption) (bytestream.ByteStream_ReadClient, error) {
	return &fakeCASReader{
		res: []*bytestream.ReadResponse{
			{Data: []byte("contentspart1\n")},
			{Data: []byte("contentspart2\n")},
		},
	}, nil
}

func (c *fakeByteStreamClient) Write(ctx context.Context, opts ...grpc.CallOption) (bytestream.ByteStream_WriteClient, error) {
	return nil, nil
}

func (c *fakeByteStreamClient) QueryWriteStatus(ctx context.Context, in *bytestream.QueryWriteStatusRequest, opts ...grpc.CallOption) (*bytestream.QueryWriteStatusResponse, error) {
	return nil, nil
}

func TestDownloadRBECASContent(t *testing.T) {
	Convey(`TestDownloadRBECASContent`, t, func() {
		ctx := testutil.TestingContext()

		ac := &Reader{
			RBEInstance: "projects/p/instances/a",
			Hash:        "deadbeef",
			Size:        int64(10),
		}

		var str strings.Builder
		err := ac.DownloadRBECASContent(ctx, &fakeByteStreamClient{}, func(pr io.Reader) error {
			sc := bufio.NewScanner(pr)
			for sc.Scan() {
				str.Write(sc.Bytes())
				str.Write([]byte("\n"))
			}
			return nil
		})
		So(err, ShouldBeNil)
		So(str.String(), ShouldEqual, "contentspart1\ncontentspart2\n")
	})
}
