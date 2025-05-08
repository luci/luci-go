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

//go:build !copybara
// +build !copybara

package internal

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/oauth2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/tokenserver/api/minter/v1"
)

// requestedMinValidityDuration defines a range of expiration durations of
// tokens returned by luciTSTokenProvider.
//
// They expire within [now+requestedMinValidityDuration, now+1h) interval.
//
// If requestedMinValidityDuration is small, there's a greater cache hit ratio
// on the Token Server side, but lazy luci-auth users that just grab a single
// token and try to reuse it for e.g. 50 min without refreshing will suffer.
//
// If it is large such lazy abuse would work, but the Token Server will be
// forced to call Cloud IAM API each time MintServiceAccountToken is called,
// threatening to hit a quota on this call.
//
// We pick a value somewhere in the middle. Note that for better UX it must be
// larger than a maximum allowed `-lifetime` parameter in `luci-auth token`,
// which is currently 30 min.
const requestedMinValidityDuration = 35 * time.Minute

type luciTSTokenProvider struct {
	host      string
	actAs     string
	realm     string
	scopes    []string
	audience  string // not empty iff using ID tokens
	transport http.RoundTripper
	cacheKey  CacheKey
}

func init() {
	NewLUCITSTokenProvider = func(ctx context.Context, host, actAs, realm string, scopes []string, audience string, transport http.RoundTripper) (TokenProvider, error) {
		return &luciTSTokenProvider{
			host:      host,
			actAs:     actAs,
			realm:     realm,
			scopes:    scopes,
			audience:  audience,
			transport: transport,
			cacheKey: CacheKey{
				Key:    fmt.Sprintf("luci_ts/%s/%s/%s", actAs, host, realm),
				Scopes: scopes,
			},
		}, nil
	}
}

func (p *luciTSTokenProvider) RequiresInteraction() bool {
	return false
}

func (p *luciTSTokenProvider) MemoryCacheOnly() bool {
	return false
}

func (p *luciTSTokenProvider) Email() string {
	return p.actAs
}

func (p *luciTSTokenProvider) CacheKey(ctx context.Context) (*CacheKey, error) {
	return &p.cacheKey, nil
}

func (p *luciTSTokenProvider) MintToken(ctx context.Context, base *Token) (*Token, error) {
	client := minter.NewTokenMinterClient(&prpc.Client{
		C: &http.Client{
			Transport: &tokenInjectingTransport{
				transport: p.transport,
				token:     &base.Token,
			},
		},
		Host: p.host,
		Options: &prpc.Options{
			Retry:         nil,              // the caller of MintToken retries itself
			PerRPCTimeout: 30 * time.Second, // the call should be relatively fast
		},
	})

	// TODO(crbug.com/1179629): pRPC doesn't handle outgoing meatadata well.
	ctx = metadata.NewOutgoingContext(ctx, nil)

	req := &minter.MintServiceAccountTokenRequest{
		ServiceAccount:      p.actAs,
		Realm:               p.realm,
		MinValidityDuration: int64(requestedMinValidityDuration / time.Second),
	}
	if p.audience != "" {
		req.TokenKind = minter.ServiceAccountTokenKind_SERVICE_ACCOUNT_TOKEN_ID_TOKEN
		req.IdTokenAudience = p.audience
	} else {
		req.TokenKind = minter.ServiceAccountTokenKind_SERVICE_ACCOUNT_TOKEN_ACCESS_TOKEN
		req.OauthScope = p.scopes
	}

	resp, err := client.MintServiceAccountToken(ctx, req)
	if err != nil {
		// Want to retry on per-RPC deadlines.
		if status.Code(err) == codes.DeadlineExceeded {
			return nil, transient.Tag.Apply(err)
		}
		// And also on standard retriable errors like Unavailable.
		return nil, grpcutil.WrapIfTransient(err)
	}

	accessToken := NoAccessToken
	idToken := NoIDToken
	if p.audience != "" {
		idToken = resp.Token
	} else {
		accessToken = resp.Token
	}

	return &Token{
		Token: oauth2.Token{
			AccessToken: accessToken,
			Expiry:      resp.Expiry.AsTime(),
			TokenType:   "Bearer",
		},
		IDToken: idToken,
		Email:   p.Email(),
	}, nil
}

func (p *luciTSTokenProvider) RefreshToken(ctx context.Context, prev, base *Token) (*Token, error) {
	return p.MintToken(ctx, base)
}
