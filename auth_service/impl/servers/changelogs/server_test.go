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

package changelogs

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/auth_service/api/rpcpb"
	"go.chromium.org/luci/auth_service/impl/model"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestChangeLogsServer(t *testing.T) {
	t.Parallel()
	srv := Server{}

	Convey("ListChangeLogs RPC call", t, func() {
		ctx := memory.Use(context.Background())
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		So(datastore.Put(ctx,
			&model.AuthDBChange{
				Kind:           "AuthDBChange",
				ID:             "AuthIPWhitelist$a!3000",
				Parent:         model.ChangeLogRevisionKey(ctx, 10000, false),
				Class:          []string{"AuthDBChange", "AuthDBIPWhitelistChange"},
				ChangeType:     3000,
				Comment:        "comment",
				Description:    "description",
				OldDescription: "",
				Target:         "AuthIPWhitelist$a",
				When:           time.Date(2021, time.December, 12, 1, 0, 0, 0, time.UTC),
				Who:            "user:test@example.com",
				AppVersion:     "123-45abc",
			},
			&model.AuthDBChange{
				Kind:              "AuthDBChange",
				ID:                "AuthGlobalConfig$test!7000",
				Parent:            model.ChangeLogRevisionKey(ctx, 10000, false),
				Class:             []string{"AuthDBChange", "AuthDBConfigChange"},
				ChangeType:        7000,
				Comment:           "comment",
				OauthClientID:     "123.test.example.com",
				OauthClientSecret: "aBcD",
				Target:            "AuthGlobalConfig$test",
				When:              time.Date(2019, time.December, 11, 1, 0, 0, 0, time.UTC),
				Who:               "user:test@example.com",
				AppVersion:        "123-45abc",
			},
			&model.AuthDBChange{
				Kind:             "AuthDBChange",
				ID:               "AuthRealmsGlobals$globals!9000",
				Parent:           model.ChangeLogRevisionKey(ctx, 10020, false),
				Class:            []string{"AuthDBChange", "AuthRealmsGlobalsChange"},
				ChangeType:       9000,
				Comment:          "comment",
				PermissionsAdded: []string{"a.existInRealm"},
				Target:           "AuthRealmsGlobals$globals",
				When:             time.Date(2021, time.January, 11, 1, 0, 0, 0, time.UTC),
				Who:              "user:test@example.com",
				AppVersion:       "123-45abc",
			},
		), ShouldBeNil)

		Convey("List all", func() {
			res, err := srv.ListChangeLogs(ctx, &rpcpb.ListChangeLogsRequest{
				PageSize: 2,
			})
			So(err, ShouldBeNil)
			So(res.NextPageToken, ShouldNotBeEmpty)
			So(len(res.Changes), ShouldEqual, 2)
			So(res.Changes[0], ShouldResembleProto, &rpcpb.AuthDBChange{
				ChangeType:       "REALMS_GLOBALS_CHANGED",
				Comment:          "comment",
				PermissionsAdded: []string{"a.existInRealm"},
				Target:           "AuthRealmsGlobals$globals",
				When:             timestamppb.New(time.Date(2021, time.January, 11, 1, 0, 0, 0, time.UTC)),
				Who:              "user:test@example.com",
				AppVersion:       "123-45abc",
			})
			So(res.Changes[1], ShouldResembleProto, &rpcpb.AuthDBChange{
				ChangeType:     "IPWL_CREATED",
				Comment:        "comment",
				Description:    "description",
				OldDescription: "",
				Target:         "AuthIPWhitelist$a",
				When:           timestamppb.New(time.Date(2021, time.December, 12, 1, 0, 0, 0, time.UTC)),
				Who:            "user:test@example.com",
				AppVersion:     "123-45abc",
			})

			res, err = srv.ListChangeLogs(ctx, &rpcpb.ListChangeLogsRequest{
				PageToken: res.NextPageToken,
				PageSize:  2,
			})
			So(err, ShouldBeNil)
			So(res.NextPageToken, ShouldBeEmpty)
			So(len(res.Changes), ShouldEqual, 1)
			So(res.Changes[0], ShouldResembleProto, &rpcpb.AuthDBChange{
				ChangeType:        "CONF_OAUTH_CLIENT_CHANGED",
				Comment:           "comment",
				OauthClientId:     "123.test.example.com",
				OauthClientSecret: "aBcD",
				Target:            "AuthGlobalConfig$test",
				When:              timestamppb.New(time.Date(2019, time.December, 11, 1, 0, 0, 0, time.UTC)),
				Who:               "user:test@example.com",
				AppVersion:        "123-45abc",
			})
		})
		Convey("Filter by target", func() {
			res, err := srv.ListChangeLogs(ctx, &rpcpb.ListChangeLogsRequest{
				Target:   "AuthRealmsGlobals$globals",
				PageSize: 3,
			})
			So(err, ShouldBeNil)
			So(len(res.Changes), ShouldEqual, 1)
			So(res.Changes[0], ShouldResembleProto, &rpcpb.AuthDBChange{
				ChangeType:       "REALMS_GLOBALS_CHANGED",
				Comment:          "comment",
				PermissionsAdded: []string{"a.existInRealm"},
				Target:           "AuthRealmsGlobals$globals",
				When:             timestamppb.New(time.Date(2021, time.January, 11, 1, 0, 0, 0, time.UTC)),
				Who:              "user:test@example.com",
				AppVersion:       "123-45abc",
			})
		})
		Convey("Filter by AuthDBRev", func() {
			res, err := srv.ListChangeLogs(ctx, &rpcpb.ListChangeLogsRequest{
				AuthDbRev: 10020,
				PageSize:  3,
			})
			So(err, ShouldBeNil)
			So(len(res.Changes), ShouldEqual, 1)
			So(res.Changes[0], ShouldResembleProto, &rpcpb.AuthDBChange{
				ChangeType:       "REALMS_GLOBALS_CHANGED",
				Comment:          "comment",
				PermissionsAdded: []string{"a.existInRealm"},
				Target:           "AuthRealmsGlobals$globals",
				When:             timestamppb.New(time.Date(2021, time.January, 11, 1, 0, 0, 0, time.UTC)),
				Who:              "user:test@example.com",
				AppVersion:       "123-45abc",
			})
		})
	})
}
