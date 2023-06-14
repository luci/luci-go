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

	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestGetBuildEntities(t *testing.T) {
	t.Parallel()

	ctx := memory.Use(context.Background())
	Convey("GetBuild", t, func() {
		Convey("ok", func() {
			So(datastore.Put(ctx, &model.Build{
				Proto: &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Status: pb.Status_SUCCESS,
				},
			}), ShouldBeNil)
			b, err := GetBuild(ctx, 1)
			So(err, ShouldBeNil)
			So(b.Proto, ShouldResembleProto, &pb.Build{
				Id: 1,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Status: pb.Status_SUCCESS,
			})
		})

		Convey("wrong BuildID", func() {
			ctx := memory.Use(context.Background())
			So(datastore.Put(ctx, &model.Build{
				Proto: &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Status: pb.Status_SUCCESS,
				},
			}), ShouldBeNil)
			_, err := GetBuild(ctx, 2)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "resource not found")
		})
	})

	Convey("GetBuildEntities", t, func() {
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
		So(datastore.Put(ctx, bld, infra, bs, steps, inputProp), ShouldBeNil)

		Convey("partial not found", func() {
			_, err := GetBuildEntities(ctx, bld.Proto.Id, model.BuildOutputPropertiesKind)
			So(err, ShouldErrLike, "not found")
		})

		Convey("pass", func() {
			outputProp := &model.BuildOutputProperties{Build: bk}
			So(datastore.Put(ctx, outputProp), ShouldBeNil)
			Convey("all", func() {
				entities, err := GetBuildEntities(ctx, bld.Proto.Id, model.BuildKind, model.BuildStatusKind, model.BuildStepsKind, model.BuildInfraKind, model.BuildInputPropertiesKind, model.BuildOutputPropertiesKind)
				So(err, ShouldBeNil)
				So(entities, ShouldHaveLength, 6)
			})
			Convey("only infra", func() {
				entities, err := GetBuildEntities(ctx, bld.Proto.Id, model.BuildInfraKind)
				So(err, ShouldBeNil)
				So(entities, ShouldHaveLength, 1)
			})
		})
	})
}
