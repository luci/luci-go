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

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/paged"
	"go.chromium.org/luci/server/auth"

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
	if err := paged.Query(c, req.GetPageSize(), req.GetPageToken(), rsp, datastore.NewQuery(model.ConfigKind), func(cfg *model.Config) error {
		rsp.Configs = append(rsp.Configs, &cfg.Config)
		return nil
	}); err != nil {
		return nil, err
	}
	return rsp, nil
}

// Update handles a request to update a config.
func (*Config) Update(c context.Context, req *config.UpdateRequest) (*config.Config, error) {
	switch {
	case req.GetId() == "":
		return nil, status.Errorf(codes.InvalidArgument, "ID is required")
	case len(req.UpdateMask.GetPaths()) == 0:
		return nil, status.Errorf(codes.InvalidArgument, "update mask is required")
	}
	for _, p := range req.UpdateMask.Paths {
		if p != "config.current_amount" {
			return nil, status.Errorf(codes.InvalidArgument, "field %q is invalid or immutable", p)
		}
	}
	cfg := &model.Config{
		ID: req.Id,
	}
	switch err := datastore.Get(c, cfg); err {
	case nil:
	case datastore.ErrNoSuchEntity:
		// Update is accessible by all users. To avoid revealing information about config existence
		// to unauthorized users, not found and permission denied responses should be ambiguous.
		return nil, status.Errorf(codes.NotFound, "no config found with ID %q or unauthorized user", req.Id)
	default:
		return nil, errors.Annotate(err, "failed to fetch config").Err()
	}
	switch is, err := auth.IsMember(c, cfg.Config.GetOwner()...); {
	case err != nil:
		return nil, err
	case !is:
		return nil, status.Errorf(codes.NotFound, "no config found with ID %q or unauthorized user", req.Id)
	}
	amt, err := cfg.Config.ComputeAmount(req.Config.GetCurrentAmount(), clock.Now(c))
	if err != nil {
		return nil, errors.Annotate(err, "failed to parse amount").Err()
	}
	if amt == cfg.Config.CurrentAmount {
		return &cfg.Config, nil
	}
	if err := datastore.RunInTransaction(c, func(c context.Context) error {
		switch err := datastore.Get(c, cfg); {
		case err == datastore.ErrNoSuchEntity:
			return status.Errorf(codes.NotFound, "no config found with ID %q", req.Id)
		case err != nil:
			return errors.Annotate(err, "failed to fetch config").Err()
		case cfg.Config.CurrentAmount == amt:
			return nil
		}
		cfg.Config.CurrentAmount = amt
		if err := datastore.Put(c, cfg); err != nil {
			return errors.Annotate(err, "failed to store config").Err()
		}
		return nil
	}, nil); err != nil {
		return nil, err
	}
	return &cfg.Config, nil
}

// configPrelude ensures the user is authorized to use the config API.
func configPrelude(c context.Context, methodName string, req proto.Message) (context.Context, error) {
	if methodName == "Update" {
		// Update performs its own authorization checks, so allow all callers through.
		logging.Debugf(c, "%s called %q:\n%s", auth.CurrentIdentity(c), methodName, req)
		return c, nil
	}
	if !isReadOnly(methodName) {
		return c, status.Errorf(codes.PermissionDenied, "unauthorized user")
	}
	switch is, err := auth.IsMember(c, admins, writers, readers); {
	case err != nil:
		return c, err
	case is:
		logging.Debugf(c, "%s called %q:\n%s", auth.CurrentIdentity(c), methodName, req)
		return c, nil
	}
	return c, status.Errorf(codes.PermissionDenied, "unauthorized user")
}

// NewConfigurationServer returns a new configuration server.
func NewConfigurationServer() config.ConfigurationServer {
	return &config.DecoratedConfiguration{
		Prelude:  configPrelude,
		Service:  &Config{},
		Postlude: gRPCifyAndLogErr,
	}
}
