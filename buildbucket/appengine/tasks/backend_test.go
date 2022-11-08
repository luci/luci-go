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

package tasks

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

func TestBackendTask(t *testing.T) {
	t.Parallel()

	Convey("assert createBackendTask", t, func() {
		ctx := memory.Use(context.Background())

		build := &model.Build{
			ID: 1,
			Proto: &pb.Build{
				Builder: &pb.BuilderID{
					Builder: "builder",
					Bucket:  "bucket",
					Project: "project",
				},
				Id: 1,
			},
		}
		bk := datastore.KeyForObj(ctx, build)
		infra := &model.BuildInfra{
			Build: bk,
			Proto: &pb.BuildInfra{
				Backend: &pb.BuildInfra_Backend{
					Task: &pb.Task{
						Id: &pb.TaskID{
							Id:     "1",
							Target: "swarming://mytarget",
						},
					},
				},
				Buildbucket: &pb.BuildInfra_Buildbucket{
					Hostname: "some unique host name",
				},
			},
		}

		So(datastore.Put(ctx, build, infra), ShouldBeNil)
		err := CreateBackendTask(ctx, 1)
		So(err, ShouldErrLike, "Method not implemented")
	})
}
