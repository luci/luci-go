// Copyright 2016 The LUCI Authors.
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

package common

import (
	"testing"
	"time"

	"go.chromium.org/gae/impl/memory"
	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/buildbucket/access"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/logging/gologger"
	accessProto "go.chromium.org/luci/common/proto/access"
	"go.chromium.org/luci/config"
	memcfg "go.chromium.org/luci/config/impl/memory"
	"go.chromium.org/luci/config/server/cfgclient/backend/testconfig"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/caching"
	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

func withTestAccessClient(c context.Context, client *access.TestClient) context.Context {
	testFactory := func(context.Context, string) (accessProto.AccessClient, error) {
		return client, nil
	}
	return withAccessClientFactory(c, testFactory)
}

var (
	anon = &authtest.FakeState{
		Identity:       identity.AnonymousIdentity,
		IdentityGroups: []string{"all"},
	}
	admin = &authtest.FakeState{
		Identity:       "user:alicebob@google.com",
		IdentityGroups: []string{"administrators", "googlers", "all"},
	}
	user = &authtest.FakeState{
		Identity:       "user:alicebob@google.com",
		IdentityGroups: []string{"googlers", "all"},
	}
	external = &authtest.FakeState{
		Identity:       "user:eve@notgoogle.com",
		IdentityGroups: []string{"all"},
	}
)

func TestACL(t *testing.T) {
	t.Parallel()

	Convey("Test Environment", t, func() {
		c := memory.UseWithAppID(context.Background(), "dev~luci-milo")
		c = gologger.StdConfig.Use(c)
		c = caching.WithEmptyProcessCache(c)
		c = testconfig.WithCommonClient(c, memcfg.New(aclConfgs))
		_, err := UpdateServiceConfig(c)
		So(err, ShouldBeNil)

		Convey("Set up projects", func() {
			err := UpdateConsoles(c)
			So(err, ShouldBeNil)

			Convey("Anon wants to...", func() {
				c = auth.WithState(c, anon)
				Convey("Read public project", func() {
					ok, err := IsAllowed(c, "opensource")
					So(ok, ShouldEqual, true)
					So(err, ShouldBeNil)
				})
				Convey("Read private project", func() {
					ok, err := IsAllowed(c, "secret")
					So(ok, ShouldEqual, false)
					So(err, ShouldBeNil)
				})

			})

			Convey("admin@google.com wants to...", func() {
				c = auth.WithState(c, admin)
				Convey("Read private project", func() {
					ok, err := IsAllowed(c, "secret")
					So(ok, ShouldEqual, true)
					So(err, ShouldBeNil)
				})
				Convey("Read un/misconfigured project", func() {
					ok, err := IsAllowed(c, "misconfigured")
					So(ok, ShouldEqual, true)
					So(err, ShouldBeNil)
				})
			})

			Convey("alicebob@google.com wants to...", func() {
				c = auth.WithState(c, user)
				Convey("Read private project", func() {
					ok, err := IsAllowed(c, "secret")
					So(ok, ShouldEqual, true)
					So(err, ShouldBeNil)
				})
				Convey("Read un/misconfigured project", func() {
					ok, err := IsAllowed(c, "misconfigured")
					So(ok, ShouldEqual, false)
					So(err, ShouldNotBeNil)
					So(ErrorTag.In(err), ShouldEqual, CodeNotFound)
				})
			})

			Convey("eve@notgoogle.com wants to...", func() {
				c = auth.WithState(c, external)
				Convey("Read public project", func() {
					ok, err := IsAllowed(c, "opensource")
					So(ok, ShouldEqual, true)
					So(err, ShouldBeNil)
				})
				Convey("Read private project", func() {
					ok, err := IsAllowed(c, "secret")
					So(ok, ShouldEqual, false)
					So(err, ShouldBeNil)
				})
			})
		})

		Convey("Set up buildbucket client", func() {
			client := access.TestClient{}
			helloPermissions := access.Permissions{
				"hello": access.AccessBucket,
			}

			Convey("With anonymous identity", func() {
				c = auth.WithState(c, anon)

				Convey("Get bucket permissions uncached", func() {
					c = withTestAccessClient(c, &client)

					// Uncached call.
					client.PermittedActionsResponse = helloPermissions.ToProto(0)
					perms, err := BucketPermissions(c, "hello")
					So(err, ShouldBeNil)
					So(perms, ShouldResemble, helloPermissions)

					// Cached call. Since perms is nil and we haven't advanced the clock,
					// this would error if the value isn't cached.
					client.PermittedActionsResponse = (access.Permissions{}).ToProto(0)
					perms, err = BucketPermissions(c, "hello")
					So(err, ShouldBeNil)
					So(perms["hello"], ShouldEqual, access.AccessBucket)
				})

				Convey("Get bucket permissions cache duration", func() {
					c = withTestAccessClient(c, &client)
					c, clk := testclock.UseTime(c, testclock.TestRecentTimeLocal)

					// Uncached call.
					client.PermittedActionsResponse = helloPermissions.ToProto(1 * time.Second)
					perms, err := BucketPermissions(c, "hello")
					So(err, ShouldBeNil)
					So(perms, ShouldResemble, helloPermissions)

					// Move time forward.
					clk.Add(2 * time.Second)

					// Also uncached call.
					expectPermissions := access.Permissions{
						"hello": access.ViewBuild,
					}
					client.PermittedActionsResponse = expectPermissions.ToProto(0)
					perms, err = BucketPermissions(c, "hello")
					So(err, ShouldBeNil)
					So(perms, ShouldResemble, expectPermissions)
				})

				Convey("Get bucket permissions for cached and uncached buckets", func() {
					c = withTestAccessClient(c, &client)

					// Uncached call.
					client.PermittedActionsResponse = helloPermissions.ToProto(0)
					perms, err := BucketPermissions(c, "hello")
					So(err, ShouldBeNil)
					So(perms, ShouldResemble, helloPermissions)

					// Cached call for hello, but uncached call for goodbye.
					// Since perms is nil and we haven't advanced the clock,
					// this should fail if there's no caching happening.
					goodbyePermissions := access.Permissions{
						"goodbye": access.AccessBucket,
					}
					client.PermittedActionsResponse = goodbyePermissions.ToProto(0)
					perms, err = BucketPermissions(c, "hello", "goodbye")
					So(err, ShouldBeNil)
					So(perms["hello"], ShouldEqual, access.AccessBucket)
					So(perms["goodbye"], ShouldEqual, access.AccessBucket)
				})
			})

			Convey("Make sure cache doesn't share identity", func() {
				c = withTestAccessClient(c, &client)

				cUser := auth.WithState(c, user)
				// Uncached call from authenticated user.
				client.PermittedActionsResponse = helloPermissions.ToProto(0)
				perms, err := BucketPermissions(cUser, "hello")
				So(err, ShouldBeNil)
				So(perms, ShouldResemble, helloPermissions)

				// Alicebob's result should now be cached, but anon should make an RPC
				// for themselves, and Alicebob's result should not be used.
				cExt := auth.WithState(c, external)
				client.PermittedActionsResponse = (access.Permissions{}).ToProto(0)
				perms, err = BucketPermissions(cExt, "hello")
				So(err, ShouldBeNil)
				So(len(perms), ShouldEqual, 0)
			})

			Convey("Make sure context doesn't share identity", func() {
				// First call from authenticated user.
				c = auth.WithState(c, user)
				c = withTestAccessClient(c, &client)
				client.PermittedActionsResponse = helloPermissions.ToProto(0)
				perms, err := BucketPermissions(c, "hello")
				So(err, ShouldBeNil)
				So(perms, ShouldResemble, helloPermissions)

				// Alicebob's result should now be cached, but anon should make an RPC
				// for themselves, and Alicebob's result should not be used.
				cExt := auth.WithState(c, external)
				client.PermittedActionsResponse = (access.Permissions{}).ToProto(0)
				perms, err = BucketPermissions(cExt, "hello")
				So(err, ShouldBeNil)
				So(len(perms), ShouldEqual, 0)
			})
		})
	})
}

var secretProjectCfg = `
name: "secret"
access: "group:googlers"
`

var publicProjectCfg = `
name: "opensource"
access: "group:all"
`

var miloCfg = `
buildbucket {
  host: "cr-buildbucket-dev.appspot.com"
}
`

var aclConfgs = map[config.Set]memcfg.Files{
	"projects/secret": {
		"project.cfg": secretProjectCfg,
	},
	"projects/opensource": {
		"project.cfg": publicProjectCfg,
	},
	"project/misconfigured": {
		"probject.cfg": secretProjectCfg,
	},
	"services/luci-milo": {
		"settings.cfg": miloCfg,
	},
}
