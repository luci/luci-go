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

	"go.chromium.org/luci/common/errors"

	pb "go.chromium.org/luci/buildbucket/proto"
)

// RegisterBuildTask handles a request to register a TaskBackend task with the build it runs. Implements pb.BuildsServer.
func (*Builds) RegisterBuildTask(ctx context.Context, req *pb.RegisterBuildTaskRequest) (*pb.RegisterBuildTaskResponse, error) {
	return nil, errors.New("not implemented")
}
