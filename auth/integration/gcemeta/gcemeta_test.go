// Copyright 2019 The LUCI Authors.
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

package gcemeta

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"cloud.google.com/go/compute/metadata"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

type fakeGenerator struct {
	accessToken *oauth2.Token
	idToken     string
	lastScopes  []string
}

func (f *fakeGenerator) GenerateOAuthToken(ctx context.Context, scopes []string, lifetime time.Duration) (*oauth2.Token, error) {
	f.lastScopes = append([]string(nil), scopes...)
	return f.accessToken, nil
}

func (f *fakeGenerator) GenerateIDToken(ctx context.Context, audience string, lifetime time.Duration) (*oauth2.Token, error) {
	return &oauth2.Token{AccessToken: f.idToken}, nil
}

func TestServer(t *testing.T) {
	fakeAccessToken := &oauth2.Token{
		AccessToken: "fake_access_token",
		Expiry:      time.Now().Add(time.Hour),
	}
	fakeIDToken := "fake_id_token"

	ctx := context.Background()
	gen := &fakeGenerator{fakeAccessToken, fakeIDToken, nil}
	srv := Server{
		Generator:        gen,
		Email:            "fake@example.com",
		Scopes:           []string{"scope1", "scope2"},
		MinTokenLifetime: 2 * time.Minute,
	}

	// Need to set GCE_METADATA_HOST once before all tests because 'metadata'
	// package caches results of metadata fetches.
	addr, err := srv.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start: %s", err)
	}
	defer srv.Stop(ctx)

	os.Setenv("GCE_METADATA_HOST", addr)

	ftt.Run("Works", t, func(c *ftt.Test) {
		assert.Loosely(c, metadata.OnGCE(), should.BeTrue)

		c.Run("Metadata client works", func(c *ftt.Test) {
			cl := metadata.NewClient(http.DefaultClient)

			num, err := cl.NumericProjectIDWithContext(ctx)
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, num, should.Equal("0"))

			pid, err := cl.ProjectIDWithContext(ctx)
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, pid, should.Equal("none"))

			accounts, err := cl.GetWithContext(ctx, "instance/service-accounts/")
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, accounts, should.Equal("default/\nfake@example.com/\n"))

			for _, acc := range []string{"fake@example.com", "default"} {
				info, err := cl.GetWithContext(ctx, "instance/service-accounts/"+acc+"/?recursive=true")
				assert.Loosely(c, err, should.BeNil)
				assert.Loosely(c, info, should.Equal(
					`{"aliases":["default"],"email":"fake@example.com","scopes":["scope1","scope2"]}`))

				email, err := cl.GetWithContext(ctx, "instance/service-accounts/"+acc+"/email")
				assert.Loosely(c, err, should.BeNil)
				assert.Loosely(c, email, should.Equal("fake@example.com"))

				scopes, err := cl.ScopesWithContext(ctx, acc)
				assert.Loosely(c, err, should.BeNil)
				assert.Loosely(c, scopes, should.Resemble([]string{"scope1", "scope2"}))
			}
		})

		c.Run("OAuth2 token source works", func(c *ftt.Test) {
			c.Run("Default scopes", func(c *ftt.Test) {
				ts := google.ComputeTokenSource("default")
				tok, err := ts.Token()
				assert.Loosely(c, err, should.BeNil)
				// Do not put tokens into logs, in case we somehow accidentally hit real
				// metadata server with real tokens.
				if tok.AccessToken != fakeAccessToken.AccessToken {
					panic("Bad token")
				}
				assert.Loosely(c, time.Until(tok.Expiry), should.BeGreaterThan(55*time.Minute))
				assert.Loosely(c, gen.lastScopes, should.Resemble([]string{"scope1", "scope2"}))
			})

			c.Run("Custom scopes", func(c *ftt.Test) {
				ts := google.ComputeTokenSource("default", "custom1", "custom1", "custom2")
				tok, err := ts.Token()
				assert.Loosely(c, err, should.BeNil)
				// Do not put tokens into logs, in case we somehow accidentally hit real
				// metadata server with real tokens.
				if tok.AccessToken != fakeAccessToken.AccessToken {
					panic("Bad token")
				}
				assert.Loosely(c, time.Until(tok.Expiry), should.BeGreaterThan(55*time.Minute))
				assert.Loosely(c, gen.lastScopes, should.Resemble([]string{"custom1", "custom2"}))
			})
		})

		c.Run("ID token fetch works", func(c *ftt.Test) {
			reply, err := metadata.GetWithContext(ctx, "instance/service-accounts/default/identity?audience=boo&format=ignored")
			assert.Loosely(c, err, should.BeNil)
			// Do not put tokens into logs, in case we somehow accidentally hit real
			// metadata server with real tokens.
			if reply != fakeIDToken {
				panic("Bad token")
			}
		})

		c.Run("Unsupported metadata call", func(c *ftt.Test) {
			_, err := metadata.ExternalIPWithContext(ctx)
			assert.Loosely(c, err.Error(), should.Equal(`metadata: GCE metadata "instance/network-interfaces/0/access-configs/0/external-ip" not defined`))
		})
	})
}
