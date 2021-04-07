// Copyright 2020 The LUCI Authors.
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

package backend

import (
	"context"

	milo "go.chromium.org/luci/milo/api/config"
	milopb "go.chromium.org/luci/milo/api/service/v1"
	"go.chromium.org/luci/milo/common"
)

// GetProjectCfg implements milopb.MiloInternal service
func (s *MiloInternalService) GetProjectCfg(ctx context.Context, req *milopb.GetProjectCfgRequest) (*milo.Project, error) {
	project, err := common.GetProject(ctx, req.GetProject())
	if err != nil {
		return nil, err
	}
	return &milo.Project{
		BuildBugTemplate: &project.BuildBugTemplate,
		LogoUrl:          project.LogoURL,
	}, nil
}
