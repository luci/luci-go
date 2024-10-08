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

	gomock "github.com/golang/mock/gomock"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/secrets/testsecrets"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/buildbucket/appengine/internal/metrics"
	pb "go.chromium.org/luci/buildbucket/proto"
)

func TestBackendTaskClient(t *testing.T) {
	t.Parallel()

	ftt.Run("assert NewBackendClient", t, func(t *ftt.Test) {
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

		t.Run("global settings not defined", func(t *ftt.Test) {
			_, err := NewBackendClient(ctx, build.Builder.Project, infra.Backend.Task.Id.Target, nil)
			assert.Loosely(t, err, should.ErrLike("could not get global settings config"))
		})

		t.Run("target not in global config", func(t *ftt.Test) {
			backendSetting := []*pb.BackendSetting{}
			settingsCfg := &pb.SettingsCfg{Backends: backendSetting}
			_, err := NewBackendClient(ctx, build.Builder.Project, infra.Backend.Task.Id.Target, settingsCfg)
			assert.Loosely(t, err, should.ErrLike("could not find target in global config settings"))
		})

		t.Run("target is in global config", func(t *ftt.Test) {
			backendSetting := []*pb.BackendSetting{}
			backendSetting = append(backendSetting, &pb.BackendSetting{
				Target:   "swarming://mytarget",
				Hostname: "hostname",
			})
			settingsCfg := &pb.SettingsCfg{Backends: backendSetting}
			_, err := NewBackendClient(ctx, build.Builder.Project, infra.Backend.Task.Id.Target, settingsCfg)
			assert.Loosely(t, err, should.BeNil)
		})
	})
}
