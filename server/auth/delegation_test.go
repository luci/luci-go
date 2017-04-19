// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package auth

import (
	"math/rand"
	"testing"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/grpc"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/data/rand/mathrand"
	"github.com/luci/luci-go/server/auth/delegation"
	"github.com/luci/luci-go/server/auth/delegation/messages"
	"github.com/luci/luci-go/tokenserver/api/minter/v1"

	. "github.com/smartystreets/goconvey/convey"
)

type tokenMinterMock struct {
	request  minter.MintDelegationTokenRequest
	response minter.MintDelegationTokenResponse
	err      error
}

func (m *tokenMinterMock) MintMachineToken(context.Context, *minter.MintMachineTokenRequest, ...grpc.CallOption) (*minter.MintMachineTokenResponse, error) {
	panic("not implemented")
}

func (m *tokenMinterMock) MintDelegationToken(ctx context.Context, in *minter.MintDelegationTokenRequest, opts ...grpc.CallOption) (*minter.MintDelegationTokenResponse, error) {
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
		ctx = mathrand.Set(ctx, rand.New(rand.NewSource(12345)))

		mockedClient := &tokenMinterMock{
			response: minter.MintDelegationTokenResponse{
				Token: "tok",
				DelegationSubtoken: &messages.Subtoken{
					Kind:             messages.Subtoken_BEARER_DELEGATION_TOKEN,
					ValidityDuration: int32(MaxDelegationTokenTTL.Seconds()),
				},
			},
		}

		// Create an LRU large enough that it will never cycle during test.
		tokenCache := MemoryCache(1024)

		ctx = ModifyConfig(ctx, func(cfg Config) Config {
			cfg.Cache = tokenCache
			return cfg
		})

		ctx = WithState(ctx, &state{
			user: &User{Identity: "user:abc@example.com"},
			db:   &fakeDB{tokenServiceURL: "https://tokens.example.com"},
		})

		Convey("Works (including caching)", func(c C) {
			tok, err := MintDelegationToken(ctx, DelegationTokenParams{
				TargetHost: "hostname.example.com",
				MinTTL:     time.Hour,
				Intent:     "intent",
				rpcClient:  mockedClient,
			})
			So(err, ShouldBeNil)
			So(tok, ShouldResemble, &delegation.Token{
				Token:  "tok",
				Expiry: testclock.TestRecentTimeUTC.Add(MaxDelegationTokenTTL),
			})
			So(mockedClient.request, ShouldResemble, minter.MintDelegationTokenRequest{
				DelegatedIdentity: "user:abc@example.com",
				ValidityDuration:  10800,
				Audience:          []string{"REQUESTOR"},
				Services:          []string{"https://hostname.example.com"},
				Intent:            "intent",
			})

			// Cached now.
			So(tokenCache.(*memoryCache).cache.Len(), ShouldEqual, 1)
			v, _ := tokenCache.Get(ctx, "delegation/3/dL9oZrnNLCIxyUBBaX3eGKAwTbA")
			So(v, ShouldNotBeNil)

			// On subsequence request the cached token is used.
			mockedClient.response.Token = "another token"
			tok, err = MintDelegationToken(ctx, DelegationTokenParams{
				TargetHost: "hostname.example.com",
				MinTTL:     time.Hour,
				Intent:     "intent",
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
			So(tok, ShouldResemble, &delegation.Token{
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
