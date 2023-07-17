// Copyright 2023 The LUCI Authors.
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

package rpc

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "go.chromium.org/luci/config_service/proto"
)

// GetProjectConfigs gets the specified project configs from all projects.
// Implement pb.ConfigsServer.
func (c Configs) GetProjectConfigs(ctx context.Context, req *pb.GetProjectConfigsRequest) (*pb.GetProjectConfigsResponse, error) {
	if err := validatePath(req.GetPath()); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid path - %q: %s", req.GetPath(), err)
	}
	_, err := toConfigMask(req.GetFields())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid fields mask: %s", err)
	}

	return nil, status.Errorf(codes.Unimplemented, "GetProjectConfigs hasn't been implemented")
}
