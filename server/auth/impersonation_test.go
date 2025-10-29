// Copyright 2025 The LUCI Authors.
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
	"testing"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/server/auth/authdb"
)

func TestWithUncheckedImpersonation(t *testing.T) {
	t.Parallel()

	db := &fakeDB{
		allowedClientID: "some_client_id",
	}
	ctx := injectTestDB(context.Background(), db)

	t.Run("OK", func(t *testing.T) {
		// The original "real" context.
		auth := Authenticator{
			Methods: []Method{fakeAuthMethod{clientID: "some_client_id"}},
		}
		req := makeRequest()
		req.FakeRemoteAddr = "1.2.3.4"
		ctx, err := auth.Authenticate(ctx, req)
		assert.NoErr(t, err)
		assert.That(t, CurrentUser(ctx), should.Match(&User{
			Identity: "user:abc@example.com",
			Email:    "abc@example.com",
			ClientID: "some_client_id",
		}))

		// Impersonated context.
		ctx, err = WithUncheckedImpersonation(ctx, "user:someone@example.com")
		assert.NoErr(t, err)
		assert.That(t, CurrentUser(ctx), should.Match(&User{
			Identity: "user:someone@example.com",
			Email:    "someone@example.com",
		}))
		assert.That(t, GetState(ctx).PeerIdentity(), should.Equal(identity.Identity("user:abc@example.com")))
		assert.That(t, GetState(ctx).PeerIP().String(), should.Equal("1.2.3.4"))
		assert.That(t, GetState(ctx).DB(), should.Equal(authdb.DB(db)))
		assert.That(t, GetState(ctx).Method(), should.Equal(uncheckedImpersonationMethod))

		// Chaining impersonation is forbidden.
		_, err = WithUncheckedImpersonation(ctx, "user:someone@example.com")
		assert.That(t, err, should.ErrLike("impersonation is already in effect, can't chain it"))
	})

	t.Run("Requires an authenticated user", func(t *testing.T) {
		_, err := WithUncheckedImpersonation(ctx, "user:someone@example.com")
		assert.That(t, err, should.ErrLike("an anonymous can't use impersonation"))
	})
}
