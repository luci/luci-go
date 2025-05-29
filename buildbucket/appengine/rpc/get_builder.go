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

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/reflectutil"
	"go.chromium.org/luci/gae/service/datastore"
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
		return errors.Fmt("id: %w", err)
	}

	return nil
}

func stringsToRequestedDimensions(strDims []string) (map[string][]*pb.RequestedDimension, []string) {
	// key -> slice of dimensions (key, value, expiration) with matching keys.
	dims := make(map[string][]*pb.RequestedDimension)
	var empty []string

	for _, d := range strDims {
		exp, k, v := config.ParseDimension(d)
		if v == "" {
			empty = append(empty, k)
			continue
		}
		dim := &pb.RequestedDimension{
			Key:   k,
			Value: v,
		}
		if exp > 0 {
			dim.Expiration = &durationpb.Duration{
				Seconds: exp,
			}
		}
		dims[k] = append(dims[k], dim)
	}
	return dims, empty
}

// applyShadowAdjustment makes a copy of the builder config then applies shadow
// builder adjustments to it.
func applyShadowAdjustment(cfg *pb.BuilderConfig) *pb.BuilderConfig {
	rtnCfg := reflectutil.ShallowCopy(cfg).(*pb.BuilderConfig)
	shadowBldrCfg := cfg.GetShadowBuilderAdjustments()
	if shadowBldrCfg == nil {
		return rtnCfg
	}
	if shadowBldrCfg.ServiceAccount != "" {
		rtnCfg.ServiceAccount = shadowBldrCfg.ServiceAccount
	}

	if len(shadowBldrCfg.Dimensions) > 0 {
		dims, _ := stringsToRequestedDimensions(rtnCfg.Dimensions)
		shadowDims, empty := stringsToRequestedDimensions(shadowBldrCfg.Dimensions)

		for k, d := range shadowDims {
			dims[k] = d
		}
		for _, key := range empty {
			delete(dims, key)
		}
		var updatedDims []string
		for _, dims := range dims {
			for _, dim := range dims {
				dimStr := fmt.Sprintf("%s:%s", dim.Key, dim.Value)
				if dim.Expiration != nil {
					dimStr = fmt.Sprintf("%d:%s", dim.Expiration.Seconds, dimStr)
				}
				updatedDims = append(updatedDims, dimStr)
			}
		}
		sort.Strings(updatedDims)
		rtnCfg.Dimensions = updatedDims
	}

	if shadowBldrCfg.Properties != "" {
		if rtnCfg.GetProperties() == "" {
			rtnCfg.Properties = shadowBldrCfg.Properties
		} else {
			origProp := &structpb.Struct{}
			shadowProp := &structpb.Struct{}
			if err := protojson.Unmarshal([]byte(rtnCfg.Properties), origProp); err != nil {
				// Builder config should have been validated already.
				panic(errors.Fmt("error unmarshaling builder properties for %q: %w", rtnCfg.Name, err))
			}
			if err := protojson.Unmarshal([]byte(shadowBldrCfg.Properties), shadowProp); err != nil {
				// Builder config should have been validated already.
				panic(errors.Fmt("error unmarshaling builder shadow properties for %q: %w", rtnCfg.Name, err))
			}
			for k, v := range shadowProp.GetFields() {
				origProp.Fields[k] = v
			}
			updatedProp, err := protojson.Marshal(origProp)
			if err != nil {
				panic(errors.Fmt("error marshaling builder properties for %q: %w", rtnCfg.Name, err))
			}
			rtnCfg.Properties = string(updatedProp)
		}
	}
	rtnCfg.ShadowBuilderAdjustments = nil
	return rtnCfg
}

func trySynthesizeFromShadowedBuilder(ctx context.Context, req *pb.GetBuilderRequest) (*pb.BuilderItem, error) {
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
		return nil, errors.Fmt("failed to fetch entities: %w", err)
	}
	for _, bldr := range builders {
		if bldr.Config != nil {
			cfgCopy := applyShadowAdjustment(bldr.Config)
			return &pb.BuilderItem{
				Id:     req.Id,
				Config: cfgCopy,
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
		return trySynthesizeFromShadowedBuilder(ctx, req)
	case err != nil:
		return nil, err
	}

	if req.Mask == nil {
		req.Mask = &pb.BuilderMask{Type: pb.BuilderMask_CONFIG_ONLY}
	}

	response := &pb.BuilderItem{Id: req.Id}

	switch req.Mask.Type {
	case pb.BuilderMask_ALL:
		response.Config = builder.Config
		response.Metadata = builder.Metadata
	case pb.BuilderMask_CONFIG_ONLY:
		response.Config = builder.Config
	case pb.BuilderMask_METADATA_ONLY:
		response.Metadata = builder.Metadata
	}

	return response, nil
}
