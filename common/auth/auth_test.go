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
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/net/context"
	"golang.org/x/oauth2"

	"go.chromium.org/luci/common/auth/internal"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

var (
	now    = time.Date(2015, time.January, 1, 0, 0, 0, 0, time.UTC)
	past   = time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)
	future = now.Add(24 * time.Hour)
)

func TestTransportFactory(t *testing.T) {
	t.Parallel()

	Convey("InteractiveLogin + interactive provider: invokes Login", t, func() {
		provider := &fakeTokenProvider{
			interactive: true,
		}
		auth, _ := newAuth(InteractiveLogin, provider, nil, "")

		// Returns "hooked" transport, not default.
		//
		// Note: we don't use ShouldNotEqual because it tries to read guts of
		// http.DefaultTransport and it sometimes triggers race detector.
		t, err := auth.Transport()
		So(err, ShouldBeNil)
		So(t != http.DefaultTransport, ShouldBeTrue)

		// MintToken is called by Login.
		So(provider.mintTokenCalled, ShouldBeTrue)
	})

	Convey(" SilentLogin + interactive provider: ErrLoginRequired", t, func() {
		auth, _ := newAuth(SilentLogin, &fakeTokenProvider{
			interactive: true,
		}, nil, "")
		_, err := auth.Transport()
		So(err, ShouldEqual, ErrLoginRequired)
	})

	Convey("OptionalLogin + interactive provider: Fallback to non-auth", t, func() {
		auth, _ := newAuth(OptionalLogin, &fakeTokenProvider{
			interactive: true,
		}, nil, "")
		t, err := auth.Transport()
		So(err, ShouldBeNil)
		So(t == http.DefaultTransport, ShouldBeTrue)
	})

	Convey("Always uses authenticating transport for non-interactive provider", t, func() {
		modes := []LoginMode{InteractiveLogin, SilentLogin, OptionalLogin}
		for _, mode := range modes {
			auth, _ := newAuth(mode, &fakeTokenProvider{}, nil, "")
			So(auth.Login(), ShouldBeNil) // noop
			t, err := auth.Transport()
			So(err, ShouldBeNil)
			So(t != http.DefaultTransport, ShouldBeTrue)
		}
	})
}

