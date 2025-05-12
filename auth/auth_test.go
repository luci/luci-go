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
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"go.chromium.org/luci/auth/internal"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/gcloud/googleoauth"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

var (
	now    = time.Date(2015, time.January, 1, 0, 0, 0, 0, time.UTC)
	past   = time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)
	future = now.Add(24 * time.Hour)
)

func TestTransportFactory(t *testing.T) {
	t.Parallel()

	ftt.Run("InteractiveLogin + interactive provider: invokes Login", t, func(t *ftt.Test) {
		provider := &fakeTokenProvider{
			interactive: true,
		}
		auth, _ := newAuth(InteractiveLogin, provider, nil, "")

		// Returns "hooked" transport, not default.
		//
		// Note: we don't use ShouldNotEqual because it tries to read guts of
		// http.DefaultTransport and it sometimes triggers race detector.
		transport, err := auth.Transport()
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, transport != http.DefaultTransport, should.BeTrue)

		// MintToken is called by Login.
		assert.Loosely(t, provider.mintTokenCalled, should.BeTrue)
	})

	ftt.Run("SilentLogin + interactive provider: ErrLoginRequired", t, func(t *ftt.Test) {
		auth, _ := newAuth(SilentLogin, &fakeTokenProvider{
			interactive: true,
		}, nil, "")
		_, err := auth.Transport()
		assert.Loosely(t, err, should.Equal(ErrLoginRequired))
	})

	ftt.Run("OptionalLogin + interactive provider: Fallback to non-auth", t, func(t *ftt.Test) {
		auth, _ := newAuth(OptionalLogin, &fakeTokenProvider{
			interactive: true,
		}, nil, "")
		transport, err := auth.Transport()
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, transport == http.DefaultTransport, should.BeTrue)
	})

	ftt.Run("Always uses authenticating transport for non-interactive provider", t, func(t *ftt.Test) {
		modes := []LoginMode{InteractiveLogin, SilentLogin, OptionalLogin}
		for _, mode := range modes {
			auth, _ := newAuth(mode, &fakeTokenProvider{}, nil, "")
			assert.Loosely(t, auth.Login(), should.BeNil) // noop
			transport, err := auth.Transport()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, transport != http.DefaultTransport, should.BeTrue)
		}
	})
}

