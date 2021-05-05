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
	ResDataPos  int
	ResErr      error
	ResErrIndex int

	ReadLimit int
	nRead     int
}

func (r *FakeCASReader) Recv() (*bytestream.ReadResponse, error) {
	if r.ResErr != nil && r.ResErrIndex == r.ResIndex {
		return nil, r.ResErr
	}

	limitAvail := r.ReadLimit - r.nRead
	if r.ReadLimit > 0 && limitAvail == 0 {
		return nil, io.EOF
	}

	if r.ResIndex < len(r.Res) {
		// calculate how much data can be read further from the current Res.
		res := r.Res[r.ResIndex]
		nRead := len(res.Data) - r.ResDataPos
		if r.ReadLimit != 0 && nRead > limitAvail {
			nRead = limitAvail
		}

		data := res.Data[r.ResDataPos : r.ResDataPos+nRead]
		r.nRead += nRead
		r.ResDataPos += nRead
		if r.ResDataPos == len(res.Data) {
			r.ResDataPos = 0
			r.ResIndex++
		}
		return &bytestream.ReadResponse{Data: data}, nil
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
