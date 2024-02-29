// Copyright 2024 The LUCI Authors.
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

package rpcs

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/acls"
	"go.chromium.org/luci/swarming/server/model"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func createMockTaskResult(ctx context.Context, tags []string, state apipb.TaskState, testTime time.Time) *model.TaskResultSummary {
	reqKey, err := model.TimestampToRequestKey(ctx, testTime.Add(-2*time.Hour), 0)
	if err != nil {
		panic(err)
	}
	tags = append(tags, "pool:example-pool")
	trs := &model.TaskResultSummary{
		TaskResultCommon: model.TaskResultCommon{
			State:            state,
			Modified:         testTime,
			BotVersion:       "bot_version_123",
			BotDimensions:    model.BotDimensions{"os": []string{"linux"}, "cpu": []string{"x86_64"}},
			CurrentTaskSlice: 1,
			Started:          datastore.NewIndexedNullable(testTime.Add(-1 * time.Hour)),
			Completed:        datastore.NewIndexedNullable(testTime),
			DurationSecs:     datastore.NewUnindexedOptional(3600.0),
			ExitCode:         datastore.NewUnindexedOptional(int64(0)),
			Failure:          false,
			InternalFailure:  false,
		},
		Key:                  model.TaskResultSummaryKey(ctx, reqKey),
		BotID:                datastore.NewUnindexedOptional("bot123"),
		Created:              testTime.Add(-2 * time.Hour),
		Tags:                 tags,
		RequestName:          "example-request",
		RequestUser:          "user@example.com",
		RequestPriority:      50,
		RequestAuthenticated: "authenticated-user@example.com",
		RequestRealm:         "project:visible-realm",
		RequestPool:          "example-pool",
		RequestBotID:         "bot123",
		PropertiesHash:       datastore.NewIndexedOptional([]byte("prop-hash")),
		TryNumber:            datastore.NewIndexedNullable(int64(1)),
		CostUSD:              0.05,
		CostSavedUSD:         0.00,
		DedupedFrom:          "",
		ExpirationDelay:      datastore.NewUnindexedOptional(0.0),
	}
	return trs
}