func TestRefreshToken(t *testing.T) {
	t.Parallel()

	ftt.Run("Test non-interactive auth (no cache)", t, func(t *ftt.Test) {
		tokenProvider := &fakeTokenProvider{
			interactive: false,
			tokenToMint: &internal.Token{
				Token: oauth2.Token{AccessToken: "minted"},
				Email: "freshly-minted@example.com",
			},
		}
		auth, _ := newAuth(SilentLogin, tokenProvider, nil, "")
		assert.Loosely(t, auth.CheckLoginRequired(), should.BeNil)

		// No token yet, it is is lazily loaded below.
		tok, err := auth.currentToken()
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, tok, should.BeNil)

		// The token is minted on first request.
		oauthTok, err := auth.GetAccessToken(time.Minute)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, oauthTok.AccessToken, should.Equal("minted"))

		// And we also get an email straight from MintToken call.
		email, err := auth.GetEmail()
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, email, should.Equal("freshly-minted@example.com"))
	})

	ftt.Run("Test non-interactive auth (with non-expired cache)", t, func(t *ftt.Test) {
		tokenProvider := &fakeTokenProvider{
			interactive: false,
		}
		auth, _ := newAuth(SilentLogin, tokenProvider, nil, "")
		cacheToken(auth, tokenProvider, &internal.Token{
			Token: oauth2.Token{
				AccessToken: "cached",
				Expiry:      future,
			},
			Email: "cached-email@example.com",
		})

		assert.Loosely(t, auth.CheckLoginRequired(), should.BeNil)

		// Cached token is used.
		oauthTok, err := auth.GetAccessToken(time.Minute)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, oauthTok.AccessToken, should.Equal("cached"))

		// Cached email is used.
		email, err := auth.GetEmail()
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, email, should.Equal("cached-email@example.com"))
	})

	ftt.Run("Test non-interactive auth (with expired cache)", t, func(t *ftt.Test) {
		tokenProvider := &fakeTokenProvider{
			interactive: false,
			tokenToRefresh: &internal.Token{
				Token: oauth2.Token{AccessToken: "refreshed"},
				Email: "new-email@example.com",
			},
		}
		auth, _ := newAuth(SilentLogin, tokenProvider, nil, "")
		cacheToken(auth, tokenProvider, &internal.Token{
			Token: oauth2.Token{
				AccessToken: "cached",
				Expiry:      past,
			},
			Email: "cached-email@example.com",
		})

		assert.Loosely(t, auth.CheckLoginRequired(), should.BeNil)

		// The usage triggers refresh procedure.
		oauthTok, err := auth.GetAccessToken(time.Minute)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, oauthTok.AccessToken, should.Equal("refreshed"))

		// Using a newly fetched email.
		email, err := auth.GetEmail()
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, email, should.Equal("new-email@example.com"))
	})

	ftt.Run("Test interactive auth (no cache)", t, func(t *ftt.Test) {
		tokenProvider := &fakeTokenProvider{
			interactive: true,
			tokenToMint: &internal.Token{
				Token: oauth2.Token{AccessToken: "minted"},
				Email: "freshly-minted@example.com",
			},
		}

		auth, _ := newAuth(SilentLogin, tokenProvider, nil, "")

		// No token cached.
		tok, err := auth.currentToken()
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, tok, should.BeNil)

		// Login is required, as reported by various methods.
		assert.Loosely(t, auth.CheckLoginRequired(), should.Equal(ErrLoginRequired))

		oauthTok, err := auth.GetAccessToken(time.Minute)
		assert.Loosely(t, oauthTok, should.BeNil)
		assert.Loosely(t, err, should.Equal(ErrLoginRequired))

		email, err := auth.GetEmail()
		assert.Loosely(t, email, should.BeEmpty)
		assert.Loosely(t, err, should.Equal(ErrLoginRequired))

		// Do it.
		err = auth.Login()
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, auth.CheckLoginRequired(), should.BeNil)

		// Minted initial token.
		tok, err = auth.currentToken()
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, tok.AccessToken, should.Equal("minted"))

		// And it is actually used.
		oauthTok, err = auth.GetAccessToken(time.Minute)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, oauthTok.AccessToken, should.Equal("minted"))

		// Email works too now.
		email, err = auth.GetEmail()
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, email, should.Equal("freshly-minted@example.com"))
	})

	ftt.Run("Test interactive auth (with non-expired cache)", t, func(t *ftt.Test) {
		tokenProvider := &fakeTokenProvider{
			interactive: true,
		}
		auth, _ := newAuth(SilentLogin, tokenProvider, nil, "")
		cacheToken(auth, tokenProvider, &internal.Token{
			Token: oauth2.Token{
				AccessToken: "cached",
				Expiry:      future,
			},
			Email: "cached-email@example.com",
		})

		// No need to login, already have a token.
		assert.Loosely(t, auth.CheckLoginRequired(), should.BeNil)

		// Loaded cached token.
		tok, err := auth.currentToken()
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, tok.AccessToken, should.Equal("cached"))

		// And it is actually used.
		oauthTok, err := auth.GetAccessToken(time.Minute)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, oauthTok.AccessToken, should.Equal("cached"))

		// Email works too now.
		email, err := auth.GetEmail()
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, email, should.Equal("cached-email@example.com"))
	})

	ftt.Run("Test interactive auth (with expired cache)", t, func(t *ftt.Test) {
		tokenProvider := &fakeTokenProvider{
			interactive: true,
			tokenToRefresh: &internal.Token{
				Token: oauth2.Token{AccessToken: "refreshed"},
				Email: "refreshed-email@example.com",
			},
		}
		auth, _ := newAuth(SilentLogin, tokenProvider, nil, "")
		cacheToken(auth, tokenProvider, &internal.Token{
			Token: oauth2.Token{
				AccessToken: "cached",
				Expiry:      past,
			},
			Email: "cached-email@example.com",
		})

		// No need to login, already have a token. Only its "access_token" part is
		// expired. Refresh token part is still valid, so no login is required.
		assert.Loosely(t, auth.CheckLoginRequired(), should.BeNil)

		// Loaded cached token.
		tok, err := auth.currentToken()
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, tok.AccessToken, should.Equal("cached"))

		// Attempting to use it triggers a refresh.
		oauthTok, err := auth.GetAccessToken(time.Minute)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, oauthTok.AccessToken, should.Equal("refreshed"))

		// Email is also refreshed.
		email, err := auth.GetEmail()
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, email, should.Equal("refreshed-email@example.com"))
	})

	ftt.Run("Test revoked refresh_token", t, func(t *ftt.Test) {
		tokenProvider := &fakeTokenProvider{
			interactive:  true,
			revokedToken: true,
		}
		auth, _ := newAuth(SilentLogin, tokenProvider, nil, "")
		cacheToken(auth, tokenProvider, &internal.Token{
			Token: oauth2.Token{
				AccessToken: "cached",
				Expiry:      past,
			},
			Email: "cached@example.com",
		})

		// No need to login, already have a token. Only its "access_token" part is
		// expired. Refresh token part is still presumably valid, there's no way to
		// detect that it has been revoked without attempting to use it.
		assert.Loosely(t, auth.CheckLoginRequired(), should.BeNil)

		// Loaded cached token.
		tok, err := auth.currentToken()
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, tok.AccessToken, should.Equal("cached"))

		// Attempting to use it triggers a refresh that fails.
		_, err = auth.GetAccessToken(time.Minute)
		assert.Loosely(t, err, should.Equal(ErrLoginRequired))

		// Same happens when trying to grab an email.
		_, err = auth.GetEmail()
		assert.Loosely(t, err, should.Equal(ErrLoginRequired))
	})

	ftt.Run("Test revoked credentials", t, func(t *ftt.Test) {
		tokenProvider := &fakeTokenProvider{
			interactive:  false,
			revokedCreds: true,
		}
		auth, _ := newAuth(SilentLogin, tokenProvider, nil, "")
		cacheToken(auth, tokenProvider, &internal.Token{
			Token: oauth2.Token{
				AccessToken: "cached",
				Expiry:      past,
			},
			Email: "cached@example.com",
		})

		assert.Loosely(t, auth.CheckLoginRequired(), should.BeNil)

		// Attempting to use expired cached token triggers a refresh that fails.
		_, err := auth.GetAccessToken(time.Minute)
		assert.Loosely(t, err, should.ErrLike(
			"failed to refresh auth token: invalid or unavailable service account credentials"))

		// Same happens when trying to grab an email.
		_, err = auth.GetEmail()
		assert.Loosely(t, err, should.ErrLike(
			"failed to refresh auth token: invalid or unavailable service account credentials"))
	})

	ftt.Run("Test transient errors when refreshing, success", t, func(t *ftt.Test) {
		tokenProvider := &fakeTokenProvider{
			interactive:            false,
			transientRefreshErrors: 5,
			tokenToRefresh: &internal.Token{
				Token: oauth2.Token{AccessToken: "refreshed"},
			},
		}
		auth, _ := newAuth(SilentLogin, tokenProvider, nil, "")
		cacheToken(auth, tokenProvider, &internal.Token{
			Token: oauth2.Token{
				AccessToken: "cached",
				Expiry:      past,
			},
		})

		assert.Loosely(t, auth.CheckLoginRequired(), should.BeNil)

		// Attempting to use expired cached token triggers a refresh that fails a
		// bunch of times, but the succeeds.
		tok, err := auth.GetAccessToken(time.Minute)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, tok.AccessToken, should.Equal("refreshed"))

		// All calls were actually made.
		assert.Loosely(t, tokenProvider.transientRefreshErrors, should.BeZero)
	})

	ftt.Run("Test transient errors when refreshing, timeout", t, func(t *ftt.Test) {
		tokenProvider := &fakeTokenProvider{
			interactive:            false,
			transientRefreshErrors: 5000, // never succeeds
		}
		auth, ctx := newAuth(SilentLogin, tokenProvider, nil, "")
		cacheToken(auth, tokenProvider, &internal.Token{
			Token: oauth2.Token{
				AccessToken: "cached",
				Expiry:      past,
			},
		})

		assert.Loosely(t, auth.CheckLoginRequired(), should.BeNil)

		// Attempting to use expired cached token triggers a refresh that constantly
		// fails. Eventually we give up.
		before := clock.Now(ctx)
		_, err := auth.GetAccessToken(time.Minute)
		assert.Loosely(t, err, should.ErrLike("transient error"))
		after := clock.Now(ctx)

		// It took reasonable amount of time and number of attempts.
		assert.Loosely(t, after.Sub(before), should.BeLessThan(4*time.Minute))
		assert.Loosely(t, 5000-tokenProvider.transientRefreshErrors, should.Equal(15))
	})

	ftt.Run("Test token provider not really refreshing the token", t, func(t *ftt.Test) {
		almostExpiredTok := &internal.Token{
			Token: oauth2.Token{
				AccessToken: "cached",
				Expiry:      now.Add(time.Second), // very close to expiration
			},
			Email: "cached-email@example.com",
		}
		tokenProvider := &fakeTokenProvider{
			interactive:    false,
			tokenToRefresh: almostExpiredTok, // returns it unchanged
		}
		auth, _ := newAuth(SilentLogin, tokenProvider, nil, "")
		cacheToken(auth, tokenProvider, almostExpiredTok)

		// Gets the token back, even though it doesn't have enough lifetime left.
		oauthTok, err := auth.GetAccessToken(time.Minute)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, oauthTok.AccessToken, should.Equal("cached"))

		// Still attempted to refresh it though.
		assert.That(t, tokenProvider.refreshTokenCalled, should.BeTrue)
	})
}

