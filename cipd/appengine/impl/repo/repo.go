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

package repo

import (
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/grpc/grpcutil"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
)

// Public returns publicly exposed implementation of cipd.Repository service.
//
// It checks ACLs.
func Public() api.RepositoryServer {
	return &repoImpl{}
}

// repoImpl implements api.RepositoryServer.
type repoImpl struct{}

// GetPrefixMetadata implements the corresponding RPC method, see the proto doc.
func (impl *repoImpl) GetPrefixMetadata(c context.Context, r *api.PrefixRequest) (resp *api.PrefixMetadata, err error) {
	defer func() { err = grpcutil.GRPCifyAndLogErr(c, err) }()

	return nil, status.Errorf(codes.Unimplemented, "not implemented yet")
}

// GetInheritedPrefixMetadata implements the corresponding RPC method, see the proto doc.
func (impl *repoImpl) GetInheritedPrefixMetadata(c context.Context, r *api.PrefixRequest) (resp *api.InheritedPrefixMetadata, err error) {
	defer func() { err = grpcutil.GRPCifyAndLogErr(c, err) }()

	return nil, status.Errorf(codes.Unimplemented, "not implemented yet")
}

// UpdatePrefixMetadata implements the corresponding RPC method, see the proto doc.
func (impl *repoImpl) UpdatePrefixMetadata(c context.Context, r *api.PrefixMetadata) (resp *api.PrefixMetadata, err error) {
	defer func() { err = grpcutil.GRPCifyAndLogErr(c, err) }()

	return nil, status.Errorf(codes.Unimplemented, "not implemented yet")
}
