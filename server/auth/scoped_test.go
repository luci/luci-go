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
	"math/rand"
	"testing"
	"time"

	"go.chromium.org/luci/common/proto/google"
	"golang.org/x/oauth2"
	"google.golang.org/grpc"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/tokenserver/api/minter/v1"

	. "github.com/smartystreets/goconvey/convey"
)

type scopedTokenMinterMock struct {
	request  minter.MintServiceOAuthTokenRequest
	response minter.MintServiceOAuthTokenResponse
	err      error
}

func (m *scopedTokenMinterMock) MintServiceOAuthToken(ctx context.Context, in *minter.MintServiceOAuthTokenRequest, opts ...grpc.CallOption) (*minter.MintServiceOAuthTokenResponse, error) {
	m.request = *in
	if m.err != nil {
		return nil, m.err
	}
	return &m.response, nil
}

func TestMintServiceOAuthToken(t *testing.T) {
	t.Parallel()

	Convey("MintServiceOAuthToken works", t, func() {
		ctx := context.Background()
		ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeUTC)
		ctx = mathrand.Set(ctx, rand.New(rand.NewSource(12345)))
		ctx = caching.WithEmptyProcessCache(ctx)
		ctx = Initialize(ctx, &Config{})

		mockedClient := &scopedTokenMinterMock{
			response: minter.MintServiceOAuthTokenResponse{
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
				MinTTL:      time.Hour,
				rpcClient:   mockedClient,
				LuciProject: "infra",
				OAuthScopes: defaultOAuthScopes,
			})
			So(err, ShouldBeNil)
			So(tok, ShouldResemble, &oauth2.Token{
				AccessToken: "tok",
				TokenType:   "Bearer",
				Expiry:      testclock.TestRecentTimeUTC.Add(MaxScopedTokenTTL).Truncate(time.Second),
			})
			So(mockedClient.request, ShouldResemble, minter.MintServiceOAuthTokenRequest{
				LuciProject:         "infra",
				OauthScope:          defaultOAuthScopes,
				MinValidityDuration: 10800,
			})

			// Cached now.
			So(scopedTokenCache.lc.ProcessLRUCache.LRU(ctx).Len(), ShouldEqual, 1)

			// On subsequence request the cached token is used.
			mockedClient.response.AccessToken = "another token"
			tok, err = MintScopedToken(ctx, ScopedTokenParams{
				MinTTL:      time.Hour,
				rpcClient:   mockedClient,
				LuciProject: "infra",
				OAuthScopes: defaultOAuthScopes,
			})
			So(err, ShouldBeNil)
			So(tok.AccessToken, ShouldResemble, "tok") // old one

			// Unless it expires sooner than requested TTL.
			rollTimeForward := MaxDelegationTokenTTL - 30*time.Minute
			clock.Get(ctx).(testclock.TestClock).Add(rollTimeForward)
			mockedClient.response.Expiry = google.NewTimestamp(clock.Now(ctx).Add(MaxScopedTokenTTL))

			tok, err = MintScopedToken(ctx, ScopedTokenParams{
				MinTTL:      time.Hour,
				rpcClient:   mockedClient,
				LuciProject: "infra",
				OAuthScopes: defaultOAuthScopes,
			})
			So(err, ShouldBeNil)
			So(tok.AccessToken, ShouldResemble, "another token") // new one
		})

	})
}
