// Copyright 2023 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package common

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"
)

func TestGetBuildEntities(t *testing.T) {
	t.Parallel()

	ctx := memory.Use(context.Background())
	ftt.Run("GetBuild", t, func(t *ftt.Test) {
		t.Run("ok", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx, &model.Build{
				Proto: &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Status: pb.Status_SUCCESS,
				},
			}), should.BeNil)
			b, err := GetBuild(ctx, 1)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, b.Proto, should.Resemble(&pb.Build{
				Id: 1,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Status: pb.Status_SUCCESS,
			}))
		})

		t.Run("wrong BuildID", func(t *ftt.Test) {
			ctx := memory.Use(context.Background())
			assert.Loosely(t, datastore.Put(ctx, &model.Build{
				Proto: &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Status: pb.Status_SUCCESS,
				},
			}), should.BeNil)
			_, err := GetBuild(ctx, 2)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("resource not found"))
		})
	})

	ftt.Run("GetBuildEntities", t, func(t *ftt.Test) {
		bld := &model.Build{
			ID: 1,
			Proto: &pb.Build{
				Id: 1,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Status: pb.Status_SUCCESS,
			},
		}
		bk := datastore.KeyForObj(ctx, bld)
		bs := &model.BuildStatus{Build: bk}
		infra := &model.BuildInfra{Build: bk}
		steps := &model.BuildSteps{Build: bk}
		inputProp := &model.BuildInputProperties{Build: bk}
		assert.Loosely(t, datastore.Put(ctx, bld, infra, bs, steps, inputProp), should.BeNil)

		t.Run("partial not found", func(t *ftt.Test) {
			_, err := GetBuildEntities(ctx, bld.Proto.Id, model.BuildOutputPropertiesKind)
			assert.Loosely(t, err, should.ErrLike("not found"))
		})

		t.Run("pass", func(t *ftt.Test) {
			outputProp := &model.BuildOutputProperties{Build: bk}
			assert.Loosely(t, datastore.Put(ctx, outputProp), should.BeNil)
			t.Run("all", func(t *ftt.Test) {
				entities, err := GetBuildEntities(ctx, bld.Proto.Id, model.BuildKind, model.BuildStatusKind, model.BuildStepsKind, model.BuildInfraKind, model.BuildInputPropertiesKind, model.BuildOutputPropertiesKind)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, entities, should.HaveLength(6))
			})
			t.Run("only infra", func(t *ftt.Test) {
				entities, err := GetBuildEntities(ctx, bld.Proto.Id, model.BuildInfraKind)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, entities, should.HaveLength(1))
			})
		})
	})
}
