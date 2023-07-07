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
	"path"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/config"
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
	return nil, status.Errorf(codes.Unimplemented, "GetConfig hasn't been implemented yet.")
}
