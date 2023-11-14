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
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/mask"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/config_service/internal/acl"
	"go.chromium.org/luci/config_service/internal/model"
	pb "go.chromium.org/luci/config_service/proto"
)

// ListConfigSets returns a list of config sets. Implements pb.ConfigsServer.
func (c Configs) ListConfigSets(ctx context.Context, req *pb.ListConfigSetsRequest) (*pb.ListConfigSetsResponse, error) {
	// Validate fields mask
	m, err := toConfigSetMask(req.GetFields())
	switch {
	case err != nil:
		return nil, status.Errorf(codes.InvalidArgument, "invalid fields mask: %s", err)
	case m.MustIncludes("file_paths") != mask.Exclude:
		return nil, status.Errorf(codes.InvalidArgument, `
			'file_paths' is not supported in fields mask. Must specify a config set in
			order to include file paths (via GetConfigSet rpc).`)
	case m.MustIncludes("configs") != mask.Exclude:
		return nil, status.Errorf(codes.InvalidArgument, `
			'configs' is not supported in fields mask. Must specify a config set in
			order to include file paths (via GetConfigSet rpc).`)
	}

	// Fetch config sets
	query := datastore.NewQuery(model.ConfigSetKind)
	switch req.GetDomain() {
	case pb.ListConfigSetsRequest_SERVICE:
		// Range query for config sets which start with "services/".
		// Use Lt("__key__", "serivces0") because '0' is the next char of "/".
		query = query.Gt("__key__", datastore.MakeKey(ctx, model.ConfigSetKind, string(config.ServiceDomain)+"/")).
			Lt("__key__", datastore.MakeKey(ctx, model.ConfigSetKind, string(config.ServiceDomain)+"0"))
	case pb.ListConfigSetsRequest_PROJECT:
		query = query.Gt("__key__", datastore.MakeKey(ctx, model.ConfigSetKind, string(config.ProjectDomain)+"/")).
			Lt("__key__", datastore.MakeKey(ctx, model.ConfigSetKind, string(config.ProjectDomain)+"0"))
	}
	var cfgSets []*model.ConfigSet
	err = datastore.Run(ctx, query, func(cs *model.ConfigSet) error {
		switch hasPerm, err := acl.CanReadConfigSet(ctx, cs.ID); {
		case err != nil:
			logging.Errorf(ctx, "cannot check %q read access for %q: %s", cs.ID, auth.CurrentIdentity(ctx), err)
			return err
		case hasPerm:
			cfgSets = append(cfgSets, cs)
		}
		return nil
	})
	if err != nil {
		logging.Errorf(ctx, "error when querying %s config sets: %s", req.Domain.String(), err)
		return nil, status.Errorf(codes.Internal, "error while fetching config sets")
	}

	cfgSetsPb := make([]*pb.ConfigSet, len(cfgSets))
	for i, cs := range cfgSets {
		cfgSetsPb[i] = toConfigSetPb(cs)
	}

	// Fetch last_import_attempt if needed.
	// TODO(crbug.com/1465995): Might be inconsistent between ConfigSet and ImportAttempt
	// if the ConfigSet gets updated during the two fetches. But it’s rare and costs an
	// extra call every time. So not putting it in a transaction for now.
	if m.MustIncludes("last_import_attempt") != mask.Exclude {
		attempts := make([]*model.ImportAttempt, len(cfgSets))
		for i, cs := range cfgSets {
			attempts[i] = &model.ImportAttempt{
				ConfigSet: datastore.KeyForObj(ctx, cs),
			}
		}
		if err := datastore.Get(ctx, attempts); err != nil {
			logging.Errorf(ctx, "failed to fetch last import attempts: %s", err)
			return nil, status.Errorf(codes.Internal, "error while fetching last import attempts")
		}
		for i, attempt := range attempts {
			cfgSetsPb[i].LastImportAttempt = toImportAttempt(attempt)
		}
	}

	// Trim ConfigSet proto.
	for _, cfgSetPb := range cfgSetsPb {
		if err := m.Trim(cfgSetPb); err != nil {
			logging.Errorf(ctx, "cannot trim ConfigSet (%q) proto: %s", cfgSetPb.Name, err)
			return nil, status.Errorf(codes.Internal, "error while constructing the response")
		}
	}
	return &pb.ListConfigSetsResponse{ConfigSets: cfgSetsPb}, nil
}
