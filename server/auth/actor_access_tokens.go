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
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"

	"go.chromium.org/luci/server/auth/internal/tracing"
)

// MintAccessTokenParams is passed to MintAccessTokenForServiceAccount.
type MintAccessTokenParams struct {
	// ServiceAccount is an email of a service account to mint a token for.
	ServiceAccount string

	// Scopes is a list of OAuth scopes the token should have.
	Scopes []string

	// Delegates is a the sequence of service accounts in a delegation chain.
	//
	// Each service account must be granted the "iam.serviceAccountTokenCreator"
	// role on its next service account in the chain. The last service account in
	// the chain must be granted the "iam.serviceAccountTokenCreator" role on
	// the service account specified by ServiceAccount field.
	Delegates []string

	// MinTTL defines an acceptable token lifetime.
	//
	// The returned token will be valid for at least MinTTL, but no longer than
	// one hour.
	//
	// Default is 2 min.
	MinTTL time.Duration
}

// actorAccessTokenCache is used to store access tokens of service accounts
// the current service has "iam.serviceAccountTokenCreator" role in.
//
// The token is stored in OAuth2Token field.
var actorAccessTokenCache = newTokenCache(tokenCacheConfig{
	Kind:                         "as_actor_access_tok",
	Version:                      1,
	ProcessCacheCapacity:         8192,
	ExpiryRandomizationThreshold: 5 * time.Minute, // ~10% of regular 1h expiration
})

// MintAccessTokenForServiceAccount produces an access token for some service
// account that the current service has "iam.serviceAccountTokenCreator" role
// in.
//
// Used to implement AsActor authorization kind, but can also be used directly,
// if needed. The token is cached internally. Same token may be returned by
// multiple calls, if its lifetime allows.
//
// Recognizes transient errors and marks them, but does not automatically
// retry. Has internal timeout of 10 sec.
func MintAccessTokenForServiceAccount(ctx context.Context, params MintAccessTokenParams) (_ *Token, err error) {
	ctx, span := tracing.Start(ctx, "go.chromium.org/luci/server/auth.MintAccessTokenForServiceAccount",
		attribute.String("cr.dev.account", params.ServiceAccount),
	)
	defer func() { tracing.End(span, err) }()

	report := durationReporter(ctx, mintAccessTokenDuration)

	cfg := getConfig(ctx)
	if cfg == nil || !cfg.actorTokensConfigured() {
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

	// Construct the cache key. Note that it is hashed by 'actorAccessTokenCache'
	// and thus can be as long as necessary.
	cacheKey := cacheKeyBuilder{}
	if err := cacheKey.add("service account", params.ServiceAccount); err != nil {
		report(err, "ERROR_BAD_ARGUMENTS")
		return nil, err
	}
	for _, scope := range sortedScopes {
		if err := cacheKey.add("scope", scope); err != nil {
			report(err, "ERROR_BAD_ARGUMENTS")
			return nil, err
		}
	}
	for _, delegate := range params.Delegates {
		if err := cacheKey.add("delegate", delegate); err != nil {
			report(err, "ERROR_BAD_ARGUMENTS")
			return nil, err
		}
	}

	ctx = logging.SetFields(ctx, logging.Fields{
		"token":     "actor",
		"account":   params.ServiceAccount,
		"scopes":    strings.Join(sortedScopes, " "),
		"delegates": strings.Join(params.Delegates, ":"),
	})

	cached, err, label := actorAccessTokenCache.fetchOrMintToken(ctx, &fetchOrMintTokenOp{
		CacheKey:    cacheKey.finish(),
		MinTTL:      params.MinTTL,
		MintTimeout: cfg.adjustedTimeout(10 * time.Second),

		// Mint is called on cache miss, under the lock.
		Mint: func(ctx context.Context) (t *cachedToken, err error, label string) {
			tok, err := cfg.actorTokensProvider().GenerateAccessToken(ctx, params.ServiceAccount, sortedScopes, params.Delegates)
			if err != nil {
				if transient.Tag.In(err) {
					return nil, err, "ERROR_TRANSIENT_IN_MINTING"
				}
				return nil, err, "ERROR_FATAL_IN_MINTING"
			}

			now := clock.Now(ctx).UTC()
			logging.Fields{
				"fingerprint": tokenFingerprint(tok.AccessToken),
				"validity":    tok.Expiry.Sub(now),
			}.Debugf(ctx, "Minted new actor OAuth token")

			return &cachedToken{
				Created:     now,
				Expiry:      tok.Expiry.UTC(),
				OAuth2Token: tok.AccessToken,
			}, nil, "SUCCESS_CACHE_MISS"
		},
	})

	report(err, label)
	if err != nil {
		return nil, err
	}
	return &Token{
		Token:  cached.OAuth2Token,
		Expiry: cached.Expiry,
	}, nil
}
