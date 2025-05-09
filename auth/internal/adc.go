// Copyright 2025 The LUCI Authors.
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
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

type adcTokenProvider struct {
	ts       oauth2.TokenSource
	cacheKey CacheKey // used only for in-memory cache
}

// NewGoogleADCTokenProvider returns TokenProvider that uses Google ADC.
func NewGoogleADCTokenProvider(ctx context.Context, scopes []string) (TokenProvider, error) {
	creds, err := google.FindDefaultCredentialsWithParams(ctx, google.CredentialsParams{
		Scopes: scopes,
		// We cache token already on a higher level. Set this EarlyTokenRefresh to
		// a big value to make sure that whenever we ask for a token, we get a new
		// one (instead of potentially cached one).
		EarlyTokenRefresh: 10 * time.Minute,
	})
	if err != nil {
		return nil, err
	}
	return &adcTokenProvider{
		ts: creds.TokenSource,
		cacheKey: CacheKey{
			Key:    "adc",
			Scopes: scopes,
		},
	}, nil
}

func (p *adcTokenProvider) RequiresInteraction() bool {
	return false
}

func (p *adcTokenProvider) RequiresWarmup() bool {
	return false
}

func (p *adcTokenProvider) MemoryCacheOnly() bool {
	return true
}

func (p *adcTokenProvider) Email() (string, error) {
	return "", ErrUnimplementedEmail
}

func (p *adcTokenProvider) CacheKey(ctx context.Context) (*CacheKey, error) {
	return &p.cacheKey, nil
}

func (p *adcTokenProvider) MintToken(ctx context.Context, _ *Token) (*Token, error) {
	tok, err := p.ts.Token()
	if err != nil {
		return nil, err
	}
	return &Token{
		Token:   *tok,
		IDToken: NoIDToken,
		Email:   UnknownEmail,
	}, nil
}

func (p *adcTokenProvider) RefreshToken(ctx context.Context, _, _ *Token) (*Token, error) {
	return p.MintToken(ctx, nil)
}
