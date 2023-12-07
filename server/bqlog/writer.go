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

package bqlog

import (
	"context"

	"cloud.google.com/go/bigquery/storage/apiv1beta2/storagepb"
	gax "github.com/googleapis/gax-go/v2"
	"google.golang.org/grpc/metadata"
)

// BigQueryWriter is a subset of BigQueryWriteClient API used by Bundler.
//
// Use storage.NewBigQueryWriteClient to construct a production instance.
type BigQueryWriter interface {
	AppendRows(ctx context.Context, opts ...gax.CallOption) (storagepb.BigQueryWrite_AppendRowsClient, error)
	Close() error
}

// FakeBigQueryWriter is a fake for BigQueryWriter.
//
// Calls given callbacks if they are non-nil. Useful in tests.
type FakeBigQueryWriter struct {
	Send func(*storagepb.AppendRowsRequest) error
	Recv func() (*storagepb.AppendRowsResponse, error)
}

func (f *FakeBigQueryWriter) AppendRows(ctx context.Context, opts ...gax.CallOption) (storagepb.BigQueryWrite_AppendRowsClient, error) {
	return &noopAppendRowsClient{send: f.Send, recv: f.Recv}, nil
}

func (*FakeBigQueryWriter) Close() error {
	return nil
}

type noopAppendRowsClient struct {
	send func(*storagepb.AppendRowsRequest) error
	recv func() (*storagepb.AppendRowsResponse, error)
}

func (c *noopAppendRowsClient) Send(r *storagepb.AppendRowsRequest) error {
	if c.send != nil {
		return c.send(r)
	}
	return nil
}

func (c *noopAppendRowsClient) Recv() (*storagepb.AppendRowsResponse, error) {
	if c.recv != nil {
		return c.recv()
	}
	return &storagepb.AppendRowsResponse{}, nil
}

func (*noopAppendRowsClient) Header() (metadata.MD, error) { panic("unused") }
func (*noopAppendRowsClient) Trailer() metadata.MD         { panic("unused") }
func (*noopAppendRowsClient) CloseSend() error             { return nil }
func (*noopAppendRowsClient) Context() context.Context     { panic("unused") }
func (*noopAppendRowsClient) SendMsg(m any) error          { panic("unused") }
func (*noopAppendRowsClient) RecvMsg(m any) error          { panic("unused") }
