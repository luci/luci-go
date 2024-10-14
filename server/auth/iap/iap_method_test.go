// Copyright 2020 The LUCI Authors.
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

package iap

import (
	"context"
	"testing"

	"google.golang.org/api/idtoken"

	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/server/auth/authtest"
)

func TestIAPAuthenticator(t *testing.T) {
	t.Parallel()
	ftt.Run("iap", t, func(t *ftt.Test) {
		c := context.Background()
		c = gologger.StdConfig.Use(c)

		t.Run("missing iap jwt assertion header", func(t *ftt.Test) {
			a := &IAPAuthMethod{}
			r := authtest.NewFakeRequestMetadata()
			user, session, err := a.Authenticate(c, r)
			assert.Loosely(t, user, should.BeNil)
			assert.Loosely(t, session, should.BeNil)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("invalid jwt assertion header bytes", func(t *ftt.Test) {
			a := &IAPAuthMethod{}
			r := authtest.NewFakeRequestMetadata()
			r.FakeHeader.Set(iapJWTAssertionHeader, "some invalid header bytes")

			user, session, err := a.Authenticate(c, r)
			assert.Loosely(t, user, should.BeNil)
			assert.Loosely(t, session, should.BeNil)
			assert.Loosely(t, err, should.NotBeNil)
		})

		t.Run("invalid no email claims", func(t *ftt.Test) {
			a := &IAPAuthMethod{
				Aud: AudForGAE("1234", "some-app-id"),
				validator: func(ctx context.Context, idToken string, audience string) (*idtoken.Payload, error) {
					return &idtoken.Payload{
						Issuer:   "",
						IssuedAt: 0,
						Subject:  "",
					}, nil
				},
			}
			r := authtest.NewFakeRequestMetadata()
			r.FakeHeader.Set(iapJWTAssertionHeader, "just needs to be non-empty for testing")

			user, session, err := a.Authenticate(c, r)
			assert.Loosely(t, user, should.BeNil)
			assert.Loosely(t, session, should.BeNil)
			assert.Loosely(t, err, should.NotBeNil)
		})

		t.Run("happy path", func(t *ftt.Test) {
			a := &IAPAuthMethod{
				Aud: AudForGAE("1234", "some-app-id"),
				validator: func(ctx context.Context, idToken string, audience string) (*idtoken.Payload, error) {
					return &idtoken.Payload{
						Issuer:   "",
						IssuedAt: 0,
						Subject:  "",
						Claims: map[string]any{
							"email": "someemail@somedomain.com",
						},
					}, nil
				},
			}

			r := authtest.NewFakeRequestMetadata()
			r.FakeHeader.Set(iapJWTAssertionHeader, "just needs to be non-empty for testing")

			user, session, err := a.Authenticate(c, r)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, user, should.NotBeNil)
			assert.Loosely(t, user.Email, should.Equal("someemail@somedomain.com"))
			assert.Loosely(t, session, should.BeNil)
		})
	})
}
