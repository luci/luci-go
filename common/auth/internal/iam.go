// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package internal

import (
	"fmt"
	"net/http"

	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"google.golang.org/api/googleapi"

	"github.com/luci/luci-go/common/gcloud/googleoauth"
	"github.com/luci/luci-go/common/gcloud/iam"
	"github.com/luci/luci-go/common/retry/transient"
)

type iamTokenProvider struct {
	actAs     string
	scopes    []string
	transport http.RoundTripper
	cacheKey  CacheKey
}

// NewIAMTokenProvider returns TokenProvider that uses SignBlob IAM API to
// sign assertions on behalf of some service account.
func NewIAMTokenProvider(ctx context.Context, actAs string, scopes []string, transport http.RoundTripper) (TokenProvider, error) {
	return &iamTokenProvider{
		actAs:     actAs,
		scopes:    scopes,
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

func (p *iamTokenProvider) Lightweight() bool {
	return false
}

func (p *iamTokenProvider) CacheKey(ctx context.Context) (*CacheKey, error) {
	return &p.cacheKey, nil
}

func (p *iamTokenProvider) MintToken(ctx context.Context, base *oauth2.Token) (*oauth2.Token, error) {
	tok, err := googleoauth.GetAccessToken(ctx, googleoauth.JwtFlowParams{
		ServiceAccount: p.actAs,
		Scopes:         p.scopes,
		Signer: &iam.Client{
			Client: &http.Client{
				Transport: &tokenInjectingTransport{
					transport: p.transport,
					token:     base,
				},
			},
		},
	})
	if err == nil {
		return tok, nil
	}
	// Any 4** HTTP response is a fatal error. Everything else is transient.
	if apiErr, _ := err.(*googleapi.Error); apiErr != nil && apiErr.Code < 500 {
		return nil, err
	}
	return nil, transient.Tag.Apply(err)
}

func (p *iamTokenProvider) RefreshToken(ctx context.Context, prev, base *oauth2.Token) (*oauth2.Token, error) {
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
