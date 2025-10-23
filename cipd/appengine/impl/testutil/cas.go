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

	caspb "go.chromium.org/luci/cipd/api/cipd/v1/caspb"
	casgrpcpb "go.chromium.org/luci/cipd/api/cipd/v1/caspb/grpcpb"
	"go.chromium.org/luci/cipd/appengine/impl/gs"
)

// MockCAS implements cas.StorageServer interface.
type MockCAS struct {
	casgrpcpb.UnimplementedStorageServer

	Err error // an error to return or nil to pass through to the callback

	GetReaderImpl    func(context.Context, *caspb.ObjectRef, string) (gs.Reader, error)
	GetObjectURLImpl func(context.Context, *caspb.GetObjectURLRequest) (*caspb.ObjectURL, error)
	BeginUploadImpl  func(context.Context, *caspb.BeginUploadRequest) (*caspb.UploadOperation, error)
	FinishUploadImpl func(context.Context, *caspb.FinishUploadRequest) (*caspb.UploadOperation, error)
	CancelUploadImpl func(context.Context, *caspb.CancelUploadRequest) (*caspb.UploadOperation, error)
}

// GetReader implements the corresponding method of cas.StorageServer interface.
func (m *MockCAS) GetReader(ctx context.Context, ref *caspb.ObjectRef, userProject string) (gs.Reader, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	if m.GetReaderImpl == nil {
		panic("must not be called")
	}
	return m.GetReaderImpl(ctx, ref, userProject)
}

// GetObjectURL implements the corresponding RPC method, see the proto doc.
func (m *MockCAS) GetObjectURL(ctx context.Context, r *caspb.GetObjectURLRequest) (*caspb.ObjectURL, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	if m.GetObjectURLImpl == nil {
		panic("must not be called")
	}
	return m.GetObjectURLImpl(ctx, r)
}

// BeginUpload implements the corresponding RPC method, see the proto doc.
func (m *MockCAS) BeginUpload(ctx context.Context, r *caspb.BeginUploadRequest) (*caspb.UploadOperation, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	if m.BeginUploadImpl == nil {
		panic("must not be called")
	}
	return m.BeginUploadImpl(ctx, r)
}

// FinishUpload implements the corresponding RPC method, see the proto doc.
func (m *MockCAS) FinishUpload(ctx context.Context, r *caspb.FinishUploadRequest) (*caspb.UploadOperation, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	if m.FinishUploadImpl == nil {
		panic("must not be called")
	}
	return m.FinishUploadImpl(ctx, r)
}

// CancelUpload implements the corresponding RPC method, see the proto doc.
func (m *MockCAS) CancelUpload(ctx context.Context, r *caspb.CancelUploadRequest) (*caspb.UploadOperation, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	if m.CancelUploadImpl == nil {
		panic("must not be called")
	}
	return m.CancelUploadImpl(ctx, r)
}
