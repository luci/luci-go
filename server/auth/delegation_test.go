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
	"testing"
	"time"

	"google.golang.org/grpc"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/tokenserver/api/minter/v1"

	"go.chromium.org/luci/server/auth/delegation/messages"
	"go.chromium.org/luci/server/caching"

	. "github.com/smartystreets/goconvey/convey"
)

type delegationTokenMinterMock struct {
	request  minter.MintDelegationTokenRequest
	response minter.MintDelegationTokenResponse
	err      error
}

func (m *delegationTokenMinterMock) MintDelegationToken(ctx context.Context, in *minter.MintDelegationTokenRequest, opts ...grpc.CallOption) (*minter.MintDelegationTokenResponse, error) {
	m.request = *in
	if m.err != nil {
		return nil, m.err
	}
	return &m.response, nil
}

func TestMintDelegationToken(t *testing.T) {
	t.Parallel()

	Convey("MintDelegationToken works", t, func() {
		ctx := context.Background()
		ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeUTC)
		ctx = caching.WithEmptyProcessCache(ctx)
		ctx = Initialize(ctx, &Config{})

		mockedClient := &delegationTokenMinterMock{
			response: minter.MintDelegationTokenResponse{
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
				TargetHost: "hostname.example.com",
				MinTTL:     time.Hour,
				Tags:       []string{"c:d", "a:b"},
				Intent:     "intent",
				rpcClient:  mockedClient,
			})
			So(err, ShouldBeNil)
			So(tok, ShouldResemble, &Token{
				Token:  "tok",
				Expiry: testclock.TestRecentTimeUTC.Add(MaxDelegationTokenTTL),
			})
			So(mockedClient.request, ShouldResemble, minter.MintDelegationTokenRequest{
				DelegatedIdentity: "user:abc@example.com",
				ValidityDuration:  10800,
				Audience:          []string{"REQUESTOR"},
				Services:          []string{"https://hostname.example.com"},
				Intent:            "intent",
				Tags:              []string{"a:b", "c:d"},
			})

			// Cached now.
			So(delegationTokenCache.lc.CachedLocally(ctx), ShouldEqual, 1)

			// On subsequence request the cached token is used.
			mockedClient.response.Token = "another token"
			tok, err = MintDelegationToken(ctx, DelegationTokenParams{
				TargetHost: "hostname.example.com",
				MinTTL:     time.Hour,
				Intent:     "intent",
				Tags:       []string{"c:d", "a:b"},
				rpcClient:  mockedClient,
			})
			So(err, ShouldBeNil)
			So(tok.Token, ShouldResemble, "tok") // old one

			// Unless it expires sooner than requested TTL.
			clock.Get(ctx).(testclock.TestClock).Add(MaxDelegationTokenTTL - 30*time.Minute)
			tok, err = MintDelegationToken(ctx, DelegationTokenParams{
				TargetHost: "hostname.example.com",
				MinTTL:     time.Hour,
				Intent:     "intent",
				Tags:       []string{"c:d", "a:b"},
				rpcClient:  mockedClient,
			})
			So(err, ShouldBeNil)
			So(tok.Token, ShouldResemble, "another token") // new one
		})

		Convey("Untargeted token works", func(c C) {
			tok, err := MintDelegationToken(ctx, DelegationTokenParams{
				Untargeted: true,
				MinTTL:     time.Hour,
				Intent:     "intent",
				rpcClient:  mockedClient,
			})
			So(err, ShouldBeNil)
			So(tok, ShouldResemble, &Token{
				Token:  "tok",
				Expiry: testclock.TestRecentTimeUTC.Add(MaxDelegationTokenTTL),
			})
			So(mockedClient.request, ShouldResemble, minter.MintDelegationTokenRequest{
				DelegatedIdentity: "user:abc@example.com",
				ValidityDuration:  10800,
				Audience:          []string{"REQUESTOR"},
				Services:          []string{"*"},
				Intent:            "intent",
			})
		})
	})
}
