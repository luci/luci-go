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

package clients

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/secrets/testsecrets"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/buildbucket/appengine/internal/metrics"
	pb "go.chromium.org/luci/buildbucket/proto"

	gomock "github.com/golang/mock/gomock"
	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestBackendTaskClient(t *testing.T) {
	t.Parallel()

	Convey("assert NewBackendClient", t, func() {
		ctl := gomock.NewController(t)
		defer ctl.Finish()
		mockBackend := NewMockTaskBackendClient(ctl)
		now := testclock.TestRecentTimeUTC
		ctx, _ := testclock.UseTime(context.Background(), now)
		ctx = context.WithValue(ctx, MockTaskBackendClientKey, mockBackend)
		ctx = memory.UseWithAppID(ctx, "dev~app-id")
		ctx = txndefer.FilterRDS(ctx)
		ctx = metrics.WithServiceInfo(ctx, "svc", "job", "ins")
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		ctx, _ = tq.TestingContext(ctx, nil)
		store := &testsecrets.Store{
			Secrets: map[string]secrets.Secret{
				"key": {Active: []byte("stuff")},
			},
		}
		ctx = secrets.Use(ctx, store)
		ctx = secrets.GeneratePrimaryTinkAEADForTest(ctx)

		build := &pb.Build{
			Builder: &pb.BuilderID{
				Builder: "builder",
				Bucket:  "bucket",
				Project: "project",
			},
			Id: 1,
		}

		infra := &pb.BuildInfra{
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
		}

		Convey("global settings not defined", func() {
			_, err := NewBackendClient(ctx, build.Builder.Project, infra.Backend.Task.Id.Target, nil)
			So(err, ShouldErrLike, "could not get global settings config")
		})

		Convey("target not in global config", func() {
			backendSetting := []*pb.BackendSetting{}
			settingsCfg := &pb.SettingsCfg{Backends: backendSetting}
			_, err := NewBackendClient(ctx, build.Builder.Project, infra.Backend.Task.Id.Target, settingsCfg)
			So(err, ShouldErrLike, "could not find target in global config settings")
		})

		Convey("target is in global config", func() {
			backendSetting := []*pb.BackendSetting{}
			backendSetting = append(backendSetting, &pb.BackendSetting{
				Target:   "swarming://mytarget",
				Hostname: "hostname",
			})
			settingsCfg := &pb.SettingsCfg{Backends: backendSetting}
			_, err := NewBackendClient(ctx, build.Builder.Project, infra.Backend.Task.Id.Target, settingsCfg)
			So(err, ShouldBeNil)
		})
	})
}
