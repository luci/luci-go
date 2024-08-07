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

package rpc

import (
	"context"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/grpc/appstatus"

	pb "go.chromium.org/luci/source_index/proto/v1"
)

// sourceIndexServer implements pb.SourceIndexServer.
type sourceIndexServer struct {
}

// NewSourceIndexServer returns a new pb.SourceIndexServer.
func NewSourceIndexServer() pb.SourceIndexServer {
	return &pb.DecoratedSourceIndex{
		Service:  &sourceIndexServer{},
		Postlude: gRPCifyAndLogPostlude,
	}
}

// QueryCommitHash returns commit that matches desired position of commit,
// based on QueryCommitHashRequest parameters
func (*sourceIndexServer) QueryCommitHash(ctx context.Context, req *pb.QueryCommitHashRequest) (*pb.QueryCommitHashResponse, error) {
	return nil, appstatus.Error(codes.Unimplemented, "method QueryCommitHash not implemented")
}
