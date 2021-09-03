// Copyright 2021 The LUCI Authors.
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
package groups

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/auth_service/api/rpcpb"
	"go.chromium.org/luci/auth_service/impl/model"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
	_ "go.chromium.org/luci/gae/service/datastore/crbug1242998safeget"
)

func TestGroupsServer(t *testing.T) {
	t.Parallel()
	srv := Server{}
	createdTime := time.Date(2021, time.August, 16, 15, 20, 0, 0, time.UTC)

	Convey("ListGroups RPC call", t, func() {
		ctx := memory.Use(context.Background())

		// Groups built from model.AuthGroup definition.
		So(datastore.Put(ctx,
			&model.AuthGroup{
				ID:     "z-test-group",
				Parent: model.RootKey(ctx),
				Members: []string{
					"user:test-user-1",
					"user:test-user-2",
				},
				Globs: []string{
					"test-user-1@example.com",
					"test-user-2@example.com",
				},
				Nested: []string{
					"group/tester",
				},
				Description: "This is a test group.",
				Owners:      "testers",
				CreatedTS:   createdTime,
				CreatedBy:   "user:test-user-1@example.com",
			},
			&model.AuthGroup{
				ID:     "test-group-2",
				Parent: model.RootKey(ctx),
				Members: []string{
					"user:test-user-2",
				},
				Globs: []string{
					"test-user-2@example.com",
				},
				Nested: []string{
					"group/test-group",
				},
				Description: "This is another test group.",
				Owners:      "test-group",
				CreatedTS:   createdTime,
				CreatedBy:   "user:test-user-2@example.com",
			},
			&model.AuthGroup{
				ID:      "test-group-3",
				Parent:  model.RootKey(ctx),
				Members: []string{},
				Globs:   []string{},
				Nested: []string{
					"group/tester",
				},
				Description: "This is yet another test group.",
				Owners:      "testers",
				CreatedTS:   createdTime,
				CreatedBy:   "user:test-user-1@example.com",
			}), ShouldBeNil)

		// What expected response should be, built with pb.
		responseGroups := &rpcpb.ListGroupsResponse{
			Groups: []*rpcpb.AuthGroup{
				{
					Name:        "test-group-2",
					Members:     []string{"user:test-user-2"},
					Globs:       []string{"test-user-2@example.com"},
					Nested:      []string{"group/test-group"},
					Description: "This is another test group.",
					Owners:      "test-group",
					CreatedTs:   timestamppb.New(createdTime),
					CreatedBy:   "user:test-user-2@example.com",
				},
				{
					Name:        "test-group-3",
					Members:     []string{},
					Globs:       []string{},
					Nested:      []string{"group/tester"},
					Description: "This is yet another test group.",
					Owners:      "testers",
					CreatedTs:   timestamppb.New(createdTime),
					CreatedBy:   "user:test-user-1@example.com",
				},
				{
					Name:        "z-test-group",
					Members:     []string{"user:test-user-1", "user:test-user-2"},
					Globs:       []string{"test-user-1@example.com", "test-user-2@example.com"},
					Nested:      []string{"group/tester"},
					Description: "This is a test group.",
					Owners:      "testers",
					CreatedTs:   timestamppb.New(createdTime),
					CreatedBy:   "user:test-user-1@example.com",
				},
			},
		}

		resp, err := srv.ListGroups(ctx, &emptypb.Empty{})
		So(err, ShouldBeNil)
		So(responseGroups.Groups, ShouldResembleProto, resp.Groups)
	})

	Convey("GetGroup RPC call", t, func() {
		ctx := memory.Use(context.Background())

		request := &rpcpb.GetGroupRequest{
			Name: "test-group",
		}

		_, err := srv.GetGroup(ctx, request)
		So(err, ShouldHaveGRPCStatus, codes.NotFound)

		// Groups built from model.AuthGroup definition.
		So(datastore.Put(ctx,
			&model.AuthGroup{
				ID:     "test-group",
				Parent: model.RootKey(ctx),
				Members: []string{
					"user:test-user-1",
					"user:test-user-2",
				},
				Globs: []string{
					"test-user-1@example.com",
					"test-user-2@example.com",
				},
				Nested: []string{
					"group/tester",
				},
				Description: "This is a test group.",
				Owners:      "testers",
				CreatedTS:   createdTime,
				CreatedBy:   "user:test-user-1@example.com",
			}), ShouldBeNil)

		expectedResponse := &rpcpb.AuthGroup{
			Name: "test-group",
			Members: []string{
				"user:test-user-1",
				"user:test-user-2",
			},
			Globs: []string{
				"test-user-1@example.com",
				"test-user-2@example.com",
			},
			Nested: []string{
				"group/tester",
			},
			Description: "This is a test group.",
			Owners:      "testers",
			CreatedTs:   timestamppb.New(createdTime),
			CreatedBy:   "user:test-user-1@example.com",
		}

		actualGroupResponse, err := srv.GetGroup(ctx, request)
		So(err, ShouldBeNil)
		So(actualGroupResponse, ShouldResembleProto, expectedResponse)

	})

}
