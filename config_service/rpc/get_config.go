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
	"path"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/gcloud/gs"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/mask"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/config_service/internal/acl"
	"go.chromium.org/luci/config_service/internal/clients"
	"go.chromium.org/luci/config_service/internal/common"
	"go.chromium.org/luci/config_service/internal/model"
	pb "go.chromium.org/luci/config_service/proto"
)

// validateGetConfig validates the given request.
func validateGetConfig(req *pb.GetConfigRequest) error {
	switch {
	case req.GetContentSha256() != "":
		if req.ConfigSet != "" || req.Path != "" {
			return errors.Reason("content_sha256 is mutually exclusive with (config_set and path)").Err()
		}
	case req.GetConfigSet() != "" && req.GetPath() != "":
		if err := config.Set(req.ConfigSet).Validate(); err != nil {
			return errors.Annotate(err, "config_set %q", req.ConfigSet).Err()
		}
		if err := validatePath(req.Path); err != nil {
			return errors.Annotate(err, "path %q", req.Path).Err()
		}
	default:
		return errors.Reason("one of content_sha256 or (config_set and path) is required").Err()
	}
	return nil
}

func validatePath(p string) error {
	if path.IsAbs(p) {
		return errors.Reason("must not be absolute").Err()
	}
	if strings.HasPrefix(p, "./") || strings.HasPrefix(p, "../") {
		return errors.Reason("should not start with './' or '../'").Err()
	}
	return nil
}

// GetConfig handles a request to retrieve a config. Implements pb.ConfigsServer.
func (c Configs) GetConfig(ctx context.Context, req *pb.GetConfigRequest) (*pb.Config, error) {
	if err := validateGetConfig(req); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%s", err)
	}

	fieldMask := req.Fields
	// Convert "content" path to "raw_content" and "signed_url", as "content" is
	// 'oneof' field type and the mask lib hasn't supported to parse it yet.
	if fieldSet := stringset.NewFromSlice(fieldMask.GetPaths()...); fieldSet.Has("content") {
		fieldSet.Del("content")
		fieldSet.Add("raw_content")
		fieldSet.Add("signed_url")
		fieldMask.Paths = fieldSet.ToSlice()
	}
	m, err := mask.FromFieldMask(fieldMask, &pb.Config{}, false, false)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid fields mask: %s", err)
	}

	var f *model.File
	var noSuchCfgErr *model.NoSuchConfigError
	cs := req.ConfigSet

	// Fetch by ContentSha256
	if req.ContentSha256 != "" {
		f = &model.File{ContentSHA256: req.ContentSha256}
		switch err := f.Load(ctx, false); {
		case errors.As(err, &noSuchCfgErr):
			logging.Infof(ctx, "no such config: %s", noSuchCfgErr.Error())
			return nil, notFoundErr(auth.CurrentIdentity(ctx))
		case err != nil:
			logging.Errorf(ctx, "cannot fetch config (hash %s): %s", req.ContentSha256, err)
			return nil, status.Errorf(codes.Internal, "error while fetching the config")
		}
		cs = f.Revision.Root().StringID()
	}

	// Check read access for the config set.
	switch hasPerm, err := acl.CanReadConfigSet(ctx, config.Set(cs)); {
	case err != nil:
		logging.Errorf(ctx, "cannot check permission for %q: %s", auth.CurrentIdentity(ctx), err)
		return nil, status.Errorf(codes.Internal, "error while checking permission for %q", auth.CurrentIdentity(ctx))
	case !hasPerm:
		logging.Infof(ctx, "%q does not have access to %s", auth.CurrentIdentity(ctx), cs)
		return nil, notFoundErr(auth.CurrentIdentity(ctx))
	}

	// Fetch by config set + path.
	if req.ContentSha256 == "" {
		switch f, err = model.GetLatestConfigFile(ctx, config.Set(cs), req.Path, false); {
		case errors.As(err, &noSuchCfgErr):
			logging.Infof(ctx, "no such config: %s", noSuchCfgErr.Error())
			return nil, notFoundErr(auth.CurrentIdentity(ctx))
		case err != nil:
			logging.Errorf(ctx, "cannot fetch config (configset:%s, path:%s): %s", cs, req.Path, err)
			return nil, status.Errorf(codes.Internal, "error while fetching the config")
		}
	}

	// Convert to the response proto by the Fields mask.
	configPb := &pb.Config{
		ConfigSet:     cs,
		Path:          f.Path,
		ContentSha256: f.ContentSHA256,
		Revision:      f.Revision.StringID(),
		Url:           common.GitilesURL(f.Location.GetGitilesLocation()),
	}
	if len(f.Content) != 0 {
		configPb.Content = &pb.Config_RawContent{RawContent: f.Content}
	} else if m.MustIncludes("signed_url") != mask.Exclude {
		urls, err := common.CreateSignedURLs(ctx, clients.GetGsClient(ctx), []gs.Path{f.GcsURI}, http.MethodGet, nil)
		if err != nil {
			logging.Errorf(ctx, "failed to generate signed url for %s-%s: %s", cs, f.Path, err)
			return nil, status.Errorf(codes.Internal, "error while generating the config signed url")
		}
		configPb.Content = &pb.Config_SignedUrl{SignedUrl: urls[0]}
	}
	if err := m.Trim(configPb); err != nil {
		logging.Errorf(ctx, "cannot trim the config proto: %s", err)
		return nil, status.Errorf(codes.Internal, "error while constructing response")
	}
	return configPb, nil
}

// notFoundErr returns an gRPC NotFound error with a generic message, which
// obfuscates "not found" and "permission denied" intentionally to avoid
// revealing accurate information to unauthorized users.
func notFoundErr(id identity.Identity) error {
	return status.Errorf(codes.NotFound, "requested resource not found or %q dose not have permission to access it", id)
}
