// Copyright 2017 The LUCI Authors.
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

package internal

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/oauth2"
	"google.golang.org/api/googleapi"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/gcloud/iam"
	"go.chromium.org/luci/common/retry/transient"
)

type iamTokenProvider struct {
	actAs     string
	scopes    []string
	audience  string // not empty iff using ID tokens
	transport http.RoundTripper
	cacheKey  CacheKey
}

// NewIAMTokenProvider returns TokenProvider that uses generateAccessToken IAM
// API to grab tokens belonging to some service account.
func NewIAMTokenProvider(ctx context.Context, actAs string, scopes []string, audience string, transport http.RoundTripper) (TokenProvider, error) {
	return &iamTokenProvider{
		actAs:     actAs,
		scopes:    scopes,
		audience:  audience,
		transport: transport,
		cacheKey: CacheKey{
			Key:    fmt.Sprintf("iam/%s", actAs),
			Scopes: scopes,
		},
	}, nil
}

func (p *iamTokenProvider) RequiresInteraction() bool {
	return false
}

func (p *iamTokenProvider) RequiresWarmup() bool {
	return false
}

func (p *iamTokenProvider) MemoryCacheOnly() bool {
	return false
}

func (p *iamTokenProvider) Email() (string, error) {
	return p.actAs, nil
}

func (p *iamTokenProvider) CacheKey(ctx context.Context) (*CacheKey, error) {
	return &p.cacheKey, nil
}

func (p *iamTokenProvider) MintToken(ctx context.Context, base *Token) (*Token, error) {
	client := &iam.CredentialsClient{
		Client: &http.Client{
			Transport: &tokenInjectingTransport{
				transport: p.transport,
				token:     &base.Token,
			},
		},
	}

	var err error

	if p.audience != "" {
		var tok string
		tok, err = client.GenerateIDToken(ctx, p.actAs, p.audience, true, nil)
		if err == nil {
			claims, err := ParseIDTokenClaims(tok)
			if err != nil {
				return nil, errors.Annotate(err, "IAM service returned bad ID token").Err()
			}
			return &Token{
				Token: oauth2.Token{
					TokenType:   "Bearer",
					AccessToken: NoAccessToken,
					Expiry:      time.Unix(claims.Exp, 0),
				},
				IDToken: tok,
				Email:   p.actAs,
			}, nil
		}
	} else {
		var tok *oauth2.Token
		tok, err = client.GenerateAccessToken(ctx, p.actAs, p.scopes, nil, 0)
		if err == nil {
			return &Token{
				Token:   *tok,
				IDToken: NoIDToken,
				Email:   p.actAs,
			}, nil
		}
	}

	// Any 4** HTTP response is a fatal error. Everything else is transient.
	if apiErr, _ := err.(*googleapi.Error); apiErr != nil && apiErr.Code < 500 {
		return nil, err
	}
	return nil, transient.Tag.Apply(err)
}

func (p *iamTokenProvider) RefreshToken(ctx context.Context, prev, base *Token) (*Token, error) {
	// Service account tokens are self sufficient, there's no need for refresh
	// token. Minting a token and "refreshing" it is a same thing.
	return p.MintToken(ctx, base)
}

////////////////////////////////////////////////////////////////////////////////

type tokenInjectingTransport struct {
	transport http.RoundTripper
	token     *oauth2.Token
}

func (t *tokenInjectingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := *req
	clone.Header = make(http.Header, len(req.Header)+1)
	for k, v := range req.Header {
		clone.Header[k] = v
	}
	t.token.SetAuthHeader(&clone)
	return t.transport.RoundTrip(&clone)
}
