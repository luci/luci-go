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

package internal

import (
	"fmt"

	"cloud.google.com/go/compute/metadata"
	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/logging"
)

type gceTokenProvider struct {
	account  string
	cacheKey CacheKey
}

// NewGCETokenProvider returns TokenProvider that knows how to use GCE metadata server.
func NewGCETokenProvider(ctx context.Context, account string, scopes []string) (TokenProvider, error) {
	// Ensure account has requested scopes.
	availableScopes, err := metadata.Scopes(account)
	if err != nil {
		return nil, err
	}
	availableSet := stringset.NewFromSlice(availableScopes...)
	for _, requested := range scopes {
		if !availableSet.Has(requested) {
			logging.Warningf(ctx, "GCE service account %q doesn't have required scope %q", account, requested)
			return nil, ErrInsufficientAccess
		}
	}
	return &gceTokenProvider{
		account: account,
		cacheKey: CacheKey{
			Key:    fmt.Sprintf("gce/%s", account),
			Scopes: scopes,
		},
	}, nil
}

func (p *gceTokenProvider) RequiresInteraction() bool {
	return false
}

func (p *gceTokenProvider) Lightweight() bool {
	return true
}

func (p *gceTokenProvider) CacheKey(ctx context.Context) (*CacheKey, error) {
	return &p.cacheKey, nil
}

func (p *gceTokenProvider) MintToken(ctx context.Context, base *oauth2.Token) (*oauth2.Token, error) {
	src := google.ComputeTokenSource(p.account)
	return src.Token()
}

func (p *gceTokenProvider) RefreshToken(ctx context.Context, prev, base *oauth2.Token) (*oauth2.Token, error) {
	// Minting and refreshing on GCE is the same thing: a call to metadata server.
	return p.MintToken(ctx, base)
}
