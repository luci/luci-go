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

	gax "github.com/googleapis/gax-go/v2"
	storagepb "google.golang.org/genproto/googleapis/cloud/bigquery/storage/v1beta2"
	"google.golang.org/grpc/metadata"
)

// BigQueryWriter is a subset of BigQueryWriteClient API used by Bundler.
//
// Use storage.NewBigQueryWriteClient to construct a production instance.
type BigQueryWriter interface {
	AppendRows(ctx context.Context, opts ...gax.CallOption) (storagepb.BigQueryWrite_AppendRowsClient, error)
	Close() error
}

// NoopBigQueryWriter ignores all calls by pretending they succeed.
type NoopBigQueryWriter struct {
}

func (*NoopBigQueryWriter) AppendRows(ctx context.Context, opts ...gax.CallOption) (storagepb.BigQueryWrite_AppendRowsClient, error) {
	return &noopAppendRowsClient{}, nil
}

func (*NoopBigQueryWriter) Close() error {
	return nil
}

type noopAppendRowsClient struct {
}

func (*noopAppendRowsClient) Send(*storagepb.AppendRowsRequest) error {
	return nil
}

func (*noopAppendRowsClient) Recv() (*storagepb.AppendRowsResponse, error) {
	return &storagepb.AppendRowsResponse{}, nil
}

func (*noopAppendRowsClient) Header() (metadata.MD, error) { panic("unused") }
func (*noopAppendRowsClient) Trailer() metadata.MD         { panic("unused") }
func (*noopAppendRowsClient) CloseSend() error             { return nil }
func (*noopAppendRowsClient) Context() context.Context     { panic("unused") }
func (*noopAppendRowsClient) SendMsg(m interface{}) error  { panic("unused") }
func (*noopAppendRowsClient) RecvMsg(m interface{}) error  { panic("unused") }