func TestRefreshToken(t *testing.T) {
	t.Parallel()

	Convey("Test non-interactive auth (no cache)", t, func() {
		tokenProvider := &fakeTokenProvider{
			interactive: false,
			tokenToMint: &oauth2.Token{AccessToken: "minted"},
		}
		auth, _ := newAuth(SilentLogin, tokenProvider, nil, "")
		So(auth.CheckLoginRequired(), ShouldBeNil)

		// No token yet, it is is lazily loaded below.
		tok, err := auth.currentToken()
		So(err, ShouldBeNil)
		So(tok, ShouldBeNil)

		// The token is minted on first request.
		tok, err = auth.GetAccessToken(time.Minute)
		So(err, ShouldBeNil)
		So(tok.AccessToken, ShouldEqual, "minted")
	})

	Convey("Test non-interactive auth (with non-expired cache)", t, func() {
		tokenProvider := &fakeTokenProvider{
			interactive: false,
		}
		auth, _ := newAuth(SilentLogin, tokenProvider, nil, "")
		cacheToken(auth, tokenProvider, &oauth2.Token{
			AccessToken: "cached",
			Expiry:      future,
		})

		So(auth.CheckLoginRequired(), ShouldBeNil)

		// No token yet, it is is lazily loaded below.
		tok, err := auth.currentToken()
		So(err, ShouldBeNil)
		So(tok, ShouldBeNil)

		// Cached token is used.
		tok, err = auth.GetAccessToken(time.Minute)
		So(err, ShouldBeNil)
		So(tok.AccessToken, ShouldEqual, "cached")
	})

	Convey("Test non-interactive auth (with expired cache)", t, func() {
		tokenProvider := &fakeTokenProvider{
			interactive:    false,
			tokenToRefresh: &oauth2.Token{AccessToken: "refreshed"},
		}
		auth, _ := newAuth(SilentLogin, tokenProvider, nil, "")
		cacheToken(auth, tokenProvider, &oauth2.Token{
			AccessToken: "cached",
			Expiry:      past,
		})

		So(auth.CheckLoginRequired(), ShouldBeNil)

		// No token yet, it is is lazily loaded below.
		tok, err := auth.currentToken()
		So(err, ShouldBeNil)
		So(tok, ShouldBeNil)

		// The usage triggers refresh procedure.
		tok, err = auth.GetAccessToken(time.Minute)
		So(err, ShouldBeNil)
		So(tok.AccessToken, ShouldEqual, "refreshed")
	})

	Convey("Test interactive auth (no cache)", t, func() {
		tokenProvider := &fakeTokenProvider{
			interactive: true,
			tokenToMint: &oauth2.Token{AccessToken: "minted"},
		}

		// No token cached, login is required.
		auth, _ := newAuth(SilentLogin, tokenProvider, nil, "")
		So(auth.CheckLoginRequired(), ShouldEqual, ErrLoginRequired)
		tok, err := auth.currentToken()
		So(err, ShouldBeNil)
		So(tok, ShouldBeNil)

		// Do it.
		err = auth.Login()
		So(err, ShouldBeNil)
		So(auth.CheckLoginRequired(), ShouldBeNil)

		// Minted initial token.
		tok, err = auth.currentToken()
		So(err, ShouldBeNil)
		So(tok.AccessToken, ShouldEqual, "minted")

		// And it is actually used.
		tok, err = auth.GetAccessToken(time.Minute)
		So(err, ShouldBeNil)
		So(tok.AccessToken, ShouldEqual, "minted")
	})

	Convey("Test interactive auth (with non-expired cache)", t, func() {
		tokenProvider := &fakeTokenProvider{
			interactive: true,
		}
		auth, _ := newAuth(SilentLogin, tokenProvider, nil, "")
		cacheToken(auth, tokenProvider, &oauth2.Token{
			AccessToken: "cached",
			Expiry:      future,
		})

		// No need to login, already have a token.
		So(auth.CheckLoginRequired(), ShouldBeNil)

		// Loaded cached token.
		tok, err := auth.currentToken()
		So(err, ShouldBeNil)
		So(tok.AccessToken, ShouldEqual, "cached")

		// And it is actually used.
		tok, err = auth.GetAccessToken(time.Minute)
		So(err, ShouldBeNil)
		So(tok.AccessToken, ShouldEqual, "cached")
	})

	Convey("Test interactive auth (with expired cache)", t, func() {
		tokenProvider := &fakeTokenProvider{
			interactive:    true,
			tokenToRefresh: &oauth2.Token{AccessToken: "refreshed"},
		}
		auth, _ := newAuth(SilentLogin, tokenProvider, nil, "")
		cacheToken(auth, tokenProvider, &oauth2.Token{
			AccessToken: "cached",
			Expiry:      past,
		})

		// No need to login, already have a token. Only its "access_token" part is
		// expired. Refresh token part is still valid, so no login is required.
		So(auth.CheckLoginRequired(), ShouldBeNil)

		// Loaded cached token.
		tok, err := auth.currentToken()
		So(err, ShouldBeNil)
		So(tok.AccessToken, ShouldEqual, "cached")

		// Attempting to use it triggers a refresh.
		tok, err = auth.GetAccessToken(time.Minute)
		So(err, ShouldBeNil)
		So(tok.AccessToken, ShouldEqual, "refreshed")
	})

	Convey("Test revoked refresh_token", t, func() {
		tokenProvider := &fakeTokenProvider{
			interactive:  true,
			revokedToken: true,
		}
		auth, _ := newAuth(SilentLogin, tokenProvider, nil, "")
		cacheToken(auth, tokenProvider, &oauth2.Token{
			AccessToken: "cached",
			Expiry:      past,
		})

		// No need to login, already have a token. Only its "access_token" part is
		// expired. Refresh token part is still presumably valid, there's no way to
		// detect that it has been revoked without attempting to use it.
		So(auth.CheckLoginRequired(), ShouldBeNil)

		// Loaded cached token.
		tok, err := auth.currentToken()
		So(err, ShouldBeNil)
		So(tok.AccessToken, ShouldEqual, "cached")

		// Attempting to use it triggers a refresh that fails.
		_, err = auth.GetAccessToken(time.Minute)
		So(err, ShouldEqual, ErrLoginRequired)
	})

	Convey("Test revoked credentials", t, func() {
		tokenProvider := &fakeTokenProvider{
			interactive:  false,
			revokedCreds: true,
		}
		auth, _ := newAuth(SilentLogin, tokenProvider, nil, "")
		cacheToken(auth, tokenProvider, &oauth2.Token{
			AccessToken: "cached",
			Expiry:      past,
		})

		So(auth.CheckLoginRequired(), ShouldBeNil)

		// Attempting to use expired cached token triggers a refresh that fails.
		_, err := auth.GetAccessToken(time.Minute)
		So(err, ShouldEqual, ErrBadCredentials)
	})

	Convey("Test transient errors when refreshing, success", t, func() {
		tokenProvider := &fakeTokenProvider{
			interactive:            false,
			transientRefreshErrors: 5,
			tokenToRefresh:         &oauth2.Token{AccessToken: "refreshed"},
		}
		auth, _ := newAuth(SilentLogin, tokenProvider, nil, "")
		cacheToken(auth, tokenProvider, &oauth2.Token{
			AccessToken: "cached",
			Expiry:      past,
		})

		So(auth.CheckLoginRequired(), ShouldBeNil)

		// Attempting to use expired cached token triggers a refresh that fails a
		// bunch of times, but the succeeds.
		tok, err := auth.GetAccessToken(time.Minute)
		So(err, ShouldBeNil)
		So(tok.AccessToken, ShouldEqual, "refreshed")

		// All calls were actually made.
		So(tokenProvider.transientRefreshErrors, ShouldEqual, 0)
	})

	Convey("Test transient errors when refreshing, timeout", t, func() {
		tokenProvider := &fakeTokenProvider{
			interactive:            false,
			transientRefreshErrors: 5000, // never succeeds
		}
		auth, ctx := newAuth(SilentLogin, tokenProvider, nil, "")
		cacheToken(auth, tokenProvider, &oauth2.Token{
			AccessToken: "cached",
			Expiry:      past,
		})

		So(auth.CheckLoginRequired(), ShouldBeNil)

		// Attempting to use expired cached token triggers a refresh that constantly
		// fails. Eventually we give up (on 17th attempt).
		before := clock.Now(ctx)
		_, err := auth.GetAccessToken(time.Minute)
		So(err, ShouldErrLike, "transient error")
		after := clock.Now(ctx)

		// It took reasonable amount of time and number of attempts.
		So(after.Sub(before), ShouldBeLessThan, 15*time.Second)
		So(5000-tokenProvider.transientRefreshErrors, ShouldEqual, 8)
	})
}

