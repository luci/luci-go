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

// Package memory contains a config.ConfigurationServer backed by in-memory storage.
package memory

import (
	"context"
	"sync"

	"github.com/golang/protobuf/ptypes/empty"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/gce/api/config/v1"
)

// Ensure Config implements config.ConfigurationServer.
var _ config.ConfigurationServer = &Config{}

// Config implements config.ConfigurationServer.
type Config struct {
	// cfg is a map of IDs to known configs.
	cfg sync.Map
}

// Delete handles a request to delete a config.
func (srv *Config) Delete(c context.Context, req *config.DeleteRequest) (*empty.Empty, error) {
	srv.cfg.Delete(req.GetId())
	return &empty.Empty{}, nil
}

// Ensure handles a request to create or update a config.
func (srv *Config) Ensure(c context.Context, req *config.EnsureRequest) (*config.Config, error) {
	srv.cfg.Store(req.GetId(), req.GetConfig())
	return req.GetConfig(), nil
}

// Get handles a request to get a config.
func (srv *Config) Get(c context.Context, req *config.GetRequest) (*config.Config, error) {
	cfg, ok := srv.cfg.Load(req.GetId())
	if !ok {
		return nil, status.Errorf(codes.NotFound, "no config found with ID %q", req.GetId())
	}
	return cfg.(*config.Config), nil
}

// List handles a request to list all configs.
func (srv *Config) List(c context.Context, req *config.ListRequest) (*config.ListResponse, error) {
	rsp := &config.ListResponse{}
	// TODO(smut): Handle page tokens.
	if req.GetPageToken() != "" {
		return rsp, nil
	}
	srv.cfg.Range(func(_, val interface{}) bool {
		rsp.Configs = append(rsp.Configs, val.(*config.Config))
		return true
	})
	return rsp, nil
}
