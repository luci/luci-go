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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/mask"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/config_service/internal/acl"
	"go.chromium.org/luci/config_service/internal/model"
	pb "go.chromium.org/luci/config_service/proto"
)

// GetConfigSet fetches a single config set. Implements pb.ConfigsServer.
func (c Configs) GetConfigSet(ctx context.Context, req *pb.GetConfigSetRequest) (*pb.ConfigSet, error) {
	// Validate the request.
	if req.GetConfigSet() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "config_set is not specified")
	} else if err := config.Set(req.ConfigSet).Validate(); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	m, err := toConfigSetMask(req.Fields)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid fields mask: %s", err)
	}

	ctx = logging.SetFields(ctx, logging.Fields{"ConfigSet": req.ConfigSet})

	// Check read access for the config set.
	switch hasPerm, err := acl.CanReadConfigSet(ctx, config.Set(req.ConfigSet)); {
	case err != nil:
		logging.Errorf(ctx, "cannot check permission for %q: %s", auth.CurrentIdentity(ctx), err)
		return nil, status.Errorf(codes.Internal, "error while checking permission for %q", auth.CurrentIdentity(ctx))
	case !hasPerm:
		logging.Infof(ctx, "%q does not have the read access", auth.CurrentIdentity(ctx))
		return nil, notFoundErr(auth.CurrentIdentity(ctx))
	}

	cfgSet := &model.ConfigSet{ID: config.Set(req.ConfigSet)}
	var attempt *model.ImportAttempt
	switch {
	case m.MustIncludes("last_import_attempt") != mask.Exclude:
		attempt = &model.ImportAttempt{ConfigSet: datastore.KeyForObj(ctx, cfgSet)}
		// Fetch in the transaction to make sure the consistency between the
		// ConfigSet and ImportAttempt entities.
		err = datastore.RunInTransaction(ctx, func(ctx context.Context) error {
			return datastore.Get(ctx, cfgSet, attempt)
		}, &datastore.TransactionOptions{ReadOnly: true})
	default:
		err = datastore.Get(ctx, cfgSet)
	}
	if err != nil {
		if errors.Contains(err, datastore.ErrNoSuchEntity) {
			logging.Warningf(ctx, "cannot find the config set or its import attempt: %s", err)
			return nil, notFoundErr(auth.CurrentIdentity(ctx))
		}
		logging.Errorf(ctx, "cannot fetch config set or its import attempt: %s", err)
		return nil, status.Errorf(codes.Internal, "error while fetching config set")
	}

	var files []*model.File
	if m.MustIncludes("file_paths") != mask.Exclude || m.MustIncludes("configs") != mask.Exclude {
		query := datastore.NewQuery(model.FileKind).
			Ancestor(datastore.MakeKey(ctx, model.ConfigSetKind, string(cfgSet.ID), model.RevisionKind, cfgSet.LatestRevision.ID))
		if err = datastore.GetAll(ctx, query, &files); err != nil {
			logging.Errorf(ctx, "error while fetching config files: %s", err)
			return nil, status.Errorf(codes.Internal, "error while fetching config files")
		}
	}

	// To proto.
	cfgSetPb := toConfigSetPb(cfgSet)
	cfgSetPb.LastImportAttempt = toImportAttempt(attempt)
	if m.MustIncludes("file_paths") != mask.Exclude {
		cfgSetPb.FilePaths = extractFilePaths(files)
	}
	if m.MustIncludes("configs") != mask.Exclude {
		cfgSetPb.Configs = extractConfigsPb(req.ConfigSet, files)
	}
	if err := m.Trim(cfgSetPb); err != nil {
		logging.Errorf(ctx, "cannot trim the config set proto: %s", err)
		return nil, status.Errorf(codes.Internal, "error while constructing response")
	}
	return cfgSetPb, nil
}

func extractFilePaths(files []*model.File) []string {
	if files == nil {
		return nil
	}
	filePaths := make([]string, len(files))
	for i, f := range files {
		filePaths[i] = f.Path
	}
	return filePaths
}

func extractConfigsPb(cs string, files []*model.File) []*pb.Config {
	if files == nil {
		return nil
	}
	cfgs := make([]*pb.Config, len(files))
	for i, f := range files {
		cfgs[i] = toConfigPb(cs, f)
	}
	return cfgs
}
