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

package rpc

import (
	"context"
	"fmt"
	"sort"

	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"

	"go.chromium.org/luci/buildbucket/appengine/internal/config"
	"go.chromium.org/luci/buildbucket/appengine/internal/perm"
	"go.chromium.org/luci/buildbucket/appengine/model"
	"go.chromium.org/luci/buildbucket/bbperms"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

// validateGetBuilder validates the given request.
func validateGetBuilder(req *pb.GetBuilderRequest) error {
	if err := protoutil.ValidateBuilderID(req.Id); err != nil {
		return errors.Annotate(err, "id").Err()
	}

	return nil
}

// applyShadowAdjustment applies shadow builder adjustments to builder config.
//
// It will mutate the given cfg.
func applyShadowAdjustment(cfg *pb.BuilderConfig) {
	shadowBldrCfg := cfg.GetShadowBuilderAdjustments()
	if shadowBldrCfg == nil {
		return
	}
	if shadowBldrCfg.ServiceAccount != "" {
		cfg.ServiceAccount = shadowBldrCfg.ServiceAccount
	}
	if shadowBldrCfg.Pool != "" {
		poolIdx := sort.Search(len(cfg.GetDimensions()), func(i int) bool {
			_, k, _ := config.ParseDimension(cfg.GetDimensions()[i])
			return k == "pool"
		})
		poolDim := fmt.Sprintf("pool:%s", shadowBldrCfg.Pool)
		if poolIdx < len(cfg.GetDimensions()) {
			cfg.Dimensions[poolIdx] = poolDim
		} else {
			cfg.Dimensions = append(cfg.Dimensions, poolDim)
		}
	}
	cfg.ShadowBuilderAdjustments = nil
}

func trySynthesizeFromShadowdBuilder(ctx context.Context, req *pb.GetBuilderRequest) (*pb.BuilderItem, error) {
	reqBucket := &model.Bucket{
		Parent: model.ProjectKey(ctx, req.Id.Project),
		ID:     req.Id.Bucket,
	}
	switch err := datastore.Get(ctx, reqBucket); {
	case err == datastore.ErrNoSuchEntity:
		return nil, perm.NotFoundErr(ctx)
	case err != nil:
		return nil, err
	}
	if len(reqBucket.Shadows) == 0 {
		// This bucket doesn't shadow any other buckets.
		return nil, perm.NotFoundErr(ctx)
	}
	var builders []*model.Builder
	for _, shadowedBkt := range reqBucket.Shadows {
		builders = append(builders, &model.Builder{
			Parent: model.BucketKey(ctx, req.Id.Project, shadowedBkt),
			ID:     req.Id.Builder,
		})
	}
	if err := model.GetIgnoreMissing(ctx, builders); err != nil {
		return nil, errors.Annotate(err, "failed to fetch entities").Err()
	}
	for _, bldr := range builders {
		if bldr.Config != nil {
			applyShadowAdjustment(bldr.Config)
			return &pb.BuilderItem{
				Id:     req.Id,
				Config: bldr.Config,
			}, nil
		}
	}
	return nil, perm.NotFoundErr(ctx)
}

// GetBuilder handles a request to retrieve a builder. Implements pb.BuildersServer.
func (*Builders) GetBuilder(ctx context.Context, req *pb.GetBuilderRequest) (*pb.BuilderItem, error) {
	if err := validateGetBuilder(req); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	if err := perm.HasInBuilder(ctx, bbperms.BuildersGet, req.Id); err != nil {
		return nil, err
	}

	builder := &model.Builder{
		Parent: model.BucketKey(ctx, req.Id.Project, req.Id.Bucket),
		ID:     req.Id.Builder,
	}
	switch err := datastore.Get(ctx, builder); {
	case err == datastore.ErrNoSuchEntity:
		return trySynthesizeFromShadowdBuilder(ctx, req)
	case err != nil:
		return nil, err
	}

	return &pb.BuilderItem{
		Id:     req.Id,
		Config: builder.Config,
	}, nil
}
