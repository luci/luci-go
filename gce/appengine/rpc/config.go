// Copyright 2018 The LUCI Authors.
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

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/empty"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/gce/api/config/v1"
)

// Config implements config.ConfigServer.
type Config struct {
}

// CreateVMs handles a request to create a VMs block.
func (*Config) CreateVMs(c context.Context, req *config.CreateVMsRequest) (*config.Block, error) {
	return &config.Block{}, nil
}

// DeleteVMs handles a request to delete a VMs block.
func (*Config) DeleteVMs(c context.Context, req *config.DeleteVMsRequest) (*empty.Empty, error) {
	return &empty.Empty{}, nil
}

// GetVMs handles a request to get a VMs block.
func (*Config) GetVMs(c context.Context, req *config.GetVMsRequest) (*config.Block, error) {
	return &config.Block{}, nil
}

// authPrelude ensures the user is authorized to use the config API.
func authPrelude(c context.Context, methodName string, req proto.Message) (context.Context, error) {
	groups := []string{"administrators"}
	switch is, err := auth.IsMember(c, groups...); {
	case err != nil:
		return c, err
	case !is:
		return c, status.Errorf(codes.PermissionDenied, "unauthorized user")
	}
	logging.Debugf(c, "%s called %q:\n%s", auth.CurrentIdentity(c), methodName, req)
	return c, nil
}

// gRPCifyAndLogErr ensures any error being returned is a gRPC error, logging Internal and Unknown errors.
func gRPCifyAndLogErr(c context.Context, methodName string, rsp proto.Message, err error) error {
	return grpcutil.GRPCifyAndLogErr(c, err)
}

// New returns a new config RPC server.
func New() config.ConfigServer {
	return &config.DecoratedConfig{
		Prelude:  authPrelude,
		Service:  &Config{},
		Postlude: gRPCifyAndLogErr,
	}
}
