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
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	_ "go.chromium.org/luci/gae/service/datastore/crbug1242998safeget"
)

func testAuthVersionedEntityMixin() AuthVersionedEntityMixin {
	return AuthVersionedEntityMixin{
		ModifiedTS:    time.Date(2021, time.August, 16, 12, 20, 0, 0, time.UTC),
		ModifiedBy:    "user:test-user@example.com",
		AuthDBRev:     1337,
		AuthDBPrevRev: 1336,
	}
}

func testAuthGlobalConfig(ctx context.Context) *AuthGlobalConfig {
	return &AuthGlobalConfig{
		Kind:                     "AuthGlobalConfig",
		ID:                       "root",
		AuthVersionedEntityMixin: testAuthVersionedEntityMixin(),
		OAuthClientID:            "test-client-id",
		OAuthAdditionalClientIDs: []string{
			"additional-client-id-0",
			"additional-client-id-1",
		},
		OAuthClientSecret: "test-client-secret",
		TokenServerURL:    "https://token-server.example.com",
		SecurityConfig:    []byte{0, 1, 2, 3}, // TODO: use something real here
	}
}

func testAuthReplicationState(ctx context.Context, rev int64) *AuthReplicationState {
	return &AuthReplicationState{
		Kind:       "AuthReplicationState",
		ID:         "self",
		Parent:     RootKey(ctx),
		AuthDBRev:  rev,
		ModifiedTS: time.Date(2021, time.August, 16, 12, 20, 0, 0, time.UTC),
		PrimaryID:  "test-primary-id",
	}
}

func testAuthGroup(ctx context.Context, name string, members []string) *AuthGroup {
	return &AuthGroup{
		Kind:                     "AuthGroup",
		ID:                       name,
		Parent:                   RootKey(ctx),
		AuthVersionedEntityMixin: testAuthVersionedEntityMixin(),
		Members:                  members,
		Globs:                    []string{"user:*@example.com"},
		Nested:                   []string{"nested-" + name},
		Description:              fmt.Sprintf("This is a test auth group %q.", name),
		Owners:                   "owners-" + name,
		CreatedTS:                time.Date(2021, time.August, 16, 15, 20, 0, 0, time.UTC),
		CreatedBy:                "user:test-user@example.com",
	}
}

func testIPAllowlist(ctx context.Context, name string, subnets []string) *AuthIPAllowlist {
	return &AuthIPAllowlist{
		Kind:                     "AuthIPWhitelist",
		ID:                       name,
		Parent:                   RootKey(ctx),
		AuthVersionedEntityMixin: testAuthVersionedEntityMixin(),
		Subnets:                  subnets,
		Description:              fmt.Sprintf("This is a test AuthIPAllowlist %q", name),
		CreatedTS:                time.Date(2021, time.August, 16, 15, 20, 0, 0, time.UTC),
		CreatedBy:                "user:test-user@example.com",
	}
}

////////////////////////////////////////////////////////////////////////////////

func TestGetAuthGroup(t *testing.T) {
	t.Parallel()

	Convey("Testing GetAuthGroup", t, func() {
		ctx := memory.Use(context.Background())

		authGroup := testAuthGroup(ctx, "test-auth-group-1", []string{
			"user:a@example.com",
			"user:b@example.com",
		})

		_, err := GetAuthGroup(ctx, "test-auth-group-1")
		So(err, ShouldEqual, datastore.ErrNoSuchEntity)

		So(datastore.Put(ctx, authGroup), ShouldBeNil)

		actual, err := GetAuthGroup(ctx, "test-auth-group-1")
		So(err, ShouldBeNil)
		So(actual, ShouldResemble, authGroup)
	})
}

func TestGetAllAuthGroups(t *testing.T) {
	t.Parallel()

	Convey("Testing GetAllAuthGroups", t, func() {
		ctx := memory.Use(context.Background())

		// Out of order alphabetically by ID.
		So(datastore.Put(ctx,
			testAuthGroup(ctx, "test-auth-group-3", nil),
			testAuthGroup(ctx, "test-auth-group-1", nil),
			testAuthGroup(ctx, "test-auth-group-2", nil),
		), ShouldBeNil)

		actualAuthGroups, err := GetAllAuthGroups(ctx)
		So(err, ShouldBeNil)

		// Returned in alphabetical order.
		So(actualAuthGroups, ShouldResemble, []*AuthGroup{
			testAuthGroup(ctx, "test-auth-group-1", nil),
			testAuthGroup(ctx, "test-auth-group-2", nil),
			testAuthGroup(ctx, "test-auth-group-3", nil),
		})
	})
}

func TestGetAuthIPAllowlist(t *testing.T) {
	t.Parallel()

	Convey("Testing GetAuthIPAllowlist", t, func() {
		ctx := memory.Use(context.Background())

		authIPAllowlist := testIPAllowlist(ctx, "test-auth-ip-allowlist-1", []string{
			"123.456.789.101/24",
			"123.456.789.112/24",
		})

		_, err := GetAuthIPAllowlist(ctx, "test-auth-ip-allowlist-1")
		So(err, ShouldEqual, datastore.ErrNoSuchEntity)

		So(datastore.Put(ctx, authIPAllowlist), ShouldBeNil)

		actual, err := GetAuthIPAllowlist(ctx, "test-auth-ip-allowlist-1")
		So(err, ShouldBeNil)
		So(actual, ShouldResemble, authIPAllowlist)
	})
}

func TestGetAllAuthIPAllowlists(t *testing.T) {
	t.Parallel()

	Convey("Testing GetAllAuthIPAllowlists", t, func() {
		ctx := memory.Use(context.Background())

		// Out of order alphabetically by ID.
		So(datastore.Put(ctx,
			testIPAllowlist(ctx, "test-allowlist-3", nil),
			testIPAllowlist(ctx, "test-allowlist-1", nil),
			testIPAllowlist(ctx, "test-allowlist-2", nil),
		), ShouldBeNil)

		actualAuthIPAllowlists, err := GetAllAuthIPAllowlists(ctx)
		So(err, ShouldBeNil)

		// Returned in alphabetical order.
		So(actualAuthIPAllowlists, ShouldResemble, []*AuthIPAllowlist{
			testIPAllowlist(ctx, "test-allowlist-1", nil),
			testIPAllowlist(ctx, "test-allowlist-2", nil),
			testIPAllowlist(ctx, "test-allowlist-3", nil),
		})
	})
}

func TestGetAuthGlobalConfig(t *testing.T) {
	t.Parallel()

	Convey("Testing GetAuthGlobalConfig", t, func() {
		ctx := memory.Use(context.Background())

		_, err := GetAuthGlobalConfig(ctx)
		So(err, ShouldEqual, datastore.ErrNoSuchEntity)

		cfg := testAuthGlobalConfig(ctx)
		err = datastore.Put(ctx, cfg)
		So(err, ShouldBeNil)

		actual, err := GetAuthGlobalConfig(ctx)
		So(err, ShouldBeNil)
		So(actual, ShouldResemble, cfg)
	})
}
