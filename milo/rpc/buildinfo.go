// Copyright 2017 The LUCI Authors.
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
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/luci_config/common/cfgtypes"
	milo "go.chromium.org/luci/milo/api/proto"
	"go.chromium.org/luci/milo/buildsource/buildbot"
	"go.chromium.org/luci/milo/buildsource/buildbucket"
	"go.chromium.org/luci/milo/buildsource/swarming"

	"google.golang.org/grpc/codes"

	"golang.org/x/net/context"
)

// BuildInfoService is a BuildInfoServer implementation.
type BuildInfoService struct {
	// Swarming is the BuildInfoProvider for the Swarming service.
	Swarming swarming.BuildInfoProvider
}

var _ milo.BuildInfoServer = (*BuildInfoService)(nil)

// Get implements milo.BuildInfoServer.
func (svc *BuildInfoService) Get(c context.Context, req *milo.BuildInfoRequest) (*milo.BuildInfoResponse, error) {
	projectHint := cfgtypes.ProjectName(req.ProjectHint)
	if projectHint != "" {
		if err := projectHint.Validate(); err != nil {
			return nil, grpcutil.Errf(codes.InvalidArgument, "invalid project hint: %s", err.Error())
		}
	}

	switch {
	case req.GetBuildbot() != nil:
		return buildbot.GetBuildInfo(c, req.GetBuildbot(), projectHint)

	case req.GetSwarming() != nil:
		return svc.Swarming.GetBuildInfo(c, req.GetSwarming(), projectHint)

	case req.GetBuildbucket() != nil:
		// Resolve the swarming host/task from buildbucket.
		sID, err := buildbucket.GetSwarmingID(c, req.GetBuildbucket().GetId())
		if err != nil {
			return nil, err
		}
		sReq := &milo.BuildInfoRequest_Swarming{
			Host: sID.Host,
			Task: sID.TaskID,
		}
		// TODO(hinoka): Unify buildbucket.BuildID.Get() and this. crbug.com/765061.
		return svc.Swarming.GetBuildInfo(c, sReq, projectHint)

	default:
		return nil, grpcutil.Errf(codes.InvalidArgument, "must supply a build")
	}
}