func TestActorMode(t *testing.T) {
	t.Parallel()

	ftt.Run("Test non-interactive auth (no cache)", t, func(t *ftt.Test) {
		baseProvider := &fakeTokenProvider{
			interactive: false,
			tokenToMint: &internal.Token{
				Token: oauth2.Token{
					AccessToken: "minted-base",
					Expiry:      now.Add(time.Hour),
				},
				Email: "must-be-ignored@example.com",
			},
			tokenToRefresh: &internal.Token{
				Token: oauth2.Token{
					AccessToken: "refreshed-base",
					Expiry:      now.Add(2 * time.Hour),
				},
				Email: "must-be-ignored@example.com",
			},
		}
		iamProvider := &fakeTokenProvider{
			interactive: false,
			tokenToMint: &internal.Token{
				Token: oauth2.Token{
					AccessToken: "minted-iam",
					Expiry:      now.Add(30 * time.Minute),
				},
				Email: "minted-iam@example.com",
			},
			tokenToRefresh: &internal.Token{
				Token: oauth2.Token{
					AccessToken: "refreshed-iam",
					Expiry:      now.Add(2 * time.Hour),
				},
				Email: "refreshed-iam@example.com",
			},
		}
		auth, ctx := newAuth(SilentLogin, baseProvider, iamProvider, "as-actor")
		assert.Loosely(t, auth.CheckLoginRequired(), should.BeNil)

		// No token yet, it is is lazily loaded below.
		tok, err := auth.currentToken()
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, tok, should.BeNil)

		// The token is minted on the first request. It is IAM-derived token.
		oauthTok, err := auth.GetAccessToken(time.Minute)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, oauthTok.AccessToken, should.Equal("minted-iam"))

		// The email also matches the IAM token.
		email, err := auth.GetEmail()
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, email, should.Equal("minted-iam@example.com"))

		// The correct base token was minted as well and used by IAM call.
		assert.Loosely(t, iamProvider.baseTokenInMint.AccessToken, should.Equal("minted-base"))
		iamProvider.baseTokenInMint = nil

		// After 40 min the IAM-generated token expires, but base is still ok.
		clock.Get(ctx).(testclock.TestClock).Add(40 * time.Minute)

		// Getting a refreshed IAM token.
		oauthTok, err = auth.GetAccessToken(time.Minute)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, oauthTok.AccessToken, should.Equal("refreshed-iam"))

		// The email also matches the IAM token.
		email, err = auth.GetEmail()
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, email, should.Equal("refreshed-iam@example.com"))

		// Using existing base token (still valid).
		assert.Loosely(t, iamProvider.baseTokenInRefresh.AccessToken, should.Equal("minted-base"))
		iamProvider.baseTokenInRefresh = nil
	})
}

