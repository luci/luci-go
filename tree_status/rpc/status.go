// Copyright 2024 The LUCI Authors.
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

// Package rpc contains the RPC handlers for the tree status service.
package rpc

import (
	"context"

	"go.chromium.org/luci/common/logging"

	pb "go.chromium.org/luci/tree_status/proto/v1"
)

type TreeStatusServer struct{}

var _ pb.TreeStatusServer = &TreeStatusServer{}

func (*TreeStatusServer) ListStatus(ctx context.Context, request *pb.ListStatusRequest) (*pb.ListStatusResponse, error) {
	logging.Infof(ctx, "ListStatus")
	return &pb.ListStatusResponse{}, nil
}

func (*TreeStatusServer) GetStatus(ctx context.Context, request *pb.GetStatusRequest) (*pb.Status, error) {
	logging.Infof(ctx, "CurrentStatus")
	return &pb.Status{}, nil
}

func (*TreeStatusServer) CreateStatus(ctx context.Context, request *pb.CreateStatusRequest) (*pb.Status, error) {
	logging.Infof(ctx, "CreateStatus")
	return &pb.Status{}, nil
}