func TestActorMode(t *testing.T) {
	t.Parallel()

	Convey("Test non-interactive auth (no cache)", t, func() {
		baseProvider := &fakeTokenProvider{
			interactive: false,
			tokenToMint: &oauth2.Token{
				AccessToken: "minted-base",
				Expiry:      now.Add(time.Hour),
			},
			tokenToRefresh: &oauth2.Token{
				AccessToken: "refreshed-base",
				Expiry:      now.Add(2 * time.Hour),
			},
		}
		iamProvider := &fakeTokenProvider{
			interactive: false,
			tokenToMint: &oauth2.Token{
				AccessToken: "minted-iam",
				Expiry:      now.Add(30 * time.Minute),
			},
			tokenToRefresh: &oauth2.Token{
				AccessToken: "refreshed-iam",
				Expiry:      now.Add(2 * time.Hour),
			},
		}
		auth, ctx := newAuth(SilentLogin, baseProvider, iamProvider, "as-actor")
		So(auth.CheckLoginRequired(), ShouldBeNil)

		// No token yet, it is is lazily loaded below.
		tok, err := auth.currentToken()
		So(err, ShouldBeNil)
		So(tok, ShouldBeNil)

		// The token is minted on the first request. It is IAM-derived token.
		tok, err = auth.GetAccessToken(time.Minute)
		So(err, ShouldBeNil)
		So(tok.AccessToken, ShouldEqual, "minted-iam")

		// The correct base token was minted as well and used by IAM call.
		So(iamProvider.baseTokenInMint.AccessToken, ShouldEqual, "minted-base")
		iamProvider.baseTokenInMint = nil

		// After 40 min the IAM-generated token expires, but base is still ok.
		clock.Get(ctx).(testclock.TestClock).Add(40 * time.Minute)

		// Getting a refreshed IAM token.
		tok, err = auth.GetAccessToken(time.Minute)
		So(err, ShouldBeNil)
		So(tok.AccessToken, ShouldEqual, "refreshed-iam")

		// Using existing base token (still valid).
		So(iamProvider.baseTokenInRefresh.AccessToken, ShouldEqual, "minted-base")
		iamProvider.baseTokenInRefresh = nil
	})
}

