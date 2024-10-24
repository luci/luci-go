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

package auth

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/tokenserver/api/minter/v1"

	"go.chromium.org/luci/server/caching"
)

type scopedTokenMinterMock struct {
	request  minter.MintProjectTokenRequest
	response minter.MintProjectTokenResponse
	err      error
}

func (m *scopedTokenMinterMock) MintProjectToken(ctx context.Context, in *minter.MintProjectTokenRequest, opts ...grpc.CallOption) (*minter.MintProjectTokenResponse, error) {
	m.request = *in
	if m.err != nil {
		return nil, m.err
	}
	return &m.response, nil
}

func TestMintServiceOAuthToken(t *testing.T) {
	t.Parallel()

	ftt.Run("MintProjectToken works", t, func(t *ftt.Test) {
		ctx := context.Background()
		ctx, tc := testclock.UseTime(ctx, testclock.TestRecentTimeUTC)
		ctx = caching.WithEmptyProcessCache(ctx)
		ctx = Initialize(ctx, &Config{})

		mockedClient := &scopedTokenMinterMock{
			response: minter.MintProjectTokenResponse{
				ServiceAccountEmail: "foobarserviceaccount",
				AccessToken:         "tok",
				Expiry:              timestamppb.New(clock.Now(ctx).Add(MaxScopedTokenTTL)),
			},
		}

		ctx = WithState(ctx, &state{
			user: &User{Identity: "user:abc@example.com"},
			db:   &fakeDB{tokenServiceURL: "https://tokens.example.com"},
		})

		t.Run("Works (including caching)", func(t *ftt.Test) {
			tok, err := MintProjectToken(ctx, ProjectTokenParams{
				MinTTL:      10 * time.Minute,
				rpcClient:   mockedClient,
				LuciProject: "infra",
				OAuthScopes: defaultOAuthScopes,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tok, should.Resemble(&Token{
				Token:  "tok",
				Expiry: testclock.TestRecentTimeUTC.Add(MaxScopedTokenTTL).Truncate(time.Second),
			}))
			assert.Loosely(t, mockedClient.request, should.Resemble(minter.MintProjectTokenRequest{
				LuciProject:         "infra",
				OauthScope:          defaultOAuthScopes,
				MinValidityDuration: 900,
			}))

			// Cached now.
			assert.Loosely(t, scopedTokenCache.lc.CachedLocally(ctx), should.Equal(1))

			// On subsequence request the cached token is used.
			mockedClient.response.AccessToken = "another token"
			tok, err = MintProjectToken(ctx, ProjectTokenParams{
				MinTTL:      10 * time.Minute,
				rpcClient:   mockedClient,
				LuciProject: "infra",
				OAuthScopes: defaultOAuthScopes,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tok.Token, should.Match("tok")) // old one

			// Unless it expires sooner than requested TTL.
			rollTimeForward := MaxDelegationTokenTTL - 30*time.Minute
			clock.Get(ctx).(testclock.TestClock).Add(rollTimeForward)
			mockedClient.response.Expiry = timestamppb.New(clock.Now(ctx).Add(MaxScopedTokenTTL))

			tok, err = MintProjectToken(ctx, ProjectTokenParams{
				MinTTL:      10 * time.Minute,
				rpcClient:   mockedClient,
				LuciProject: "infra",
				OAuthScopes: defaultOAuthScopes,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tok.Token, should.Match("another token")) // new one
		})

		t.Run("Project scoped fallback works (including caching)", func(t *ftt.Test) {
			mockedClient = &scopedTokenMinterMock{
				response: minter.MintProjectTokenResponse{},
				err:      status.Errorf(codes.NotFound, "unable to find project identity for project"),
			}

			tok, err := MintProjectToken(ctx, ProjectTokenParams{
				MinTTL:      4 * time.Minute,
				rpcClient:   mockedClient,
				LuciProject: "infra",
				OAuthScopes: defaultOAuthScopes,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tok, should.BeNil)

			// On subsequence request the cached token is used.
			mockedClient.response = minter.MintProjectTokenResponse{
				ServiceAccountEmail: "foobarserviceaccount",
				AccessToken:         "tok",
				Expiry:              timestamppb.New(clock.Now(ctx).Add(MaxScopedTokenTTL)),
			}
			mockedClient.err = nil
			tok, err = MintProjectToken(ctx, ProjectTokenParams{
				MinTTL:      4 * time.Minute,
				rpcClient:   mockedClient,
				LuciProject: "infra",
				OAuthScopes: defaultOAuthScopes,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tok, should.BeNil)

			// However requesting for another project produces a different result
			mockedClient.response = minter.MintProjectTokenResponse{
				ServiceAccountEmail: "foobarserviceaccount",
				AccessToken:         "tok",
				Expiry:              timestamppb.New(clock.Now(ctx).Add(MaxScopedTokenTTL)),
			}
			mockedClient.err = nil
			tok, err = MintProjectToken(ctx, ProjectTokenParams{
				MinTTL:      4 * time.Minute,
				rpcClient:   mockedClient,
				LuciProject: "infra-experimental",
				OAuthScopes: defaultOAuthScopes,
			})

			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tok, should.NotBeNil)

			// Simulate cache expiry, check that a new token attempt is sent out
			tc.Add(5 * time.Minute)
			tok, err = MintProjectToken(ctx, ProjectTokenParams{
				MinTTL:      4 * time.Minute,
				rpcClient:   mockedClient,
				LuciProject: "infra",
				OAuthScopes: defaultOAuthScopes,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tok.Token, should.Match("tok"))
		})
	})
}
