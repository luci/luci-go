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

	"google.golang.org/protobuf/types/known/structpb"

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
		return errors.New("builder or template_build_id is required")
	}
	if req.GetBuilder() != nil && req.GetTemplateBuildId() != 0 {
		return errors.New("builder and template_build_id are mutually exclusive")
	}
	if req.GetBuilder() != nil {
		if err := protoutil.ValidateRequiredBuilderID(req.Builder); err != nil {
			return errors.Fmt("builder: %w", err)
		}
	}
	return nil
}

func synthesizeBuild(ctx context.Context, schReq *pb.ScheduleBuildRequest) (*pb.Build, error) {
	builder := schReq.GetBuilder()
	if builder == nil {
		return nil, errors.New("builder must be specified")
	}
	if err := perm.HasInBuilder(ctx, bbperms.BuildersGet, builder); err != nil {
		return nil, err
	}
	globalCfg, err := config.GetSettingsCfg(ctx)
	if err != nil {
		return nil, errors.Fmt("error fetching service config: %w", err)
	}

	bktCfg := &model.Bucket{
		Parent: model.ProjectKey(ctx, builder.Project),
		ID:     builder.Bucket,
	}
	bldrCfg := &model.Builder{
		Parent: model.BucketKey(ctx, builder.Project, builder.Bucket),
		ID:     builder.Builder,
	}
	switch err := datastore.Get(ctx, bktCfg, bldrCfg); {
	case errors.Contains(err, datastore.ErrNoSuchEntity):
		switch {
		case bktCfg == nil:
			// Bucket not found.
			return nil, perm.NotFoundErr(ctx)
		case len(bktCfg.Shadows) > 0:
			// This is a shadow bucket. Synthesizing a build from shadow bucket
			// is not supported.
			return nil, appstatus.BadRequest(errors.New("Synthesizing a build from a shadow bucket is not supported"))
		default:
			// Builder not found.
			return nil, perm.NotFoundErr(ctx)
		}
	case err != nil:
		return nil, errors.Fmt("failed to get builder config: %w", err)
	default:
		bld := scheduleShadowBuild(ctx, schReq, nil, bktCfg.Proto.Shadow, globalCfg, bldrCfg.Config)
		return bld, nil
	}
}

func scheduleShadowBuild(ctx context.Context, schReq *pb.ScheduleBuildRequest, ancestors []int64, shadowBucket string, globalCfg *pb.SettingsCfg, cfg *pb.BuilderConfig) *pb.Build {
	origBucket := schReq.Builder.Bucket

	cfgCopy := cfg
	if shadowBucket != "" && shadowBucket != origBucket {
		cfgCopy = applyShadowAdjustment(cfg)
	}

	bld := buildFromScheduleRequest(ctx, schReq, ancestors, "", cfgCopy, globalCfg)

	if shadowBucket != "" && shadowBucket != origBucket {
		bld.Infra.Led = &pb.BuildInfra_Led{
			ShadowedBucket: origBucket,
		}
		bld.Input.Properties.Fields["$recipe_engine/led"] = &structpb.Value{
			Kind: &structpb.Value_StructValue{
				StructValue: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"shadowed_bucket": {
							Kind: &structpb.Value_StringValue{
								StringValue: origBucket,
							},
						},
					},
				},
			},
		}
		bld.Builder.Bucket = shadowBucket
	}
	return bld
}

// synthesizeBuildFromTemplate returns a request with fields populated by the
// given template_build_id if there is one. Fields set in the request override
// fields populated from the template. Does not modify the incoming request.
func synthesizeBuildFromTemplate(ctx context.Context, req *pb.SynthesizeBuildRequest) (*pb.Build, error) {
	ret, err := scheduleRequestFromBuildID(ctx, req.TemplateBuildId, false)
	if err != nil {
		return nil, err
	}

	if len(req.GetExperiments()) > 0 {
		ret.Experiments = req.Experiments
	} else {
		ret.Experiments = map[string]bool{}
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

	exps := map[string]bool{}
	if len(req.GetExperiments()) > 0 {
		exps = req.Experiments
	}
	return synthesizeBuild(ctx, &pb.ScheduleBuildRequest{
		Builder:     req.GetBuilder(),
		Experiments: exps,
	})
}
