// Copyright 2017 The LUCI Authors.
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

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	cipdgrpcpb "go.chromium.org/luci/cipd/api/cipd/v1/grpcpb"
	"go.chromium.org/luci/cipd/appengine/impl/gs"
)

// MockCAS implements cas.StorageServer interface.
type MockCAS struct {
	cipdgrpcpb.UnimplementedStorageServer

	Err error // an error to return or nil to pass through to the callback

	GetReaderImpl    func(context.Context, *api.ObjectRef) (gs.Reader, error)
	GetObjectURLImpl func(context.Context, *api.GetObjectURLRequest) (*api.ObjectURL, error)
	BeginUploadImpl  func(context.Context, *api.BeginUploadRequest) (*api.UploadOperation, error)
	FinishUploadImpl func(context.Context, *api.FinishUploadRequest) (*api.UploadOperation, error)
	CancelUploadImpl func(context.Context, *api.CancelUploadRequest) (*api.UploadOperation, error)
}

// GetReader implements the corresponding method of cas.StorageServer interface.
func (m *MockCAS) GetReader(ctx context.Context, ref *api.ObjectRef) (gs.Reader, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	if m.GetReaderImpl == nil {
		panic("must not be called")
	}
	return m.GetReaderImpl(ctx, ref)
}

// GetObjectURL implements the corresponding RPC method, see the proto doc.
func (m *MockCAS) GetObjectURL(ctx context.Context, r *api.GetObjectURLRequest) (*api.ObjectURL, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	if m.GetObjectURLImpl == nil {
		panic("must not be called")
	}
	return m.GetObjectURLImpl(ctx, r)
}

// BeginUpload implements the corresponding RPC method, see the proto doc.
func (m *MockCAS) BeginUpload(ctx context.Context, r *api.BeginUploadRequest) (*api.UploadOperation, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	if m.BeginUploadImpl == nil {
		panic("must not be called")
	}
	return m.BeginUploadImpl(ctx, r)
}

// FinishUpload implements the corresponding RPC method, see the proto doc.
func (m *MockCAS) FinishUpload(ctx context.Context, r *api.FinishUploadRequest) (*api.UploadOperation, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	if m.FinishUploadImpl == nil {
		panic("must not be called")
	}
	return m.FinishUploadImpl(ctx, r)
}

// CancelUpload implements the corresponding RPC method, see the proto doc.
func (m *MockCAS) CancelUpload(ctx context.Context, r *api.CancelUploadRequest) (*api.UploadOperation, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	if m.CancelUploadImpl == nil {
		panic("must not be called")
	}
	return m.CancelUploadImpl(ctx, r)
}
