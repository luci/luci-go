// Copyright 2025 The LUCI Authors.
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
	"testing"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/buildbucket/appengine/internal/config"
	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"
)

func TestScheduleBuildOp(t *testing.T) {
	t.Parallel()

	ctx := memory.Use(t.Context())

	assert.NoErr(t, config.SetTestSettingsCfg(ctx, &pb.SettingsCfg{
		Experiment: &pb.ExperimentSettings{
			Experiments: []*pb.ExperimentSettings_Experiment{
				{Name: "some-experiment"},
			},
		},
	}))

	bucketEntity := func(proj, buck string) *model.Bucket {
		return &model.Bucket{
			Parent: model.ProjectKey(ctx, proj),
			ID:     buck,
			Proto:  &pb.Bucket{},
		}
	}

	buildEntity := func(id int64) *model.Build {
		return &model.Build{
			ID: id,
			Proto: &pb.Build{
				Builder: &pb.BuilderID{
					Project: "ignored-project",
					Bucket:  "ignored-bucket",
					Builder: "ignored-builder",
				},
			},
		}
	}

	buildInfraEntity := func(id int64) *model.BuildInfra {
		return &model.BuildInfra{
			Build: datastore.KeyForObj(ctx, buildEntity(id)),
			Proto: &pb.BuildInfra{},
		}
	}

	scheduleReq := func(proj, buck, builder string, parentBuildID int64) *pb.ScheduleBuildRequest {
		return &pb.ScheduleBuildRequest{
			Builder: &pb.BuilderID{
				Project: proj,
				Bucket:  buck,
				Builder: builder,
			},
			ParentBuildId: parentBuildID,
			DryRun:        true,
		}
	}

	const parentBuildID = 123

	assert.NoErr(t, datastore.Put(ctx,
		bucketEntity("proj1", "bucket1"),
		bucketEntity("proj1", "bucket2"),
		buildEntity(parentBuildID),
		buildInfraEntity(parentBuildID),
	))

	op, err := newScheduleBuildOp(ctx, []*pb.ScheduleBuildRequest{
		scheduleReq("proj1", "bucket1", "builder", 0),
		scheduleReq("proj1", "bucket1", "builder", parentBuildID),
		scheduleReq("proj1", "bucket2", "builder", 0),
	})
	assert.NoErr(t, err)

	assert.Loosely(t, op.Reqs, should.HaveLength(3))
	assert.Loosely(t, op.Errs, should.HaveLength(3))
	assert.Loosely(t, op.Builds, should.HaveLength(3))

	assert.That(t, op.WellKnownExperiments.ToSortedSlice(), should.Match([]string{"some-experiment"}))

	pending := 0
	for range op.Pending() {
		pending++
	}
	assert.That(t, pending, should.Equal(3))

	op.Fail(op.Reqs[0], errors.New("boom"))
	pending = 0
	for range op.Pending() {
		pending++
	}
	assert.That(t, pending, should.Equal(2))

	op.SetBuild(op.Reqs[1], &model.Build{
		Proto: &pb.Build{Id: 666},
	})

	out, errs := op.Finalize(ctx)
	assert.Loosely(t, out, should.HaveLength(3))
	assert.Loosely(t, errs, should.HaveLength(3))

	assert.That(t, errs[0], should.ErrLike("boom"))
	assert.Loosely(t, out[0], should.BeNil)

	assert.NoErr(t, errs[1])
	assert.That(t, out[1], should.Match(&pb.Build{Id: 666}))

	assert.That(t, errs[2], should.ErrLike("was left unprocessed"))
	assert.Loosely(t, out[2], should.BeNil)
}

func TestPrefetchBuilders(t *testing.T) {
	t.Parallel()

	ctx := memory.Use(t.Context())

	assert.NoErr(t, config.SetTestSettingsCfg(ctx, &pb.SettingsCfg{}))

	bucketEntity := func(proj, buck string, dynamic bool) *model.Bucket {
		var swarming *pb.Swarming
		if !dynamic {
			swarming = &pb.Swarming{}
		}
		return &model.Bucket{
			Parent: model.ProjectKey(ctx, proj),
			ID:     buck,
			Proto:  &pb.Bucket{Swarming: swarming},
		}
	}

	builderEntity := func(proj, buck, builder string) *model.Builder {
		return &model.Builder{
			Parent: model.BucketKey(ctx, proj, buck),
			ID:     builder,
			Config: &pb.BuilderConfig{Name: builder},
		}
	}

	scheduleReq := func(proj, buck, builder string) *pb.ScheduleBuildRequest {
		return &pb.ScheduleBuildRequest{
			Builder: &pb.BuilderID{
				Project: proj,
				Bucket:  buck,
				Builder: builder,
			},
			DryRun: true,
		}
	}

	assert.NoErr(t, datastore.Put(ctx,
		bucketEntity("proj", "static", false),
		bucketEntity("proj", "dynamic", true),
		builderEntity("proj", "static", "builder1"),
		builderEntity("proj", "static", "builder2"),
	))

	op, err := newScheduleBuildOp(ctx, []*pb.ScheduleBuildRequest{
		scheduleReq("proj", "", ""), // broken request
		scheduleReq("proj", "static", "builder1"),
		scheduleReq("proj", "static", "builder1"), // multiple requests for the same static builder
		scheduleReq("proj", "static", "builder2"),
		scheduleReq("proj", "dynamic", "builder1"),
		scheduleReq("proj", "dynamic", "builder1"), // multiple requests for the same dynamic builder
		scheduleReq("proj", "dynamic", "builder2"),
		scheduleReq("proj", "static", "missing"),   // missing static builder
		scheduleReq("proj", "missing", "builder1"), // missing bucket
	})
	assert.NoErr(t, err)

	op.PrefetchBuilders(ctx)

	assert.That(t, op.Builders, should.Match(map[string]*pb.BuilderConfig{
		"proj/static/builder1": {Name: "builder1"},
		"proj/static/builder2": {Name: "builder2"},
	}))

	errs := make([]string, len(op.Errs))
	for i, err := range op.Errs {
		if err != nil {
			errs[i] = err.Error()
		}
	}
	assert.That(t, errs, should.Match([]string{
		"bucket is required",
		"",
		"",
		"",
		"",
		"",
		"",
		`rpc error: code = NotFound desc = builder not found: "missing"`,
		`rpc error: code = NotFound desc = bucket not found: "proj/missing"`,
	}))
}
