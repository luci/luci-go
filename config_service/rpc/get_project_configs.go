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
	"net/http"
	"slices"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/gcloud/gs"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/mask"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/config_service/internal/acl"
	"go.chromium.org/luci/config_service/internal/clients"
	"go.chromium.org/luci/config_service/internal/common"
	"go.chromium.org/luci/config_service/internal/model"
	pb "go.chromium.org/luci/config_service/proto"
)

// maxProjConfigsResSize limits the sum of small configs size GetProjectConfigs
// rpc can return. In the current situation, it most likely won't happen. Treat
// this as a defensive coding, which can:
//  1. Prevent accidentally fetching hundreds of small config which causes the
//     Config Server OOM.
//  2. In the future, when this rpc expand to support regex files fetching, the
//     response is more likely large. It should limit the response size and use
//     some other ways to return the entire results (e.g pagination.)
//
// Note: Make it a var instead of a const to facilitate the unit test.
var maxProjConfigsResSize = 200 * 1024 * 1024

// GetProjectConfigs gets the specified project configs from all projects.
// Implement pb.ConfigsServer.
func (c Configs) GetProjectConfigs(ctx context.Context, req *pb.GetProjectConfigsRequest) (*pb.GetProjectConfigsResponse, error) {
	if err := validatePath(req.GetPath()); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid path - %q: %s", req.GetPath(), err)
	}
	m, err := toConfigMask(req.Fields)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid fields mask: %s", err)
	}

	var files []*model.File
	// Query all config sets which start with "projects/". Use
	// Lt("__key__", string(config.ProjectDomain)+"0"),  because '0' is the next
	// char of "/" in ASCII table.
	// The query only fetches keys with `latest_revision.id` field.
	query := datastore.NewQuery(model.ConfigSetKind).
		Project("latest_revision.id").
		Gt("__key__", datastore.MakeKey(ctx, model.ConfigSetKind, string(config.ProjectDomain)+"/")).
		Lt("__key__", datastore.MakeKey(ctx, model.ConfigSetKind, string(config.ProjectDomain)+"0"))
	err = datastore.Run(ctx, query, func(cs *model.ConfigSet) error {
		switch hasPerm, err := acl.CanReadConfigSet(ctx, cs.ID); {
		case err != nil:
			logging.Errorf(ctx, "cannot check %q read access for %q: %s", cs.ID, auth.CurrentIdentity(ctx), err)
			return err
		case hasPerm:
			files = append(files, &model.File{
				Path:     req.Path,
				Revision: datastore.MakeKey(ctx, model.ConfigSetKind, string(cs.ID), model.RevisionKind, cs.LatestRevision.ID),
			})
		}
		return nil
	})
	if err != nil {
		logging.Errorf(ctx, "error when querying project config sets: %s", err)
		return nil, status.Errorf(codes.Internal, "error while fetching project configs")
	}

	// Get latest revision files from Datastore.
	if err := errors.Filter(datastore.Get(ctx, files), datastore.ErrNoSuchEntity); err != nil {
		logging.Errorf(ctx, "failed to fetch config files from Datastore: %s", err)
		return nil, status.Errorf(codes.Internal, "error while fetching project configs")
	}
	var configs []*pb.Config
	totalSize := 0
	// Sort the files from smallest to largest so that the response will include
	// as many smaller configs as possible.
	slices.SortFunc(files, func(a, b *model.File) int {
		return int(a.Size - b.Size)
	})
	for _, f := range files {
		cfgPb := &pb.Config{
			ConfigSet:     f.Revision.Root().StringID(),
			Path:          f.Path,
			ContentSha256: f.ContentSHA256,
			Revision:      f.Revision.StringID(),
			Url:           common.GitilesURL(f.Location.GetGitilesLocation()),
		}
		totalSize += int(f.Size)
		switch {
		case len(f.Content) == 0 && f.GcsURI == "":
			// This file is not found.
			continue
		case len(f.Content) != 0 && f.Size < int64(maxRawContentSize) && totalSize < maxProjConfigsResSize && m.MustIncludes("raw_content") != mask.Exclude:
			rawContent, err := f.GetRawContent(ctx)
			if err != nil {
				logging.Errorf(ctx, "failed to get the raw content of the config for %s-%s: %s", cfgPb.ConfigSet, cfgPb.Path, err)
				return nil, status.Errorf(codes.Internal, "error while getting raw content")
			}
			cfgPb.Content = &pb.Config_RawContent{RawContent: rawContent}
		case f.GcsURI != "" && m.MustIncludes("signed_url") != mask.Exclude:
			urls, err := common.CreateSignedURLs(ctx, clients.GetGsClient(ctx), []gs.Path{f.GcsURI}, http.MethodGet, nil)
			if err != nil {
				logging.Errorf(ctx, "cannot generate a signed url for GCS object %s: %s", f.GcsURI, err)
				return nil, status.Errorf(codes.Internal, "error while generating the config signed url")
			}
			cfgPb.Content = &pb.Config_SignedUrl{SignedUrl: urls[0]}
		}

		// Trim config proto by the Fields mask.
		if err := m.Trim(cfgPb); err != nil {
			logging.Errorf(ctx, "cannot trim the config proto in config set %q: %s", cfgPb.ConfigSet, err)
			return nil, status.Errorf(codes.Internal, "error while constructing the response")
		}
		configs = append(configs, cfgPb)
	}
	return &pb.GetProjectConfigsResponse{Configs: configs}, nil
}