func TestTransport(t *testing.T) {
	t.Parallel()

	ftt.Run("Test transport works", t, func(c *ftt.Test) {
		calls := 0
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			switch r.URL.Path {
			case "/1":
				assert.Loosely(c, r.Header.Get("Authorization"), should.Equal("Bearer minted"))
			case "/2":
				assert.Loosely(c, r.Header.Get("Authorization"), should.Equal("Bearer minted"))
			case "/3":
				assert.Loosely(c, r.Header.Get("Authorization"), should.Equal("Bearer refreshed"))
			default:
				assert.Loosely(c, r.URL.Path, should.BeZero) // just fail in some helpful way
			}
			w.WriteHeader(200)
		}))
		defer ts.Close()

		tokenProvider := &fakeTokenProvider{
			interactive: false,
			tokenToMint: &internal.Token{
				Token: oauth2.Token{AccessToken: "minted", Expiry: now.Add(time.Hour)},
			},
			tokenToRefresh: &internal.Token{
				Token: oauth2.Token{AccessToken: "refreshed", Expiry: now.Add(2 * time.Hour)},
			},
		}

		auth, ctx := newAuth(SilentLogin, tokenProvider, nil, "")
		client, err := auth.Client()
		assert.Loosely(c, err, should.BeNil)
		assert.Loosely(c, client, should.NotBeNil)

		// Initial call will mint new token.
		resp, err := client.Get(ts.URL + "/1")
		assert.Loosely(c, err, should.BeNil)
		io.ReadAll(resp.Body)
		defer resp.Body.Close()

		// Minted token is now cached.
		tok, err := auth.currentToken()
		assert.Loosely(c, err, should.BeNil)
		assert.Loosely(c, tok.AccessToken, should.Equal("minted"))

		cacheKey, _ := tokenProvider.CacheKey(ctx)
		cached, err := auth.opts.testingCache.GetToken(cacheKey)
		assert.Loosely(c, err, should.BeNil)
		assert.Loosely(c, cached.AccessToken, should.Equal("minted"))

		// 40 minutes later it is still OK to use.
		clock.Get(ctx).(testclock.TestClock).Add(40 * time.Minute)
		resp, err = client.Get(ts.URL + "/2")
		assert.Loosely(c, err, should.BeNil)
		io.ReadAll(resp.Body)
		defer resp.Body.Close()

		// 30 min later (70 min since the start) it is expired and refreshed.
		clock.Get(ctx).(testclock.TestClock).Add(30 * time.Minute)
		resp, err = client.Get(ts.URL + "/3")
		assert.Loosely(c, err, should.BeNil)
		io.ReadAll(resp.Body)
		defer resp.Body.Close()

		tok, err = auth.currentToken()
		assert.Loosely(c, err, should.BeNil)
		assert.Loosely(c, tok.AccessToken, should.Equal("refreshed"))

		// All calls are actually made.
		assert.Loosely(c, calls, should.Equal(3))
	})
}

