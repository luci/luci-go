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

	"go.chromium.org/luci/grpc/appstatus"

	pb "go.chromium.org/luci/config_service/proto"
)

// GetConfig handles a request to retrieve a config. Implements pb.ConfigsServer.
func (c Configs) GetConfig(ctx context.Context, req *pb.GetConfigRequest) (*pb.Config, error) {
	return nil, appstatus.Errorf(codes.Unimplemented, "GetConfig hasn't been implemented yet.")
}
