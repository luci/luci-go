// Copyright 2015 The LUCI Authors.
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

package auth

import (
	"context"
	"net"
	"testing"

	"go.chromium.org/luci/auth/identity"

	"go.chromium.org/luci/server/auth/service/protocol"
	"go.chromium.org/luci/server/auth/signing"
	"go.chromium.org/luci/server/auth/signing/signingtest"

	. "github.com/smartystreets/goconvey/convey"
)

func TestState(t *testing.T) {
	t.Parallel()

	Convey("Check empty ctx", t, func() {
		ctx := context.Background()
		So(GetState(ctx), ShouldBeNil)
		So(CurrentUser(ctx).Identity, ShouldEqual, identity.AnonymousIdentity)
		So(CurrentIdentity(ctx), ShouldEqual, identity.AnonymousIdentity)

		res, err := IsMember(ctx, "group")
		So(res, ShouldBeFalse)
		So(err, ShouldEqual, ErrNotConfigured)

		res, err = IsAllowedIP(ctx, "bots")
		So(res, ShouldBeFalse)
		So(err, ShouldEqual, ErrNotConfigured)
	})

	Convey("Check non-empty ctx", t, func() {
		s := state{
			db: &fakeDB{
				groups: map[string][]identity.Identity{
					"group": {"user:abc@example.com"},
				},
			},
			user:      &User{Identity: "user:abc@example.com"},
			peerIdent: "user:abc@example.com",
			peerIP:    net.IP{1, 2, 3, 4},
		}
		ctx := context.WithValue(context.Background(), stateContextKey(0), &s)
		So(GetState(ctx), ShouldNotBeNil)
		So(GetState(ctx).Method(), ShouldBeNil)
		So(GetState(ctx).PeerIdentity(), ShouldEqual, identity.Identity("user:abc@example.com"))
		So(GetState(ctx).PeerIP().String(), ShouldEqual, "1.2.3.4")
		So(CurrentUser(ctx).Identity, ShouldEqual, identity.Identity("user:abc@example.com"))
		So(CurrentIdentity(ctx), ShouldEqual, identity.Identity("user:abc@example.com"))

		res, err := IsMember(ctx, "group")
		So(err, ShouldBeNil)
		So(res, ShouldBeTrue)

		res, err = IsAllowedIP(ctx, "bots")
		So(err, ShouldBeNil)
		So(res, ShouldBeTrue) // fakeDB contains the list "bots" with member "1.2.3.4"
	})

	Convey("Check background ctx", t, func() {
		ctx := injectTestDB(context.Background(), &fakeDB{
			authServiceURL: "https://example.com/auth_service",
		})
		url, err := GetState(ctx).DB().GetAuthServiceURL(ctx)
		So(err, ShouldBeNil)
		So(url, ShouldEqual, "https://example.com/auth_service")
	})

	Convey("ShouldEnforceRealmACL", t, func() {
		ctx := ModifyConfig(context.Background(), func(cfg Config) Config {
			cfg.Signer = signingtest.NewSigner(&signing.ServiceInfo{
				AppID: "my-app-id",
			})
			return cfg
		})

		ctx = WithState(ctx, &state{
			db: &fakeDB{
				realmData: map[string]*protocol.RealmData{
					"proj:empty": {},
					"proj:yes":   {EnforceInService: []string{"zzz", "my-app-id"}},
					"proj:no":    {EnforceInService: []string{"zzz", "xxx"}},
				},
			},
		})

		// No data.
		yes, err := ShouldEnforceRealmACL(ctx, "proj:unknown")
		So(err, ShouldBeNil)
		So(yes, ShouldBeFalse)

		// Empty data.
		yes, err = ShouldEnforceRealmACL(ctx, "proj:empty")
		So(err, ShouldBeNil)
		So(yes, ShouldBeFalse)

		// In the set.
		yes, err = ShouldEnforceRealmACL(ctx, "proj:yes")
		So(err, ShouldBeNil)
		So(yes, ShouldBeTrue)

		// Not in the set.
		yes, err = ShouldEnforceRealmACL(ctx, "proj:no")
		So(err, ShouldBeNil)
		So(yes, ShouldBeFalse)
	})
}
