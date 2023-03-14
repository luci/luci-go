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

package main

import (
	"context"
	"errors"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/buildbucket"
	bbpb "go.chromium.org/luci/buildbucket/proto"
)

type testBBClient struct {
	requests []*bbpb.UpdateBuildRequest
}

func (t *testBBClient) UpdateBuild(ctx context.Context, in *bbpb.UpdateBuildRequest, opts ...grpc.CallOption) (*bbpb.Build, error) {
	req := proto.Clone(in).(*bbpb.UpdateBuildRequest)
	t.requests = append(t.requests, req)
	return &bbpb.Build{}, nil
}

func (t *testBBClient) StartBuild(ctx context.Context, in *bbpb.StartBuildRequest, opts ...grpc.CallOption) (*bbpb.StartBuildResponse, error) {
	switch in.TaskId {
	case "duplicate":
		return nil, buildbucket.DuplicateTask.Apply(errors.New("duplicate"))
	case "collide":
		return nil, buildbucket.DuplicateTask.Apply(errors.New("duplicate"))
	default:
		return &bbpb.StartBuildResponse{}, nil
	}
}