func TestRequiresWarmupProvider(t *testing.T) {
	t.Parallel()

	t.Run("Mints the token eagerly: OK", func(t *testing.T) {
		tokenProvider := &fakeTokenProvider{
			requiresWarmup: true,
			revokedCreds:   false,
		}
		auth, _ := newAuth(SilentLogin, tokenProvider, nil, "")
		_, err := auth.Transport()
		assert.NoErr(t, err)
	})

	t.Run("Mints the token eagerly: fail early", func(t *testing.T) {
		tokenProvider := &fakeTokenProvider{
			requiresWarmup: true,
			revokedCreds:   true,
		}
		auth, _ := newAuth(SilentLogin, tokenProvider, nil, "")
		_, err := auth.Transport()
		assert.That(t, err, should.ErrLike(internal.ErrBadCredentials))
	})

	t.Run("Mints the token lazily: fail late", func(t *testing.T) {
		tokenProvider := &fakeTokenProvider{
			requiresWarmup: false,
			revokedCreds:   true,
		}
		auth, _ := newAuth(SilentLogin, tokenProvider, nil, "")
		rt, err := auth.Transport()
		assert.NoErr(t, err)
		_, err = rt.RoundTrip(&http.Request{})
		assert.That(t, err, should.ErrLike(internal.ErrBadCredentials))
	})
}

