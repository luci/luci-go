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

	"github.com/golang/mock/gomock"
	grpc "google.golang.org/grpc"
	emptypb "google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/buildbucket/appengine/internal/config"
	"go.chromium.org/luci/buildbucket/appengine/internal/metrics"
	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

type MockedClient struct {
	Client *MockTaskBackendClient
	Ctx    context.Context
}

// MockTaskBackendClient is a mock of TaskBackendClient interface.
type MockTaskBackendClient struct {
	ctrl     *gomock.Controller
	recorder *MockTaskBackendClientMockRecorder
}

// MockTaskBackendClientMockRecorder is the mock recorder for MockTaskBackendClient.
type MockTaskBackendClientMockRecorder struct {
	mock *MockTaskBackendClient
}

// NewMockTaskBackendClient creates a new mock instance.
func NewMockTaskBackendClient(ctrl *gomock.Controller) *MockTaskBackendClient {
	mock := &MockTaskBackendClient{ctrl: ctrl}
	mock.recorder = &MockTaskBackendClientMockRecorder{mock}
	return mock
}

// NewMockedClient creates a MockedClient for testing.
func NewMockedClient(ctx context.Context, ctl *gomock.Controller) *MockedClient {
	mockClient := NewMockTaskBackendClient(ctl)
	return &MockedClient{
		Client: mockClient,
		Ctx:    useTaskBackendClientForTesting(ctx, mockClient),
	}
}

// RunTask Mocks the RunTask RPC.
func (mc *MockedClient) RunTask(ctx context.Context, taskReq *pb.RunTaskRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return new(emptypb.Empty), nil
}

// useBuildsClientForTesting specifies that the given test double shall be used
// instead of making calls to TaskBackend.
func useTaskBackendClientForTesting(ctx context.Context, client *MockTaskBackendClient) context.Context {
	return context.WithValue(ctx, MockTaskBackendClientKey{}, client)
}

func TestBackendTask(t *testing.T) {
	t.Parallel()

	Convey("assert createBackendTask", t, func() {
		ctl := gomock.NewController(t)
		defer ctl.Finish()
		mc := NewMockedClient(context.Background(), ctl)
		now := testclock.TestRecentTimeUTC
		ctx, _ := testclock.UseTime(context.Background(), now)
		ctx = context.WithValue(ctx, MockTaskBackendClientKey{}, mc)
		ctx = memory.UseWithAppID(ctx, "dev~app-id")
		ctx = txndefer.FilterRDS(ctx)
		ctx = metrics.WithServiceInfo(ctx, "svc", "job", "ins")
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		ctx, _ = tq.TestingContext(ctx, nil)

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

		Convey("global settings not defined", func() {
			err := CreateBackendTask(ctx, 1)
			So(err, ShouldErrLike, "could not get global settings config")
		})

		Convey("target not in global config", func() {
			backendSettings := []*pb.BackendSettings{}
			settingsCfg := &pb.SettingsCfg{Backend: backendSettings}
			err := config.SetTestSettingsCfg(ctx, settingsCfg)
			So(err, ShouldBeNil)
			err = CreateBackendTask(ctx, 1)
			So(err, ShouldErrLike, "could not find target in global config settings")
		})

		Convey("target is in global config", func() {
			backendSettings := []*pb.BackendSettings{}
			backendSettings = append(backendSettings, &pb.BackendSettings{
				Target:   "swarming://mytarget",
				Hostname: "hostname",
			})
			settingsCfg := &pb.SettingsCfg{Backend: backendSettings}
			err := config.SetTestSettingsCfg(ctx, settingsCfg)
			So(err, ShouldBeNil)
			err = CreateBackendTask(ctx, 1)
			So(err, ShouldBeNil)
		})

	})
}
