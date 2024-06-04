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

	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/model"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestCountTasks(t *testing.T) {
	t.Parallel()

	ctx := memory.Use(context.Background())
	datastore.GetTestable(ctx).AutoIndex(true)
	datastore.GetTestable(ctx).Consistent(true)

	state, _ := SetupTestTasks(ctx)

	startTS := timestamppb.New(TestTime)
	endTS := timestamppb.New(TestTime.Add(time.Hour))

	callImpl := func(ctx context.Context, req *apipb.TasksCountRequest) (*apipb.TasksCount, error) {
		return (&TasksServer{
			// memory.Use(...) datastore fake doesn't support IN queries currently.
			TaskQuerySplitMode: model.SplitCompletely,
		}).CountTasks(ctx, req)
	}
	call := func(req *apipb.TasksCountRequest) (*apipb.TasksCount, error) {
		return callImpl(MockRequestState(ctx, state), req)
	}
	callAsAdmin := func(req *apipb.TasksCountRequest) (*apipb.TasksCount, error) {
		return callImpl(MockRequestState(ctx, state.SetCaller(AdminFakeCaller)), req)
	}

	Convey("Tags filter is checked", t, func() {
		_, err := callAsAdmin(&apipb.TasksCountRequest{
			Start: startTS,
			End:   endTS,
			Tags:  []string{"k:"},
		})
		So(err, ShouldHaveGRPCStatus, codes.InvalidArgument)
	})

	Convey("Time range is checked", t, func() {
		Convey("No start time", func() {
			_, err := callAsAdmin(&apipb.TasksCountRequest{})
			So(err, ShouldHaveGRPCStatus, codes.InvalidArgument)
			So(err, ShouldErrLike, "start timestamp is required")
		})

		Convey("Ancient start time", func() {
			_, err := callAsAdmin(&apipb.TasksCountRequest{
				Start: timestamppb.New(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)),
			})
			So(err, ShouldHaveGRPCStatus, codes.InvalidArgument)
			So(err, ShouldErrLike, "invalid time range")
		})

		Convey("Ancient end time", func() {
			_, err := callAsAdmin(&apipb.TasksCountRequest{
				Start: startTS,
				End:   timestamppb.New(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)),
			})
			So(err, ShouldHaveGRPCStatus, codes.InvalidArgument)
			So(err, ShouldErrLike, "invalid time range")
		})

		Convey("End must be after start", func() {
			_, err := callAsAdmin(&apipb.TasksCountRequest{
				Start: endTS,
				End:   startTS,
			})
			So(err, ShouldHaveGRPCStatus, codes.InvalidArgument)
			So(err, ShouldErrLike, "invalid time range")
		})
	})

	Convey("ACLs", t, func() {
		Convey("Listing only visible pools: OK", func() {
			_, err := call(&apipb.TasksCountRequest{
				Start: startTS,
				End:   endTS,
				Tags:  []string{"pool:visible-pool1|visible-pool2"},
			})
			So(err, ShouldBeNil)
		})

		Convey("Listing visible and invisible pool: permission denied", func() {
			_, err := call(&apipb.TasksCountRequest{
				Start: startTS,
				End:   endTS,
				Tags:  []string{"pool:visible-pool1|hidden-pool1"},
			})
			So(err, ShouldHaveGRPCStatus, codes.PermissionDenied)
		})

		Convey("Listing visible and invisible pool as admin: OK", func() {
			_, err := callAsAdmin(&apipb.TasksCountRequest{
				Start: startTS,
				End:   endTS,
				Tags:  []string{"pool:visible-pool1|hidden-pool1"},
			})
			So(err, ShouldBeNil)
		})

		Convey("Listing all pools as non-admin: permission denied", func() {
			_, err := call(&apipb.TasksCountRequest{
				Start: startTS,
				End:   endTS,
			})
			So(err, ShouldHaveGRPCStatus, codes.PermissionDenied)
		})

		Convey("Listing all pools as admin: OK", func() {
			_, err := callAsAdmin(&apipb.TasksCountRequest{
				Start: startTS,
				End:   endTS,
			})
			So(err, ShouldBeNil)
		})
	})

	Convey("Filtering", t, func() {
		endRange := endTS

		count := func(state apipb.StateQuery, tags ...string) int {
			resp, err := callAsAdmin(&apipb.TasksCountRequest{
				State: state,
				Start: startTS,
				End:   endRange,
				Tags:  tags,
			})
			So(err, ShouldBeNil)
			So(resp.Now, ShouldNotBeNil)
			return int(resp.Count)
		}

		// No filters at all.
		So(count(apipb.StateQuery_QUERY_ALL), ShouldEqual, 36)

		// State filters on their own (without tag filtering).
		So(count(apipb.StateQuery_QUERY_PENDING), ShouldEqual, 3)
		So(count(apipb.StateQuery_QUERY_RUNNING), ShouldEqual, 3)
		So(count(apipb.StateQuery_QUERY_PENDING_RUNNING), ShouldEqual, 6)
		So(count(apipb.StateQuery_QUERY_COMPLETED), ShouldEqual, 9)         // success+failure+dedup
		So(count(apipb.StateQuery_QUERY_COMPLETED_SUCCESS), ShouldEqual, 6) // success+dedup
		So(count(apipb.StateQuery_QUERY_COMPLETED_FAILURE), ShouldEqual, 3)
		So(count(apipb.StateQuery_QUERY_EXPIRED), ShouldEqual, 3)
		So(count(apipb.StateQuery_QUERY_TIMED_OUT), ShouldEqual, 3)
		So(count(apipb.StateQuery_QUERY_BOT_DIED), ShouldEqual, 3)
		So(count(apipb.StateQuery_QUERY_CANCELED), ShouldEqual, 3)
		So(count(apipb.StateQuery_QUERY_DEDUPED), ShouldEqual, 3)
		So(count(apipb.StateQuery_QUERY_KILLED), ShouldEqual, 3)
		So(count(apipb.StateQuery_QUERY_NO_RESOURCE), ShouldEqual, 3)
		So(count(apipb.StateQuery_QUERY_CLIENT_ERROR), ShouldEqual, 3)

		// Simple tags filter.
		So(count(apipb.StateQuery_QUERY_ALL, "idx:0"), ShouldEqual, 12)
		// AND tags filter.
		So(count(apipb.StateQuery_QUERY_ALL, "idx:0", "pfx:pending"), ShouldEqual, 1)
		// OR tags filter.
		So(count(apipb.StateQuery_QUERY_ALL, "idx:0|1"), ShouldEqual, 24)
		// OR tags filter with intersecting results.
		So(count(apipb.StateQuery_QUERY_ALL, "idx:0|1", "dup:0|1"), ShouldEqual, 24)
		// OR tags filter with no results.
		So(count(apipb.StateQuery_QUERY_ALL, "idx:4|5|6"), ShouldEqual, 0)

		// Filtering on state + tags (selected non-trivial cases).
		So(count(apipb.StateQuery_QUERY_PENDING, "idx:0|1", "dup:0|1"), ShouldEqual, 2)
		So(count(apipb.StateQuery_QUERY_PENDING_RUNNING, "idx:0|1", "dup:0|1"), ShouldEqual, 4)
		So(count(apipb.StateQuery_QUERY_COMPLETED, "idx:0|1", "dup:0|1"), ShouldEqual, 6)
		So(count(apipb.StateQuery_QUERY_COMPLETED_SUCCESS, "idx:0|1", "dup:0|1"), ShouldEqual, 4)
		So(count(apipb.StateQuery_QUERY_DEDUPED, "idx:0|1", "dup:0|1"), ShouldEqual, 2)

		// Limited time range (covers only 1 mocked task per category instead of 3).
		endRange = timestamppb.New(TestTime.Add(5 * time.Minute))

		So(count(apipb.StateQuery_QUERY_ALL), ShouldEqual, 12)
		So(count(apipb.StateQuery_QUERY_ALL, "idx:0"), ShouldEqual, 12)
		So(count(apipb.StateQuery_QUERY_ALL, "idx:1"), ShouldEqual, 0)
		So(count(apipb.StateQuery_QUERY_COMPLETED), ShouldEqual, 3)
		So(count(apipb.StateQuery_QUERY_PENDING_RUNNING), ShouldEqual, 2)
		So(count(apipb.StateQuery_QUERY_PENDING_RUNNING, "idx:0|1", "dup:0|1"), ShouldEqual, 2)
	})
}
