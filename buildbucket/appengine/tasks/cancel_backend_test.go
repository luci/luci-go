// Copyright 2023 The LUCI Authors.
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
	"time"

	"github.com/golang/mock/gomock"
	"google.golang.org/api/googleapi"
	"google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/caching"

	"go.chromium.org/luci/buildbucket/appengine/internal/clients"
	"go.chromium.org/luci/buildbucket/appengine/internal/config"
	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestHandleCancelBackendTask(t *testing.T) {
	t.Parallel()
	Convey("TestHandleCancelBackendTask", t, func() {
		ctl := gomock.NewController(t)
		defer ctl.Finish()
		mockBackend := clients.NewMockTaskBackendClient(ctl)
		now := testclock.TestRecentTimeUTC
		ctx, _ := testclock.UseTime(context.Background(), now)
		ctx = context.WithValue(ctx, clients.MockTaskBackendClientKey, mockBackend)
		ctx = caching.WithEmptyProcessCache(ctx)
		ctx = memory.UseWithAppID(ctx, "dev~app-id")
		backendSetting := []*pb.BackendSetting{
			&pb.BackendSetting{
				Target:   "swarming://chromium-swarm-dev",
				Hostname: "hostname2",
				Mode: &pb.BackendSetting_FullMode_{
					FullMode: &pb.BackendSetting_FullMode{
						BuildSyncSetting: &pb.BackendSetting_BuildSyncSetting{
							Shards:              5,
							SyncIntervalSeconds: 300,
						},
					},
				},
				TaskCreatingTimeout: durationpb.New(8 * time.Minute),
			},
		}
		settingsCfg := &pb.SettingsCfg{Backends: backendSetting}
		err := config.SetTestSettingsCfg(ctx, settingsCfg)
		So(err, ShouldBeNil)

		Convey("ok", func() {
			mockBackend.EXPECT().CancelTasks(gomock.Any(), gomock.Any()).Return(&pb.CancelTasksResponse{
				Tasks: []*pb.Task{
					&pb.Task{
						Id:       &pb.TaskID{Id: "abc123", Target: "swarming://chromium-swarm"},
						Link:     "this_is_a_url_link",
						UpdateId: 1,
					},
				},
			}, nil)
			So(HandleCancelBackendTask(ctx, "project:bucket", "swarming://chromium-swarm-dev", "a83908f94as40"), ShouldBeNil)
		})

		Convey("failed CancelTasks RPC call 400", func() {
			mockBackend.EXPECT().CancelTasks(gomock.Any(), gomock.Any()).Return(nil, &googleapi.Error{Code: 400})
			err := HandleCancelBackendTask(ctx, "project:bucket", "swarming://chromium-swarm-dev", "a83908f94as40")
			So(err, ShouldErrLike, "fatal error in cancelling task a83908f94as40 for target swarming://chromium-swarm-dev: googleapi: got HTTP response code 400")
		})

		Convey("failed CancelTasks RPC call 500", func() {
			mockBackend.EXPECT().CancelTasks(gomock.Any(), gomock.Any()).Return(nil, &googleapi.Error{Code: 500})
			err := HandleCancelBackendTask(ctx, "project:bucket", "swarming://chromium-swarm-dev", "a83908f94as40")
			So(err, ShouldErrLike, "transient error in canceling task a83908f94as40 for target swarming://chromium-swarm-dev: googleapi: got HTTP response code 500")
		})
	})
}
