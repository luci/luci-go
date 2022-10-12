// Copyright 2022 The LUCI Authors.
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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/appstatus"

	"go.chromium.org/luci/buildbucket/appengine/internal/config"
	"go.chromium.org/luci/buildbucket/appengine/internal/perm"
	"go.chromium.org/luci/buildbucket/appengine/model"
	"go.chromium.org/luci/buildbucket/bbperms"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

func validateSynthesize(req *pb.SynthesizeBuildRequest) error {
	if req.GetBuilder() == nil && req.GetTemplateBuildId() == 0 {
		return errors.Reason("builder or template_build_id is required").Err()
	}
	if req.GetBuilder() != nil && req.GetTemplateBuildId() != 0 {
		return errors.Reason("builder and template_build_id are mutually exclusive").Err()
	}
	if req.GetBuilder() != nil {
		if err := protoutil.ValidateRequiredBuilderID(req.Builder); err != nil {
			return errors.Annotate(err, "builder").Err()
		}
	}
	return nil
}

func synthesizeBuild(ctx context.Context, schReq *pb.ScheduleBuildRequest) (*pb.Build, error) {
	builder := schReq.GetBuilder()
	if builder == nil {
		return nil, errors.Reason("builder must be specified").Err()
	}
	if err := perm.HasInBuilder(ctx, bbperms.BuildersGet, builder); err != nil {
		return nil, err
	}
	globalCfg, err := config.GetSettingsCfg(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "error fetching service config").Err()
	}

	bktCfg := &model.Bucket{
		Parent: model.ProjectKey(ctx, builder.Project),
		ID:     builder.Bucket,
	}
	bldrCfg := &model.Builder{
		Parent: model.BucketKey(ctx, builder.Project, builder.Bucket),
		ID:     builder.Builder,
	}
	if err := datastore.Get(ctx, bktCfg, bldrCfg); err != nil {
		return nil, errors.Annotate(err, "failed to get builder config").Err()
	}

	bld := buildFromScheduleRequest(ctx, schReq, nil, "", bldrCfg.Config, globalCfg)

	if bktCfg.Proto.GetShadow() != "" && bktCfg.Proto.Shadow != builder.Bucket {
		bld.Builder.Bucket = bktCfg.Proto.Shadow
	}
	return bld, nil
}

// synthesizeBuildFromTemplate returns a request with fields populated by the
// given template_build_id if there is one. Fields set in the request override
// fields populated from the template. Does not modify the incoming request.
func synthesizeBuildFromTemplate(ctx context.Context, req *pb.SynthesizeBuildRequest) (*pb.Build, error) {
	ret, err := scheduleRequestFromBuildID(ctx, req.TemplateBuildId)
	if err != nil {
		return nil, err
	}

	return synthesizeBuild(ctx, ret)
}

// SynthesizeBuild handles a request to synthesize a build. Implements pb.BuildsServer.
func (*Builds) SynthesizeBuild(ctx context.Context, req *pb.SynthesizeBuildRequest) (*pb.Build, error) {
	if err := validateSynthesize(req); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	if req.GetTemplateBuildId() != 0 {
		return synthesizeBuildFromTemplate(ctx, req)
	}

	return synthesizeBuild(ctx, &pb.ScheduleBuildRequest{Builder: req.GetBuilder()})
}
