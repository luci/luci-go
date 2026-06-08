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
	"testing"
	"time"

	"cloud.google.com/go/compute/metadata"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

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

type gcemetaTestContext struct {
	srv             *Server
	gen             *fakeGenerator
	fakeAccessToken *oauth2.Token
	fakeIDToken     string
}

func setupGCEMetaTest(t *testing.T, ctx context.Context) *gcemetaTestContext {
	fakeAccessToken := &oauth2.Token{
		AccessToken: "fake_access_token",
		Expiry:      time.Now().Add(time.Hour),
	}
	fakeIDToken := "fake_id_token"

	gen := &fakeGenerator{fakeAccessToken, fakeIDToken, nil}
	srv := &Server{
		Generator:        gen,
		Email:            "fake@example.com",
		Scopes:           []string{"scope1", "scope2"},
		MinTokenLifetime: 2 * time.Minute,
	}

	addr, err := srv.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start: %s", err)
	}
	t.Cleanup(func() { srv.Stop(ctx) })

	t.Setenv("GCE_METADATA_HOST", addr)

	return &gcemetaTestContext{
		srv:             srv,
		gen:             gen,
		fakeAccessToken: fakeAccessToken,
		fakeIDToken:     fakeIDToken,
	}
}

func TestServer_OnGCE(t *testing.T) {
	ctx := t.Context()
	setupGCEMetaTest(t, ctx)
	assert.Loosely(t, metadata.OnGCE(), should.BeTrue)
}

func TestServer_MetadataClient(t *testing.T) {
	ctx := t.Context()
	setupGCEMetaTest(t, ctx)
	cl := metadata.NewClient(http.DefaultClient)

	num, err := cl.NumericProjectIDWithContext(ctx)
	assert.Loosely(t, err, should.BeNil)
	assert.Loosely(t, num, should.Equal("0"))

	pid, err := cl.ProjectIDWithContext(ctx)
	assert.Loosely(t, err, should.BeNil)
	assert.Loosely(t, pid, should.Equal("none"))

	accounts, err := cl.GetWithContext(ctx, "instance/service-accounts/")
	assert.Loosely(t, err, should.BeNil)
	assert.Loosely(t, accounts, should.Equal("default/\nfake@example.com/\n"))

	for _, acc := range []string{"fake@example.com", "default"} {
		info, err := cl.GetWithContext(ctx, "instance/service-accounts/"+acc+"/?recursive=true")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, info, should.Equal(
			`{"aliases":["default"],"email":"fake@example.com","scopes":["scope1","scope2"]}`))

		email, err := cl.GetWithContext(ctx, "instance/service-accounts/"+acc+"/email")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, email, should.Equal("fake@example.com"))

		scopes, err := cl.ScopesWithContext(ctx, acc)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, scopes, should.Match([]string{"scope1", "scope2"}))
	}
}

func TestServer_OAuth2TokenSource_DefaultScopes(t *testing.T) {
	ctx := t.Context()
	tc := setupGCEMetaTest(t, ctx)

	ts := google.ComputeTokenSource("default")
	tok, err := ts.Token()
	assert.Loosely(t, err, should.BeNil)
	if tok.AccessToken != tc.fakeAccessToken.AccessToken {
		panic("Bad token")
	}
	assert.Loosely(t, time.Until(tok.Expiry), should.BeGreaterThan(55*time.Minute))
	assert.Loosely(t, tc.gen.lastScopes, should.Match([]string{"scope1", "scope2"}))
}

func TestServer_OAuth2TokenSource_CustomScopes(t *testing.T) {
	ctx := t.Context()
	tc := setupGCEMetaTest(t, ctx)

	ts := google.ComputeTokenSource("default", "custom1", "custom1", "custom2")
	tok, err := ts.Token()
	assert.Loosely(t, err, should.BeNil)
	if tok.AccessToken != tc.fakeAccessToken.AccessToken {
		panic("Bad token")
	}
	assert.Loosely(t, time.Until(tok.Expiry), should.BeGreaterThan(55*time.Minute))
	assert.Loosely(t, tc.gen.lastScopes, should.Match([]string{"custom1", "custom2"}))
}

func TestServer_IDTokenFetch(t *testing.T) {
	ctx := t.Context()
	tc := setupGCEMetaTest(t, ctx)

	reply, err := metadata.GetWithContext(ctx, "instance/service-accounts/default/identity?audience=boo&format=ignored")
	assert.Loosely(t, err, should.BeNil)
	if reply != tc.fakeIDToken {
		panic("Bad token")
	}
}

func TestServer_UnsupportedMetadataCall(t *testing.T) {
	ctx := t.Context()
	setupGCEMetaTest(t, ctx)

	_, err := metadata.ExternalIPWithContext(ctx)
	assert.Loosely(t, err.Error(), should.Equal(`metadata: GCE metadata "instance/network-interfaces/0/access-configs/0/external-ip" not defined`))
}
