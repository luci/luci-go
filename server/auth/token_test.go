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

package auth

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"go.chromium.org/luci/common/proto/google"
	"golang.org/x/oauth2"
	"google.golang.org/grpc"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/jsontime"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/server/auth/delegation"
	"go.chromium.org/luci/server/auth/delegation/messages"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/tokenserver/api/minter/v1"

	. "github.com/smartystreets/goconvey/convey"
)

type tokenMinterMock struct {
	delegationRequest    minter.MintDelegationTokenRequest
	delegationResponse   minter.MintDelegationTokenResponse
	serviceTokenRequest  minter.MintServiceOAuthTokenRequest
	serviceTokenResponse minter.MintServiceOAuthTokenResponse
	err                  error
}

func (m *tokenMinterMock) MintDelegationToken(ctx context.Context, in *minter.MintDelegationTokenRequest, opts ...grpc.CallOption) (*minter.MintDelegationTokenResponse, error) {
	m.delegationRequest = *in
	if m.err != nil {
		return nil, m.err
	}
	return &m.delegationResponse, nil
}

func (m *tokenMinterMock) MintServiceOAuthToken(ctx context.Context, in *minter.MintServiceOAuthTokenRequest, opts ...grpc.CallOption) (*minter.MintServiceOAuthTokenResponse, error) {
	m.serviceTokenRequest = *in
	if m.err != nil {
		return nil, m.err
	}
	return &m.serviceTokenResponse, nil
}

func TestMintServiceOAuthToken(t *testing.T) {
	t.Parallel()

	Convey("MintServiceOAuthToken works", t, func() {
		ctx := context.Background()
		ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeUTC)
		ctx = mathrand.Set(ctx, rand.New(rand.NewSource(12345)))
		ctx = caching.WithEmptyProcessCache(ctx)
		ctx = SetConfig(ctx, &Config{})

		mockedClient := &tokenMinterMock{
			serviceTokenResponse: minter.MintServiceOAuthTokenResponse{
				ServiceAccount: "foobarserviceaccount",
				AccessToken:    "tok",
				Expiry:         google.NewTimestamp(clock.Now(ctx).Add(MaxScopedTokenTTL)),
			},
		}

		ctx = WithState(ctx, &state{
			user: &User{Identity: "user:abc@example.com"},
			db:   &fakeDB{tokenServiceURL: "https://tokens.example.com"},
		})

		Convey("Works (including caching)", func(c C) {
			tok, err := MintScopedToken(ctx, ScopedTokenParams{
				TokenParams: TokenParams{
					TargetHost: "hostname.example.com",
					MinTTL:     time.Hour,
					Intent:     "intent",
					rpcClient:  mockedClient,
				},
				LuciProject: "infra",
				OAuthScopes: defaultOAuthScopes,
			})
			So(err, ShouldBeNil)
			So(tok, ShouldResemble, &oauth2.Token{
				AccessToken: "tok",
				TokenType:   "Bearer",
				Expiry:      testclock.TestRecentTimeUTC.Add(MaxScopedTokenTTL).Truncate(time.Second),
			})
			So(mockedClient.serviceTokenRequest, ShouldResemble, minter.MintServiceOAuthTokenRequest{
				LuciProject:         "infra",
				OauthScope:          defaultOAuthScopes,
				MinValidityDuration: 10800,
			})

			// Cached now.
			So(scopedTokenCache.lc.ProcessLRUCache.LRU(ctx).Len(), ShouldEqual, 1)

			// On subsequence request the cached token is used.
			mockedClient.serviceTokenResponse.AccessToken = "another token"
			tok, err = MintScopedToken(ctx, ScopedTokenParams{
				TokenParams: TokenParams{
					TargetHost: "hostname.example.com",
					MinTTL:     time.Hour,
					Intent:     "intent",
					rpcClient:  mockedClient,
				},
				LuciProject: "infra",
				OAuthScopes: defaultOAuthScopes,
			})
			So(err, ShouldBeNil)
			So(tok.AccessToken, ShouldResemble, "tok") // old one

			// Unless it expires sooner than requested TTL.
			rollTimeForward := MaxDelegationTokenTTL - 30*time.Minute
			clock.Get(ctx).(testclock.TestClock).Add(rollTimeForward)
			mockedClient.serviceTokenResponse.Expiry = google.NewTimestamp(clock.Now(ctx).Add(MaxScopedTokenTTL))

			tok, err = MintScopedToken(ctx, ScopedTokenParams{
				TokenParams: TokenParams{
					TargetHost: "hostname.example.com",
					MinTTL:     time.Hour,
					Intent:     "intent",
					rpcClient:  mockedClient,
				},
				LuciProject: "infra",
				OAuthScopes: defaultOAuthScopes,
			})
			So(err, ShouldBeNil)
			So(tok.AccessToken, ShouldResemble, "another token") // new one
		})

	})
}

