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

// Package testutil contains util functions for testing buildbucket RPCs.
package testutil

import (
	"context"

	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"
)

// PutBuilder saves a *model.Builder to datastore for test usage.
func PutBuilder(ctx context.Context, project, bucket, builder string, backend string) {
	bldr := &model.Builder{
		Parent: model.BucketKey(ctx, project, bucket),
		ID:     builder,
		Config: &pb.BuilderConfig{
			Name:         builder,
			SwarmingHost: "host",
		},
	}

	if backend != "" {
		bldr.Config.Backend = &pb.BuilderConfig_Backend{
			Target: backend,
		}
	}

	// TODO: pass in testing.TB and Assert this instead.
	if err := datastore.Put(ctx, bldr); err != nil {
		panic(err)
	}
}

// PutBucket saves a *model.Bucket to datastore for test usage.
func PutBucket(ctx context.Context, project, bucket string, cfg *pb.Bucket) {
	if cfg == nil {
		cfg = &pb.Bucket{}
	}
	if cfg.Name == "" {
		cfg.Name = bucket
	}

	// TODO: pass in testing.TB and Assert this instead.
	err := datastore.Put(ctx, &model.Bucket{
		Parent: model.ProjectKey(ctx, project),
		ID:     bucket,
		Proto:  cfg,
	})
	if err != nil {
		panic(err)
	}
}
