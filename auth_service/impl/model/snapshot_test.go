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

package model

import (
	"context"
	"fmt"
	"testing"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth/service/protocol"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestTakeSnapshot(t *testing.T) {
	t.Parallel()

	const testAuthDBRev = 12345

	Convey("Testing TakeSnapshot", t, func() {
		ctx := memory.Use(context.Background())

		_, err := TakeSnapshot(ctx)
		So(err.(errors.MultiError).First(), ShouldEqual, datastore.ErrNoSuchEntity)

		So(datastore.Put(ctx,
			testAuthReplicationState(ctx, testAuthDBRev),
			testAuthGlobalConfig(ctx),
			testAuthGroup(ctx, "group-2", nil),
			testAuthGroup(ctx, "group-1", nil),
			testIPAllowlist(ctx, "ip-allowlist-2", nil),
			testIPAllowlist(ctx, "ip-allowlist-1", nil),
		), ShouldBeNil)

		snap, err := TakeSnapshot(ctx)
		So(err, ShouldBeNil)

		So(snap, ShouldResemble, &Snapshot{
			ReplicationState: testAuthReplicationState(ctx, 12345),
			GlobalConfig:     testAuthGlobalConfig(ctx),
			Groups: []*AuthGroup{
				testAuthGroup(ctx, "group-1", nil),
				testAuthGroup(ctx, "group-2", nil),
			},
			IPAllowlists: []*AuthIPAllowlist{
				testIPAllowlist(ctx, "ip-allowlist-1", nil),
				testIPAllowlist(ctx, "ip-allowlist-2", nil),
			},
		})

		Convey("ToAuthDBProto", func() {
			groupProto := func(name string) *protocol.AuthGroup {
				return &protocol.AuthGroup{
					Name: name,
					Members: []string{
						fmt.Sprintf("user:%s-m1@example.com", name),
						fmt.Sprintf("user:%s-m2@example.com", name),
					},
					Globs:       []string{"user:*@example.com"},
					Nested:      []string{"nested-" + name},
					Description: fmt.Sprintf("This is a test auth group %q.", name),
					CreatedTs:   testCreatedTS.UnixNano() / 1000,
					CreatedBy:   "user:test-creator@example.com",
					ModifiedTs:  testModifiedTS.UnixNano() / 1000,
					ModifiedBy:  "user:test-modifier@example.com",
					Owners:      "owners-" + name,
				}
			}

			allowlistProto := func(name string) *protocol.AuthIPWhitelist {
				return &protocol.AuthIPWhitelist{
					Name: name,
					Subnets: []string{
						"127.0.0.1/10",
						"127.0.0.1/20",
					},
					Description: fmt.Sprintf("This is a test AuthIPAllowlist %q.", name),
					CreatedTs:   testCreatedTS.UnixNano() / 1000,
					CreatedBy:   "user:test-creator@example.com",
					ModifiedTs:  testModifiedTS.UnixNano() / 1000,
					ModifiedBy:  "user:test-modifier@example.com",
				}
			}

			So(snap.ToAuthDBProto(), ShouldResembleProto, &protocol.AuthDB{
				OauthClientId:     "test-client-id",
				OauthClientSecret: "test-client-secret",
				OauthAdditionalClientIds: []string{
					"additional-client-id-0",
					"additional-client-id-1",
				},
				TokenServerUrl: "https://token-server.example.com",
				SecurityConfig: testSecurityConfigBlob(),
				Groups: []*protocol.AuthGroup{
					groupProto("group-1"),
					groupProto("group-2"),
				},
				IpWhitelists: []*protocol.AuthIPWhitelist{
					allowlistProto("ip-allowlist-1"),
					allowlistProto("ip-allowlist-2"),
				},
			})
		})

		Convey("ToAuthDB", func() {
			db, err := snap.ToAuthDB()
			So(err, ShouldBeNil)
			So(db.Rev, ShouldEqual, testAuthDBRev)
		})
	})
}