func TestMintDelegationToken(t *testing.T) {
	t.Parallel()

	Convey("MintDelegationToken works", t, func() {
		ctx := context.Background()
		ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeUTC)
		ctx = mathrand.Set(ctx, rand.New(rand.NewSource(12345)))
		ctx = caching.WithEmptyProcessCache(ctx)
		ctx = SetConfig(ctx, &Config{})

		mockedClient := &tokenMinterMock{
			delegationResponse: minter.MintDelegationTokenResponse{
				Token: "tok",
				DelegationSubtoken: &messages.Subtoken{
					Kind:             messages.Subtoken_BEARER_DELEGATION_TOKEN,
					ValidityDuration: int32(MaxDelegationTokenTTL.Seconds()),
				},
			},
		}

		ctx = WithState(ctx, &state{
			user: &User{Identity: "user:abc@example.com"},
			db:   &fakeDB{tokenServiceURL: "https://tokens.example.com"},
		})

		Convey("Works (including caching)", func(c C) {
			tok, err := MintDelegationToken(ctx, DelegationTokenParams{
				TokenParams{
					TargetHost: "hostname.example.com",
					MinTTL:     time.Hour,
					Tags:       []string{"c:d", "a:b"},
					Intent:     "intent",
					rpcClient:  mockedClient,
				},
			})
			So(err, ShouldBeNil)
			So(tok, ShouldResemble, &delegation.Token{
				Token:  "tok",
				Expiry: jsontime.Time{testclock.TestRecentTimeUTC.Add(MaxDelegationTokenTTL)},
			})
			So(mockedClient.delegationRequest, ShouldResemble, minter.MintDelegationTokenRequest{
				DelegatedIdentity: "user:abc@example.com",
				ValidityDuration:  10800,
				Audience:          []string{"REQUESTOR"},
				Services:          []string{"https://hostname.example.com"},
				Intent:            "intent",
				Tags:              []string{"a:b", "c:d"},
			})

			// Cached now.
			So(delegationTokenCache.lc.ProcessLRUCache.LRU(ctx).Len(), ShouldEqual, 1)

			// On subsequence request the cached token is used.
			mockedClient.delegationResponse.Token = "another token"
			tok, err = MintDelegationToken(ctx, DelegationTokenParams{
				TokenParams{
					TargetHost: "hostname.example.com",
					MinTTL:     time.Hour,
					Intent:     "intent",
					Tags:       []string{"c:d", "a:b"},
					rpcClient:  mockedClient,
				},
			})
			So(err, ShouldBeNil)
			So(tok.Token, ShouldResemble, "tok") // old one

			// Unless it expires sooner than requested TTL.
			clock.Get(ctx).(testclock.TestClock).Add(MaxDelegationTokenTTL - 30*time.Minute)
			tok, err = MintDelegationToken(ctx, DelegationTokenParams{
				TokenParams{
					TargetHost: "hostname.example.com",
					MinTTL:     time.Hour,
					Intent:     "intent",
					Tags:       []string{"c:d", "a:b"},
					rpcClient:  mockedClient,
				},
			})
			So(err, ShouldBeNil)
			So(tok.Token, ShouldResemble, "another token") // new one
		})

		Convey("Untargeted token works", func(c C) {
			tok, err := MintDelegationToken(ctx, DelegationTokenParams{
				TokenParams{
					Untargeted: true,
					MinTTL:     time.Hour,
					Intent:     "intent",
					rpcClient:  mockedClient,
				},
			})
			So(err, ShouldBeNil)
			So(tok, ShouldResemble, &delegation.Token{
				Token:  "tok",
				Expiry: jsontime.Time{testclock.TestRecentTimeUTC.Add(MaxDelegationTokenTTL)},
			})
			So(mockedClient.delegationRequest, ShouldResemble, minter.MintDelegationTokenRequest{
				DelegatedIdentity: "user:abc@example.com",
				ValidityDuration:  10800,
				Audience:          []string{"REQUESTOR"},
				Services:          []string{"*"},
				Intent:            "intent",
			})
		})
	})
}