func TestCountTasks(t *testing.T) {
	t.Parallel()

	Convey("TestCountTasks", t, func() {
		ctx := memory.Use(context.Background())
		datastore.GetTestable(ctx).Consistent(true)
		state := NewMockedRequestState()
		state.MockPool("example-pool", "project:visible-realm")
		state.MockPerm("project:visible-realm", acls.PermPoolsListTasks)
		srv := TasksServer{TaskQuerySplitMode: model.SplitCompletely}
		testTime := time.Date(2023, 1, 1, 2, 3, 4, 0, time.UTC)

		Convey("ok; no tags in req", func() {
			one := createMockTaskResult(ctx, nil, apipb.TaskState_CANCELED, testTime)
			two := createMockTaskResult(ctx, nil, apipb.TaskState_CANCELED, testTime.Add(time.Minute*10))
			So(datastore.Put(ctx, one, two), ShouldBeNil)
			req := &apipb.TasksCountRequest{
				Start: timestamppb.New(testTime.Add(-10 * time.Hour)),
				State: apipb.StateQuery_QUERY_ALL,
			}
			resp, err := srv.CountTasks(MockRequestState(ctx, state.SetCaller(AdminFakeCaller)), req)
			So(err, ShouldBeNil)
			So(resp.Count, ShouldEqual, 2)
			So(resp.Now, ShouldNotBeNil)
		})

		Convey("ok; tags in req", func() {
			tags := []string{"os:ubuntu", "os:ubuntu22"}
			one := createMockTaskResult(ctx, tags, apipb.TaskState_CANCELED, testTime)
			two := createMockTaskResult(ctx, tags, apipb.TaskState_CANCELED, testTime.Add(time.Minute*10))
			So(datastore.Put(ctx, one, two), ShouldBeNil)
			req := &apipb.TasksCountRequest{
				Start: timestamppb.New(testTime.Add(-10 * time.Hour)),
				State: apipb.StateQuery_QUERY_ALL,
				Tags:  []string{"pool:example-pool", "os:ubuntu|ubuntu22"},
			}
			resp, err := srv.CountTasks(MockRequestState(ctx, state), req)
			So(err, ShouldBeNil)
			So(resp.Count, ShouldEqual, 2)
			So(resp.Now, ShouldNotBeNil)
		})

		Convey("ok; tags in req; no entities", func() {
			tags := []string{"os:ubuntu"}
			one := createMockTaskResult(ctx, tags, apipb.TaskState_CANCELED, testTime)
			two := createMockTaskResult(ctx, tags, apipb.TaskState_CANCELED, testTime.Add(time.Minute*10))
			So(datastore.Put(ctx, one, two), ShouldBeNil)
			req := &apipb.TasksCountRequest{
				Start: timestamppb.New(testTime.Add(-10 * time.Hour)),
				State: apipb.StateQuery_QUERY_ALL,
				Tags:  []string{"pool:example-pool", "os:ubuntu15|ubuntu22"},
			}
			resp, err := srv.CountTasks(MockRequestState(ctx, state), req)
			So(err, ShouldBeNil)
			So(resp.Count, ShouldEqual, 0)
			So(resp.Now, ShouldNotBeNil)
		})

		Convey("not ok; no pool ACL", func() {
			tags := []string{"os:ubuntu|ubuntu22"}
			one := createMockTaskResult(ctx, tags, apipb.TaskState_CANCELED, testTime)
			two := createMockTaskResult(ctx, tags, apipb.TaskState_CANCELED, testTime.Add(time.Minute*10))
			So(datastore.Put(ctx, one, two), ShouldBeNil)
			req := &apipb.TasksCountRequest{
				Start: timestamppb.New(testTime.Add(-10 * time.Hour)),
				State: apipb.StateQuery_QUERY_ALL,
				Tags:  []string{"pool:example-pool", "os:ubuntu15|ubuntu22"},
			}
			resp, err := srv.CountTasks(MockRequestState(ctx, state.SetCaller(identity.AnonymousIdentity)), req)
			So(err, ShouldHaveGRPCStatus, codes.PermissionDenied)
			So(err, ShouldErrLike, "the caller \"anonymous:anonymous\" doesn't have permission \"swarming.pools.listTasks\" in the pool \"example-pool\" or the pool doesn't exist")
			So(resp, ShouldBeNil)
		})

		Convey("not ok; bad query filter", func() {
			one := createMockTaskResult(ctx, nil, apipb.TaskState_CANCELED, testTime)
			two := createMockTaskResult(ctx, nil, apipb.TaskState_CANCELED, testTime.Add(time.Minute*10))
			So(datastore.Put(ctx, one, two), ShouldBeNil)
			req := &apipb.TasksCountRequest{
				Start: timestamppb.New(testTime.Add(-10 * time.Hour)),
				State: apipb.StateQuery_QUERY_ALL,
				End:   timestamppb.New(testTime.Add(-115 * time.Hour)),
				Tags:  []string{"pool:example-pool"},
			}
			resp, err := srv.CountTasks(MockRequestState(ctx, state), req)
			So(err, ShouldHaveGRPCStatus, codes.Internal)
			So(err, ShouldErrLike, "error in query")
			So(resp, ShouldBeNil)
		})

		Convey("not ok; start time before 2010", func() {
			one := createMockTaskResult(ctx, nil, apipb.TaskState_CANCELED, testTime)
			two := createMockTaskResult(ctx, nil, apipb.TaskState_CANCELED, testTime.Add(time.Minute*10))
			So(datastore.Put(ctx, one, two), ShouldBeNil)
			req := &apipb.TasksCountRequest{
				Start: timestamppb.New(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)),
				State: apipb.StateQuery_QUERY_ALL,
				Tags:  []string{"pool:example-pool"},
			}
			resp, err := srv.CountTasks(MockRequestState(ctx, state), req)
			So(err, ShouldHaveGRPCStatus, codes.Internal)
			So(err, ShouldErrLike, "error in query creation")
			So(resp, ShouldBeNil)
		})

		Convey("not ok; no start time", func() {
			one := createMockTaskResult(ctx, nil, apipb.TaskState_CANCELED, testTime)
			two := createMockTaskResult(ctx, nil, apipb.TaskState_CANCELED, testTime.Add(time.Minute*10))
			So(datastore.Put(ctx, one, two), ShouldBeNil)
			req := &apipb.TasksCountRequest{
				State: apipb.StateQuery_QUERY_ALL,
				Tags:  []string{"pool:example-pool"},
			}
			resp, err := srv.CountTasks(MockRequestState(ctx, state), req)
			So(err, ShouldHaveGRPCStatus, codes.InvalidArgument)
			So(err, ShouldErrLike, "start timestamp is required")
			So(resp, ShouldBeNil)
		})
	})
}
