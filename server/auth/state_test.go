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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/server/auth/service/protocol"
	"go.chromium.org/luci/server/auth/signing"
	"go.chromium.org/luci/server/auth/signing/signingtest"
)

func TestState(t *testing.T) {
	t.Parallel()

	ftt.Run("Check empty ctx", t, func(t *ftt.Test) {
		ctx := context.Background()
		assert.Loosely(t, GetState(ctx), should.BeNil)
		assert.Loosely(t, CurrentUser(ctx).Identity, should.Equal(identity.AnonymousIdentity))
		assert.Loosely(t, CurrentIdentity(ctx), should.Equal(identity.AnonymousIdentity))

		res, err := IsMember(ctx, "group")
		assert.Loosely(t, res, should.BeFalse)
		assert.Loosely(t, err, should.Equal(ErrNotConfigured))

		res, err = IsAllowedIP(ctx, "bots")
		assert.Loosely(t, res, should.BeFalse)
		assert.Loosely(t, err, should.Equal(ErrNotConfigured))
	})

	ftt.Run("Check non-empty ctx", t, func(t *ftt.Test) {
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
		assert.Loosely(t, GetState(ctx), should.NotBeNil)
		assert.Loosely(t, GetState(ctx).Method(), should.BeNil)
		assert.Loosely(t, GetState(ctx).PeerIdentity(), should.Equal(identity.Identity("user:abc@example.com")))
		assert.Loosely(t, GetState(ctx).PeerIP().String(), should.Equal("1.2.3.4"))
		assert.Loosely(t, CurrentUser(ctx).Identity, should.Equal(identity.Identity("user:abc@example.com")))
		assert.Loosely(t, CurrentIdentity(ctx), should.Equal(identity.Identity("user:abc@example.com")))

		res, err := IsMember(ctx, "group")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, res, should.BeTrue)

		res, err = IsAllowedIP(ctx, "bots")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, res, should.BeTrue) // fakeDB contains the list "bots" with member "1.2.3.4"
	})

	ftt.Run("Check background ctx", t, func(t *ftt.Test) {
		ctx := injectTestDB(context.Background(), &fakeDB{
			authServiceURL: "https://example.com/auth_service",
		})
		url, err := GetState(ctx).DB().GetAuthServiceURL(ctx)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, url, should.Equal("https://example.com/auth_service"))
	})

	ftt.Run("ShouldEnforceRealmACL", t, func(t *ftt.Test) {
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
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, yes, should.BeFalse)

		// Empty data.
		yes, err = ShouldEnforceRealmACL(ctx, "proj:empty")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, yes, should.BeFalse)

		// In the set.
		yes, err = ShouldEnforceRealmACL(ctx, "proj:yes")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, yes, should.BeTrue)

		// Not in the set.
		yes, err = ShouldEnforceRealmACL(ctx, "proj:no")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, yes, should.BeFalse)
	})
}
