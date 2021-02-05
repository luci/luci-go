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

package testutil

import (
	"context"
	"io"

	"google.golang.org/genproto/googleapis/bytestream"
	"google.golang.org/grpc"
)

type FakeCASReader struct {
	grpc.ClientStream // implements the rest of grpc.ClientStream

	Res         []*bytestream.ReadResponse
	ResIndex    int
	ResErr      error
	ResErrIndex int
}

func (r *FakeCASReader) Recv() (*bytestream.ReadResponse, error) {
	if r.ResErr != nil && r.ResErrIndex == r.ResIndex {
		return nil, r.ResErr
	}

	if r.ResIndex < len(r.Res) {
		res := r.Res[r.ResIndex]
		r.ResIndex++
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
	return &FakeCASReader{
		Res: res,
	}, nil
}

func (c *FakeByteStreamClient) Write(ctx context.Context, opts ...grpc.CallOption) (bytestream.ByteStream_WriteClient, error) {
	return nil, nil
}

func (c *FakeByteStreamClient) QueryWriteStatus(ctx context.Context, in *bytestream.QueryWriteStatusRequest, opts ...grpc.CallOption) (*bytestream.QueryWriteStatusResponse, error) {
	return nil, nil
}
