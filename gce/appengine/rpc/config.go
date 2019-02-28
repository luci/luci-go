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

	"github.com/golang/protobuf/ptypes/empty"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/gce/api/config/v1"
	"go.chromium.org/luci/gce/appengine/model"
)

// Config implements config.ConfigurationServer.
type Config struct {
}

// Ensure Config implements config.ConfigurationServer.
var _ config.ConfigurationServer = &Config{}

// Delete handles a request to delete a config.
// For app-internal use only.
func (*Config) Delete(c context.Context, req *config.DeleteRequest) (*empty.Empty, error) {
	if req.GetId() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "ID is required")
	}
	if err := datastore.Delete(c, &model.Config{ID: req.Id}); err != nil {
		return nil, errors.Annotate(err, "failed to delete config").Err()
	}
	return &empty.Empty{}, nil
}

// Ensure handles a request to create or update a config.
// For app-internal use only.
func (*Config) Ensure(c context.Context, req *config.EnsureRequest) (*config.Config, error) {
	if req.GetId() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "ID is required")
	}
	cfg := &model.Config{
		ID:     req.Id,
		Config: *req.Config,
	}
	if err := datastore.Put(c, cfg); err != nil {
		return nil, errors.Annotate(err, "failed to store config").Err()
	}
	return &cfg.Config, nil
}

// Get handles a request to get a config.
func (*Config) Get(c context.Context, req *config.GetRequest) (*config.Config, error) {
	if req.GetId() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "ID is required")
	}
	cfg := &model.Config{
		ID: req.Id,
	}
	switch err := datastore.Get(c, cfg); err {
	case nil:
		return &cfg.Config, nil
	case datastore.ErrNoSuchEntity:
		return nil, status.Errorf(codes.NotFound, "no config found with ID %q", req.Id)
	default:
		return nil, errors.Annotate(err, "failed to fetch config").Err()
	}
}

// List handles a request to list all configs.
func (*Config) List(c context.Context, req *config.ListRequest) (*config.ListResponse, error) {
	rsp := &config.ListResponse{}
	q := datastore.NewQuery(model.ConfigKind)
	if err := datastore.Run(c, q, func(cfg *model.Config, f datastore.CursorCB) error {
		rsp.Configs = append(rsp.Configs, &cfg.Config)
		return nil
	}); err != nil {
		return nil, errors.Annotate(err, "failed to fetch configs").Err()
	}
	return rsp, nil
}

// NewConfigurationServer returns a new configuration server.
func NewConfigurationServer() config.ConfigurationServer {
	return &config.DecoratedConfiguration{
		Prelude:  authPrelude,
		Service:  &Config{},
		Postlude: gRPCifyAndLogErr,
	}
}
