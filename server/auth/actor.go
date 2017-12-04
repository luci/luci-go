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

package auth

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"google.golang.org/api/googleapi"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/jsontime"
	"go.chromium.org/luci/common/gcloud/googleoauth"
	"go.chromium.org/luci/common/gcloud/iam"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/caching"
)

// MintAccessTokenParams is passed to MintAccessTokenForServiceAccount.
type MintAccessTokenParams struct {
	// ServiceAccount is an email of a service account to mint a token for.
	ServiceAccount string

	// Scopes is a list of OAuth scopes the token should have.
	Scopes []string

	// MinTTL defines an acceptable token lifetime.
	//
	// The returned token will be valid for at least MinTTL, but no longer than
	// one hour.
	//
	// Default is 2 min.
	MinTTL time.Duration
}

// actorTokenCache is used to store access tokens of a service accounts the
// current service has "iam.serviceAccountActor" role in.
//
// The underlying token type is cachedOAuth2Token.
var actorTokenCache = newTokenCache(tokenCacheConfig{
	Kind:                         "as_actor_tokens",
	Version:                      4,
	ProcessLRUCache:              caching.RegisterLRUCache(8192),
	ExpiryRandomizationThreshold: 5 * time.Minute, // ~10% of regular 1h expiration
})

// cachedOAuth2Token is JSON-serializable representation of the oauth2.Token.
//
// It explicitly contains only stuff we want to be in the cache. Storing
// oauth2.Token directly is dangerous because we don't control what oauth2 lib
// has in the Token struct (it may be non-serializable).
type cachedOAuth2Token struct {
	AccessToken string        `json:"access_token,omitempty"`
	TokenType   string        `json:"token_type,omitempty"`
	Expiry      jsontime.Time `json:"expiry,omitempty"`
}

func makeCachedOAuth2Token(tok *oauth2.Token) *cachedOAuth2Token {
	return &cachedOAuth2Token{
		AccessToken: tok.AccessToken,
		TokenType:   tok.TokenType,
		Expiry:      jsontime.Time{tok.Expiry},
	}
}

func (c *cachedOAuth2Token) toToken() *oauth2.Token {
	return &oauth2.Token{
		AccessToken: c.AccessToken,
		TokenType:   c.TokenType,
		Expiry:      c.Expiry.Time,
	}
}

// MintAccessTokenForServiceAccount produces an access token for some service
// account that the current service has "iam.serviceAccountActor" role in.
//
// Used to implement AsActor authorization kind, but can also be used directly,
// if needed. The token is cached internally. Same token may be returned by
// multiple calls, if its lifetime allows.
//
// Recognizes transient errors and marks them, but does not automatically
// retry. Has internal timeout of 10 sec.
func MintAccessTokenForServiceAccount(ctx context.Context, params MintAccessTokenParams) (*oauth2.Token, error) {
	report := durationReporter(ctx, mintAccessTokenDuration)

	cfg := getConfig(ctx)
	if cfg == nil || cfg.AccessTokenProvider == nil {
		report(ErrNotConfigured, "ERROR_NOT_CONFIGURED")
		return nil, ErrNotConfigured
	}

	if params.ServiceAccount == "" || len(params.Scopes) == 0 {
		err := fmt.Errorf("invalid parameters")
		report(err, "ERROR_BAD_ARGUMENTS")
		return nil, err
	}

	if params.MinTTL == 0 {
		params.MinTTL = 2 * time.Minute
	}

	sortedScopes := append([]string(nil), params.Scopes...)
	sort.Strings(sortedScopes)

	// Construct the cache key. Note that it is hashed by 'actorTokenCache' and
	// thus can be as long as necessary. Double check there's no malicious input.
	parts := append([]string{params.ServiceAccount}, sortedScopes...)
	for _, p := range parts {
		if strings.ContainsRune(p, '\n') {
			err := fmt.Errorf("forbidding character in a service account or scope name: %q", p)
			report(err, "ERROR_BAD_ARGUMENTS")
			return nil, err
		}
	}

	ctx = logging.SetFields(ctx, logging.Fields{
		"token":   "actor",
		"account": params.ServiceAccount,
		"scopes":  strings.Join(sortedScopes, " "),
	})

	cached, err, label := actorTokenCache.fetchOrMintToken(ctx, &fetchOrMintTokenOp{
		CacheKey:    strings.Join(parts, "\n"),
		MinTTL:      params.MinTTL,
		MintTimeout: cfg.adjustedTimeout(10 * time.Second),

		// Mint is called on cache miss, under the lock.
		Mint: func(ctx context.Context) (t *cachedToken, err error, label string) {
			// Need an authenticating transport to talk to IAM.
			asSelf, err := GetRPCTransport(ctx, AsSelf, WithScopes(iam.OAuthScope))
			if err != nil {
				return nil, err, "ERROR_NO_TRANSPORT"
			}

			// This will do two HTTP calls: one to 'signBytes' IAM API, another to the
			// token exchange endpoint.
			tok, err := googleoauth.GetAccessToken(ctx, googleoauth.JwtFlowParams{
				ServiceAccount: params.ServiceAccount,
				Signer: &iam.Client{
					Client: &http.Client{Transport: asSelf},
				},
				Scopes: sortedScopes,
				Client: cfg.anonymousClient(ctx),
			})

			// Both iam.Signer and googleoauth.GetAccessToken return googleapi.Error
			// on HTTP-level responses. Recognize fatal HTTP errors. Everything else
			// (stuff like connection timeouts, deadlines, etc) are transient errors.
			if err != nil {
				if apiErr, ok := err.(*googleapi.Error); ok && apiErr.Code < 500 {
					return nil, err, fmt.Sprintf("ERROR_MINTING_HTTP_%d", apiErr.Code)
				}
				return nil, transient.Tag.Apply(err), "ERROR_TRANSIENT_IN_MINTING"
			}

			// Log details about the new token.
			now := clock.Now(ctx).UTC()
			logging.Fields{
				"fingerprint": tokenFingerprint(tok.AccessToken),
				"validity":    tok.Expiry.Sub(now),
			}.Debugf(ctx, "Minted new actor OAuth token")

			return &cachedToken{
				OAuth2Token: makeCachedOAuth2Token(tok),
				Created:     jsontime.Time{now},
				Expiry:      jsontime.Time{tok.Expiry},
			}, nil, "SUCCESS_CACHE_MISS"
		},
	})

	report(err, label)
	if err != nil {
		return nil, err
	}
	return cached.OAuth2Token.toToken(), nil
}