func TestOptionalLogin(t *testing.T) {
	t.Parallel()

	ftt.Run("Test optional login works", t, func(c *ftt.Test) {
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
				assert.Loosely(c, r.Header.Get("Authorization"), should.Equal("Bearer cached"))
			case "/2":
				assert.Loosely(c, r.Header.Get("Authorization"), should.BeEmpty)
			default:
				assert.Loosely(c, r.URL.Path, should.BeZero) // just fail in some helpful way
			}
			w.WriteHeader(200)
		}))
		defer ts.Close()

		tokenProvider := &fakeTokenProvider{
			interactive:  true,
			revokedToken: true,
		}
		auth, ctx := newAuth(OptionalLogin, tokenProvider, nil, "")
		cacheToken(auth, tokenProvider, &internal.Token{
			Token: oauth2.Token{
				AccessToken: "cached",
				Expiry:      now.Add(time.Hour),
			},
		})

		client, err := auth.Client()
		assert.Loosely(c, err, should.BeNil)
		assert.Loosely(c, client, should.NotBeNil)

		// Initial call uses existing cached token.
		resp, err := client.Get(ts.URL + "/1")
		assert.Loosely(c, err, should.BeNil)
		io.ReadAll(resp.Body)
		defer resp.Body.Close()

		// It expires at ~60 minutes, refresh fails, authenticator switches to
		// anonymous access.
		clock.Get(ctx).(testclock.TestClock).Add(65 * time.Minute)
		resp, err = client.Get(ts.URL + "/2")
		assert.Loosely(c, err, should.BeNil)
		io.ReadAll(resp.Body)
		defer resp.Body.Close()

		// Bad token is removed from the cache.
		tok, err := auth.currentToken()
		assert.Loosely(c, err, should.BeNil)
		assert.Loosely(c, tok, should.BeNil)
		cacheKey, _ := tokenProvider.CacheKey(ctx)
		cached, err := auth.opts.testingCache.GetToken(cacheKey)
		assert.Loosely(c, cached, should.BeNil)
		assert.Loosely(c, err, should.BeNil)

		// All calls are actually made.
		assert.Loosely(c, calls, should.Equal(2))
	})
}

