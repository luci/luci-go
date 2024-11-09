// Copyright 2021 The LUCI Authors.
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
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/milo/internal/projectconfig"
	projectconfigpb "go.chromium.org/luci/milo/proto/projectconfig"
	milopb "go.chromium.org/luci/milo/proto/v1"
)

// GetProjectCfg implements milopb.MiloInternal service
func (s *MiloInternalService) GetProjectCfg(ctx context.Context, req *milopb.GetProjectCfgRequest) (_ *projectconfigpb.Project, err error) {
	projectName := req.GetProject()
	if projectName == "" {
		return nil, appstatus.Error(codes.InvalidArgument, "project must be specified")
	}

	allowed, err := projectconfig.IsAllowed(ctx, projectName)
	if err != nil {
		return nil, err
	}
	if !allowed {
		if auth.CurrentIdentity(ctx) == identity.AnonymousIdentity {
			return nil, appstatus.Error(codes.Unauthenticated, "not logged in ")
		}
		return nil, appstatus.Error(codes.PermissionDenied, "no access to the project")
	}

	project, err := projectconfig.GetProject(ctx, projectName)
	if err != nil {
		return nil, err
	}
	metadataConfig := &projectconfigpb.MetadataConfig{}
	if err := proto.Unmarshal(project.MetadataConfig, metadataConfig); err != nil {
		return nil, err
	}
	return &projectconfigpb.Project{
		LogoUrl:        project.LogoURL,
		BugUrlTemplate: project.BugURLTemplate,
		MetadataConfig: metadataConfig,
	}, nil
}
