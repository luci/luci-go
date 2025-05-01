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

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/auth/identity"
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
	case req.GetConfigSet() == "":
		return errors.Reason("config_set is not specified").Err()
	case req.GetContentSha256() != "" && req.GetPath() != "":
		return errors.Reason("content_sha256 and path are mutually exclusive").Err()
	case req.GetContentSha256() == "" && req.GetPath() == "":
		return errors.Reason("content_sha256 or path is required").Err()
	case req.GetPath() != "":
		if err := validatePath(req.Path); err != nil {
			return errors.Annotate(err, "path %q", req.Path).Err()
		}
	}
	return errors.WrapIf(config.Set(req.ConfigSet).Validate(), "config_set %q", req.ConfigSet)
}

// GetConfig handles a request to retrieve a config. Implements pb.ConfigsServer.
func (c Configs) GetConfig(ctx context.Context, req *pb.GetConfigRequest) (*pb.Config, error) {
	if err := validateGetConfig(req); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%s", err)
	}
	m, err := toConfigMask(req.Fields)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid fields mask: %s", err)
	}

	var f *model.File
	var noSuchCfgErr *model.NoSuchConfigError
	cs := req.ConfigSet

	// Check read access for the config set.
	switch hasPerm, err := acl.CanReadConfigSet(ctx, config.Set(cs)); {
	case err != nil:
		logging.Errorf(ctx, "cannot check permission for %q: %s", auth.CurrentIdentity(ctx), err)
		return nil, status.Errorf(codes.Internal, "error while checking permission for %q", auth.CurrentIdentity(ctx))
	case !hasPerm:
		logging.Infof(ctx, "%q does not have access to %s", auth.CurrentIdentity(ctx), cs)
		return nil, notFoundErr(auth.CurrentIdentity(ctx))
	}

	// Fetch by ContentSha256
	if req.ContentSha256 != "" {
		switch f, err = model.GetConfigFileByHash(ctx, config.Set(cs), req.ContentSha256); {
		case errors.As(err, &noSuchCfgErr):
			logging.Infof(ctx, "no such config: %s", noSuchCfgErr.Error())
			return nil, notFoundErr(auth.CurrentIdentity(ctx))
		case err != nil:
			logging.Errorf(ctx, "cannot fetch config (hash %s): %s", req.ContentSha256, err)
			return nil, status.Errorf(codes.Internal, "error while fetching the config")
		}
	} else {
		// Fetch by config set + path.
		switch f, err = model.GetLatestConfigFile(ctx, config.Set(cs), req.Path); {
		case errors.As(err, &noSuchCfgErr):
			logging.Infof(ctx, "no such config: %s", noSuchCfgErr.Error())
			return nil, notFoundErr(auth.CurrentIdentity(ctx))
		case err != nil:
			logging.Errorf(ctx, "cannot fetch config (configset:%s, path:%s): %s", cs, req.Path, err)
			return nil, status.Errorf(codes.Internal, "error while fetching the config")
		}
	}

	// Convert to the response proto.
	configPb := toConfigPb(cs, f)

	// Fetch the actual content only if it is requested by the field mask.
	if len(f.Content) != 0 && f.Size < int64(maxRawContentSize) {
		if m.MustIncludes("raw_content") != mask.Exclude {
			rawContent, err := f.GetRawContent(ctx)
			if err != nil {
				logging.Errorf(ctx, "failed to get the raw content of the config for %s-%s: %s", cs, f.Path, err)
				return nil, status.Errorf(codes.Internal, "error while getting raw content")
			}
			configPb.Content = &pb.Config_RawContent{RawContent: rawContent}
		}
	} else if m.MustIncludes("signed_url") != mask.Exclude {
		urls, err := common.CreateSignedURLs(ctx, clients.GetGsClient(ctx), []gs.Path{f.GcsURI}, http.MethodGet, nil)
		if err != nil {
			logging.Errorf(ctx, "failed to generate signed url for %s-%s: %s", cs, f.Path, err)
			return nil, status.Errorf(codes.Internal, "error while generating the config signed url")
		}
		configPb.Content = &pb.Config_SignedUrl{SignedUrl: urls[0]}
	}

	// Leave only fields specified in the field mask.
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
	return status.Errorf(codes.NotFound, "requested resource not found or %q does not have permission to access it", id)
}
