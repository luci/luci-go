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
	"context"
	"io"

	"google.golang.org/genproto/googleapis/bytestream"
	"google.golang.org/grpc"
)

type fakeCASReader struct {
	grpc.ClientStream // implements the rest of grpc.ClientStream

	res         []*bytestream.ReadResponse
	resIndex    int
	resErr      error
	resErrIndex int
}

func (r *fakeCASReader) Recv() (*bytestream.ReadResponse, error) {
	if r.resErr != nil && r.resErrIndex == r.resIndex {
		return nil, r.resErr
	}

	if r.resIndex < len(r.res) {
		res := r.res[r.resIndex]
		r.resIndex++
		return res, nil
	}
	return nil, io.EOF
}

type FakeByteStreamClient struct {
	ExtraResponseData []byte
}

func (c *FakeByteStreamClient) Read(ctx context.Context, in *bytestream.ReadRequest, opts ...grpc.CallOption) (bytestream.ByteStream_ReadClient, error) {
	res := []*bytestream.ReadResponse{
		{Data: []byte("contentspart1\n")},
	}
	if len(c.ExtraResponseData) > 0 {
		res = append(res, &bytestream.ReadResponse{Data: c.ExtraResponseData})
	}
	return &fakeCASReader{
		res: res,
	}, nil
}

func (c *FakeByteStreamClient) Write(ctx context.Context, opts ...grpc.CallOption) (bytestream.ByteStream_WriteClient, error) {
	return nil, nil
}

func (c *FakeByteStreamClient) QueryWriteStatus(ctx context.Context, in *bytestream.QueryWriteStatusRequest, opts ...grpc.CallOption) (*bytestream.QueryWriteStatusResponse, error) {
	return nil, nil
}
