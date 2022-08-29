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
	"testing"

	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/buildbucket"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/buildbucket/appengine/internal/buildtoken"
	"go.chromium.org/luci/buildbucket/appengine/internal/metrics"
	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"

	. "go.chromium.org/luci/common/testing/assertions"
)

func TestUpdateBuildTask(t *testing.T) {
	t.Parallel()

	tk, _ := buildtoken.GenerateToken(1, pb.TokenBody_TASK)

	Convey("UpdateBuildTask", t, func() {
		srv := &Builds{}
		ctx := memory.Use(context.Background())
		ctx = metrics.WithServiceInfo(ctx, "svc", "job", "ins")
		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(buildbucket.BuildbucketBackendTokenHeader, tk))
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		ctx, _ = tsmon.WithDummyInMemory(ctx)
		ctx = txndefer.FilterRDS(ctx)

		t0 := testclock.TestRecentTimeUTC

		// helper function to call UpdateBuild.
		updateBuildTask := func(ctx context.Context, req *pb.UpdateBuildTaskRequest) error {
			_, err := srv.UpdateBuildTask(ctx, req)
			return err
		}

		// create and save a sample build in the datastore
		build := &model.Build{
			ID: 1,
			Proto: &pb.Build{
				Id: 1,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Status: pb.Status_STARTED,
			},
			CreateTime:  t0,
			UpdateToken: tk,
		}
		bk := datastore.KeyForObj(ctx, build)
		infra := &model.BuildInfra{
			Build: bk,
			Proto: &pb.BuildInfra{
				Buildbucket: &pb.BuildInfra_Buildbucket{
					Hostname: "bbhost",
					Agent: &pb.BuildInfra_Buildbucket_Agent{
						Input: &pb.BuildInfra_Buildbucket_Agent_Input{
							Data: map[string]*pb.InputDataRef{},
						},
					},
				},
			},
		}
		So(datastore.Put(ctx, build, infra), ShouldBeNil)

		Convey("tokenValidation", func() {
			Convey("buildID matches token", func() {
				req := &pb.UpdateBuildTaskRequest{
					BuildId: "1",
					Task:    &pb.Task{},
				}
				So(updateBuildTask(ctx, req), ShouldBeRPCOK)
			})
			Convey("buildID does not match token", func() {
				req := &pb.UpdateBuildTaskRequest{
					BuildId: "one",
					Task:    &pb.Task{},
				}
				So(updateBuildTask(ctx, req), ShouldBeRPCUnknown)
			})
		})
	})
}
