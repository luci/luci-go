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

package rpcs

import (
	"context"
	"testing"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/auth/realms"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	configpb "go.chromium.org/luci/swarming/proto/config"
	"go.chromium.org/luci/swarming/server/acls"
	"go.chromium.org/luci/swarming/server/model"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestSwarmingServer(t *testing.T) {
	t.Parallel()

	Convey("With mocks", t, func() {
		const (
			adminID      identity.Identity = "user:admin@example.com"
			unknownID    identity.Identity = "user:unknown@example.com"
			authorizedID identity.Identity = "user:authorized@example.com"
			submitterID  identity.Identity = "user:submitter@example.com"

			taskID = "65aba3a3e6b99310"
		)

		configs := MockedConfigs{
			Settings: &configpb.SettingsCfg{
				Auth: &configpb.AuthSettings{
					AdminsGroup: "admins",
				},
			},
			Pools: &configpb.PoolsCfg{
				Pool: []*configpb.Pool{
					{
						Name:  []string{"visible-pool-1", "visible-pool-2"},
						Realm: "project:visible-realm",
					},
					{
						Name:  []string{"hidden-pool-1", "hidden-pool-2"},
						Realm: "project:hidden-realm",
					},
					{
						Name:  []string{"task-pool"},
						Realm: "project:task-pool-realm",
					},
				},
			},
		}
		db := authtest.NewFakeDB(
			authtest.MockMembership(adminID, "admins"),
		)
		authorized := []realms.Permission{
			acls.PermPoolsDeleteBot,
			acls.PermPoolsTerminateBot,
			acls.PermPoolsCreateBot,
			acls.PermPoolsCancelTask,
			acls.PermPoolsListBots,
			acls.PermPoolsListTasks,
			acls.PermTasksCancel,
		}
		for _, perm := range authorized {
			db.AddMocks(authtest.MockPermission(authorizedID, "project:visible-realm", perm))
		}

		ctx := memory.Use(context.Background())

		key, err := model.TaskIDToRequestKey(ctx, taskID)
		So(err, ShouldBeNil)
		So(datastore.Put(ctx, &model.TaskRequest{
			Key:           key,
			Realm:         "project:task-realm",
			Authenticated: submitterID,
			TaskSlices: []model.TaskSlice{
				{
					Properties: model.TaskProperties{
						Dimensions: map[string][]string{
							"pool": {"task-pool"},
						},
					},
				},
			},
		}), ShouldBeNil)

		srv := SwarmingServer{}

		call := func(caller identity.Identity, botID, taskID string, tags []string) *apipb.ClientPermissions {
			ctx := MockRequestState(ctx, MockedRequestState{
				Caller:  caller,
				AuthDB:  db,
				Configs: configs,
			})
			resp, err := srv.GetPermissions(ctx, &apipb.PermissionsRequest{
				BotId:  botID,
				TaskId: taskID,
				Tags:   tags,
			})
			So(err, ShouldBeNil)
			return resp
		}

		Convey("Admin", func() {
			So(call(adminID, "", taskID, nil), ShouldResembleProto, &apipb.ClientPermissions{
				DeleteBot:         true,
				DeleteBots:        true,
				TerminateBot:      true,
				GetConfigs:        false,
				PutConfigs:        false,
				CancelTask:        true,
				GetBootstrapToken: true,
				CancelTasks:       true,
				ListBots: []string{
					"hidden-pool-1",
					"hidden-pool-2",
					"task-pool",
					"visible-pool-1",
					"visible-pool-2",
				},
				ListTasks: []string{
					"hidden-pool-1",
					"hidden-pool-2",
					"task-pool",
					"visible-pool-1",
					"visible-pool-2",
				},
			})
		})

		Convey("Unknown", func() {
			So(call(unknownID, "", taskID, nil), ShouldResembleProto, &apipb.ClientPermissions{
				// All empty.
			})
		})

		Convey("Authorized pools", func() {
			So(call(authorizedID, "", "", []string{"pool:visible-pool-1"}),
				ShouldResembleProto,
				&apipb.ClientPermissions{
					CancelTask:  true,
					CancelTasks: true,
					DeleteBots:  true,
					ListBots:    []string{"visible-pool-1", "visible-pool-2"},
					ListTasks:   []string{"visible-pool-1", "visible-pool-2"},
				},
			)
		})

		Convey("Hidden pools", func() {
			So(call(authorizedID, "", "", []string{"pool:hidden-pool-1"}),
				ShouldResembleProto,
				&apipb.ClientPermissions{
					ListBots:  []string{"visible-pool-1", "visible-pool-2"},
					ListTasks: []string{"visible-pool-1", "visible-pool-2"},
				},
			)
		})

		Convey("Accessing task", func() {
			// TODO(vadimsh): Add a test once CheckTaskPerm is implemented.
		})

		Convey("Accessing bot", func() {
			// TODO(vadimsh): Add a test once CheckBotPerm is implemented.
		})
	})
}