func TestTransport(t *testing.T) {
	t.Parallel()

	Convey("Test transport works", t, func(c C) {
		calls := 0
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			switch r.URL.Path {
			case "/1":
				c.So(r.Header.Get("Authorization"), ShouldEqual, "Bearer minted")
			case "/2":
				c.So(r.Header.Get("Authorization"), ShouldEqual, "Bearer minted")
			case "/3":
				c.So(r.Header.Get("Authorization"), ShouldEqual, "Bearer refreshed")
			default:
				c.So(r.URL.Path, ShouldBeBlank) // just fail in some helpful way
			}
			w.WriteHeader(200)
		}))
		defer ts.Close()

		tokenProvider := &fakeTokenProvider{
			interactive:    false,
			tokenToMint:    &oauth2.Token{AccessToken: "minted", Expiry: now.Add(time.Hour)},
			tokenToRefresh: &oauth2.Token{AccessToken: "refreshed", Expiry: now.Add(2 * time.Hour)},
		}

		auth, ctx := newAuth(SilentLogin, tokenProvider, nil, "")
		client, err := auth.Client()
		So(err, ShouldBeNil)
		So(client, ShouldNotBeNil)

		// Initial call will mint new token.
		resp, err := client.Get(ts.URL + "/1")
		So(err, ShouldBeNil)
		ioutil.ReadAll(resp.Body)
		defer resp.Body.Close()

		// Minted token is now cached.
		tok, err := auth.currentToken()
		So(err, ShouldBeNil)
		So(tok.AccessToken, ShouldEqual, "minted")

		cacheKey, _ := tokenProvider.CacheKey(ctx)
		cached, err := auth.testingCache.GetToken(cacheKey)
		So(err, ShouldBeNil)
		So(cached.AccessToken, ShouldEqual, "minted")

		// 40 minutes later it is still OK to use.
		clock.Get(ctx).(testclock.TestClock).Add(40 * time.Minute)
		resp, err = client.Get(ts.URL + "/2")
		So(err, ShouldBeNil)
		ioutil.ReadAll(resp.Body)
		defer resp.Body.Close()

		// 30 min later (70 min since the start) it is expired and refreshed.
		clock.Get(ctx).(testclock.TestClock).Add(30 * time.Minute)
		resp, err = client.Get(ts.URL + "/3")
		So(err, ShouldBeNil)
		ioutil.ReadAll(resp.Body)
		defer resp.Body.Close()

		tok, err = auth.currentToken()
		So(err, ShouldBeNil)
		So(tok.AccessToken, ShouldEqual, "refreshed")

		// All calls are actually made.
		So(calls, ShouldEqual, 3)
	})
}

