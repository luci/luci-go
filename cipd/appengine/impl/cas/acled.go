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

package cas

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	caspb "go.chromium.org/luci/cipd/api/cipd/v1/caspb"
	casgrpcpb "go.chromium.org/luci/cipd/api/cipd/v1/caspb/grpcpb"
	"go.chromium.org/luci/cipd/appengine/impl/rpcacl"
)

// Public returns publicly exposed implementation of cipd.Storage service that
// wraps the given internal implementation with ACLs.
func Public(internal casgrpcpb.StorageServer) casgrpcpb.StorageServer {
	return &acledStorage{internal: internal}
}

type acledStorage struct {
	// We want the compilation to fail when new methods are added.
	casgrpcpb.UnsafeStorageServer

	internal casgrpcpb.StorageServer
}

func (s *acledStorage) GetObjectURL(ctx context.Context, req *caspb.GetObjectURLRequest) (*caspb.ObjectURL, error) {
	if err := rpcacl.CheckAdmin(ctx); err != nil {
		return nil, err
	}
	return s.internal.GetObjectURL(ctx, req)
}

func (s *acledStorage) BeginUpload(ctx context.Context, req *caspb.BeginUploadRequest) (*caspb.UploadOperation, error) {
	if err := rpcacl.CheckAdmin(ctx); err != nil {
		return nil, err
	}
	return s.internal.BeginUpload(ctx, req)
}

// Upload operations are initiated by the backend, but finalized by whoever
// uploads the data, thus 'FinishUpload' and 'CancelUpload' is accessible to
// anyone (the authorization happens through upload operation IDs which should
// be treated as secrets). Except we don't trust external API users to assign
// hashes, so usage of 'force_hash' field is forbidden.

func (s *acledStorage) FinishUpload(ctx context.Context, req *caspb.FinishUploadRequest) (*caspb.UploadOperation, error) {
	if req.ForceHash != nil {
		return nil, status.Errorf(codes.PermissionDenied, "usage of 'force_hash' is forbidden")
	}
	return s.internal.FinishUpload(ctx, req)
}

func (s *acledStorage) CancelUpload(ctx context.Context, req *caspb.CancelUploadRequest) (*caspb.UploadOperation, error) {
	return s.internal.CancelUpload(ctx, req)
}