func TestGetEmail(t *testing.T) {
	t.Parallel()

	t.Run("Test non-interactive auth (no cache)", func(t *testing.T) {
		tokenProvider := &fakeTokenProvider{
			interactive: false,
			knownEmail:  "known-email@example.com",
			tokenToMint: &internal.Token{
				Token: oauth2.Token{AccessToken: "must-not-be-called"},
				Email: "must-not-be-called@example.com",
			},
		}
		auth, _ := newAuth(SilentLogin, tokenProvider, nil, "")

		// No cached token.
		tok, err := auth.currentToken()
		assert.NoErr(t, err)
		assert.Loosely(t, tok, should.BeNil)

		// We get the email directly from the provider.
		email, err := auth.GetEmail()
		assert.NoErr(t, err)
		assert.That(t, email, should.Equal("known-email@example.com"))

		// MintToken was NOT called.
		assert.That(t, tokenProvider.mintTokenCalled, should.BeFalse)
	})

	t.Run("Non-expired cache without email is upgraded", func(t *testing.T) {
		tokenProvider := &fakeTokenProvider{
			interactive: true,
		}
		auth, _ := newAuth(SilentLogin, tokenProvider, nil, "")
		cacheToken(auth, tokenProvider, &internal.Token{
			Token: oauth2.Token{
				AccessToken: "cached",
				Expiry:      future,
			},
			Email: "", // "old style" cache without an email
		})

		// No need to login, already have a token.
		assert.NoErr(t, auth.CheckLoginRequired())

		// GetAccessToken returns the cached token.
		oauthTok, err := auth.GetAccessToken(time.Minute)
		assert.NoErr(t, err)
		assert.That(t, oauthTok.AccessToken, should.Equal("cached"))

		// But getting an email triggers a refresh, since the cached token doesn't
		// have an email.
		email, err := auth.GetEmail()
		assert.NoErr(t, err)
		assert.That(t, email, should.Equal("some-email-refreshtoken@example.com"))

		// GetAccessToken picks up the refreshed token too.
		oauthTok, err = auth.GetAccessToken(time.Minute)
		assert.NoErr(t, err)
		assert.That(t, oauthTok.AccessToken, should.Equal("some refreshed access token"))
	})

	t.Run("No email triggers ErrNoEmail", func(t *testing.T) {
		tokenProvider := &fakeTokenProvider{
			interactive: false,
			tokenToMint: &internal.Token{
				Token: oauth2.Token{AccessToken: "minted"},
				Email: internal.NoEmail,
			},
		}
		auth, _ := newAuth(SilentLogin, tokenProvider, nil, "")
		assert.NoErr(t, auth.CheckLoginRequired())

		// The token is minted on first request.
		oauthTok, err := auth.GetAccessToken(time.Minute)
		assert.NoErr(t, err)
		assert.That(t, oauthTok.AccessToken, should.Equal("minted"))

		// But getting an email fails with ErrNoEmail.
		email, err := auth.GetEmail()
		assert.That(t, err, should.Equal(ErrNoEmail))
		assert.That(t, email, should.Equal(""))
	})

	t.Run("Fallback to the token info endpoint", func(t *testing.T) {
		tokenProvider := &fakeTokenProvider{
			interactive:        false,
			emailUnimplemented: true,
			tokenToMint: &internal.Token{
				Token: oauth2.Token{AccessToken: "minted"},
				Email: internal.UnknownEmail,
			},
		}
		auth, _ := newAuth(SilentLogin, tokenProvider, nil, "")
		assert.NoErr(t, auth.CheckLoginRequired())

		// Getting an email triggers the token info endpoint call.
		calls := 0
		auth.opts.testingTokenInfoEndpoint = func(_ context.Context, params googleoauth.TokenInfoParams) (*googleoauth.TokenInfo, error) {
			calls++
			assert.That(t, params.AccessToken, should.Equal("minted"))
			return &googleoauth.TokenInfo{
				Email:         fmt.Sprintf("token-info-%d@example.com", calls),
				EmailVerified: true,
			}, nil
		}
		email, err := auth.GetEmail()
		assert.NoErr(t, err)
		assert.That(t, email, should.Equal("token-info-1@example.com"))

		// Calling it again returns the cached email.
		email, err = auth.GetEmail()
		assert.NoErr(t, err)
		assert.That(t, email, should.Equal("token-info-1@example.com"))
	})
}

func TestNormalizeScopes(t *testing.T) {
	t.Parallel()

	checkExactSameSlice := func(a, b []string) {
		assert.Loosely(t, a, should.Match(b))
		assert.Loosely(t, &a[0], should.Equal(&b[0]))
	}

	ftt.Run("Works", t, func(t *ftt.Test) {
		assert.Loosely(t, normalizeScopes(nil), should.BeNil)

		// Doesn't copy already normalized slices.
		slice := []string{"a"}
		checkExactSameSlice(slice, normalizeScopes(slice))
		slice = []string{"a", "b"}
		checkExactSameSlice(slice, normalizeScopes(slice))
		slice = []string{"a", "b", "c"}
		checkExactSameSlice(slice, normalizeScopes(slice))

		// Removes dups and sorts.
		assert.Loosely(t, normalizeScopes([]string{"b", "a"}), should.Match([]string{"a", "b"}))
		assert.Loosely(t, normalizeScopes([]string{"a", "a"}), should.Match([]string{"a"}))
		assert.Loosely(t, normalizeScopes([]string{"a", "b", "a"}), should.Match([]string{"a", "b"}))
	})
}