func TestOptionalLogin(t *testing.T) {
	t.Parallel()

	Convey("Test optional login works", t, func(c C) {
		// This test simulates following scenario for OptionalLogin mode:
		//   1. There's existing cached access token.
		//   2. At some point it expires.
		//   3. Refresh fails with ErrBadRefreshToken (refresh token is revoked).
		//   4. Authenticator switches to anonymous calls.
		calls := 0
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			switch r.URL.Path {
			case "/1":
				c.So(r.Header.Get("Authorization"), ShouldEqual, "Bearer cached")
			case "/2":
				c.So(r.Header.Get("Authorization"), ShouldEqual, "")
			default:
				c.So(r.URL.Path, ShouldBeBlank) // just fail in some helpful way
			}
			w.WriteHeader(200)
		}))
		defer ts.Close()

		tokenProvider := &fakeTokenProvider{
			interactive:  true,
			revokedToken: true,
		}
		auth, ctx := newAuth(OptionalLogin, tokenProvider, nil, "")
		cacheToken(auth, tokenProvider, &oauth2.Token{
			AccessToken: "cached",
			Expiry:      now.Add(time.Hour),
		})

		client, err := auth.Client()
		So(err, ShouldBeNil)
		So(client, ShouldNotBeNil)

		// Initial call uses existing cached token.
		resp, err := client.Get(ts.URL + "/1")
		So(err, ShouldBeNil)
		ioutil.ReadAll(resp.Body)
		defer resp.Body.Close()

		// It expires at ~60 minutes, refresh fails, authenticator switches to
		// anonymous access.
		clock.Get(ctx).(testclock.TestClock).Add(65 * time.Minute)
		resp, err = client.Get(ts.URL + "/2")
		So(err, ShouldBeNil)
		ioutil.ReadAll(resp.Body)
		defer resp.Body.Close()

		// Bad token is removed from the cache.
		tok, err := auth.currentToken()
		So(err, ShouldBeNil)
		So(tok, ShouldBeNil)
		cacheKey, _ := tokenProvider.CacheKey(ctx)
		cached, err := auth.testingCache.GetToken(cacheKey)
		So(cached, ShouldBeNil)
		So(err, ShouldBeNil)

		// All calls are actually made.
		So(calls, ShouldEqual, 2)
	})
}

func newAuth(loginMode LoginMode, base, iam internal.TokenProvider, actAs string) (*Authenticator, context.Context) {
	// Use auto-advancing fake time.
	ctx := mathrand.Set(context.Background(), rand.New(rand.NewSource(123)))
	ctx, tc := testclock.UseTime(ctx, now)
	tc.SetTimerCallback(func(d time.Duration, t clock.Timer) {
		tc.Add(d)
	})
	a := NewAuthenticator(ctx, loginMode, Options{
		ActAsServiceAccount:      actAs,
		testingBaseTokenProvider: base,
		testingIAMTokenProvider:  iam,
	})
	a.testingCache = &internal.MemoryTokenCache{}
	return a, ctx
}

func cacheToken(a *Authenticator, p internal.TokenProvider, tok *oauth2.Token) {
	cacheKey, err := p.CacheKey(a.ctx)
	if err != nil {
		panic(err)
	}
	err = a.testingCache.PutToken(cacheKey, tok)
	if err != nil {
		panic(err)
	}
}

////////////////////////////////////////////////////////////////////////////////

type fakeTokenProvider struct {
	interactive            bool
	revokedCreds           bool
	revokedToken           bool
	transientRefreshErrors int
	tokenToMint            *oauth2.Token
	tokenToRefresh         *oauth2.Token

	mintTokenCalled    bool
	refreshTokenCalled bool

	baseTokenInMint    *oauth2.Token
	baseTokenInRefresh *oauth2.Token
}

func (p *fakeTokenProvider) RequiresInteraction() bool {
	return p.interactive
}

func (p *fakeTokenProvider) Lightweight() bool {
	return true
}

func (p *fakeTokenProvider) CacheKey(ctx context.Context) (*internal.CacheKey, error) {
	return &internal.CacheKey{Key: "fake"}, nil
}

func (p *fakeTokenProvider) MintToken(ctx context.Context, base *oauth2.Token) (*oauth2.Token, error) {
	p.mintTokenCalled = true
	p.baseTokenInMint = base
	if p.revokedCreds {
		return nil, internal.ErrBadCredentials
	}
	if p.tokenToMint != nil {
		return p.tokenToMint, nil
	}
	return &oauth2.Token{AccessToken: "some minted token"}, nil
}

func (p *fakeTokenProvider) RefreshToken(ctx context.Context, prev, base *oauth2.Token) (*oauth2.Token, error) {
	p.refreshTokenCalled = true
	p.baseTokenInRefresh = base
	if p.transientRefreshErrors != 0 {
		p.transientRefreshErrors--
		return nil, errors.New("transient error", transient.Tag)
	}
	if p.revokedCreds {
		return nil, internal.ErrBadCredentials
	}
	if p.revokedToken {
		return nil, internal.ErrBadRefreshToken
	}
	if p.tokenToRefresh != nil {
		return p.tokenToRefresh, nil
	}
	return &oauth2.Token{AccessToken: "some refreshed token"}, nil
}
