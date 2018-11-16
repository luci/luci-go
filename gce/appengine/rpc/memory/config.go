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

// Package memory contains a config.ConfigServer backed by in-memory storage.
package memory

import (
	"context"
	"sync"

	"github.com/golang/protobuf/ptypes/empty"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/gce/api/config/v1"
)

// Ensure Config implements config.ConfigServer.
var _ config.ConfigServer = (*Config)(nil)

// Config implements config.ConfigServer.
type Config struct {
	// vms is a map of IDs to known VMs blocks.
	vms sync.Map
}

// DeleteVMs handles a request to delete a VMs block.
func (srv *Config) DeleteVMs(c context.Context, req *config.DeleteVMsRequest) (*empty.Empty, error) {
	srv.vms.Delete(req.GetId())
	return &empty.Empty{}, nil
}

// EnsureVMs handles a request to create or update a VMs block.
func (srv *Config) EnsureVMs(c context.Context, req *config.EnsureVMsRequest) (*config.Block, error) {
	srv.vms.Store(req.GetId(), req.GetVms())
	return req.GetVms(), nil
}

// GetVMs handles a request to get a VMs block.
func (srv *Config) GetVMs(c context.Context, req *config.GetVMsRequest) (*config.Block, error) {
	vms, ok := srv.vms.Load(req.GetId())
	if !ok {
		return nil, status.Errorf(codes.NotFound, "no VMs block found with ID %q", req.GetId())
	}
	return vms.(*config.Block), nil
}