func TestMetadata(t *testing.T) {
	t.Parallel()

	ftt.Run("Test pack and unpack", t, func(t *ftt.Test) {
		provider := &fakeTokenProvider{
			interactive: false,
		}
		auth, ctx := newAuth(InteractiveLogin, provider, nil, "")
		cacheToken(auth, provider, &internal.Token{
			Token: oauth2.Token{
				AccessToken: "cached",
				Expiry:      future,
			},
		})

		err := auth.PackTokenMetadata(ctx, "nahi", "teri")
		assert.Loosely(t, err, should.BeNil)

		var got string
		err = auth.UnpackTokenMetadata("nahi", &got)
		assert.Loosely(t, err, should.BeNil)
		assert.That(t, got, should.Equal("teri"))
	})

	ftt.Run("Test survives refresh", t, func(t *ftt.Test) {
		provider := &fakeTokenProvider{
			interactive: false,
			tokenToRefresh: &internal.Token{
				Token: oauth2.Token{AccessToken: "refreshed", Expiry: now.Add(2 * time.Hour)},
			},
		}
		auth, ctx := newAuth(InteractiveLogin, provider, nil, "")
		cacheToken(auth, provider, &internal.Token{
			Token: oauth2.Token{
				AccessToken: "cached",
				Expiry:      now.Add(time.Hour),
			},
		})

		err := auth.PackTokenMetadata(ctx, "nahi", "teri")
		assert.Loosely(t, err, should.BeNil)

		// cached token expires
		clock.Get(ctx).(testclock.TestClock).Add(80 * time.Minute)
		tok, err := auth.GetAccessToken(time.Minute)
		assert.Loosely(t, err, should.BeNil)
		assert.That(t, tok.AccessToken, should.Equal("refreshed"))

		var got string
		err = auth.UnpackTokenMetadata("nahi", &got)
		assert.Loosely(t, err, should.BeNil)
		assert.That(t, got, should.Equal("teri"))
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
		testingCache:             &internal.MemoryTokenCache{},
		testingBaseTokenProvider: base,
		testingIAMTokenProvider:  iam,
	})
	return a, ctx
}

func cacheToken(a *Authenticator, p internal.TokenProvider, tok *internal.Token) {
	cacheKey, err := p.CacheKey(a.ctx)
	if err != nil {
		panic(err)
	}
	err = a.opts.testingCache.PutToken(cacheKey, tok)
	if err != nil {
		panic(err)
	}
}

////////////////////////////////////////////////////////////////////////////////

type fakeTokenProvider struct {
	interactive            bool
	requiresWarmup         bool
	revokedCreds           bool
	revokedToken           bool
	transientRefreshErrors int
	tokenToMint            *internal.Token
	tokenToRefresh         *internal.Token

	mintTokenCalled    bool
	refreshTokenCalled bool
	useIDTokens        bool

	baseTokenInMint    *internal.Token
	baseTokenInRefresh *internal.Token

	emailUnimplemented bool
	knownEmail         string
}

func (p *fakeTokenProvider) RequiresInteraction() bool {
	return p.interactive
}

func (p *fakeTokenProvider) RequiresWarmup() bool {
	return p.requiresWarmup
}

func (p *fakeTokenProvider) MemoryCacheOnly() bool {
	return true
}

func (p *fakeTokenProvider) Email() (string, error) {
	if p.emailUnimplemented {
		return "", internal.ErrUnimplementedEmail
	}
	switch p.knownEmail {
	case "":
		return "", internal.ErrUnknownEmail
	case internal.NoEmail:
		return "", internal.ErrNoEmail
	default:
		return p.knownEmail, nil
	}
}

func (p *fakeTokenProvider) CacheKey(ctx context.Context) (*internal.CacheKey, error) {
	return &internal.CacheKey{Key: "fake"}, nil
}

func (p *fakeTokenProvider) MintToken(ctx context.Context, base *internal.Token) (*internal.Token, error) {
	p.mintTokenCalled = true
	p.baseTokenInMint = base
	if p.revokedCreds {
		return nil, internal.ErrBadCredentials
	}
	if p.tokenToMint != nil {
		return p.tokenToMint, nil
	}
	idTok := internal.NoIDToken
	accessTok := internal.NoAccessToken
	if p.useIDTokens {
		idTok = "some minted ID token"
	} else {
		accessTok = "some minted access token"
	}
	return &internal.Token{
		Token:   oauth2.Token{AccessToken: accessTok},
		IDToken: idTok,
		Email:   "some-email-minttoken@example.com",
	}, nil
}

func (p *fakeTokenProvider) RefreshToken(ctx context.Context, prev, base *internal.Token) (*internal.Token, error) {
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
	idTok := internal.NoIDToken
	accessTok := internal.NoAccessToken
	if p.useIDTokens {
		idTok = "some refreshed ID token"
	} else {
		accessTok = "some refreshed access token"
	}
	return &internal.Token{
		Token:   oauth2.Token{AccessToken: accessTok},
		IDToken: idTok,
		Email:   "some-email-refreshtoken@example.com",
	}, nil
}
