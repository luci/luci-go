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

package authtest

import (
	"context"
	"net"
	"testing"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/realms"
)

var testPerm = realms.RegisterPermission("testing.tests.perm")

func TestFakeState(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	ftt.Run("Default FakeState works", t, func(t *ftt.Test) {
		state := FakeState{}
		assert.Loosely(t, state.DB(), should.Resemble(&FakeDB{}))
		assert.Loosely(t, state.Method(), should.NotBeNil)
		assert.Loosely(t, state.User(), should.Resemble(&auth.User{Identity: identity.AnonymousIdentity}))
		assert.Loosely(t, state.PeerIdentity(), should.Equal(identity.AnonymousIdentity))
		assert.Loosely(t, state.PeerIP().String(), should.Equal("127.0.0.1"))
	})

	ftt.Run("Non-default FakeState works", t, func(t *ftt.Test) {
		state := FakeState{
			Identity:       "user:abc@def.com",
			IdentityGroups: []string{"abc"},
			IdentityPermissions: []RealmPermission{
				{"proj:realm1", testPerm},
			},
			PeerIPAllowlist:      []string{"allowlist"},
			PeerIdentityOverride: "bot:blah",
			PeerIPOverride:       net.ParseIP("192.192.192.192"),
			UserExtra:            "blah",
		}

		assert.Loosely(t, state.Method(), should.NotBeNil)
		assert.Loosely(t, state.User(), should.Resemble(&auth.User{
			Identity: "user:abc@def.com",
			Email:    "abc@def.com",
			Extra:    "blah",
		}))
		assert.Loosely(t, state.PeerIdentity(), should.Equal(identity.Identity("bot:blah")))
		assert.Loosely(t, state.PeerIP().String(), should.Equal("192.192.192.192"))

		db := state.DB()

		yes, err := db.IsMember(ctx, "user:abc@def.com", []string{"abc"})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, yes, should.BeTrue)

		yes, err = db.HasPermission(ctx, "user:abc@def.com", testPerm, "proj:realm1", nil)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, yes, should.BeTrue)

		yes, err = db.IsAllowedIP(ctx, net.ParseIP("192.192.192.192"), "allowlist")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, yes, should.BeTrue)
	})
}
