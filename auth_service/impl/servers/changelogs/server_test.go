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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/auth_service/api/rpcpb"
	"go.chromium.org/luci/auth_service/impl/model"
)

func TestChangeLogsServer(t *testing.T) {
	t.Parallel()
	srv := Server{}

	ftt.Run("ListChangeLogs RPC call", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		assert.Loosely(t, datastore.Put(ctx,
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
		), should.BeNil)

		t.Run("List all", func(t *ftt.Test) {
			res, err := srv.ListChangeLogs(ctx, &rpcpb.ListChangeLogsRequest{
				PageSize: 2,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.NextPageToken, should.NotBeEmpty)
			assert.Loosely(t, len(res.Changes), should.Equal(2))
			assert.Loosely(t, res.Changes[0], should.Resemble(&rpcpb.AuthDBChange{
				ChangeType:       "REALMS_GLOBALS_CHANGED",
				Comment:          "comment",
				PermissionsAdded: []string{"a.existInRealm"},
				Target:           "AuthRealmsGlobals$globals",
				When:             timestamppb.New(time.Date(2021, time.January, 11, 1, 0, 0, 0, time.UTC)),
				Who:              "user:test@example.com",
				AppVersion:       "123-45abc",
			}))
			assert.Loosely(t, res.Changes[1], should.Resemble(&rpcpb.AuthDBChange{
				ChangeType:     "IPWL_CREATED",
				Comment:        "comment",
				Description:    "description",
				OldDescription: "",
				Target:         "AuthIPWhitelist$a",
				When:           timestamppb.New(time.Date(2021, time.December, 12, 1, 0, 0, 0, time.UTC)),
				Who:            "user:test@example.com",
				AppVersion:     "123-45abc",
			}))

			res, err = srv.ListChangeLogs(ctx, &rpcpb.ListChangeLogsRequest{
				PageToken: res.NextPageToken,
				PageSize:  2,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.NextPageToken, should.BeEmpty)
			assert.Loosely(t, len(res.Changes), should.Equal(1))
			assert.Loosely(t, res.Changes[0], should.Resemble(&rpcpb.AuthDBChange{
				ChangeType:        "CONF_OAUTH_CLIENT_CHANGED",
				Comment:           "comment",
				OauthClientId:     "123.test.example.com",
				OauthClientSecret: "aBcD",
				Target:            "AuthGlobalConfig$test",
				When:              timestamppb.New(time.Date(2019, time.December, 11, 1, 0, 0, 0, time.UTC)),
				Who:               "user:test@example.com",
				AppVersion:        "123-45abc",
			}))
		})
		t.Run("Filter by target", func(t *ftt.Test) {
			res, err := srv.ListChangeLogs(ctx, &rpcpb.ListChangeLogsRequest{
				Target:   "AuthRealmsGlobals$globals",
				PageSize: 3,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(res.Changes), should.Equal(1))
			assert.Loosely(t, res.Changes[0], should.Resemble(&rpcpb.AuthDBChange{
				ChangeType:       "REALMS_GLOBALS_CHANGED",
				Comment:          "comment",
				PermissionsAdded: []string{"a.existInRealm"},
				Target:           "AuthRealmsGlobals$globals",
				When:             timestamppb.New(time.Date(2021, time.January, 11, 1, 0, 0, 0, time.UTC)),
				Who:              "user:test@example.com",
				AppVersion:       "123-45abc",
			}))
		})
		t.Run("Filter by AuthDBRev", func(t *ftt.Test) {
			res, err := srv.ListChangeLogs(ctx, &rpcpb.ListChangeLogsRequest{
				AuthDbRev: 10020,
				PageSize:  3,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(res.Changes), should.Equal(1))
			assert.Loosely(t, res.Changes[0], should.Resemble(&rpcpb.AuthDBChange{
				ChangeType:       "REALMS_GLOBALS_CHANGED",
				Comment:          "comment",
				PermissionsAdded: []string{"a.existInRealm"},
				Target:           "AuthRealmsGlobals$globals",
				When:             timestamppb.New(time.Date(2021, time.January, 11, 1, 0, 0, 0, time.UTC)),
				Who:              "user:test@example.com",
				AppVersion:       "123-45abc",
			}))
		})
	})
}
